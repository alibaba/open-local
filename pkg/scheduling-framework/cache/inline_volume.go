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
	"fmt"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

type InlineVolumes []*InlineVolumeAllocated

func (volumes *InlineVolumes) HaveLocalVolumes() bool {
	if volumes == nil {
		return false
	}
	return len(*volumes) > 0
}

//InlineVolume allocated details
type InlineVolumeAllocated struct {
	VgName       string `json:"vgName,string"`
	VolumeName   string `json:"volumeName,string"`
	VolumeSize   int64  `json:"volumeSize,string"`
	PodName      string `json:"podName,string"`
	PodNamespace string `json:"podNamespace,string"`

	Allocated int64 // actual allocated size for the pvc
}

func (allocated *InlineVolumeAllocated) DeepCopy() *InlineVolumeAllocated {
	if allocated == nil {
		return nil
	}
	return &InlineVolumeAllocated{
		VgName:       allocated.VgName,
		VolumeName:   allocated.VolumeName,
		VolumeSize:   allocated.VolumeSize,
		PodName:      allocated.PodName,
		PodNamespace: allocated.PodNamespace,
		Allocated:    allocated.Allocated,
	}
}

type PodInlineVolumeAllocatedDetails []*InlineVolumeAllocated

func (details *PodInlineVolumeAllocatedDetails) DeepCopy() *PodInlineVolumeAllocatedDetails {
	if details == nil {
		return nil
	}
	copy := make(PodInlineVolumeAllocatedDetails, 0, len(*details))
	for _, detail := range *details {
		copy = append(copy, detail.DeepCopy())
	}
	return &copy
}

type NodeInlineVolumeAllocatedDetails map[string] /*podUid*/ *PodInlineVolumeAllocatedDetails

func (details NodeInlineVolumeAllocatedDetails) DeepCopy() NodeInlineVolumeAllocatedDetails {
	if details == nil {
		return nil
	}
	copy := map[string]*PodInlineVolumeAllocatedDetails{}
	for podUid, podInlineVolumeAllocatedDetails := range details {
		copy[podUid] = podInlineVolumeAllocatedDetails.DeepCopy()
	}
	return copy
}

func NewInlineVolumeAllocated(podName, podNameSpace, vgName, volumeName string, volumeSize int64) *InlineVolumeAllocated {
	return &InlineVolumeAllocated{
		PodName:      podName,
		PodNamespace: podNameSpace,
		VgName:       vgName,
		VolumeName:   volumeName,
		VolumeSize:   volumeSize,
	}
}

var _ InlineVolumeAllocator = &inlineVolumeAllocator{}

type inlineVolumeAllocator struct {
	cache *NodeStorageAllocatedCache
}

func NewInlineVolumeAllocator(cache *NodeStorageAllocatedCache) *inlineVolumeAllocator {
	return &inlineVolumeAllocator{cache: cache}
}

