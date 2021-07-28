package algorithm

import (
	"sync"

	"github.com/oecp/open-local-storage-service/pkg"

	volumesnapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v3/informers/externalversions/volumesnapshot/v1beta1"
	nodelocalstorageinformer "github.com/oecp/open-local-storage-service/pkg/generated/informers/externalversions/storage/v1alpha1"
	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm/cache"
	corev1 "k8s.io/api/core/v1"
	corev1informers "k8s.io/client-go/informers/core/v1"
	storagev1informers "k8s.io/client-go/informers/storage/v1"
)

type DiskBindingResult struct {
}

// NodeBinder is the interface implements the decisions
// for picking a piece of storage for a PersistentVolumeClaim
type NodeBinder interface {
	BindNode(pvc *corev1.PersistentVolumeClaim, node string) DiskBindingResult
	BindNodes(pvc *corev1.PersistentVolumeClaim) DiskBindingResult
}

// A context contains all necessary info for a extender to trigger
// an local volume scheduling/provisioning; these info are:
// 1. node cache
// 2. pv cache
// 3. nodelocalstorage cache

type SchedulingContext struct {
	CtxLock                sync.RWMutex
	ClusterNodeCache       *cache.ClusterNodeCache
	CoreV1Informers        corev1informers.Interface
	StorageV1Informers     storagev1informers.Interface
	SnapshotInformers      volumesnapshotinformers.Interface
	LocalStorageInformer   nodelocalstorageinformer.Interface
	NodeAntiAffinityWeight *pkg.NodeAntiAffinityWeight
}

func NewSchedulingContext(coreV1Informers corev1informers.Interface,
	storageV1informers storagev1informers.Interface,
	localStorageInformer nodelocalstorageinformer.Interface,
	snapshotInformer volumesnapshotinformers.Interface,
	weights *pkg.NodeAntiAffinityWeight) *SchedulingContext {

	return &SchedulingContext{
		CtxLock:                sync.RWMutex{},
		ClusterNodeCache:       cache.NewClusterNodeCache(),
		CoreV1Informers:        coreV1Informers,
		StorageV1Informers:     storageV1informers,
		LocalStorageInformer:   localStorageInformer,
		SnapshotInformers:      snapshotInformer,
		NodeAntiAffinityWeight: weights,
	}

}
