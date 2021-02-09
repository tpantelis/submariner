/*
© 2021 Red Hat, Inc. and others

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package framework

import (
	"fmt"

	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/shipyard/test/e2e/framework"
	"github.com/submariner-io/submariner/pkg/client/clientset/versioned/scheme"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	submarinerClientset "github.com/submariner-io/submariner/pkg/client/clientset/versioned"
	"github.com/submariner-io/submariner/pkg/client/informers/externalversions"
)

var gatewayGVR = &schema.GroupVersionResource{
	Group:    "submariner.io",
	Version:  "v1",
	Resource: "gateways",
}

// Framework supports common operations used by e2e tests; it will keep a client & a namespace for you.
type Framework struct {
	*framework.Framework
}

var SubmarinerClients []*submarinerClientset.Clientset

func init() {
	framework.AddBeforeSuite(beforeSuite)
}

// NewFramework creates a test framework.
func NewFramework(baseName string) *Framework {
	f := &Framework{Framework: framework.NewFramework(baseName)}
	framework.AddCleanupAction(f.GatewayCleanup)

	return f
}

func beforeSuite() {
	framework.By("Creating submariner clients")

	for _, restConfig := range framework.RestConfigs {
		SubmarinerClients = append(SubmarinerClients, createSubmarinerClient(restConfig))
	}

	framework.DetectGlobalnet()
}

func (f *Framework) awaitGatewayWithStatus(cluster framework.ClusterIndex,
	name, status string) *unstructured.Unstructured {
	gwClient := framework.DynClients[cluster].Resource(*gatewayGVR).Namespace(framework.TestContext.SubmarinerNamespace)
	obj := framework.AwaitUntil(fmt.Sprintf("await Gateway on %q with status %q", name, status),
		func() (interface{}, error) {
			resGw, err := gwClient.Get(name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return resGw, err
		},
		func(result interface{}) (bool, string, error) {
			if result == nil {
				return false, "gateway not found yet", nil
			}

			gw := result.(*unstructured.Unstructured)
			haStatus := NestedString(gw.Object, "status", "haStatus")
			if haStatus != status {
				return false, "", fmt.Errorf("Gateway %q exists but has wrong status %q, expected %q",
					gw.GetName(), haStatus, status)
			}
			return true, "", nil
		})

	return obj.(*unstructured.Unstructured)
}

func (f *Framework) AwaitGatewayWithStatus(cluster framework.ClusterIndex,
	name string, status submarinerv1.HAStatus) *submarinerv1.Gateway {
	return toGateway(f.awaitGatewayWithStatus(cluster, name, string(status)))
}

func toGateway(from *unstructured.Unstructured) *submarinerv1.Gateway {
	to := &submarinerv1.Gateway{}
	Expect(scheme.Scheme.Convert(from, to, nil)).To(Succeed())

	return to
}

func toGateways(from []unstructured.Unstructured) []submarinerv1.Gateway {
	gateways := make([]submarinerv1.Gateway, len(from))
	for i := range from {
		gateways[i] = *toGateway(&from[i])
	}

	return gateways
}

func (f *Framework) awaitGatewaysWithStatus(
	cluster framework.ClusterIndex, status string) []unstructured.Unstructured {
	gwList := framework.AwaitUntil(fmt.Sprintf("await Gateways with status %q", status),
		func() (interface{}, error) {
			return f.getGatewaysWithHAStatus(cluster, status), nil
		},
		func(result interface{}) (bool, string, error) {
			gateways := result.([]unstructured.Unstructured)
			if len(gateways) == 0 {
				return false, "no gateway found yet", nil
			}

			return true, "", nil
		})

	return gwList.([]unstructured.Unstructured)
}

func (f *Framework) AwaitGatewaysWithStatus(
	cluster framework.ClusterIndex, status submarinerv1.HAStatus) []submarinerv1.Gateway {
	return toGateways(f.awaitGatewaysWithStatus(cluster, string(status)))
}

func (f *Framework) AwaitGatewayRemoved(cluster framework.ClusterIndex, name string) {
	gwClient := framework.DynClients[cluster].Resource(*gatewayGVR).Namespace(framework.TestContext.SubmarinerNamespace)
	framework.AwaitUntil(fmt.Sprintf("await Gateway on %q removed", name),
		func() (interface{}, error) {
			_, err := gwClient.Get(name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		},
		func(result interface{}) (bool, string, error) {
			gone := result.(bool)
			return gone, "", nil
		})
}

func NestedString(obj map[string]interface{}, fields ...string) string {
	str, _, err := unstructured.NestedString(obj, fields...)
	Expect(err).To(Succeed())

	return str
}

func (f *Framework) awaitGatewayFullyConnected(cluster framework.ClusterIndex, name string) *unstructured.Unstructured {
	gwClient := framework.DynClients[cluster].Resource(*gatewayGVR).Namespace(framework.TestContext.SubmarinerNamespace)
	gw := framework.AwaitUntil(fmt.Sprintf("await Gateway on %q with status active and connections UP", name),
		func() (interface{}, error) {
			resGw, err := gwClient.Get(name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return resGw, err
		},
		func(result interface{}) (bool, string, error) {
			if result == nil {
				return false, "gateway not found yet", nil
			}

			gw := result.(*unstructured.Unstructured)
			haStatus := NestedString(gw.Object, "status", "haStatus")
			if haStatus != "active" {
				return false, fmt.Sprintf("Gateway %q exists but not active yet",
					gw.GetName()), nil
			}

			connections, _, _ := unstructured.NestedSlice(gw.Object, "status", "connections")
			if len(connections) == 0 {
				return false, fmt.Sprintf("Gateway %q is active but has no connections yet", name), nil
			}

			for _, o := range connections {
				conn := o.(map[string]interface{})
				status, _, _ := unstructured.NestedString(conn, "status")
				if status != "connected" {
					return false, fmt.Sprintf("Gateway %q is active but cluster %q is not connected: Status: %q, Message: %q",
						name, NestedString(conn, "endpoint", "cluster_id"), status, NestedString(conn, "statusMessage")), nil
				}
			}

			return true, "", nil
		})

	return gw.(*unstructured.Unstructured)
}

func (f *Framework) AwaitGatewayFullyConnected(cluster framework.ClusterIndex, name string) *submarinerv1.Gateway {
	return toGateway(f.awaitGatewayFullyConnected(cluster, name))
}

// GatewayCleanup ensures that only the active gateway node is flagged as gateway node
//                which could not be after a failed test which left the system on an
//                unexpected state
func (f *Framework) GatewayCleanup() {
	for cluster := range framework.DynClients {
		passiveGateways := f.getGatewaysWithHAStatus(framework.ClusterIndex(cluster), "passive")

		if len(passiveGateways) == 0 {
			continue
		}

		ginkgo.By(fmt.Sprintf("Cleaning up any non-active gateways: %v", gatewayNames(passiveGateways)))

		for _, nonActiveGw := range passiveGateways {
			f.SetGatewayLabelOnNode(framework.ClusterIndex(cluster), nonActiveGw.GetName(), false)
			f.AwaitGatewayRemoved(framework.ClusterIndex(cluster), nonActiveGw.GetName())
		}
	}
}

func gatewayNames(gateways []unstructured.Unstructured) []string {
	names := []string{}
	for _, gw := range gateways {
		names = append(names, gw.GetName())
	}

	return names
}

func (f *Framework) getGatewaysWithHAStatus(
	cluster framework.ClusterIndex, status string) []unstructured.Unstructured {
	gwClient := framework.DynClients[cluster].Resource(*gatewayGVR).Namespace(framework.TestContext.SubmarinerNamespace)
	gwList, err := gwClient.List(metav1.ListOptions{})

	filteredGateways := []unstructured.Unstructured{}

	// List will return "NotFound" if the CRD is not registered in the specific cluster (broker-only)
	if apierrors.IsNotFound(err) {
		return filteredGateways
	}

	Expect(err).NotTo(HaveOccurred())

	for _, gw := range gwList.Items {
		haStatus := NestedString(gw.Object, "status", "haStatus")
		if haStatus == status {
			filteredGateways = append(filteredGateways, gw)
		}
	}

	return filteredGateways
}

func (f *Framework) GetGatewaysWithHAStatus(
	cluster framework.ClusterIndex, status submarinerv1.HAStatus) []submarinerv1.Gateway {
	return toGateways(f.getGatewaysWithHAStatus(cluster, string(status)))
}

func (f *Framework) DeleteGateway(cluster framework.ClusterIndex, name string) {
	framework.AwaitUntil("delete gateway", func() (interface{}, error) {
		err := SubmarinerClients[cluster].SubmarinerV1().Gateways(
			framework.TestContext.SubmarinerNamespace).Delete(name, &metav1.DeleteOptions{})
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}, framework.NoopCheckResult)
}

func (f *Framework) GetGatewayInformer(cluster framework.ClusterIndex) (cache.SharedIndexInformer, chan struct{}) {
	stopCh := make(chan struct{})
	informerFactory := externalversions.NewSharedInformerFactory(SubmarinerClients[cluster], 0)
	informer := informerFactory.Submariner().V1().Gateways().Informer()

	go informer.Run(stopCh)
	Expect(cache.WaitForCacheSync(stopCh, informer.HasSynced)).To(BeTrue())

	return informer, stopCh
}

func GetDeletionChannel(informer cache.SharedIndexInformer) chan string {
	deletionChannel := make(chan string, 100)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			if object, ok := obj.(metav1.Object); !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				Expect(ok).To(BeTrue(), "tombstone extraction failed")
				object, ok = tombstone.Obj.(metav1.Object)
				Expect(ok).To(BeTrue(), "tombstone inner object extraction failed")
				deletionChannel <- object.GetName()
			} else {
				deletionChannel <- object.GetName()
			}
		},
	})

	return deletionChannel
}
