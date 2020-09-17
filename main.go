package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/submariner-io/admiral/pkg/log"
	subv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/submariner/pkg/cable"
	"github.com/submariner-io/submariner/pkg/cableengine"
	"github.com/submariner-io/submariner/pkg/cableengine/syncer"
	submarinerClientset "github.com/submariner-io/submariner/pkg/client/clientset/versioned"
	submarinerInformers "github.com/submariner-io/submariner/pkg/client/informers/externalversions"
	"github.com/submariner-io/submariner/pkg/controllers/datastoresyncer"
	"github.com/submariner-io/submariner/pkg/controllers/tunnel"
	"github.com/submariner-io/submariner/pkg/datastore"
	subk8s "github.com/submariner-io/submariner/pkg/datastore/kubernetes"
	"github.com/submariner-io/submariner/pkg/types"
	"github.com/submariner-io/submariner/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	localMasterURL  string
	localKubeconfig string
)

func init() {
	flag.StringVar(&localKubeconfig, "kubeconfig", "", "Path to kubeconfig of local cluster. Only required if out-of-cluster.")
	flag.StringVar(&localMasterURL, "master", "",
		"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}

type leaderConfig struct {
	LeaseDuration int64
	RenewDeadline int64
	RetryPeriod   int64
}

const (
	leadershipConfigEnvPrefix = "leadership"
	defaultLeaseDuration      = 10 // In Seconds
	defaultRenewDeadline      = 5  // In Seconds
	defaultRetryPeriod        = 2  // In Seconds
)

var VERSION = "not-compiled-properly"

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	klog.Info("Starting the submariner gateway engine")

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	var submSpec types.SubmarinerSpecification
	err := envconfig.Process("submariner", &submSpec)
	if err != nil {
		klog.Fatal(err)
	}

	cfg, err := clientcmd.BuildConfigFromFlags(localMasterURL, localKubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	submarinerClient, err := submarinerClientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error creating submariner clientset: %s", err.Error())
	}

	submarinerInformerFactory := submarinerInformers.NewSharedInformerFactoryWithOptions(submarinerClient, time.Second*30,
		submarinerInformers.WithNamespace(submSpec.Namespace))

	var localSubnets []string

	klog.Info("Creating the cable engine")

	localCluster := types.SubmarinerCluster{
		ID: submSpec.ClusterID,
		Spec: subv1.ClusterSpec{
			ClusterID:   submSpec.ClusterID,
			ColorCodes:  submSpec.ColorCodes,
			ServiceCIDR: submSpec.ServiceCidr,
			ClusterCIDR: submSpec.ClusterCidr,
			GlobalCIDR:  submSpec.GlobalCidr,
		},
	}

	if len(submSpec.GlobalCidr) > 0 {
		localSubnets = submSpec.GlobalCidr
	} else {
		localSubnets = append(localSubnets, submSpec.ServiceCidr...)
		localSubnets = append(localSubnets, submSpec.ClusterCidr...)
	}

	if submSpec.CableDriver == "" {
		submSpec.CableDriver = cable.GetDefaultCableDriver()
	}

	submSpec.CableDriver = strings.ToLower(submSpec.CableDriver)

	localEndpoint, err := util.GetLocalEndpoint(submSpec.ClusterID, submSpec.CableDriver, map[string]string{}, submSpec.NatEnabled,
		localSubnets, util.GetLocalIP())

	if err != nil {
		klog.Fatalf("Error creating local endpoint object from %#v: %v", submSpec, err)
	}

	cableEngine := cableengine.NewEngine(localCluster, localEndpoint)

	cableEngineSyncer := syncer.NewGatewaySyncer(
		cableEngine,
		submarinerClient.SubmarinerV1().Gateways(submSpec.Namespace),
		VERSION)

	cableEngineSyncer.Run(stopCh)

	becameLeader := func(context.Context) {
		klog.Info("Creating the tunnel controller")

		tunnelController := tunnel.NewController(cableEngine, submarinerInformerFactory.Submariner().V1().Endpoints())

		var datastore datastore.Datastore

		switch submSpec.Broker {
		case "k8s":
			klog.Info("Creating the kubernetes central datastore")

			datastore, err = subk8s.NewDatastore(submSpec.ClusterID, stopCh)
			if err != nil {
				fatal(cableEngineSyncer, "Error creating the kubernetes datastore: %v", err)
			}
		default:
			fatal(cableEngineSyncer, "Invalid backend %q was specified", submSpec.Broker)
		}

		klog.Info("Creating the datastore syncer")

		dsSyncer := datastoresyncer.NewDatastoreSyncer(submSpec.ClusterID, submarinerClient.SubmarinerV1().Clusters(submSpec.Namespace),
			submarinerInformerFactory.Submariner().V1().Clusters(), submarinerClient.SubmarinerV1().Endpoints(submSpec.Namespace),
			submarinerInformerFactory.Submariner().V1().Endpoints(), datastore, submSpec.ColorCodes, localCluster, localEndpoint)

		submarinerInformerFactory.Start(stopCh)

		if err = cableEngine.StartEngine(); err != nil {
			fatal(cableEngineSyncer, "Error starting the cable engine: %v", err)
		}

		var wg sync.WaitGroup

		wg.Add(2)

		go func() {
			defer wg.Done()

			if err = tunnelController.Run(stopCh); err != nil {
				fatal(cableEngineSyncer, "Error running the tunnel controller: %v", err)
			}
		}()

		go func() {
			defer wg.Done()

			if err = dsSyncer.Run(stopCh); err != nil {
				fatal(cableEngineSyncer, "Error running the datastore syncer: %v", err)
			}
		}()

		wg.Wait()
	}

	leClient, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "leader-election"))
	if err != nil {
		fatal(cableEngineSyncer, "Error creating leader election kubernetes clientset: %s", err)
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.V(log.DEBUG).Infof)
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "submariner-controller"})

	lostLeader := func() {
		cableEngineSyncer.CleanupGatewayEntry()
		klog.Fatalf("Leader election lost, shutting down")
	}

	go func() {
		if err = startLeaderElection(leClient, recorder, becameLeader, lostLeader); err != nil {
			fatal(cableEngineSyncer, "Error starting leader election: %v", err)
		}
	}()

	<-stopCh
	klog.Info("All controllers stopped or exited. Stopping main loop")
}

