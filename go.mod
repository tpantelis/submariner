module github.com/submariner-io/submariner

go 1.13

require (
	github.com/bronze1man/goStrongswanVici v0.0.0-20201105010758-936f38b697fd
	github.com/cenk/hub v1.0.1 // indirect
	github.com/coreos/go-iptables v0.5.0
	github.com/ebay/go-ovn v0.1.1-0.20201007164241-da67e9744ec0
	github.com/go-logr/zapr v0.1.1 // indirect
	github.com/go-ping/ping v0.0.0-20201022122018-3977ed72668a
	github.com/imdario/mergo v0.3.9 // indirect
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.4
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.9.0
	github.com/rdegges/go-ipify v0.0.0-20150526035502-2d94a6a86c40
	github.com/submariner-io/admiral v0.8.1-0.20201218153659-b6de92b83dd5
	github.com/submariner-io/shipyard v0.8.0
	github.com/vishvananda/netlink v1.1.0
	go.uber.org/zap v1.15.0 // indirect
	golang.org/x/sys v0.0.0-20201214210602-f9fddec55a1e
	golang.zx2c4.com/wireguard/wgctrl v0.0.0-20200324154536-ceff61240acf
	k8s.io/api v0.17.0
	k8s.io/apimachinery v0.17.0
	k8s.io/client-go v0.17.0
	k8s.io/klog v1.0.0
	sigs.k8s.io/controller-runtime v0.3.0
	sigs.k8s.io/testing_frameworks v0.1.2 // indirect
)

// Pinned to kubernetes-1.17.0
replace (
	k8s.io/api => k8s.io/api v0.17.0
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.17.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.0
	k8s.io/client-go => k8s.io/client-go v0.17.0
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.17.0
)
