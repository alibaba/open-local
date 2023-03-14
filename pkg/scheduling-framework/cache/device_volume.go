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
	"sort"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/errors"
	"github.com/alibaba/open-local/pkg/utils"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/klog/v2"
)

type DevicePVCInfo struct {
	MediaType localtype.MediaType
	Request   int64
	PVC       *corev1.PersistentVolumeClaim
}

var _ PVCInfos = &DevicePVCInfos{}

type DevicePVCInfos struct {
	SSDDevicePVCs []*DevicePVCInfo //local lvm PVC have vgName and  had not allocated before
	HDDDevicePVCs []*DevicePVCInfo //local lvm PVC have no vgName and had not allocated before
}

func NewDevicePVCInfos() *DevicePVCInfos {
	return &DevicePVCInfos{
		SSDDevicePVCs: []*DevicePVCInfo{},
		HDDDevicePVCs: []*DevicePVCInfo{},
	}
}

func (info *DevicePVCInfos) HaveLocalVolumes() bool {
	if info == nil {
		return false
	}
	return len(info.SSDDevicePVCs) > 0 || len(info.HDDDevicePVCs) > 0
}

type DeviceTypePVAllocated struct {
	BasePVAllocated
	DeviceName string
}

func NewDeviceTypePVAllocatedFromPV(pv *corev1.PersistentVolume, deviceName, nodeName string) *DeviceTypePVAllocated {

	allocated := &DeviceTypePVAllocated{BasePVAllocated: BasePVAllocated{
		VolumeName: pv.Name,
		NodeName:   nodeName,
	}, DeviceName: deviceName}

	pvcName, pvcNamespace := utils.PVCNameFromPV(pv)
	if pvcName != "" {
		allocated.PVCName = pvcName
		allocated.PVCNamespace = pvcNamespace
	}

	request, ok := pv.Spec.Capacity[corev1.ResourceStorage]
	if !ok {
		klog.Errorf("get request from pv(%s) failed, skipped", pv.Name)
		return allocated
	}

	allocated.Requested = request.Value()
	return allocated
}

func (device *DeviceTypePVAllocated) GetVolumeType() localtype.VolumeType {
	return localtype.VolumeTypeDevice
}

func (device *DeviceTypePVAllocated) DeepCopy() PVAllocated {
	if device == nil {
		return nil
	}
	return &DeviceTypePVAllocated{
		BasePVAllocated: *device.BasePVAllocated.DeepCopy(),
		DeviceName:      device.DeviceName,
	}
}

func (device *DeviceTypePVAllocated) GetBasePVAllocated() *BasePVAllocated {
	if device == nil {
		return nil
	}
	return &device.BasePVAllocated
}

var _ PVPVCAllocator = &devicePVAllocator{}

type devicePVAllocator struct {
	cache *NodeStorageAllocatedCache
}

func NewDevicePVAllocator(cache *NodeStorageAllocatedCache) *devicePVAllocator {
	return &devicePVAllocator{cache: cache}
}

func (allocator *devicePVAllocator) pvcAdd(nodeName string, pvc *corev1.PersistentVolumeClaim, volumeName string) {
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
}

func (allocator *devicePVAllocator) pvcUpdate(nodeName string, oldPVC, newPVC *corev1.PersistentVolumeClaim, volumeName string) {
	allocator.pvcAdd(nodeName, newPVC, volumeName)
}

func (allocator *devicePVAllocator) pvDelete(nodeName string, pv *corev1.PersistentVolume) {
	deviceName := utils.GetDeviceNameFromCsiPV(pv)
	if deviceName == "" {
		klog.Errorf("deleteDevice: pv %s is not a valid open-local pv(device with name)", pv.Name)
		return
	}

	old := allocator.cache.pvAllocatedDetails.GetByPV(pv.Name)
	if old == nil {
		return
	}

	allocator.revertDeviceForState(nodeName, deviceName)
	allocator.cache.pvAllocatedDetails.DeleteByPV(old)
}

func (allocator *devicePVAllocator) pvAdd(nodeName string, pv *corev1.PersistentVolume) {
	allocator.pvUpdate(nodeName, nil, pv)
}

