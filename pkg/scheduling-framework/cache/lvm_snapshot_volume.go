/*
Copyright 2022/9/9 Alibaba Cloud.
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
package cache

import (
	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"

	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
)

var _ PVCInfos = &LVMSnapshotPVCInfos{}

type LVMSnapshotPVCInfos []*LVMPVCInfo

func (info *LVMSnapshotPVCInfos) HaveLocalVolumes() bool {
	if info == nil {
		return false
	}
	return len(*info) > 0
}

var _ PVPVCAllocator = &lvmSnapshotPVAllocator{}

type lvmSnapshotPVAllocator struct {
	cache *NodeStorageAllocatedCache
}

func NewLVMSnapshotPVAllocator(cache *NodeStorageAllocatedCache) *lvmSnapshotPVAllocator {
	return &lvmSnapshotPVAllocator{
		cache: cache,
	}
}

func (allocator *lvmSnapshotPVAllocator) pvcAdd(nodeName string, pvc *corev1.PersistentVolumeClaim, volumeName string) {
}

func (allocator *lvmSnapshotPVAllocator) pvcUpdate(nodeName string, oldPVC, newPVC *corev1.PersistentVolumeClaim, volumeName string) {
}

func (allocator *lvmSnapshotPVAllocator) pvDelete(nodeName string, pv *corev1.PersistentVolume) {
}

func (allocator *lvmSnapshotPVAllocator) pvAdd(nodeName string, pv *corev1.PersistentVolume) {
}

func (allocator *lvmSnapshotPVAllocator) pvUpdate(nodeName string, oldPV, newPV *corev1.PersistentVolume) {
}

func (allocator *lvmSnapshotPVAllocator) prefilter(scLister storagelisters.StorageClassLister, snapClient snapshot.Interface, snapshotLocalPVC *corev1.PersistentVolumeClaim, podVolumeInfos *PodLocalVolumeInfo) error {
	if !utils.IsReadOnlySnapshotPVC2(snapshotLocalPVC, snapClient) {
		return nil
	}
	if podVolumeInfos.LVMPVCsROSnapshot == nil {
		podVolumeInfos.LVMPVCsROSnapshot = LVMSnapshotPVCInfos{}
	}
	lvmPVCInfo := &LVMPVCInfo{
		PVC:     snapshotLocalPVC,
		Request: utils.GetPVCRequested(snapshotLocalPVC),
	}
	podVolumeInfos.LVMPVCsROSnapshot = append(podVolumeInfos.LVMPVCsROSnapshot, lvmPVCInfo)
	return nil
}

func (allocator *lvmSnapshotPVAllocator) preAllocate(nodeName string, podVolumeInfos *PodLocalVolumeInfo, nodeStateClone *NodeStorageState) ([]PVAllocated, error) {
	return nil, nil
}

func (allocator *lvmSnapshotPVAllocator) allocateInfo(detail PVAllocated) *pkg.PVCAllocateInfo {
	return nil
}

func (allocator *lvmSnapshotPVAllocator) isPVHaveAllocateInfo(pv *corev1.PersistentVolume) bool {
	return true
}

func (allocator *lvmSnapshotPVAllocator) reserve(nodeName string, units *NodeAllocateUnits) error {
	return nil
}

func (allocator *lvmSnapshotPVAllocator) unreserve(nodeName string, units *NodeAllocateUnits) {
}
