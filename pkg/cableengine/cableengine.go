package cableengine

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/submariner-io/admiral/pkg/log"
	v1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/submariner/pkg/cable"
	"github.com/submariner-io/submariner/pkg/types"
	"github.com/submariner-io/submariner/pkg/util"
	"k8s.io/klog"

	// Add supported drivers
	_ "github.com/submariner-io/submariner/pkg/cable/libreswan"
	_ "github.com/submariner-io/submariner/pkg/cable/strongswan"
	_ "github.com/submariner-io/submariner/pkg/cable/wireguard"
)

// Engine represents an implementation of some remote connectivity mechanism, such as
// a VPN gateway.
// An Engine cooperates with, and delegates work to, a cable.Driver for implementing
// a secure connection to remote clusters.
type Engine interface {
	// StartEngine performs any general set up work needed independent of any remote connections.
	StartEngine() error
	// InstallCable performs any set up work needed for connecting to given remote endpoint.
	// Once InstallCable completes, it should be possible to connect to remote
	// Pods or Services behind the given endpoint.
	InstallCable(remote types.SubmarinerEndpoint) error
	// RemoveCable disconnects the Engine from the given remote endpoint. Upon completion.
	// remote Pods and Service may not be accessible any more.
	RemoveCable(remote types.SubmarinerEndpoint) error
	// ListCableConnections returns a list of cable connection, and the related status
	ListCableConnections() (*[]v1.Connection, error)
	// GetLocalEndpoint returns the local endpoint for this cable engine
	GetLocalEndpoint() *types.SubmarinerEndpoint
	// GetHAStatus returns the HA status for this cable engine
	GetHAStatus() v1.HAStatus
}

type engine struct {
	sync.Mutex
	driver        cable.Driver
	localCluster  types.SubmarinerCluster
	localEndpoint types.SubmarinerEndpoint
}

// NewEngine creates a new Engine for the local cluster
func NewEngine(localCluster types.SubmarinerCluster, localEndpoint types.SubmarinerEndpoint) Engine {
	return &engine{
		localCluster:  localCluster,
		localEndpoint: localEndpoint,
		driver:        nil,
	}
}

func (i *engine) GetLocalEndpoint() *types.SubmarinerEndpoint {
	return &i.localEndpoint
}

func (i *engine) StartEngine() error {
	i.Lock()
	defer i.Unlock()

	if err := i.startDriver(); err != nil {
		return err
	}

	klog.Infof("CableEngine controller started, driver: %q", i.driver.GetName())

	return nil
}

func (i *engine) startDriver() error {
	var err error

	if i.driver, err = cable.NewDriver(i.localEndpoint, i.localCluster); err != nil {
		return err
	}

	if err := i.driver.Init(); err != nil {
		return err
	}

	return nil
}

func (i *engine) InstallCable(endpoint types.SubmarinerEndpoint) error {
	if endpoint.Spec.ClusterID == i.localCluster.ID {
		klog.V(log.DEBUG).Infof("Not installing cable for local cluster")
		return nil
	}

	if reflect.DeepEqual(endpoint.Spec, i.localEndpoint.Spec) {
		klog.V(log.DEBUG).Infof("Not installing cable for local endpoint")
		return nil
	}

	klog.Infof("Installing Endpoint cable %q", endpoint.Spec.CableName)

	i.Lock()
	defer i.Unlock()

	activeConnections, err := i.driver.GetActiveConnections(endpoint.Spec.ClusterID)
	if err != nil {
		return err
	}

	for _, active := range activeConnections {
		klog.V(log.TRACE).Infof("Analyzing currently active connection %q", active)

		if active == endpoint.Spec.CableName {
			klog.V(log.DEBUG).Infof("Cable %q is already installed - not installing again", active)
			return nil
		}

		if util.GetClusterIDFromCableName(active) == endpoint.Spec.ClusterID {
			return fmt.Errorf("found a pre-existing cable %q that belongs to this cluster %s", active, endpoint.Spec.ClusterID)
		}
	}

	remoteEndpointIP, err := i.driver.ConnectToEndpoint(endpoint)
	if err != nil {
		return err
	}

	klog.Infof("Successfully installed Endpoint cable %q with remote IP %s", endpoint.Spec.CableName, remoteEndpointIP)

	return nil
}

func (i *engine) RemoveCable(endpoint types.SubmarinerEndpoint) error {
	klog.Infof("Removing Endpoint cable %q", endpoint.Spec.CableName)

	i.Lock()
	defer i.Unlock()

	err := i.driver.DisconnectFromEndpoint(endpoint)
	if err != nil {
		return err
	}

	klog.Infof("Successfully removed Endpoint cable %q", endpoint.Spec.CableName)

	return nil
}

func (i *engine) GetHAStatus() v1.HAStatus {
	i.Lock()
	defer i.Unlock()

	if i.driver == nil {
		return v1.HAStatusPassive
	} else {
		// we may want to add a call to the driver in the future, for situations where
		// the driver is running from the start, but could be in passive status, or
		// in active/active.
		return v1.HAStatusActive
	}
}

func (i *engine) ListCableConnections() (*[]v1.Connection, error) {
	i.Lock()
	defer i.Unlock()

	if i.driver != nil {
		return i.driver.GetConnections()
	}
	// if no driver, we can safely report that no connections exist
	return &[]v1.Connection{}, nil
}
