/*
Copyright © 2021 Alibaba Group Holding Ltd.

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

package algorithm

import (
	"sync"

	"github.com/alibaba/open-local/pkg"

	nodelocalstorageinformer "github.com/alibaba/open-local/pkg/generated/informers/externalversions/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	volumesnapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v4/informers/externalversions/volumesnapshot/v1"
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