func (allocator *devicePVAllocator) pvUpdate(nodeName string, oldPV, newPV *corev1.PersistentVolume) {
	if newPV == nil {
		return
	}
	if newPV.Status.Phase == corev1.VolumePending {
		klog.Infof("pv %s is in %s status, skipped", newPV.Name, newPV.Status.Phase)
		return
	}

	deviceName := utils.GetDeviceNameFromCsiPV(newPV)

	if deviceName == "" {
		switch newPV.Status.Phase {
		case corev1.VolumeBound:
			pvcName, pvcNamespace := utils.PVCNameFromPV(newPV)
			if pvcName == "" {
				klog.Errorf("pv(%s) is bound, but not found pvcName on pv", newPV.Name)
				return
			}
			pvcDetail := allocator.cache.pvAllocatedDetails.GetByPVC(pvcNamespace, pvcName)
			if pvcDetail == nil {
				klog.Errorf("can't find pvcDetail for pvc(%s)", utils.GetNameKey(pvcNamespace, pvcName))
				return
			}
			deviceDetail := pvcDetail.(*DeviceTypePVAllocated)
			deviceName = deviceDetail.DeviceName
		default:
			klog.V(6).Infof("pv %s is not bound to any device, skipped", newPV.Name)
			return

		}
	}

	if deviceName == "" {
		klog.Errorf("allocateDevice : pv %s is not a valid open-local pv(device with name)", newPV.Name)
		return
	}

	new := NewDeviceTypePVAllocatedFromPV(newPV, deviceName, nodeName)

	deviceState, initNLS := allocator.initIfNeedAndGetDeviceState(nodeName, deviceName)

	if initNLS {
		new.Allocated = deviceState.Allocatable
	} else {
		klog.Infof("device(%s) have not not init by NLS(inti:%v) for pv(%s) on node %s", deviceName, initNLS, newPV.Name, nodeName)
		new.Allocated = new.Requested
	}

	//resolve allocated duplicate for staticBounding by volumeBinding plugin may allocate pvc on on other device,so should revert
	allocator.revertDeviceByPVCIfNeed(nodeName, new.PVCNamespace, new.PVCName)

	allocator.allocateDeviceForState(nodeName, deviceName, new.Allocated)
	allocator.cache.pvAllocatedDetails.AssumeByPVEvent(new)
	klog.V(6).Infof("allocate for device pv (%#v) success, current deviceState: %#v", new, deviceState)
}

func (allocator *devicePVAllocator) prefilter(scLister storagelisters.StorageClassLister, snapClient snapshot.Interface, localPVC *corev1.PersistentVolumeClaim, podVolumeInfos *PodLocalVolumeInfo) error {
	if allocator.cache.GetPVCAllocatedDetailCopy(localPVC.Namespace, localPVC.Name) != nil {
		return nil
	}
	pvcInfo := &DevicePVCInfo{PVC: localPVC, Request: utils.GetPVCRequested(localPVC)}
	var mediaType localtype.MediaType
	mediaType, err := utils.GetMediaTypeFromPVC(localPVC, scLister)
	if err != nil {
		error := fmt.Errorf("get MediaType from PVC(%s) error: %s", utils.PVCName(localPVC), err.Error())
		return error
	}

	if podVolumeInfos.DevicePVCs == nil {
		podVolumeInfos.DevicePVCs = NewDevicePVCInfos()
	}

	deviceInfos := podVolumeInfos.DevicePVCs

	switch mediaType {
	case localtype.MediaTypeSSD:
		pvcInfo.MediaType = mediaType
		deviceInfos.SSDDevicePVCs = append(deviceInfos.SSDDevicePVCs, pvcInfo)
	case localtype.MediaTypeHDD:
		pvcInfo.MediaType = mediaType
		deviceInfos.HDDDevicePVCs = append(deviceInfos.HDDDevicePVCs, pvcInfo)
	default:
		return fmt.Errorf("mediaType %s not support! pvc: %s", mediaType, utils.GetName(localPVC.ObjectMeta))
	}
	return nil
}

func (allocator *devicePVAllocator) preAllocate(nodeName string, podVolumeInfos *PodLocalVolumeInfo, nodeStateClone *NodeStorageState) ([]PVAllocated, error) {
	if podVolumeInfos.DevicePVCs == nil {
		return nil, nil
	}
	devicePVCS := podVolumeInfos.DevicePVCs
	if len(devicePVCS.SSDDevicePVCs)+len(devicePVCS.HDDDevicePVCs) <= 0 {
		return nil, nil
	}
	freeSSD, freeHDD := getFreeDevice(nodeStateClone)

	var result []PVAllocated

	sddAllocateUnits, err := processDeviceByMediaType(nodeName, devicePVCS.SSDDevicePVCs, freeSSD)
	if len(sddAllocateUnits) > 0 {
		result = append(result, sddAllocateUnits...)
	}
	if err != nil {
		return result, err
	}

	hddAllocateUnits, err := processDeviceByMediaType(nodeName, devicePVCS.HDDDevicePVCs, freeHDD)
	if len(hddAllocateUnits) > 0 {
		result = append(result, hddAllocateUnits...)
	}

	return result, err
}

