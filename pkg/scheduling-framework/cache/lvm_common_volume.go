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
	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/klog/v2"
)

/**
prefilter pvc infos for pod
*/
type LVMPVCInfo struct {
	VGName  string
	Request int64
	PVC     *corev1.PersistentVolumeClaim
}

var _ PVCInfos = &LVMCommonPVCInfos{}

type LVMCommonPVCInfos struct {
	LVMPVCsWithVgNameNotAllocated    []*LVMPVCInfo //local lvm PVC have vgName and  had not allocated before
	LVMPVCsWithoutVgNameNotAllocated []*LVMPVCInfo //local lvm PVC have no vgName and had not allocated before
}

func NewLVMCommonPVCInfos() *LVMCommonPVCInfos {
	return &LVMCommonPVCInfos{
		LVMPVCsWithVgNameNotAllocated:    []*LVMPVCInfo{},
		LVMPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{},
	}
}

func (info *LVMCommonPVCInfos) HaveLocalVolumes() bool {
	if info == nil {
		return false
	}
	return len(info.LVMPVCsWithVgNameNotAllocated) > 0 || len(info.LVMPVCsWithoutVgNameNotAllocated) > 0
}

type LVMPVAllocated struct {
	BasePVAllocated
	VGName string
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

var _ PVPVCAllocator = &lvmCommonPVAllocator{}

/**
LVM Type PVC/PV allocator
*/
type lvmCommonPVAllocator struct {
	cache            *NodeStorageAllocatedCache
	scheduleStrategy VGScheduleStrategy
}

func NewLVMCommonPVAllocator(cache *NodeStorageAllocatedCache) PVPVCAllocator {
	return &lvmCommonPVAllocator{
		cache:            cache,
		scheduleStrategy: GetVGScheduleStrategy(cache.strategyType),
	}
}

func (allocator *lvmCommonPVAllocator) pvcAdd(nodeName string, pvc *corev1.PersistentVolumeClaim, volumeName string) {
	if pvc == nil {
		return
	}
	if pvc.Status.Phase != corev1.ClaimBound {
		klog.Infof("pv %s is in %s status, skipped", utils.PVCName(pvc), pvc.Status.Phase)
		return
	}

	oldPVCDetail := allocator.cache.pvAllocatedDetails.GetByPVC(pvc.Namespace, pvc.Name)

	//if local pvc, update request
	if oldPVCDetail != nil {
		oldPVCDetail.GetBasePVAllocated().Requested = utils.GetPVCRequested(pvc)
	}

	oldPVDetail := allocator.cache.pvAllocatedDetails.GetByPV(volumeName)
	if oldPVDetail == nil {
		//plugin starting and receive pvc event first, so can allocate by pv event later
		return
	}

	maxRequest := utils.GetPVCRequested(pvc)
	//max(pvcRequest,pvRequest)
	if maxRequest < oldPVDetail.GetBasePVAllocated().Requested {
		maxRequest = oldPVDetail.GetBasePVAllocated().Requested
	}

	deltaAllocate := maxRequest - oldPVDetail.GetBasePVAllocated().Allocated
	if deltaAllocate <= 0 {
		return
	}

	lvmAllocated, ok := oldPVDetail.(*LVMPVAllocated)
	if !ok {
		klog.Errorf("can not convert pv(%s) AllocateInfo to LVMPVAllocated", volumeName)
		return
	}

	vgState, initedByNLS := allocator.cache.initIfNeedAndGetVGState(nodeName, lvmAllocated.VGName)

	if vgState.Allocatable < vgState.Requested+deltaAllocate {
		klog.Warningf("volumeGroup(%s) have not enough space or not init by NLS(init:%v) for pvc(%s) on node %s", lvmAllocated.VGName, initedByNLS, utils.PVCName(pvc), nodeName)
	}

	vgState.Requested = vgState.Requested + deltaAllocate
	allocator.cache.pvAllocatedDetails.AssumeAllocateSizeToPVDetail(volumeName, maxRequest)
	klog.V(6).Infof("expand for lvm pvc old detail(%#v) to %d success, current vgState: %#v", oldPVDetail, maxRequest, vgState)
}

func (allocator *lvmCommonPVAllocator) pvcUpdate(nodeName string, oldPVC, newPVC *corev1.PersistentVolumeClaim, volumeName string) {
	allocator.pvcAdd(nodeName, newPVC, volumeName)
}

func (allocator *lvmCommonPVAllocator) pvAdd(nodeName string, pv *corev1.PersistentVolume) {
	if pv == nil {
		return
	}

	if pv.Status.Phase == corev1.VolumePending {
		klog.Infof("pv %s is in %s status, skipped", pv.Name, pv.Status.Phase)
		return
	}

	vgName := utils.GetVGNameFromCsiPV(pv)

	if vgName == "" {
		switch pv.Status.Phase {
		case corev1.VolumeBound:
			pvcName, pvcNamespace := utils.PVCNameFromPV(pv)
			if pvcName == "" {
				klog.Errorf("pv(%s) is bound, but not found pvcName on pv", pv.Name)
				return
			}
			pvcDetail := allocator.cache.pvAllocatedDetails.GetByPVC(pvcNamespace, pvcName)
			if pvcDetail == nil {
				klog.Errorf("can't find pvcDetail for pvc(%s)", utils.GetPVCKey(pvcNamespace, pvcName))
				return
			}
			lvmDetail := pvcDetail.(*LVMPVAllocated)
			vgName = lvmDetail.VGName
		default:
			klog.V(6).Infof("pv %s is not bound to any volume group, skipped", pv.Name)
			return

		}
	}

	if vgName == "" {
		klog.Errorf("AllocateLVMByPV skip : vgName not found", pv.Name)
		return
	}

	new := NewLVMPVAllocatedFromPV(pv, vgName, nodeName)

	pvcRequest := allocator.cache.getRequestFromPVCInfos(pv)

	maxRequest := pvcRequest //pv may resize small, so can update pv request
	//max(pvcRequest,newPVRequest)
	if maxRequest < new.Requested {
		maxRequest = new.Requested
	}

	old := allocator.cache.pvAllocatedDetails.GetByPV(pv.Name)
	deltaAllocate := maxRequest
	if old != nil {
		deltaAllocate = maxRequest - old.GetBasePVAllocated().Allocated
	}

	vgState, initedByNLS := allocator.cache.initIfNeedAndGetVGState(nodeName, vgName)

	// 	resolve allocated duplicate for allocate pvc by scheduler
	allocator.revertIfNeed(nodeName, new.PVCNamespace, new.PVCName)

	//request:pvc 100Gi pv 200Gi,allocate: 200Gi => request:pvc 100Gi pv 100Gi,allocate: 100Gi can success
	if deltaAllocate == 0 {
		return
	}

	if vgState.Allocatable < vgState.Requested+deltaAllocate {
		klog.Warningf("volumeGroup(%s) have not enough space or not init by NLS(init:%v) for pv(%s) on node %s", vgName, initedByNLS, pv.Name, nodeName)
	}

	vgState.Requested = vgState.Requested + deltaAllocate
	new.Allocated = maxRequest
	allocator.cache.pvAllocatedDetails.AssumeByPVEvent(new)
	klog.V(6).Infof("allocate for lvm pv (%#v) success, current vgState: %#v", new, vgState)
}

func (allocator *lvmCommonPVAllocator) pvUpdate(nodeName string, oldPV, newPV *corev1.PersistentVolume) {
	allocator.pvAdd(nodeName, newPV)
}

func (allocator *lvmCommonPVAllocator) pvDelete(nodeName string, pv *corev1.PersistentVolume) {
	vgName := utils.GetVGNameFromCsiPV(pv)
	if vgName == "" {
		klog.V(6).Infof("pv %s is not bound to any volume group, skipped", pv.Name)
		return
	}

	old := allocator.cache.pvAllocatedDetails.GetByPV(pv.Name)

	if old == nil {
		return
	}

	allocator.updateVGRequestByDelta(nodeName, vgName, -old.GetBasePVAllocated().Allocated)
	allocator.cache.pvAllocatedDetails.DeleteByPV(old)
}

func (allocator *lvmCommonPVAllocator) prefilter(scLister storagelisters.StorageClassLister, lvmPVC *corev1.PersistentVolumeClaim, podVolumeInfos *PodLocalVolumeInfo) error {
	if utils.IsSnapshotPVC(lvmPVC) {
		return nil
	}
	if allocator.cache.GetPVCAllocatedDetailCopy(lvmPVC.Namespace, lvmPVC.Name) != nil {
		return nil
	}
	var vgName string
	vgName, err := utils.GetVGNameFromPVC(lvmPVC, scLister)
	if err != nil {
		error := fmt.Errorf("get VGName from PVC(%s) error: %s", utils.PVCName(lvmPVC), err.Error())
		return error
	}

	lvmPVCInfo := &LVMPVCInfo{
		PVC:     lvmPVC,
		Request: utils.GetPVCRequested(lvmPVC),
		VGName:  vgName,
	}

	if podVolumeInfos.LVMPVCsNotSnapshot == nil {
		podVolumeInfos.LVMPVCsNotSnapshot = NewLVMCommonPVCInfos()
	}
	infos := podVolumeInfos.LVMPVCsNotSnapshot

	if lvmPVCInfo.VGName == "" {
		// 里面没有 vg 信息
		infos.LVMPVCsWithoutVgNameNotAllocated = append(infos.LVMPVCsWithoutVgNameNotAllocated, lvmPVCInfo)
	} else {
		infos.LVMPVCsWithVgNameNotAllocated = append(infos.LVMPVCsWithVgNameNotAllocated, lvmPVCInfo)
	}
	return nil
}

func (allocator *lvmCommonPVAllocator) preAllocate(nodeName string, podVolumeInfos *PodLocalVolumeInfo, nodeStateClone *NodeStorageState) ([]PVAllocated, error) {
	var allocateUnits []PVAllocated
	if !podVolumeInfos.LVMPVCsNotSnapshot.HaveLocalVolumes() {
		return allocateUnits, nil
	}

	if len(nodeStateClone.VGStates) <= 0 {
		err := fmt.Errorf("no vg found with node %s", nodeName)
		klog.Error(err)
		return allocateUnits, err
	}

	infos := podVolumeInfos.LVMPVCsNotSnapshot

	for _, pvcInfo := range infos.LVMPVCsWithVgNameNotAllocated {

		err := allocateVgState(nodeName, pvcInfo.VGName, nodeStateClone, pvcInfo.Request)
		if err != nil {
			return allocateUnits, err
		}

		allocateUnits = append(allocateUnits, &LVMPVAllocated{
			BasePVAllocated: BasePVAllocated{
				PVCName:      pvcInfo.PVC.Name,
				PVCNamespace: pvcInfo.PVC.Namespace,
				NodeName:     nodeName,
				Requested:    pvcInfo.Request,
				Allocated:    pvcInfo.Request,
			},
			VGName: pvcInfo.VGName,
		})
	}

	if len(infos.LVMPVCsWithoutVgNameNotAllocated) <= 0 {
		return allocateUnits, nil
	}

	vgStateList := make([]*VGStoragePool, 0, len(nodeStateClone.VGStates))

	for key := range nodeStateClone.VGStates {
		vgStateList = append(vgStateList, nodeStateClone.VGStates[key])
	}

	// process pvcsWithoutVG
	for _, pvcInfo := range infos.LVMPVCsWithoutVgNameNotAllocated {

		allocateUnit, err := allocator.scheduleStrategy.AllocateForPVCWithoutVgName(nodeName, &vgStateList, pvcInfo)
		if err != nil {
			return allocateUnits, err
		}
		allocateUnits = append(allocateUnits, allocateUnit)
	}
	return allocateUnits, nil
}

func (allocator *lvmCommonPVAllocator) allocateInfo(detail PVAllocated) *localtype.PVCAllocateInfo {
	allocateDetail, ok := detail.(*LVMPVAllocated)
	if !ok {
		klog.Infof("could't convert detail (%#v) to lvm type", detail)
		return nil
	}
	if allocateDetail.VGName != "" && allocateDetail.Allocated > 0 {
		return &localtype.PVCAllocateInfo{
			PVCNameSpace: allocateDetail.PVCNamespace,
			PVCName:      allocateDetail.PVCName,
			PVAllocatedInfo: localtype.PVAllocatedInfo{
				VGName:     allocateDetail.VGName,
				VolumeType: string(localtype.VolumeTypeLVM),
			},
		}
	}
	return nil
}

func (allocator *lvmCommonPVAllocator) isPVHaveAllocateInfo(pv *corev1.PersistentVolume) bool {
	return utils.GetVGNameFromCsiPV(pv) != ""
}

func (allocator *lvmCommonPVAllocator) reserve(nodeName string, nodeUnits *NodeAllocateUnits) error {
	if len(nodeUnits.LVMPVCAllocateUnits) == 0 {
		return nil
	}

	currentStorageState := allocator.cache.states[nodeName]

	for _, unit := range nodeUnits.LVMPVCAllocateUnits {

		allocateExist := allocator.cache.pvAllocatedDetails.GetByPVC(unit.PVCNamespace, unit.PVCName)
		if allocateExist != nil { //add by eventhandler for pvcBound
			continue
		}
		if pvcInfo, ok := allocator.cache.pvcInfosMap[utils.GetPVCKey(unit.PVCNamespace, unit.PVCName)]; ok && pvcInfo.PVCStatus == corev1.ClaimBound {
			klog.Infof("skip reserveLVMPVC for bound pvc %s/%s", unit.PVCNamespace, unit.PVCName)
			continue
		}

		if currentStorageState.VGStates == nil {
			err := fmt.Errorf("reserveLVMPVC fail, no VG found on node %s", nodeName)
			return err
		}

		vgState, ok := currentStorageState.VGStates[unit.VGName]
		if !ok {
			err := fmt.Errorf("reserveLVMPVC fail, volumeGroup(%s) have not found for pvc(%s) on node %s", unit.VGName, utils.GetPVCKey(unit.PVCNamespace, unit.PVCName), nodeName)
			return err
		}

		if vgState.Allocatable < vgState.Requested+unit.Requested {
			err := fmt.Errorf("reserveLVMPVC fail, volumeGroup(%s) have not enough space for pvc(%s) on node %s", unit.VGName, utils.GetPVCKey(unit.PVCNamespace, unit.PVCName), nodeName)
			return err
		}

		vgState.Requested = vgState.Requested + unit.Requested
		unit.Allocated = unit.Requested
		allocator.cache.pvAllocatedDetails.AssumeByPVC(unit.DeepCopy())
		klog.V(6).Infof("reserve for lvm pvc unit (%#v) success, current vgState: %#v", unit, vgState)
	}
	return nil

}

func (allocator *lvmCommonPVAllocator) unreserve(nodeName string, units *NodeAllocateUnits) {
	if len(units.LVMPVCAllocateUnits) == 0 {
		return
	}

	for i, unit := range units.LVMPVCAllocateUnits {

		if unit.Allocated == 0 {
			continue
		}

		allocator.revertIfNeed(nodeName, unit.PVCNamespace, unit.PVCName)
		units.LVMPVCAllocateUnits[i].Allocated = 0
	}
}

func (allocator *lvmCommonPVAllocator) revertIfNeed(nodeName, pvcNameSpace, pvcName string) {
	if pvcNameSpace == "" && pvcName == "" {
		return
	}

	//pv allocated, then remove pvc allocated size
	allocatedByPVC := allocator.cache.pvAllocatedDetails.GetByPVC(pvcNameSpace, pvcName)
	if allocatedByPVC == nil {
		return
	}

	lvmAllocated, ok := allocatedByPVC.(*LVMPVAllocated)
	if !ok {
		klog.Errorf("can not convert pvc(%s) AllocateInfo to LVMPVAllocated", utils.GetPVCKey(pvcNameSpace, pvcName))
		return
	}
	//add by pvc event, no vgName && allocateSize = 0
	if lvmAllocated.VGName == "" || allocatedByPVC.GetBasePVAllocated().Allocated <= 0 {
		return
	}

	nodeState, ok := allocator.cache.states[nodeName]
	if !ok || !nodeState.InitedByNLS {
		klog.Errorf("revertLVMByPVCIfNeed fail for node(%s), storage not init by NLS", nodeName)
		return
	}
	if len(nodeState.VGStates) <= 0 {
		klog.Errorf("revertLVMByPVCIfNeed fail, no VG found on node %s", nodeName)
		return
	}

	vgState, ok := nodeState.VGStates[lvmAllocated.VGName]
	if !ok {
		klog.Errorf("revertLVMByPVCIfNeed fail, volumeGroup(%s) have not found for pvc(%s) on node %s", lvmAllocated.VGName, utils.GetPVCKey(pvcNameSpace, pvcName), nodeName)
		return
	}

	vgState.Requested = vgState.Requested - lvmAllocated.Allocated
	allocator.cache.pvAllocatedDetails.DeleteByPVC(utils.GetPVCKey(pvcNameSpace, pvcName))
	klog.V(6).Infof("revert for pvc (%s) success, current vgState: %#v", utils.GetPVCKey(pvcNameSpace, pvcName), vgState)
}

func (allocator *lvmCommonPVAllocator) updateVGRequestByDelta(nodeName string, vgName string, delta int64) {
	nodeStoragePool, ok := allocator.cache.states[nodeName]
	if !ok {
		nodeStoragePool = NewNodeStorageState()
		allocator.cache.states[nodeName] = nodeStoragePool
	}

	_, ok = nodeStoragePool.VGStates[vgName]
	if !ok {
		nodeStoragePool.VGStates[vgName] = NewVGState(vgName)
	}

	if vgState, ok := nodeStoragePool.VGStates[vgName]; ok {
		vgState.Requested = vgState.Requested + delta
	}

}