func startLeaderElection(leaderElectionClient kubernetes.Interface, recorder resourcelock.EventRecorder,
	run func(ctx context.Context), end func()) error {
	gwLeadershipConfig := leaderConfig{}

	err := envconfig.Process(leadershipConfigEnvPrefix, &gwLeadershipConfig)
	if err != nil {
		return fmt.Errorf("Error processing environment config for %s: %v", leadershipConfigEnvPrefix, err)
	}

	// Use default values when GatewayLeadership environment variables are not configured
	if gwLeadershipConfig.LeaseDuration == 0 {
		gwLeadershipConfig.LeaseDuration = defaultLeaseDuration
	}

	if gwLeadershipConfig.RenewDeadline == 0 {
		gwLeadershipConfig.RenewDeadline = defaultRenewDeadline
	}

	if gwLeadershipConfig.RetryPeriod == 0 {
		gwLeadershipConfig.RetryPeriod = defaultRetryPeriod
	}

	klog.Infof("Gateway leader election config values: %#v", gwLeadershipConfig)

	id, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("Error getting hostname: %v", err)
	}

	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)
	namespace, _, err := kubeconfig.Namespace()
	if err != nil {
		namespace = "submariner"
		klog.Infof("Could not obtain a namespace to use for the leader election lock - the error was: %v. Using the default %q namespace.",
			namespace, err)
	} else {
		klog.Infof("Using namespace %q for the leader election lock", namespace)
	}

	// Lock required for leader election
	rl := resourcelock.ConfigMapLock{
		ConfigMapMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "submariner-engine-lock",
		},
		Client: leaderElectionClient.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      id + "-submariner-engine",
			EventRecorder: recorder,
		},
	}

	leaderelection.RunOrDie(context.TODO(), leaderelection.LeaderElectionConfig{
		Lock:          &rl,
		LeaseDuration: time.Duration(gwLeadershipConfig.LeaseDuration) * time.Second,
		RenewDeadline: time.Duration(gwLeadershipConfig.RenewDeadline) * time.Second,
		RetryPeriod:   time.Duration(gwLeadershipConfig.RetryPeriod) * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: end,
		},
	})

	return nil
}

func fatal(gwSyncer *syncer.GatewaySyncer, format string, args ...interface{}) {
	err := fmt.Errorf(format, args)
	gwSyncer.SetGatewayStatusError(err)
	klog.Fatal(err.Error())
}
