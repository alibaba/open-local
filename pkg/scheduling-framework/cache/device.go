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

type DeviceResourcePool struct {
	Name        string
	Total       int64
	Allocatable int64
	Requested   int64
	MediaType   localtype.MediaType
	IsAllocated bool
}

func NewDeviceResourcePoolForAllocate(deviceName string) *DeviceResourcePool {
	devicePool := &DeviceResourcePool{
		Name:        deviceName,
		IsAllocated: true,
	}
	return devicePool
}

func NewDeviceResourcePoolFromDeviceInfo(device nodelocalstorage.DeviceInfo) *DeviceResourcePool {
	devicePool := &DeviceResourcePool{
		Name:        device.Name,
		Total:       int64(device.Total),
		Allocatable: int64(device.Total),
		Requested:   0,
		MediaType:   localtype.MediaType(device.MediaType),
		IsAllocated: false,
	}
	return devicePool
}

func (d *DeviceResourcePool) UpdateByNLS(new *DeviceResourcePool) {
	if d == nil || new == nil {
		return
	}
	d.Total = new.Total
	d.Allocatable = new.Allocatable
	d.MediaType = new.MediaType
	if d.IsAllocated {
		d.Requested = d.Allocatable
	}
}

func (d *DeviceResourcePool) DeepCopy() *DeviceResourcePool {
	if d == nil {
		return nil
	}
	copy := &DeviceResourcePool{
		Name:        d.Name,
		Total:       d.Total,
		Allocatable: d.Allocatable,
		Requested:   d.Requested,
		MediaType:   d.MediaType,
		IsAllocated: d.IsAllocated,
	}
	return copy
}

func (d *DeviceResourcePool) GetName() string {
	if d == nil {
		return ""
	}
	return d.Name
}

type DeviceStates map[string]*DeviceResourcePool

func (s DeviceStates) DeepCopy() DeviceStates {
	copy := DeviceStates{}
	for name, state := range s {
		copy[name] = state.DeepCopy()
	}
	return copy
}

func (s DeviceStates) AllocateDevice(deviceName string, requestSize int64) {
	state, ok := s[deviceName]
	if !ok {
		state = NewDeviceResourcePoolForAllocate(deviceName)
		s[deviceName] = state
	}
	if state.Allocatable > 0 {
		state.Requested = state.Allocatable
	} else {
		state.Requested = requestSize
	}

	state.IsAllocated = true
}

func (s DeviceStates) RevertDevice(deviceName string) {
	state, ok := s[deviceName]
	if !ok {
		return
	}
	state.Requested = 0
	state.IsAllocated = false
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

type DeviceHandler struct {
}

func (h *DeviceHandler) CreateStatesByNodeLocal(nodeLocal *nodelocalstorage.NodeLocalStorage) map[string]*DeviceResourcePool {
	states := map[string]*DeviceResourcePool{}
	// Devices
	deviceInfoMap := make(map[string]nodelocalstorage.DeviceInfo)
	for _, d := range nodeLocal.Status.NodeStorageInfo.DeviceInfos {
		deviceInfoMap[d.Name] = d
	}
	// add devices
	for _, deviceName := range nodeLocal.Status.FilteredStorageInfo.Devices {
		device, ok := deviceInfoMap[deviceName]
		if !ok {
			klog.Warningf("Get deviceInfo from nodeLocal failed! deviceName:%s, nodeName %s", deviceName, nodeLocal.Name)
			continue
		}
		diskResource := NewDeviceResourcePoolFromDeviceInfo(device)
		states[deviceName] = diskResource
		klog.V(6).Infof("initDeviceStorage, add diskResource success: %#v", diskResource)
	}
	return states
}

func (h *DeviceHandler) StatesForUpdate(old, new map[string]*DeviceResourcePool) map[string]*DeviceResourcePool {
	mergeStates := map[string]*DeviceResourcePool{}
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