func (allocator *devicePVAllocator) allocateInfo(detail PVAllocated) *localtype.PVCAllocateInfo {
	allocateDetail, ok := detail.(*DeviceTypePVAllocated)
	if !ok {
		klog.Infof("could't convert detail (%#v) to device type", detail)
		return nil
	}
	if allocateDetail.DeviceName != "" && allocateDetail.Allocated > 0 {
		return &localtype.PVCAllocateInfo{
			PVCNameSpace: allocateDetail.PVCNamespace,
			PVCName:      allocateDetail.PVCName,
			PVAllocatedInfo: localtype.PVAllocatedInfo{
				DeviceName: allocateDetail.DeviceName,
				VolumeType: string(localtype.VolumeTypeDevice),
			},
		}
	}
	return nil
}

func (allocator *devicePVAllocator) isPVHaveAllocateInfo(pv *corev1.PersistentVolume) bool {
	return utils.GetDeviceNameFromCsiPV(pv) != ""
}

func (allocator *devicePVAllocator) reserve(nodeName string, nodeUnits *NodeAllocateUnits) error {
	if len(nodeUnits.DevicePVCAllocateUnits) == 0 {
		return nil
	}

	currentStorageState := allocator.cache.states[nodeName]

	for _, unit := range nodeUnits.DevicePVCAllocateUnits {

		allocateExist := allocator.cache.pvAllocatedDetails.GetByPVC(unit.PVCNamespace, unit.PVCName)
		if allocateExist != nil {
			continue
		}
		if pvcInfo, ok := allocator.cache.pvcInfosMap[utils.GetNameKey(unit.PVCNamespace, unit.PVCName)]; ok && pvcInfo.PVCStatus == corev1.ClaimBound {
			klog.Infof("skip reserveDevicePVC for bound pvc %s", utils.GetNameKey(unit.PVCNamespace, unit.PVCName))
			continue
		}

		deviceState, ok := currentStorageState.DeviceStates[unit.DeviceName]
		if !ok {
			err := fmt.Errorf("reserveDevicePVC fail, device(%s) have not found for pvc(%s) on node %s", unit.DeviceName, utils.GetNameKey(unit.PVCNamespace, unit.PVCName), nodeName)
			return err
		}

		if deviceState.IsAllocated {
			err := fmt.Errorf("reserveDevicePVC fail, device(%s) have allocated on node %s", unit.DeviceName, nodeName)
			return err
		}

		if deviceState.Allocatable < unit.Requested {
			err := fmt.Errorf("reserveDevicePVC fail, device(%s) allocatable small than pvc(%s) request on node %s", unit.DeviceName, utils.GetNameKey(unit.PVCNamespace, unit.PVCName), nodeName)
			return err
		}

		deviceState.IsAllocated = true
		deviceState.Requested = deviceState.Allocatable

		unit.Allocated = deviceState.Allocatable
		allocator.cache.pvAllocatedDetails.AssumeByPVC(unit.DeepCopy())
	}
	klog.V(6).Infof("reserve for device units (%#v) success, current allocated device: %#v", nodeUnits.DevicePVCAllocateUnits, currentStorageState.DeviceStates)
	return nil
}

func (allocator *devicePVAllocator) unreserve(nodeName string, units *NodeAllocateUnits) {
	if len(units.DevicePVCAllocateUnits) == 0 {
		return
	}

	for i, unit := range units.DevicePVCAllocateUnits {

		if unit.Allocated == 0 {
			continue
		}
		allocator.revertDeviceByPVCIfNeed(nodeName, unit.PVCNamespace, unit.PVCName)
		units.DevicePVCAllocateUnits[i].Allocated = 0
	}
}

func (allocator *devicePVAllocator) initIfNeedAndGetDeviceState(nodeName, deviceName string) (*DeviceResourcePool, bool) {
	nodeStoragePool := allocator.cache.initIfNeedAndGetNodeStoragePool(nodeName)

	if _, ok := nodeStoragePool.DeviceStates[deviceName]; !ok {
		nodeStoragePool.DeviceStates[deviceName] = &DeviceResourcePool{Name: deviceName}
	}
	return nodeStoragePool.DeviceStates[deviceName], nodeStoragePool.InitedByNLS
}

