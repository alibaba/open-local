/*
Copyright 2022/9/14 Alibaba Cloud.

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
	"testing"

	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
)

func Test_device_reserve(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()
	pvcsDevicePending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVDevice().PVCPending})[0]
	pvcsDeviceBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVDevice().PVCBounding})[0]
	pvcsDevicePendingInfo := *utils.GetTestPVCPVDevice().PVCPending
	pvcsDevicePendingInfo.Size = "1500Gi"
	pvcsDevicePendingTooLarge := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{pvcsDevicePendingInfo})[0]

	type args struct {
		nodeName string
		units    *NodeAllocateUnits
	}

	type fields struct {
		nodeLocals []*nodelocalstorage.NodeLocalStorage
		pvcs       []*corev1.PersistentVolumeClaim
	}

	type expect struct {
		units     *NodeAllocateUnits
		resultErr bool

		/*cache expect*/
		states                       map[string]*NodeStorageState
		inlineVolumeAllocatedDetails map[string]NodeInlineVolumeAllocatedDetails
		pvAllocatedDetails           *PVAllocatedDetails
	}

	tests := []struct {
		name   string
		args   args
		fields fields
		expect expect
	}{
		{
			name: "test reserve success device pvc pending",
			args: args{
				nodeName: utils.NodeName3,
				units: &NodeAllocateUnits{
					DevicePVCAllocateUnits: []*DeviceTypePVAllocated{
						{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsDevicePending.Name,
								PVCNamespace: pvcsDevicePending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsDevicePending),
								Allocated:    0,
							},
						},
					},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				pvcs:       []*corev1.PersistentVolumeClaim{pvcsDevicePending},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					DevicePVCAllocateUnits: []*DeviceTypePVAllocated{
						{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsDevicePending.Name,
								PVCNamespace: pvcsDevicePending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsDevicePending),
								Allocated:    int64(150 * utils.LocalGi),
							},
						},
					},
				},
				resultErr: false,
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   int64(150 * utils.LocalGi),
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: true,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{
						utils.PVCName(pvcsDevicePending): &DeviceTypePVAllocated{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsDevicePending.Name,
								PVCNamespace: pvcsDevicePending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsDevicePending),
								Allocated:    int64(150 * utils.LocalGi),
							},
						},
					},
					pvAllocated: map[string]PVAllocated{},
				},
			},
		},
		{
			name: "test reserve fail for device pvc bounding",
			args: args{
				nodeName: utils.NodeName3,
				units: &NodeAllocateUnits{
					DevicePVCAllocateUnits: []*DeviceTypePVAllocated{
						{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsDeviceBounding.Name,
								PVCNamespace: pvcsDeviceBounding.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsDeviceBounding),
								Allocated:    0,
							},
						},
					},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				pvcs:       []*corev1.PersistentVolumeClaim{pvcsDeviceBounding},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					DevicePVCAllocateUnits: []*DeviceTypePVAllocated{
						{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsDeviceBounding.Name,
								PVCNamespace: pvcsDeviceBounding.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsDeviceBounding),
								Allocated:    0,
							},
						},
					},
				},
				resultErr: false,
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   0,
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: false,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "test reserve fail for device pvc bounding too large",
			args: args{
				nodeName: utils.NodeName3,
				units: &NodeAllocateUnits{
					DevicePVCAllocateUnits: []*DeviceTypePVAllocated{
						{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsDevicePendingTooLarge.Name,
								PVCNamespace: pvcsDevicePendingTooLarge.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsDevicePendingTooLarge),
								Allocated:    0,
							},
						},
					},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				pvcs:       []*corev1.PersistentVolumeClaim{pvcsDevicePendingTooLarge},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					DevicePVCAllocateUnits: []*DeviceTypePVAllocated{
						{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsDevicePendingTooLarge.Name,
								PVCNamespace: pvcsDevicePendingTooLarge.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsDevicePendingTooLarge),
								Allocated:    0,
							},
						},
					},
				},
				resultErr: true,
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   0,
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: false,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := CreateTestCache()
			allocator := NewDevicePVAllocator(cache)
			for _, nodeLocal := range tt.fields.nodeLocals {
				cache.AddNodeStorage(nodeLocal)
			}

			for _, pvc := range tt.fields.pvcs {
				cache.addPVCInfo(pvc)
			}

			err := allocator.reserve(tt.args.nodeName, tt.args.units)
			assert.Equal(t, tt.expect.resultErr, err != nil, fmt.Sprintf("errInfo:%+v", err))
			assert.Equal(t, tt.expect.units, tt.args.units, "check units")
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_device_unreserve(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()
	pvcsPending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVDevice().PVCPending})[0]
	boundInfo := utils.GetTestPVCPVDevice()
	pvcsBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*boundInfo.PVCBounding})[0]

	type args struct {
		nodeName string
		units    *NodeAllocateUnits
	}

	type fields struct {
		nodeLocals []*nodelocalstorage.NodeLocalStorage
		pvcs       []*corev1.PersistentVolumeClaim

		/*cache*/
		states             map[string]*NodeStorageState
		pvAllocatedDetails *PVAllocatedDetails
	}

	type expect struct {
		units *NodeAllocateUnits

		/*cache expect*/
		states                       map[string]*NodeStorageState
		inlineVolumeAllocatedDetails map[string]NodeInlineVolumeAllocatedDetails
		pvAllocatedDetails           *PVAllocatedDetails
	}

	tests := []struct {
		name   string
		args   args
		fields fields
		expect expect
	}{
		{
			name: "test revert success for pending pvc",
			args: args{
				nodeName: utils.NodeName3,
				units: &NodeAllocateUnits{
					DevicePVCAllocateUnits: []*DeviceTypePVAllocated{
						{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsPending.Name,
								PVCNamespace: pvcsPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsPending),
								Allocated:    utils.GetPVCRequested(pvcsPending),
							},
						},
					},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				pvcs:       []*corev1.PersistentVolumeClaim{pvcsPending},

				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   int64(150 * utils.LocalGi),
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: true,
							},
						},
						InitedByNLS: true,
					},
				},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{
						utils.PVCName(pvcsPending): &DeviceTypePVAllocated{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsPending.Name,
								PVCNamespace: pvcsPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsPending),
								Allocated:    utils.GetPVCRequested(pvcsPending),
							},
						},
					},
					pvAllocated: map[string]PVAllocated{},
				},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					DevicePVCAllocateUnits: []*DeviceTypePVAllocated{
						{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsPending.Name,
								PVCNamespace: pvcsPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsPending),
								Allocated:    0,
							},
						},
					},
				},
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   0,
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: false,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "test skip revert for unit not allocated, but pvcBound and pv exist",
			args: args{
				nodeName: utils.NodeName3,
				units: &NodeAllocateUnits{
					DevicePVCAllocateUnits: []*DeviceTypePVAllocated{
						{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsPending.Name,
								PVCNamespace: pvcsPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsPending),
								Allocated:    0,
							},
						},
					},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				pvcs:       []*corev1.PersistentVolumeClaim{pvcsBounding},

				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   int64(150 * utils.LocalGi),
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: true,
							},
						},
						InitedByNLS: true,
					},
				},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated: map[string]PVAllocated{
						boundInfo.PVCBounding.VolumeName: &DeviceTypePVAllocated{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsPending.Name,
								PVCNamespace: pvcsPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsPending),
								Allocated:    utils.GetPVCRequested(pvcsPending),
							},
						},
					},
				},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					DevicePVCAllocateUnits: []*DeviceTypePVAllocated{
						{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsPending.Name,
								PVCNamespace: pvcsPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsPending),
								Allocated:    0,
							},
						},
					},
				},
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   int64(150 * utils.LocalGi),
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: true,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated: map[string]PVAllocated{
						boundInfo.PVCBounding.VolumeName: &DeviceTypePVAllocated{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsPending.Name,
								PVCNamespace: pvcsPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsPending),
								Allocated:    utils.GetPVCRequested(pvcsPending),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := CreateTestCache()
			allocator := NewDevicePVAllocator(cache)

			for _, nodeLocal := range tt.fields.nodeLocals {
				cache.AddNodeStorage(nodeLocal)
			}

			for _, pvc := range tt.fields.pvcs {
				cache.addPVCInfo(pvc)
			}

			cache.states = tt.fields.states
			cache.pvAllocatedDetails = tt.fields.pvAllocatedDetails

			allocator.unreserve(tt.args.nodeName, tt.args.units)
			assert.Equal(t, tt.expect.units, tt.args.units, "check units")
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_device_pvUpdate(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()
	//test pending
	pvPending := utils.CreateTestPersistentVolume([]utils.TestPVInfo{*utils.GetTestPVCPVDevice().PVBounding})[0]
	pvPending.Status.Phase = corev1.VolumePending

	//test without deviceName
	pvBoundingNoDeviceNameInfo := *utils.GetTestPVCPVDevice().PVBounding
	pvBoundingNoDeviceNameInfo.DeviceName = ""
	pvBoundingHaveNoDeviceName := utils.CreateTestPersistentVolume([]utils.TestPVInfo{pvBoundingNoDeviceNameInfo})[0]

	//test assume success
	pvcBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVDevice().PVCBounding})[0]
	pvBounding := utils.CreateTestPersistentVolume([]utils.TestPVInfo{*utils.GetTestPVCPVDevice().PVBounding})[0]

	type args struct {
		nodeName string
		pvc      *corev1.PersistentVolumeClaim
		pv       *corev1.PersistentVolume
	}

	type fields struct {
		nodeLocal *nodelocalstorage.NodeLocalStorage
		states    map[string]*NodeStorageState
	}

	type expect struct {
		allocatedSize int64

		/*cache expect*/
		states                       map[string]*NodeStorageState
		inlineVolumeAllocatedDetails map[string]NodeInlineVolumeAllocatedDetails
		pvAllocatedDetails           *PVAllocatedDetails
	}

	tests := []struct {
		name   string
		args   args
		fields fields
		expect expect
	}{
		{
			name: "test assume fail for pv is nil",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBounding,
				pv:       nil,
			},
			fields: fields{
				nodeLocal: nodeLocal,
			},
			expect: expect{
				allocatedSize: 0,
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   0,
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: false,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "test assume fail for pending pv",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      nil,
				pv:       pvPending,
			},
			fields: fields{
				nodeLocal: nodeLocal,
			},
			expect: expect{
				allocatedSize: 0,
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   0,
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: false,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "test assume fail for pv have no deviceName",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      nil,
				pv:       pvBoundingHaveNoDeviceName,
			},
			fields: fields{
				nodeLocal: nodeLocal,
			},
			expect: expect{
				allocatedSize: 0,
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   0,
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: false,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "test assume success even device have allocated",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBounding,
				pv:       pvBounding,
			},
			fields: fields{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   int64(150 * utils.LocalGi),
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: true,
							},
						},
						InitedByNLS: true,
					},
				},
				nodeLocal: nodeLocal,
			},
			expect: expect{
				allocatedSize: 0,
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   int64(150 * utils.LocalGi),
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: true,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &DeviceTypePVAllocated{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcBounding.Name,
								PVCNamespace: pvcBounding.Namespace,
								VolumeName:   pvBounding.Name,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVSize(pvBounding),
								Allocated:    int64(150 * utils.LocalGi),
							},
						},
					},
				},
			},
		},
		{
			name: "test assume success",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBounding,
				pv:       pvBounding,
			},
			fields: fields{
				nodeLocal: nodeLocal,
			},
			expect: expect{
				allocatedSize: int64(150 * utils.LocalGi),
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   int64(150 * utils.LocalGi),
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: true,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &DeviceTypePVAllocated{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcBounding.Name,
								PVCNamespace: pvcBounding.Namespace,
								VolumeName:   pvBounding.Name,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVSize(pvBounding),
								Allocated:    int64(150 * utils.LocalGi),
							},
						},
					},
				},
			},
		},
		{
			name: "test assume success and nodeLocal not init",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBounding,
				pv:       pvBounding,
			},
			fields: fields{},
			expect: expect{
				allocatedSize: int64(150 * utils.LocalGi),
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(0),
								Allocatable: int64(0),
								Requested:   utils.GetPVSize(pvBounding),
								IsAllocated: true,
							},
						},
						InitedByNLS: false,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &DeviceTypePVAllocated{
							DeviceName: "/dev/sdc",
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcBounding.Name,
								PVCNamespace: pvcBounding.Namespace,
								VolumeName:   pvBounding.Name,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVSize(pvBounding),
								Allocated:    utils.GetPVSize(pvBounding),
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := CreateTestCache()
			allocator := NewDevicePVAllocator(cache)

			if tt.fields.nodeLocal != nil {
				cache.AddNodeStorage(tt.fields.nodeLocal)
			}

			if len(tt.fields.states) > 0 {
				cache.states = tt.fields.states
			}

			allocator.pvUpdate(tt.args.nodeName, nil, tt.args.pv)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_device_pvDelete(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()

	//test delete success
	pvcBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVDevice().PVCBounding})[0]
	pvBounding := utils.CreateTestPersistentVolume([]utils.TestPVInfo{*utils.GetTestPVCPVDevice().PVBounding})[0]

	type args struct {
		nodeName string
		pvc      *corev1.PersistentVolumeClaim
		pv       *corev1.PersistentVolume
	}

	type fields struct {
		nodeLocal *nodelocalstorage.NodeLocalStorage
	}

	type expect struct {
		/*cache expect*/
		states                       map[string]*NodeStorageState
		inlineVolumeAllocatedDetails map[string]NodeInlineVolumeAllocatedDetails
		pvAllocatedDetails           *PVAllocatedDetails
	}

	tests := []struct {
		name   string
		args   args
		fields fields
		expect expect
	}{
		{
			name: "test assume success",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBounding,
				pv:       pvBounding,
			},
			fields: fields{
				nodeLocal: nodeLocal,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(150 * utils.LocalGi),
								Allocatable: int64(150 * utils.LocalGi),
								Requested:   0,
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: false,
							},
						},
						InitedByNLS: true,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := CreateTestCache()
			allocator := NewDevicePVAllocator(cache)

			cache.AddNodeStorage(tt.fields.nodeLocal)

			if tt.args.pvc != nil && tt.args.pvc.Status.Phase == corev1.ClaimBound {
				cache.addPVCInfo(tt.args.pvc)
			}

			allocator.pvAdd(tt.args.nodeName, tt.args.pv)

			allocator.pvDelete(tt.args.nodeName, tt.args.pv)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}
