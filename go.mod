module github.com/submariner-io/submariner

go 1.16

require (
	github.com/cenkalti/backoff/v4 v4.1.2
	github.com/coreos/go-iptables v0.6.0
	github.com/emirpasic/gods v1.18.1
	github.com/go-ping/ping v0.0.0-20210506233800-ff8be3320020
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.19.0
	github.com/ovn-org/libovsdb v0.6.1-0.20220427123326-d7b273399db4
	github.com/ovn-org/ovn-kubernetes/go-controller v0.0.0-20220511131059-ac1ce4691c0f
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.12.1
	github.com/submariner-io/admiral v0.13.0-m1
	github.com/submariner-io/shipyard v0.13.0-m1
	github.com/uw-labs/lichen v0.1.7
	github.com/vishvananda/netlink v1.1.1-0.20210518155637-4cb3795f2ccb
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9
	golang.zx2c4.com/wireguard/wgctrl v0.0.0-20211215182854-7a385b3431de
	google.golang.org/protobuf v1.27.1
	k8s.io/api v0.23.5
	k8s.io/apimachinery v0.23.5
	k8s.io/client-go v0.23.5
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.30.0
	k8s.io/utils v0.0.0-20211116205334-6203023598ed
	sigs.k8s.io/controller-runtime v0.11.2
	sigs.k8s.io/mcs-api v0.1.0
)