func (allocator *inlineVolumeAllocator) podAdd(pod *corev1.Pod) {
	contain, nodeName := utils.ContainInlineVolumes(pod)
	if !contain {
		return
	}
	nodeDetails, exist := allocator.cache.inlineVolumeAllocatedDetails[nodeName]
	if !exist {
		nodeDetails = NodeInlineVolumeAllocatedDetails{}
		allocator.cache.inlineVolumeAllocatedDetails[nodeName] = nodeDetails
	}

	//allocated return
	podDetailsExist, exist := nodeDetails[string(pod.UID)]
	if exist && len(*podDetailsExist) > 0 {
		klog.V(6).Infof("pod(%s) inlineVolume had allocated on node %s", string(pod.UID), nodeName)
		return
	}

	nodeStorageState := allocator.cache.initIfNeedAndGetNodeStoragePool(nodeName)

	podDetails := PodInlineVolumeAllocatedDetails{}

	for _, volume := range pod.Spec.Volumes {
		if volume.CSI != nil && utils.ContainsProvisioner(volume.CSI.Driver) {
			vgName, size := utils.GetInlineVolumeInfoFromParam(volume.CSI.VolumeAttributes)
			if vgName == "" {
				klog.Errorf("no vgName found in inline volume of Pod %s", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
				return
			}
			allocateInfo := NewInlineVolumeAllocated(pod.Name, pod.Namespace, vgName, volume.Name, size)

			if _, exist := nodeStorageState.VGStates[vgName]; !exist {
				nodeStorageState.VGStates[vgName] = NewVGState(vgName)
			}
			nodeStorageState.VGStates[vgName].Requested += size
			podDetails = append(podDetails, allocateInfo)
		}
	}
	nodeDetails[string(pod.UID)] = &podDetails
	klog.V(6).Infof("allocate inlineVolumes for pod(%s) success", pod.Name)
}

func (allocator *inlineVolumeAllocator) podDelete(pod *corev1.Pod) {
	contain, nodeName := utils.ContainInlineVolumes(pod)
	if !contain || nodeName == "" {
		return
	}

	nodeDetails, exist := allocator.cache.inlineVolumeAllocatedDetails[nodeName]
	if !exist {
		return
	}

	podDetails, exist := nodeDetails[string(pod.UID)]
	if !exist || len(*podDetails) == 0 {
		return
	}

	nodeStorageState, ok := allocator.cache.states[nodeName]
	if !ok {
		delete(nodeDetails, string(pod.UID))
		klog.Infof("no node(%s) state found, only delete inlineVolumes details for pod(%s) finished", nodeName, pod.Name)
		return
	}

	for _, volume := range *podDetails {
		if vgState, exist := nodeStorageState.VGStates[volume.VgName]; exist {
			vgState.Requested = vgState.Requested - volume.VolumeSize
		}
	}
	delete(nodeDetails, string(pod.UID))
	klog.V(6).Infof("allocate inlineVolumes for pod(%s) success", pod.Name)
}

func (allocator *inlineVolumeAllocator) prefilter(pod *corev1.Pod, podVolumeInfos *PodLocalVolumeInfo) error {
	var inlineVolumeAllocates []*InlineVolumeAllocated

	containInlineVolume, _ := utils.ContainInlineVolumes(pod)
	if !containInlineVolume {
		return nil
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.CSI != nil && utils.ContainsProvisioner(volume.CSI.Driver) {
			vgName, size := utils.GetInlineVolumeInfoFromParam(volume.CSI.VolumeAttributes)
			if vgName == "" {
				return fmt.Errorf("no vgName found in inline volume of Pod %s", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			}

			inlineVolumeAllocates = append(inlineVolumeAllocates, &InlineVolumeAllocated{
				PodNamespace: pod.Namespace,
				PodName:      pod.Name,
				VolumeName:   volume.Name,
				VolumeSize:   size,
				VgName:       vgName,
			})
		}
	}
	podVolumeInfos.InlineVolumes = inlineVolumeAllocates
	return nil
}

func (allocator *inlineVolumeAllocator) preAllocate(nodeName string, pod *corev1.Pod, inlineVolumes InlineVolumes, nodeStateClone *NodeStorageState) ([]*InlineVolumeAllocated, error) {
	if len(inlineVolumes) <= 0 {
		return nil, nil
	}

	allocatedInfos := allocator.cache.GetPodInlineVolumeDetailsCopy(nodeName, string(pod.UID))
	if allocatedInfos != nil && len(*allocatedInfos) > 0 {
		return nil, fmt.Errorf("node(%s) had allocated by pod", nodeName)
	}

	var units []*InlineVolumeAllocated
	for _, volume := range inlineVolumes {
		err := allocateVgState(nodeName, volume.VgName, nodeStateClone, volume.VolumeSize)
		if err != nil {
			return units, err
		}
		volumeCopy := volume.DeepCopy()
		volumeCopy.Allocated = volumeCopy.VolumeSize
		units = append(units, volumeCopy)
	}
	return units, nil
}

func (allocator *inlineVolumeAllocator) preRevert(nodeName string, pod *corev1.Pod, nodeStateClone *NodeStorageState) {
	inlineDetails := allocator.cache.GetPodInlineVolumeDetailsCopy(nodeName, string(pod.UID))
	if inlineDetails == nil || len(*inlineDetails) <= 0 {
		return
	}

	allocator.revertInlineVolumesStorage(nodeName, string(pod.UID), inlineDetails, nodeStateClone)
}

func (allocator *inlineVolumeAllocator) reserve(nodeName string, podUid string, units []*InlineVolumeAllocated) error {
	if len(units) == 0 {
		return nil
	}

	currentStorageState := allocator.cache.states[nodeName]

	nodeDetails, exist := allocator.cache.inlineVolumeAllocatedDetails[nodeName]
	if !exist {
		nodeDetails = NodeInlineVolumeAllocatedDetails{}
		allocator.cache.inlineVolumeAllocatedDetails[nodeName] = nodeDetails
	}

	//allocated return
	allocateExist, exist := nodeDetails[podUid]
	if exist && len(*allocateExist) > 0 {
		err := fmt.Errorf("reserveInlineVolumes fail, pod(%s) inlineVolume had allocated on node %s", podUid, nodeName)
		return err
	}

	podDetails := PodInlineVolumeAllocatedDetails{}
	for i, unit := range units {

		err := allocateVgState(nodeName, unit.VgName, currentStorageState, unit.VolumeSize)
		if err != nil {
			return err
		}
		units[i].Allocated = unit.VolumeSize
		podDetails = append(podDetails, units[i].DeepCopy())
		nodeDetails[podUid] = &podDetails
		klog.V(6).Infof("reserve for inlineVolume unit (%#v) success", unit)
	}
	return nil
}

func (allocator *inlineVolumeAllocator) unreserve(nodeName string, podUid string, units []*InlineVolumeAllocated) {
	if len(units) == 0 {
		return
	}

	for i := range units {
		units[i].Allocated = 0
	}

	allocator.unreserveDirect(nodeName, podUid)
}

func (allocator *inlineVolumeAllocator) unreserveDirect(nodeName, podUid string) {
	currentStorageState := allocator.cache.states[nodeName]
	nodeDetails, exist := allocator.cache.inlineVolumeAllocatedDetails[nodeName]
	if !exist {
		return
	}

	podInlineDetails := nodeDetails[podUid]

	allocator.revertInlineVolumesStorage(nodeName, podUid, podInlineDetails, currentStorageState)

	nodeDetails[podUid] = &PodInlineVolumeAllocatedDetails{}
}

func (allocator *inlineVolumeAllocator) revertInlineVolumesStorage(nodeName, podUid string, podInlineDetails *PodInlineVolumeAllocatedDetails, nodeState *NodeStorageState) {
	if podInlineDetails == nil || len(*podInlineDetails) <= 0 {
		return
	}

	for _, detail := range *podInlineDetails {

		if nodeState.VGStates == nil {
			klog.Errorf("unreserveInlineVolumes fail, no VG found on node %s", nodeName)
			continue
		}

		vgState, ok := nodeState.VGStates[detail.VgName]
		if !ok {
			klog.Errorf("unreserveInlineVolumes fail, volumeGroup(%s) have not found for pod(%s) on node %s", detail.VgName, podUid, nodeName)
			continue
		}

		vgState.Requested = vgState.Requested - detail.Allocated
		klog.V(6).Infof("unreserve for inlineVolume (%#v) success, current vgState: %#v", detail, vgState)
	}

}
