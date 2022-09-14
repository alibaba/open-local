/*
Copyright 2022/8/17 Alibaba Cloud.

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
	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

type VGStoragePool struct {
	Name        string
	Total       int64
	Allocatable int64
	Requested   int64
}

func NewVGState(vgName string) *VGStoragePool {
	return &VGStoragePool{
		Name: vgName,
	}
}

func NewVGStateFromVGInfo(vgInfo nodelocalstorage.VolumeGroup) *VGStoragePool {
	return &VGStoragePool{Name: vgInfo.Name, Total: int64(vgInfo.Total), Allocatable: int64(vgInfo.Allocatable), Requested: 0}
}

func (vg *VGStoragePool) UpdateByNLS(new *VGStoragePool) {
	if vg == nil || new == nil {
		return
	}
	vg.Total = new.Total
	vg.Allocatable = new.Allocatable
}

func (vg *VGStoragePool) DeepCopy() *VGStoragePool {
	if vg == nil {
		return nil
	}
	copy := &VGStoragePool{
		Name:        vg.Name,
		Total:       vg.Total,
		Allocatable: vg.Allocatable,
		Requested:   vg.Requested,
	}
	return copy
}

func (vg *VGStoragePool) GetName() string {
	if vg == nil {
		return ""
	}
	return vg.Name
}

/*VG support allocateType: LvmPV, InlineVolume*/
type VGStates map[string] /*vgName*/ *VGStoragePool

func (s VGStates) DeepCopy() VGStates {
	copy := VGStates{}
	for name, vgState := range s {
		copy[name] = vgState.DeepCopy()
	}
	return copy
}

func (s VGStates) GetVGStateList() []*VGStoragePool {
	result := make([]*VGStoragePool, 0, len(s))
	for _, state := range s {
		result = append(result, state)
	}
	return result
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

type LVMPVAllocated struct {
	BasePVAllocated
	VGName string
}

func NewLVMAllocatedFromPVC(pvc *corev1.PersistentVolumeClaim, nodeName, volumeName string) *LVMPVAllocated {
	return &LVMPVAllocated{
		BasePVAllocated: BasePVAllocated{
			PVCNamespace: pvc.Namespace,
			PVCName:      pvc.Name,
			VolumeName:   volumeName,
			NodeName:     nodeName,
			Requested:    int64(utils.GetPVCRequested(pvc)),
			Allocated:    int64(0),
		},
	}
}

// pv bounding status: have pvcName, other status may have no pvcName
func NewLVMPVAllocatedFromPV(pv *corev1.PersistentVolume, vgName, nodeName string) *LVMPVAllocated {

	allocated := &LVMPVAllocated{BasePVAllocated: BasePVAllocated{
		VolumeName: pv.Name,
		NodeName:   nodeName,
	}, VGName: vgName}

	request, ok := pv.Spec.Capacity[corev1.ResourceStorage]
	if !ok {
		klog.Errorf("get request from pv(%s) failed, skipped", pv.Name)
		return allocated
	}

	allocated.Requested = request.Value()
	allocated.Allocated = request.Value()

	pvcName, pvcNamespace := utils.PVCNameFromPV(pv)
	if pvcName != "" {
		allocated.PVCName = pvcName
		allocated.PVCNamespace = pvcNamespace
	}
	return allocated
}

func (lvm *LVMPVAllocated) GetBasePVAllocated() *BasePVAllocated {
	if lvm == nil {
		return nil
	}
	return &lvm.BasePVAllocated
}

func (lvm *LVMPVAllocated) GetVolumeType() localtype.VolumeType {
	return localtype.VolumeTypeLVM
}

func (lvm *LVMPVAllocated) DeepCopy() PVAllocated {
	if lvm == nil {
		return nil
	}
	return &LVMPVAllocated{
		BasePVAllocated: *lvm.BasePVAllocated.DeepCopy(),
		VGName:          lvm.VGName,
	}
}

type VGHandler struct {
}

func (h *VGHandler) CreateStatesByNodeLocal(nodeLocal *nodelocalstorage.NodeLocalStorage) map[string]*VGStoragePool {
	states := map[string]*VGStoragePool{}
	// VGs
	vgInfoMap := make(map[string]nodelocalstorage.VolumeGroup, len(nodeLocal.Status.FilteredStorageInfo.VolumeGroups))
	for _, vg := range nodeLocal.Status.NodeStorageInfo.VolumeGroups {
		vgInfoMap[vg.Name] = vg
	}
	// add vgs
	for _, vgName := range nodeLocal.Status.FilteredStorageInfo.VolumeGroups {
		vgInfo, ok := vgInfoMap[vgName]
		if !ok {
			klog.Warningf("Get VgInfo from nodeLocal failed! VGName:%s, nodeName %s", vgName, nodeLocal.Name)
			continue
		}

		vgResource := NewVGStateFromVGInfo(vgInfo)
		states[vgName] = vgResource
		klog.V(6).Infof("initVGStorage, add vgResource success: %#v", vgResource)
	}
	return states
}

func (h *VGHandler) StatesForUpdate(old, new map[string]*VGStoragePool) map[string]*VGStoragePool {
	mergeStates := map[string]*VGStoragePool{}
	for _, state := range old {
		//add state if new exist
		if _, exist := new[state.GetName()]; exist {
			mergeStates[state.GetName()] = state
		}
	}
	for _, state := range new {
		name := state.GetName()
		_, ok := mergeStates[name]
		if ok {
			mergeStates[name].UpdateByNLS(state)
		} else {
			mergeStates[name] = state
		}
	}
	return mergeStates
}
