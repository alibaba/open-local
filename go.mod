module github.com/alibaba/open-local

go 1.15

require (
	github.com/container-storage-interface/spec v1.2.0
	github.com/docker/go-units v0.4.0
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/protobuf v1.5.0
	github.com/google/credstore v0.0.0-20181218150457-e184c60ef875 // indirect
	github.com/google/go-microservice-helpers v0.0.0-20190205165657-a91942da5417
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/googleapis/gnostic v0.5.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/json-iterator/go v1.1.10 // indirect
	github.com/julienschmidt/httprouter v1.3.0
	github.com/kubernetes-csi/csi-lib-utils v0.9.1 // indirect
	github.com/kubernetes-csi/drivers v1.0.2
	github.com/kubernetes-csi/external-snapshotter/client/v3 v3.0.0
	github.com/onsi/ginkgo v1.14.1 // indirect
	github.com/onsi/gomega v1.10.2 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/peter-wangxu/simple-golang-tools v0.0.0-20210209091758-458c22961dd2
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.0.0
	github.com/ricochet2200/go-disk-usage v0.0.0-20150921141558-f0d1b743428f
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a // indirect
	golang.org/x/net v0.0.0-20200904194848-62affa334b73
	golang.org/x/sys v0.0.0-20200821140526-fda516888d29 // indirect
	golang.org/x/text v0.3.3 // indirect
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e // indirect
	golang.org/x/tools v0.0.0-20200616133436-c1934b75d054 // indirect
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/grpc v1.36.0
	google.golang.org/protobuf v1.26.0
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.19.0
	k8s.io/code-generator v0.19.0
	k8s.io/kube-scheduler v0.0.0
	k8s.io/kubernetes v1.18.9
	k8s.io/sample-controller v0.0.0-20191004105128-02bcf064a96b
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	sigs.k8s.io/controller-tools v0.5.0 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.18.9
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.9
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.9
	k8s.io/apiserver => k8s.io/apiserver v0.18.9
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.9
	k8s.io/client-go => k8s.io/client-go v0.18.9
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.18.9
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.18.9
	k8s.io/code-generator => k8s.io/code-generator v0.18.9
	k8s.io/component-base => k8s.io/component-base v0.18.9
	k8s.io/cri-api => k8s.io/cri-api v0.18.9
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.18.9
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.9
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.18.9
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.18.9
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.18.9
	k8s.io/kubectl => k8s.io/kubectl v0.18.9
	k8s.io/kubelet => k8s.io/kubelet v0.18.9
	k8s.io/kubernetes => k8s.io/kubernetes v1.18.9
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.18.9
	k8s.io/metrics => k8s.io/metrics v0.18.9
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.18.9
)
