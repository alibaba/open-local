module github.com/alibaba/open-local

go 1.18

require (
	github.com/aws/aws-sdk-go v1.44.44
	github.com/container-storage-interface/spec v1.5.0
	github.com/docker/go-units v0.4.0
	github.com/golang/protobuf v1.5.3
	github.com/google/credstore v0.0.0-20181218150457-e184c60ef875
	github.com/google/uuid v1.3.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645
	github.com/json-iterator/go v1.1.12
	github.com/julienschmidt/httprouter v1.3.0
	github.com/kata-containers/kata-containers/src/runtime v0.0.0-20220902020102-6de4bfd8607a
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0
	github.com/opentracing/opentracing-go v1.2.0
	github.com/peter-wangxu/simple-golang-tools v0.0.0-20210209091758-458c22961dd2
	github.com/prometheus/client_golang v1.17.0
	github.com/ricochet2200/go-disk-usage v0.0.0-20150921141558-f0d1b743428f
	github.com/spf13/cobra v1.5.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.0
	golang.org/x/net v0.17.0
	golang.org/x/sys v0.13.0
	google.golang.org/grpc v1.47.0
	google.golang.org/protobuf v1.31.0
	k8s.io/api v0.24.17
	k8s.io/apimachinery v0.24.17
	k8s.io/client-go v0.24.17
	k8s.io/code-generator v0.24.17
	k8s.io/component-base v0.24.17
	k8s.io/klog/v2 v2.80.1
	k8s.io/kube-scheduler v0.20.5
	k8s.io/kubernetes v1.24.17
	k8s.io/mount-utils v0.24.17
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.1.4
)

require github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect

require (
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect; indirectgo-logr
	github.com/emicklei/go-restful v2.9.5+incompatible // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.6 // indirect
	github.com/go-openapi/swag v0.21.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/selinux v1.10.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.4.1-0.20230718164431-9a2bf3000d16 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	golang.org/x/crypto v0.14.0 // indirect
	golang.org/x/mod v0.8.0 // indirect
	golang.org/x/oauth2 v0.12.0 // indirect
	golang.org/x/term v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/time v0.0.0-20220210224613-90d013bbcef8 // indirect
	golang.org/x/tools v0.6.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220624142145-8cd45d7dbd1f // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiserver v0.24.17 // indirect
	k8s.io/cloud-provider v0.24.17 // indirect
	k8s.io/component-helpers v0.24.17 // indirect
	k8s.io/gengo v0.0.0-20211129171323-c02415ce4185 // indirect
	k8s.io/kube-openapi v0.0.0-20220328201542-3ee0da9b0b42 // indirect
	sigs.k8s.io/json v0.0.0-20211208200746-9f7c6b3444d2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

require (
	github.com/google/go-microservice-helpers v0.0.0-20190205165657-a91942da5417 // indirect
	github.com/pkg/errors v0.9.1
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.54.0
	cloud.google.com/go/bigquery => cloud.google.com/go/bigquery v1.4.0
	cloud.google.com/go/datastore => cloud.google.com/go/datastore v1.1.0
	cloud.google.com/go/firestore => cloud.google.com/go/firestore v1.1.0
	cloud.google.com/go/pubsub => cloud.google.com/go/pubsub v1.2.0
	cloud.google.com/go/storage => cloud.google.com/go/storage v1.6.0
	github.com/go-logr/logr => github.com/go-logr/logr v1.2.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
	google.golang.org/grpc => google.golang.org/grpc v1.43.0
	k8s.io/api => k8s.io/api v0.24.17
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.24.17
	k8s.io/apimachinery => k8s.io/apimachinery v0.24.17
	k8s.io/apiserver => k8s.io/apiserver v0.24.17
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.24.17
	k8s.io/client-go => k8s.io/client-go v0.24.17
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.24.17
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.24.17
	k8s.io/code-generator => k8s.io/code-generator v0.24.17
	k8s.io/component-base => k8s.io/component-base v0.24.17
	k8s.io/component-helpers => k8s.io/component-helpers v0.24.17
	k8s.io/controller-manager => k8s.io/controller-manager v0.24.17
	k8s.io/cri-api => k8s.io/cri-api v0.24.17
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.24.17
	k8s.io/klog/v2 => k8s.io/klog/v2 v2.60.1
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.24.17
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.24.17
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.24.17
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.24.17
	k8s.io/kubectl => k8s.io/kubectl v0.24.17
	k8s.io/kubelet => k8s.io/kubelet v0.24.17
	k8s.io/kubernetes => k8s.io/kubernetes v1.24.15
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.24.17
	k8s.io/metrics => k8s.io/metrics v0.24.17
	k8s.io/mount-utils => k8s.io/mount-utils v0.24.17
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.24.17
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.24.17
)