func (allocator *devicePVAllocator) allocateDeviceForState(nodeName string, deviceName string, requestSize int64) {
	nodeStoragePool, ok := allocator.cache.states[nodeName]
	if !ok {
		nodeStoragePool = NewNodeStorageState()
		allocator.cache.states[nodeName] = nodeStoragePool
	}

	nodeStoragePool.DeviceStates.AllocateDevice(deviceName, requestSize)
}

func (allocator *devicePVAllocator) revertDeviceForState(nodeName string, deviceName string) {
	nodeStoragePool, ok := allocator.cache.states[nodeName]
	if !ok {
		return
	}
	if nodeStoragePool.DeviceStates == nil {
		return
	}
	nodeStoragePool.DeviceStates.RevertDevice(deviceName)
}

func (allocator *devicePVAllocator) revertDeviceByPVCIfNeed(nodeName, pvcNameSpace, pvcName string) {
	if pvcNameSpace == "" && pvcName == "" {
		return
	}
	allocatedByPVC := allocator.cache.pvAllocatedDetails.GetByPVC(pvcNameSpace, pvcName)
	if allocatedByPVC == nil {
		return
	}
	deviceAllocated, ok := allocatedByPVC.(*DeviceTypePVAllocated)
	if !ok {
		klog.Errorf("can not convert pvc(%s) AllocateInfo to DeviceTypePVAllocated", utils.GetNameKey(pvcNameSpace, pvcName))
		return
	}

	if deviceAllocated.Allocated == 0 || deviceAllocated.DeviceName == "" {
		return
	}

	allocator.revertDeviceForState(nodeName, deviceAllocated.DeviceName)
	allocator.cache.pvAllocatedDetails.DeleteByPVC(utils.GetNameKey(pvcNameSpace, pvcName))
}

func getFreeDevice(nodeStateClone *NodeStorageState) (freeDeviceSSD, freeDeviceHDD []*DeviceResourcePool) {

	for _, deviceState := range nodeStateClone.DeviceStates {
		if deviceState.MediaType == localtype.MediaTypeSSD && !deviceState.IsAllocated {
			freeDeviceSSD = append(freeDeviceSSD, deviceState)
		} else if deviceState.MediaType == localtype.MediaTypeHDD && !deviceState.IsAllocated {
			freeDeviceHDD = append(freeDeviceHDD, deviceState)
		}
	}

	return
}

func processDeviceByMediaType(nodeName string, devicePVCs []*DevicePVCInfo, freeDeviceStates []*DeviceResourcePool) ([]PVAllocated, error) {
	pvcsCount := len(devicePVCs)
	if pvcsCount <= 0 {
		return nil, nil
	}

	if pvcsCount > len(freeDeviceStates) {
		return nil, fmt.Errorf("free device not enough for device PVCs")
	}

	sort.Slice(devicePVCs, func(i, j int) bool {
		return devicePVCs[i].Request < devicePVCs[j].Request
	})
	sort.Slice(freeDeviceStates, func(i, j int) bool {
		return freeDeviceStates[i].Allocatable < freeDeviceStates[j].Allocatable
	})

	units := make([]PVAllocated, 0, pvcsCount)
	i := 0
	var pvcInfo *DevicePVCInfo
	for _, disk := range freeDeviceStates {
		pvcInfo = devicePVCs[i]
		if disk.IsAllocated || disk.Allocatable < pvcInfo.Request {
			continue
		}
		//change cache
		disk.IsAllocated = true
		disk.Requested = disk.Allocatable

		u := &DeviceTypePVAllocated{
			BasePVAllocated: BasePVAllocated{
				PVCName:      pvcInfo.PVC.Name,
				PVCNamespace: pvcInfo.PVC.Namespace,
				NodeName:     nodeName,
				Requested:    pvcInfo.Request,
				Allocated:    disk.Allocatable,
			},
			DeviceName: disk.Name,
		}

		klog.V(6).Infof("found unit: %#v for pvc %#v", u, utils.PVCName(pvcInfo.PVC))
		units = append(units, u)
		i++
		if i == int(pvcsCount) {
			return units, nil
		}
	}
	return nil, errors.NewInsufficientExclusiveResourceError(
		localtype.VolumeTypeDevice,
		pvcInfo.Request,
		pvcInfo.Request)
}
