/*
Copyright 2022/9/8 Alibaba Cloud.

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
	corev1 "k8s.io/api/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
)

type PVCInfos interface {
	HaveLocalVolumes() bool
}

/*
	calculate at step preFilter to avoid duplicate calculating after
*/
type PodLocalVolumeInfo struct {
	LVMPVCs       *LVMCommonPVCInfos
	DevicePVCs    *DevicePVCInfos
	InlineVolumes InlineVolumes
}

func NewPodLocalVolumeInfo() *PodLocalVolumeInfo {
	return &PodLocalVolumeInfo{
		DevicePVCs:    NewDevicePVCInfos(),
		LVMPVCs:       NewLVMCommonPVCInfos(),
		InlineVolumes: InlineVolumes{},
	}
}

func (info *PodLocalVolumeInfo) HaveLocalVolumes() bool {
	if info == nil {
		return false
	}
	return info.LVMPVCs.HaveLocalVolumes() ||
		info.DevicePVCs.HaveLocalVolumes() || info.InlineVolumes.HaveLocalVolumes()
}

/*
	use by event handler
*/
type PVPVCEventAllocator interface {
	pvcAdd(nodeName string, pvc *corev1.PersistentVolumeClaim, volumeName string)
	pvcUpdate(nodeName string, oldPVC, newPVC *corev1.PersistentVolumeClaim, volumeName string)
	pvDelete(nodeName string, pv *corev1.PersistentVolume)
	pvAdd(nodeName string, pv *corev1.PersistentVolume)
	pvUpdate(nodeName string, oldPV, newPV *corev1.PersistentVolume)
}

/* user by scheduler*/
type PVCScheduleAllocator interface {
	//prefilter: caculate podVolumeInfos
	prefilter(scLister storagelisters.StorageClassLister, localPVC *corev1.PersistentVolumeClaim, podVolumeInfos *PodLocalVolumeInfo) error
	//use by scheduler filter/reserve, to preAllocate resource
	preAllocate(nodeName string, podVolumeInfos *PodLocalVolumeInfo, nodeStateClone *NodeStorageState) ([]PVAllocated, error)
	//convert detail to allocateInfo which will patch to PV
	allocateInfo(detail PVAllocated) *pkg.PVCAllocateInfo
	//check pv have allocateInfo like vgName/deviceName
	isPVHaveAllocateInfo(pv *corev1.PersistentVolume) bool
	// reserve
	reserve(nodeName string, units *NodeAllocateUnits) error
	// unreserve
	unreserve(nodeName string, units *NodeAllocateUnits)
}

type PVPVCAllocator interface {
	PVPVCEventAllocator
	PVCScheduleAllocator
}

/*
	use by event handler
*/
type InlineVolumeEventAllocator interface {
	podAdd(pod *corev1.Pod)
	podDelete(pod *corev1.Pod)
}

type InlineVolumeScheduleAllocator interface {
	//prefilter: caculate podVolumeInfos
	prefilter(pod *corev1.Pod, podVolumeInfos *PodLocalVolumeInfo) error
	//use by scheduler filter/reserve, to preAllocate resource
	preAllocate(nodeName string, pod *corev1.Pod, inlineVolumes InlineVolumes, nodeStateClone *NodeStorageState) ([]*InlineVolumeAllocated, error)
	// user for reservation, when pod use reservation Pod resource, should preRevert before preAllocate
	preRevert(nodeName string, pod *corev1.Pod, nodeStateClone *NodeStorageState)
	//scheduler reserve
	reserve(nodeName string, podUid string, units []*InlineVolumeAllocated) error
	//scheduler unreserve
	unreserve(nodeName string, podUid string, reservationPodUnits []*InlineVolumeAllocated)
	//unreserve reservation pod, before reserve real pod
	unreserveDirect(nodeName string, podUid string)
}

type InlineVolumeAllocator interface {
	InlineVolumeEventAllocator
	InlineVolumeScheduleAllocator
}
