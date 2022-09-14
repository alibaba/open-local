/*
Copyright 2022/8/21 Alibaba Cloud.

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
package plugin

import (
	"fmt"
	"sort"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/algo"
	"github.com/alibaba/open-local/pkg/scheduler/errors"
	"github.com/alibaba/open-local/pkg/scheduling-framework/cache"
	"github.com/alibaba/open-local/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

func (plugin *LocalPlugin) getPodLocalVolumeInfos(pod *corev1.Pod) (*PodLocalVolumeInfo, error) {
	volumeInfos := &PodLocalVolumeInfo{
		lvmPVCsWithVgNameNotAllocated:    []*LVMPVCInfo{},
		lvmPVCsSnapshot:                  []*LVMPVCInfo{},
		lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{},
		ssdDevicePVCsNotAllocated:        []*DevicePVCInfo{},
		hddDevicePVCsNotAllocated:        []*DevicePVCInfo{},
		inlineVolumes:                    []*cache.InlineVolumeAllocated{},
	}

	err, unboundLvmPVCs, _, unboundDevicePVCs := algorithm.GetPodPvcsByLister(pod, plugin.coreV1Informers.PersistentVolumeClaims().Lister(), plugin.scLister, true, true)
	if err != nil {
		return nil, err
	}

	if len(unboundDevicePVCs) > 0 {
		volumeInfos.ssdDevicePVCsNotAllocated, volumeInfos.hddDevicePVCsNotAllocated, err = plugin.filterAllocatedAndDividePVCAccordingToMediaType(unboundDevicePVCs)
		if err != nil {
			return nil, err
		}
	}

	if len(unboundLvmPVCs) > 0 {
		volumeInfos.lvmPVCsWithVgNameNotAllocated, volumeInfos.lvmPVCsWithoutVgNameNotAllocated, volumeInfos.lvmPVCsSnapshot, err = plugin.filterAllocatedAndDivideLVMPVC(unboundLvmPVCs)
		if err != nil {
			return nil, err
		}
	}

	inlineVolumeAllocates, err := plugin.getInlineVolumeAllocates(pod)
	if err != nil {
		return volumeInfos, err
	}
	if len(inlineVolumeAllocates) > 0 {
		volumeInfos.inlineVolumes = append(volumeInfos.inlineVolumes, inlineVolumeAllocates...)
	}
	return volumeInfos, nil
}

func (plugin *LocalPlugin) getInlineVolumeAllocates(pod *corev1.Pod) ([]*cache.InlineVolumeAllocated, error) {
	var inlineVolumeAllocates []*cache.InlineVolumeAllocated

	containInlineVolume, _ := utils.ContainInlineVolumes(pod)
	if !containInlineVolume {
		return nil, nil
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.CSI != nil && utils.ContainsProvisioner(volume.CSI.Driver) {
			vgName, size := utils.GetInlineVolumeInfoFromParam(volume.CSI.VolumeAttributes)
			if vgName == "" {
				return nil, fmt.Errorf("no vgName found in inline volume of Pod %s", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			}

			inlineVolumeAllocates = append(inlineVolumeAllocates, &cache.InlineVolumeAllocated{
				PodNamespace: pod.Namespace,
				PodName:      pod.Name,
				VolumeName:   volume.Name,
				VolumeSize:   size,
				VgName:       vgName,
			})
		}
	}
	return inlineVolumeAllocates, nil
}

func (plugin *LocalPlugin) preAllocate(pod *corev1.Pod, podVolumeInfo *PodLocalVolumeInfo, reservationPod *corev1.Pod, nodeName string) (*cache.NodeAllocateState, error) {
	nodeStateClone := plugin.cache.GetNodeStorageStateCopy(nodeName)
	if nodeStateClone == nil {
		return nil, fmt.Errorf("node(%s) have no local storage pool", nodeName)
	}

	if !nodeStateClone.InitedByNLS {
		return nil, fmt.Errorf("node(%s) local storage pool had not been inited", nodeName)
	}

	nodeAllocate := &cache.NodeAllocateState{NodeStorageAllocatedByUnits: nodeStateClone, NodeName: nodeName, PodUid: string(pod.UID)}

	if reservationPod != nil {
		plugin.preRevertReservation(reservationPod, nodeName, nodeStateClone)
	}

	allocateUnits, err := plugin.preAllocateByVGs(pod, podVolumeInfo, nodeName, nodeStateClone)
	if allocateUnits != nil {
		nodeAllocate.Units = allocateUnits
	}
	if err != nil {
		return nodeAllocate, err
	}
	allocateUnitsByDevice, err := plugin.preAllocateDevice(nodeName, pod, podVolumeInfo, nodeStateClone)
	if allocateUnitsByDevice != nil {
		allocateUnits.DevicePVCAllocateUnits = allocateUnitsByDevice
	}
	return nodeAllocate, err
}

func (plugin *LocalPlugin) preRevertReservation(reservationPod *corev1.Pod, nodeName string, nodeStateClone *cache.NodeStorageState) {
	inlineDetails := plugin.cache.GetPodInlineVolumeDetailsCopy(nodeName, string(reservationPod.UID))
	if inlineDetails == nil || len(*inlineDetails) <= 0 {
		return
	}

	for _, detail := range *inlineDetails {
		if nodeStateClone.VGStates == nil {
			klog.Errorf("preRevertReservation fail, no VG found on node %s", nodeName)
			continue
		}

		vgState, ok := nodeStateClone.VGStates[detail.VgName]
		if !ok {
			klog.Errorf("preRevertReservation fail, volumeGroup(%s) have not found for pod(%s) on node %s", detail.VgName, reservationPod.UID, nodeName)
			continue
		}

		vgState.Requested = vgState.Requested - detail.Allocated
	}

}

func (plugin *LocalPlugin) filterBySnapshot(nodeName string, lvmPVCsSnapshot []*LVMPVCInfo) (bool, error) {
	// if pod has snapshot pvc
	// select all snapshot pvcs, and check if nodes of them are the same
	var fits = true
	if len(lvmPVCsSnapshot) >= 0 {

		var pvcs []*corev1.PersistentVolumeClaim
		for _, pvcInfo := range lvmPVCsSnapshot {
			pvcs = append(pvcs, pvcInfo.pvc)
		}

		var err error
		if fits, err = algo.ProcessSnapshotPVC(pvcs, nodeName, plugin.coreV1Informers, plugin.snapshotInformers); err != nil {
			return fits, err
		}
		if !fits {
			return fits, nil
		}
	}
	return fits, nil
}

func (plugin *LocalPlugin) preAllocateByVGs(pod *corev1.Pod, podVolumeInfo *PodLocalVolumeInfo, nodeName string, nodeStateClone *cache.NodeStorageState) (*cache.NodeAllocateUnits, error) {
	allocateUnits := &cache.NodeAllocateUnits{LVMPVCAllocateUnits: []*cache.LVMPVAllocated{}, DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{}, InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{}}
	inlineVolumeAllocate, err := plugin.preAllocateInlineVolume(nodeName, pod, podVolumeInfo, nodeStateClone)
	if inlineVolumeAllocate != nil {
		allocateUnits.InlineVolumeAllocateUnits = inlineVolumeAllocate
	}
	if err != nil {
		return allocateUnits, err
	}

	pvcAllocate, err := plugin.preAllocateLVMPVCs(nodeName, podVolumeInfo, nodeStateClone)
	if pvcAllocate != nil {
		allocateUnits.LVMPVCAllocateUnits = pvcAllocate
	}
	return allocateUnits, err
}

func (plugin *LocalPlugin) preAllocateInlineVolume(nodeName string, pod *corev1.Pod, podVolumeInfo *PodLocalVolumeInfo, nodeStateClone *cache.NodeStorageState) ([]*cache.InlineVolumeAllocated, error) {
	if len(podVolumeInfo.inlineVolumes) <= 0 {
		return nil, nil
	}

	allocatedInfos := plugin.cache.GetPodInlineVolumeDetailsCopy(nodeName, string(pod.UID))
	if allocatedInfos != nil && len(*allocatedInfos) > 0 {
		return nil, fmt.Errorf("node(%s) had allocated by pod", nodeName)
	}

	var inlineVolumeAllocates []*cache.InlineVolumeAllocated
	for _, volume := range podVolumeInfo.inlineVolumes {
		err := allocateVgState(nodeName, volume.VgName, nodeStateClone, volume.VolumeSize)
		if err != nil {
			return inlineVolumeAllocates, err
		}
		volumeCopy := volume.DeepCopy()
		volumeCopy.Allocated = volumeCopy.VolumeSize
		inlineVolumeAllocates = append(inlineVolumeAllocates, volumeCopy)
	}
	return inlineVolumeAllocates, nil
}

func (plugin *LocalPlugin) preAllocateLVMPVCs(nodeName string, podVolumeInfo *PodLocalVolumeInfo, nodeStateClone *cache.NodeStorageState) ([]*cache.LVMPVAllocated, error) {
	var allocateUnits []*cache.LVMPVAllocated
	if len(podVolumeInfo.lvmPVCsWithVgNameNotAllocated)+len(podVolumeInfo.lvmPVCsWithoutVgNameNotAllocated) <= 0 {
		return allocateUnits, nil
	}

	if len(nodeStateClone.VGStates) <= 0 {
		err := fmt.Errorf("no vg found with node %s", nodeName)
		klog.Error(err)
		return allocateUnits, err
	}

	for _, pvcInfo := range podVolumeInfo.lvmPVCsWithVgNameNotAllocated {

		err := allocateVgState(nodeName, pvcInfo.vgName, nodeStateClone, pvcInfo.request)
		if err != nil {
			return allocateUnits, err
		}

		allocateUnits = append(allocateUnits, &cache.LVMPVAllocated{
			BasePVAllocated: cache.BasePVAllocated{
				PVCName:      pvcInfo.pvc.Name,
				PVCNamespace: pvcInfo.pvc.Namespace,
				NodeName:     nodeName,
				Requested:    pvcInfo.request,
				Allocated:    pvcInfo.request,
			},
			VGName: pvcInfo.vgName,
		})
	}

	if len(podVolumeInfo.lvmPVCsWithoutVgNameNotAllocated) <= 0 {
		return allocateUnits, nil
	}

	vgStateList := make([]*cache.VGStoragePool, 0, len(nodeStateClone.VGStates))

	for key := range nodeStateClone.VGStates {
		vgStateList = append(vgStateList, nodeStateClone.VGStates[key])
	}

	// process pvcsWithoutVG
	for _, pvcInfo := range podVolumeInfo.lvmPVCsWithoutVgNameNotAllocated {

		allocateUnit, err := plugin.allocateStrategy.AllocatePVCWithoutVgName(nodeName, &vgStateList, pvcInfo)
		if err != nil {
			return allocateUnits, err
		}
		allocateUnits = append(allocateUnits, allocateUnit)
	}
	return allocateUnits, nil
}

func (plugin *LocalPlugin) preAllocateDevice(nodeName string, pod *corev1.Pod, podVolumeInfo *PodLocalVolumeInfo, nodeStateClone *cache.NodeStorageState) ([]*cache.DeviceTypePVAllocated, error) {
	if len(podVolumeInfo.ssdDevicePVCsNotAllocated)+len(podVolumeInfo.hddDevicePVCsNotAllocated) <= 0 {
		return nil, nil
	}
	freeSSD, freeHDD := plugin.GetFreeDevice(nodeName, nodeStateClone)

	var result []*cache.DeviceTypePVAllocated

	sddAllocateUnits, err := plugin.processDeviceByMediaType(nodeName, podVolumeInfo.ssdDevicePVCsNotAllocated, freeSSD)
	if len(sddAllocateUnits) > 0 {
		result = append(result, sddAllocateUnits...)
	}
	if err != nil {
		return result, err
	}

	hddAllocateUnits, err := plugin.processDeviceByMediaType(nodeName, podVolumeInfo.hddDevicePVCsNotAllocated, freeHDD)
	if len(hddAllocateUnits) > 0 {
		result = append(result, hddAllocateUnits...)
	}

	return result, err
}

func (plugin *LocalPlugin) processDeviceByMediaType(nodeName string, devicePVCs []*DevicePVCInfo, freeDeviceStates []*cache.DeviceResourcePool) ([]*cache.DeviceTypePVAllocated, error) {
	pvcsCount := len(devicePVCs)
	if pvcsCount <= 0 {
		return nil, nil
	}

	if pvcsCount > len(freeDeviceStates) {
		return nil, fmt.Errorf("free device not enough for device PVCs")
	}

	sort.Slice(devicePVCs, func(i, j int) bool {
		return devicePVCs[i].request < devicePVCs[j].request
	})
	sort.Slice(freeDeviceStates, func(i, j int) bool {
		return freeDeviceStates[i].Allocatable < freeDeviceStates[j].Allocatable
	})

	units := make([]*cache.DeviceTypePVAllocated, 0, pvcsCount)
	i := 0
	var pvcInfo *DevicePVCInfo
	for _, disk := range freeDeviceStates {
		pvcInfo = devicePVCs[i]
		if disk.IsAllocated || disk.Allocatable < pvcInfo.request {
			continue
		}
		//change cache
		disk.IsAllocated = true
		disk.Requested = disk.Allocatable

		u := &cache.DeviceTypePVAllocated{
			BasePVAllocated: cache.BasePVAllocated{
				PVCName:      pvcInfo.pvc.Name,
				PVCNamespace: pvcInfo.pvc.Namespace,
				NodeName:     nodeName,
				Requested:    pvcInfo.request,
				Allocated:    disk.Allocatable,
			},
			DeviceName: disk.Name,
		}

		klog.V(6).Infof("found unit: %#v for pvc %#v", u, utils.PVCName(pvcInfo.pvc))
		units = append(units, u)
		i++
		if i == int(pvcsCount) {
			return units, nil
		}
	}
	return nil, errors.NewInsufficientExclusiveResourceError(
		localtype.VolumeTypeDevice,
		pvcInfo.request,
		pvcInfo.request)
}

func allocateVgState(nodeName, vgName string, nodeStateClone *cache.NodeStorageState, size int64) error {
	vgState, ok := nodeStateClone.VGStates[vgName]
	if !ok {
		err := fmt.Errorf("no vg pool %s in node %s", vgName, nodeName)
		klog.Error(err)
		return err
	}

	// free size
	poolFreeSize := vgState.Allocatable - vgState.Requested
	if poolFreeSize < size {
		return errors.NewInsufficientLVMError(size, int64(vgState.Requested), int64(vgState.Allocatable), vgName, nodeName)
	}
	// 更新临时 cache
	vgState.Requested += size
	return nil
}

func (plugin *LocalPlugin) filterAllocatedAndDivideLVMPVC(pvcs []*corev1.PersistentVolumeClaim) (lvmPVCsWithVgName, lvmPVCsWithoutVgName, lvmPVCsSnapshot []*LVMPVCInfo, err error) {
	lvmPVCsWithVgName = []*LVMPVCInfo{}
	lvmPVCsWithoutVgName = []*LVMPVCInfo{}
	lvmPVCsSnapshot = []*LVMPVCInfo{}
	for _, pvc := range pvcs {

		lvmPVCInfo := &LVMPVCInfo{
			pvc:     pvc,
			request: utils.GetPVCRequested(pvc),
		}

		if utils.IsSnapshotPVC(pvc) {
			lvmPVCsSnapshot = append(lvmPVCsSnapshot, lvmPVCInfo)
			continue
		}

		if plugin.cache.GetPVCAllocatedDetailCopy(pvc.Namespace, pvc.Name) != nil {
			continue
		}
		var vgName string
		vgName, err = utils.GetVGNameFromPVC(pvc, plugin.scLister)
		if err != nil {
			return
		}
		lvmPVCInfo.vgName = vgName

		if lvmPVCInfo.vgName == "" {
			// 里面没有 vg 信息
			lvmPVCsWithoutVgName = append(lvmPVCsWithoutVgName, lvmPVCInfo)
		} else {
			lvmPVCsWithVgName = append(lvmPVCsWithVgName, lvmPVCInfo)
		}
	}
	return
}

func (plugin *LocalPlugin) filterAllocatedAndDividePVCAccordingToMediaType(pvcs []*corev1.PersistentVolumeClaim) (pvcsWithTypeSSD, pvcsWithTypeHDD []*DevicePVCInfo, err error) {
	pvcsWithTypeSSD = []*DevicePVCInfo{}
	pvcsWithTypeHDD = []*DevicePVCInfo{}
	for _, pvc := range pvcs {
		if plugin.cache.GetPVCAllocatedDetailCopy(pvc.Namespace, pvc.Name) != nil {
			continue
		}
		pvcInfo := &DevicePVCInfo{pvc: pvc, request: utils.GetPVCRequested(pvc)}
		var mediaType localtype.MediaType
		mediaType, err = utils.GetMediaTypeFromPVC(pvc, plugin.scLister)
		if err != nil {
			return
		}
		switch mediaType {
		case localtype.MediaTypeSSD:
			pvcInfo.mediaType = mediaType
			pvcsWithTypeSSD = append(pvcsWithTypeSSD, pvcInfo)
		case localtype.MediaTypeHDD:
			pvcInfo.mediaType = mediaType
			pvcsWithTypeHDD = append(pvcsWithTypeHDD, pvcInfo)
		default:
			err = fmt.Errorf("mediaType %s not support! pvc: %s/%s", mediaType, pvc.Namespace, pvc.Name)
			klog.Errorf(err.Error())
			return
		}

	}
	return
}

// GetFreeDevice divide nodeCache.Devices into freeDeviceSSD and freeDeviceHDD
func (plugin *LocalPlugin) GetFreeDevice(nodeName string, nodeStateClone *cache.NodeStorageState) (freeDeviceSSD, freeDeviceHDD []*cache.DeviceResourcePool) {

	for _, deviceState := range nodeStateClone.DeviceStates {
		if deviceState.MediaType == localtype.MediaTypeSSD && !deviceState.IsAllocated {
			freeDeviceSSD = append(freeDeviceSSD, deviceState)
		} else if deviceState.MediaType == localtype.MediaTypeHDD && !deviceState.IsAllocated {
			freeDeviceHDD = append(freeDeviceHDD, deviceState)
		}
	}

	return
}
