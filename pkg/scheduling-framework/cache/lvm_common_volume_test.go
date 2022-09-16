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

func Test_lvm_reserve(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()
	//pending
	pvcsWithVGPending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithVG().PVCPending})[0]
	//expect skip for bounding
	pvcsWithVGBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithVG().PVCBounding})[0]
	//expect fail
	pvcsWithVGPendingInfo := *utils.GetTestPVCPVWithVG().PVCPending
	pvcsWithVGPendingInfo.Size = "1500Gi"
	pvcsWithVGPendingTooLarge := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{pvcsWithVGPendingInfo})[0]

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
			name: "test reserve success local lvm pvc pending",
			args: args{
				nodeName: utils.NodeName3,
				units: &NodeAllocateUnits{
					LVMPVCAllocateUnits: []*LVMPVAllocated{
						{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGPending.Name,
								PVCNamespace: pvcsWithVGPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGPending),
								Allocated:    0,
							},
						},
					},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				pvcs:       []*corev1.PersistentVolumeClaim{pvcsWithVGPending},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					LVMPVCAllocateUnits: []*LVMPVAllocated{
						{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGPending.Name,
								PVCNamespace: pvcsWithVGPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGPending),
								Allocated:    utils.GetPVCRequested(pvcsWithVGPending),
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
								Requested:   utils.GetPVCRequested(pvcsWithVGPending),
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
					pvcAllocated: map[string]PVAllocated{
						utils.PVCName(pvcsWithVGPending): &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGPending.Name,
								PVCNamespace: pvcsWithVGPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGPending),
								Allocated:    utils.GetPVCRequested(pvcsWithVGPending),
							},
						},
					},
					pvAllocated: map[string]PVAllocated{},
				},
			},
		},
		{
			name: "test reserve fail for local lvm pvc bounding",
			args: args{
				nodeName: utils.NodeName3,
				units: &NodeAllocateUnits{
					LVMPVCAllocateUnits: []*LVMPVAllocated{
						{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGBounding.Name,
								PVCNamespace: pvcsWithVGBounding.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGBounding),
								Allocated:    0,
							},
						},
					},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				pvcs:       []*corev1.PersistentVolumeClaim{pvcsWithVGBounding},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					LVMPVCAllocateUnits: []*LVMPVAllocated{
						{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGBounding.Name,
								PVCNamespace: pvcsWithVGBounding.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGBounding),
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
			name: "test reserve fail for local lvm pvc too large",
			args: args{
				nodeName: utils.NodeName3,
				units: &NodeAllocateUnits{
					LVMPVCAllocateUnits: []*LVMPVAllocated{
						{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGPendingTooLarge.Name,
								PVCNamespace: pvcsWithVGPendingTooLarge.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGPendingTooLarge),
								Allocated:    0,
							},
						},
					},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				pvcs:       []*corev1.PersistentVolumeClaim{pvcsWithVGPendingTooLarge},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					LVMPVCAllocateUnits: []*LVMPVAllocated{
						{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGPendingTooLarge.Name,
								PVCNamespace: pvcsWithVGPendingTooLarge.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGPendingTooLarge),
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
			allocator := NewLVMCommonPVAllocator(cache)

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

func Test_lvm_unreserve(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()
	pvcsWithVGPending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithVG().PVCPending})[0]
	boundInfo := utils.GetTestPVCPVWithVG()
	pvcsWithVGBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*boundInfo.PVCBounding})[0]

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
					LVMPVCAllocateUnits: []*LVMPVAllocated{
						{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGPending.Name,
								PVCNamespace: pvcsWithVGPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGPending),
								Allocated:    utils.GetPVCRequested(pvcsWithVGPending),
							},
						},
					},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				pvcs:       []*corev1.PersistentVolumeClaim{pvcsWithVGPending},

				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVCRequested(pvcsWithVGPending),
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
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{
						utils.PVCName(pvcsWithVGPending): &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGPending.Name,
								PVCNamespace: pvcsWithVGPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGPending),
								Allocated:    utils.GetPVCRequested(pvcsWithVGPending),
							},
						},
					},
					pvAllocated: map[string]PVAllocated{},
				},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					LVMPVCAllocateUnits: []*LVMPVAllocated{
						{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGPending.Name,
								PVCNamespace: pvcsWithVGPending.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGPending),
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
					LVMPVCAllocateUnits: []*LVMPVAllocated{
						{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGBounding.Name,
								PVCNamespace: pvcsWithVGBounding.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGBounding),
								Allocated:    0,
							},
						},
					},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				pvcs:       []*corev1.PersistentVolumeClaim{pvcsWithVGPending},

				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVCRequested(pvcsWithVGBounding),
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
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated: map[string]PVAllocated{
						boundInfo.PVCBounding.VolumeName: &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGBounding.Name,
								PVCNamespace: pvcsWithVGBounding.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGBounding),
								Allocated:    utils.GetPVCRequested(pvcsWithVGBounding),
							},
						},
					},
				},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					LVMPVCAllocateUnits: []*LVMPVAllocated{
						{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGBounding.Name,
								PVCNamespace: pvcsWithVGBounding.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGBounding),
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
								Requested:   utils.GetPVCRequested(pvcsWithVGBounding),
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
					pvAllocated: map[string]PVAllocated{
						boundInfo.PVCBounding.VolumeName: &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcsWithVGBounding.Name,
								PVCNamespace: pvcsWithVGBounding.Namespace,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcsWithVGBounding),
								Allocated:    utils.GetPVCRequested(pvcsWithVGBounding),
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
			allocator := NewLVMCommonPVAllocator(cache)
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

func Test_lvm_pvUpdate(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()
	//test pending
	pvPending := utils.CreateTestPersistentVolume([]utils.TestPVInfo{*utils.GetTestPVCPVWithVG().PVBounding})[0]
	pvPending.Status.Phase = corev1.VolumePending

	//test without vgname
	pvBoundingHaveNoVGInfo := *utils.GetTestPVCPVWithVG().PVBounding
	pvBoundingHaveNoVGInfo.VgName = ""
	pvBoundingHaveNoVG := utils.CreateTestPersistentVolume([]utils.TestPVInfo{pvBoundingHaveNoVGInfo})[0]

	//test assume success
	pvcBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithVG().PVCBounding})[0]
	pvBounding := utils.CreateTestPersistentVolume([]utils.TestPVInfo{*utils.GetTestPVCPVWithVG().PVBounding})[0]

	//test pvc < pv
	pvcBoundingExpandSmallInfo := *utils.GetTestPVCPVWithVG().PVCBounding
	pvcBoundingExpandSmallInfo.Size = "100Gi"
	pvcBoundingExpandSmall := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{pvcBoundingExpandSmallInfo})[0]
	pvBoundingExpandSmallInfo := *utils.GetTestPVCPVWithVG().PVBounding
	pvBoundingExpandSmallInfo.VolumeSize = "100Gi"
	pvBoundingExpandSmall := utils.CreateTestPersistentVolume([]utils.TestPVInfo{pvBoundingExpandSmallInfo})[0]

	//test pvc expand size too large
	pvcBoundingExpandLargeInfo := *utils.GetTestPVCPVWithVG().PVCBounding
	pvcBoundingExpandLargeInfo.Size = "100Gi"
	pvcBoundingExpandLarge := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{pvcBoundingExpandSmallInfo})[0]
	pvBoundingLargeInfo := *utils.GetTestPVCPVWithVG().PVBounding
	pvBoundingLargeInfo.VolumeSize = "500Gi"
	pvBoundingLarge := utils.CreateTestPersistentVolume([]utils.TestPVInfo{pvBoundingLargeInfo})[0]

	type args struct {
		nodeName string
		pv       *corev1.PersistentVolume
	}

	type fields struct {
		pvc       *corev1.PersistentVolumeClaim
		pvExist   *corev1.PersistentVolume
		nodeLocal *nodelocalstorage.NodeLocalStorage
		pvcDetail *LVMPVAllocated
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
			name: "test assume fail for pv is nil",
			args: args{
				nodeName: utils.NodeName3,
				pv:       nil,
			},
			fields: fields{
				pvc:       pvcBounding,
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
		{
			name: "test assume fail for pending pv",
			args: args{
				nodeName: utils.NodeName3,
				pv:       pvPending,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvc:       nil,
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
		{
			name: "test assume fail for pv have no vgName",
			args: args{
				nodeName: utils.NodeName3,
				pv:       pvBoundingHaveNoVG,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvc:       nil,
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
		{
			name: "test assume success, have got pvc event before",
			args: args{
				nodeName: utils.NodeName3,
				pv:       pvBounding,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvc:       pvcBounding,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVSize(pvBounding),
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
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
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
		{
			name: "test assume success, have not got pvc event before",
			args: args{
				nodeName: utils.NodeName3,
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
								Requested:   utils.GetPVSize(pvBounding),
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
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
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
		{
			name: "test assume success, have got pvc event before and localStorage not init",
			args: args{
				nodeName: utils.NodeName3,
				pv:       pvBounding,
			},
			fields: fields{
				pvc: pvcBounding,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       0,
								Allocatable: 0,
								Requested:   utils.GetPVSize(pvBounding),
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{},
						InitedByNLS:  false,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
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
		{
			name: "test expand PV success, pv expand small",
			args: args{
				nodeName: utils.NodeName3,
				pv:       pvBoundingExpandSmall,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvc:       pvcBoundingExpandSmall,
				pvExist:   pvBounding,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVSize(pvBoundingExpandSmall),
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
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcBounding.Name,
								PVCNamespace: pvcBounding.Namespace,
								VolumeName:   pvBounding.Name,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVSize(pvBoundingExpandSmall),
								Allocated:    utils.GetPVSize(pvBoundingExpandSmall),
							},
						},
					},
				},
			},
		},
		{
			name: "test expand PV success, pv expand large",
			args: args{
				nodeName: utils.NodeName3,
				pv:       pvBoundingLarge,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvc:       pvcBoundingExpandLarge,
				pvExist:   pvBounding,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVSize(pvBoundingLarge),
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
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcBounding.Name,
								PVCNamespace: pvcBounding.Namespace,
								VolumeName:   pvBounding.Name,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVSize(pvBoundingLarge),
								Allocated:    utils.GetPVSize(pvBoundingLarge),
							},
						},
					},
				},
			},
		},
		{
			name: "test assume and revert pvcDetail success, have reserve by scheduler before",
			args: args{
				nodeName: utils.NodeName3,
				pv:       pvBounding,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvc:       pvcBounding,
				pvcDetail: &LVMPVAllocated{
					VGName: utils.VGSSD,
					BasePVAllocated: BasePVAllocated{
						PVCName:      pvcBounding.Name,
						PVCNamespace: pvcBounding.Namespace,
						VolumeName:   pvBounding.Name,
						NodeName:     utils.NodeName3,
						Requested:    utils.GetPVSize(pvBounding),
						Allocated:    0,
					},
				},
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVSize(pvBounding),
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
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
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
			allocator := NewLVMCommonPVAllocator(cache)

			if tt.fields.nodeLocal != nil {
				cache.AddNodeStorage(tt.fields.nodeLocal)
			}

			if tt.fields.pvc != nil {
				cache.addPVCInfo(tt.fields.pvc)
				allocator.pvcAdd(tt.args.nodeName, tt.fields.pvc, tt.fields.pvc.Spec.VolumeName)
			}

			if tt.fields.pvcDetail != nil {
				pendingPVC := tt.fields.pvc.DeepCopy()
				pendingPVC.Status.Phase = corev1.ClaimPending
				cache.DeletePVC(pendingPVC)
				err := allocator.reserve(tt.args.nodeName, &NodeAllocateUnits{LVMPVCAllocateUnits: []*LVMPVAllocated{tt.fields.pvcDetail}})
				assert.NoError(t, err)
				assert.NotEmpty(t, cache.pvAllocatedDetails.pvcAllocated, "check reserve result")
			}

			if tt.fields.pvExist != nil {
				allocator.pvUpdate(tt.args.nodeName, nil, tt.fields.pvExist)
			}

			allocator.pvAdd(tt.args.nodeName, tt.args.pv)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_lvm_pvcUpdate(t *testing.T) {
	nodeLocal := utils.CreateTestNodeLocalStorage3()

	pvcPending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithVG().PVCPending})[0]

	pvcBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithVG().PVCBounding})[0]
	pvBounding := utils.CreateTestPersistentVolume([]utils.TestPVInfo{*utils.GetTestPVCPVWithVG().PVBounding})[0]

	//test pvc > pv
	pvcBoundingExpandInfo := *utils.GetTestPVCPVWithVG().PVCBounding
	pvcBoundingExpandInfo.Size = "160Gi"
	pvcBoundingExpand := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{pvcBoundingExpandInfo})[0]

	//test pvc < pv
	pvcBoundingExpandSmallInfo := *utils.GetTestPVCPVWithVG().PVCBounding
	pvcBoundingExpandSmallInfo.Size = "100Gi"
	pvcBoundingExpandSmall := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{pvcBoundingExpandSmallInfo})[0]

	//test pvc expand size too large
	pvcBoundingTooLargeInfo := *utils.GetTestPVCPVWithVG().PVCBounding
	pvcBoundingTooLargeInfo.Size = "500Gi"
	pvBoundingTooLargeInfo := *utils.GetTestPVCPVWithVG().PVBounding
	pvBoundingTooLargeInfo.VolumeSize = "500Gi"
	pvcBoundingTooLarge := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{pvcBoundingTooLargeInfo})[0]

	type args struct {
		nodeName string
		pvc      *corev1.PersistentVolumeClaim
		pvName   string
	}

	type fields struct {
		nodeLocal *nodelocalstorage.NodeLocalStorage
		pvcExist  *corev1.PersistentVolumeClaim
		pvExist   *corev1.PersistentVolume
		pvcDetail *LVMPVAllocated
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
			name: "test allocate pvc pending",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcPending,
				pvName:   "",
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
		{
			name: "test allocate normal and have allocated by pv before",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBounding,
				pvName:   pvBounding.Name,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvExist:   pvBounding,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVCRequested(pvcBounding),
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
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcBounding.Name,
								PVCNamespace: pvcBounding.Namespace,
								VolumeName:   pvBounding.Name,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcBounding),
								Allocated:    utils.GetPVCRequested(pvcBounding),
							},
						},
					},
				},
			},
		},
		{
			name: "test expand normal and have not allocated by pv before",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBoundingExpand,
				pvName:   pvBounding.Name,
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
		{
			name: "test pvc scheduled and expand normal and have not allocated by pv before",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBoundingExpand,
				pvName:   pvBounding.Name,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvcDetail: &LVMPVAllocated{
					VGName: utils.VGSSD,
					BasePVAllocated: BasePVAllocated{
						PVCName:      pvcBounding.Name,
						PVCNamespace: pvcBounding.Namespace,
						VolumeName:   pvBounding.Name,
						NodeName:     utils.NodeName3,
						Requested:    utils.GetPVCRequested(pvcBounding),
						Allocated:    0,
					},
				},
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVCRequested(pvcBounding),
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
					pvcAllocated: map[string]PVAllocated{
						utils.GetPVCKey(pvcBounding.Namespace, pvcBounding.Name): &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcBounding.Name,
								PVCNamespace: pvcBounding.Namespace,
								VolumeName:   pvBounding.Name,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcBoundingExpand),
								Allocated:    utils.GetPVSize(pvBounding),
							},
						},
					},
					pvAllocated: map[string]PVAllocated{},
				},
			},
		},
		{
			name: "test allocate normal and have allocated by pv before and nodeLocal not init",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBounding,
				pvName:   pvBounding.Name,
			},
			fields: fields{
				pvExist: pvBounding,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(0),
								Allocatable: int64(0),
								Requested:   utils.GetPVCRequested(pvcBounding),
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{},
						InitedByNLS:  false,
					},
				},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcBounding.Name,
								PVCNamespace: pvcBounding.Namespace,
								VolumeName:   pvBounding.Name,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcBounding),
								Allocated:    utils.GetPVCRequested(pvcBounding),
							},
						},
					},
				},
			},
		},
		{
			name: "test expand pvc too large and pv had allocated before: allocate success, and store requested > allocatable",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBoundingTooLarge,
				pvName:   pvBounding.Name,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvcExist:  pvcBounding,
				pvExist:   pvBounding,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVCRequested(pvcBoundingTooLarge),
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
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcBounding.Name,
								PVCNamespace: pvcBounding.Namespace,
								VolumeName:   pvBounding.Name,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcBounding),
								Allocated:    utils.GetPVCRequested(pvcBoundingTooLarge),
							},
						},
					},
				},
			},
		},
		{
			name: "test expand pvc too large and pv had not allocated before: allocate success, and store requested > allocatable",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBoundingTooLarge,
				pvName:   pvBounding.Name,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvcExist:  pvcBounding,
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
		{
			name: "test expand pvc small then pv and pv have allocated before: allocate by pv",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBoundingExpandSmall,
				pvName:   pvBounding.Name,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvcExist:  pvcBounding,
				pvExist:   pvBounding,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVSize(pvBounding),
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
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
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
		{
			name: "test expand pvc small then pv and pv have not allocated before: allocate by pv",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBoundingExpandSmall,
				pvName:   pvBounding.Name,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvcExist:  pvcBounding,
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
		{
			name: "test expand normal and have allocated by pv before",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBoundingExpand,
				pvName:   pvBounding.Name,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvcExist:  pvcBounding,
				pvExist:   pvBounding,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   utils.GetPVCRequested(pvcBoundingExpand),
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
					pvAllocated: map[string]PVAllocated{
						pvBounding.Name: &LVMPVAllocated{
							VGName: utils.VGSSD,
							BasePVAllocated: BasePVAllocated{
								PVCName:      pvcBounding.Name,
								PVCNamespace: pvcBounding.Namespace,
								VolumeName:   pvBounding.Name,
								NodeName:     utils.NodeName3,
								Requested:    utils.GetPVCRequested(pvcBounding),
								Allocated:    utils.GetPVCRequested(pvcBoundingExpand),
							},
						},
					},
				},
			},
		},
		{
			name: "test expand normal and have not allocated by pv before",
			args: args{
				nodeName: utils.NodeName3,
				pvc:      pvcBoundingExpand,
				pvName:   pvBounding.Name,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				pvcExist:  pvcBounding,
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
			allocator := NewLVMCommonPVAllocator(cache)

			if tt.fields.nodeLocal != nil {
				cache.AddNodeStorage(tt.fields.nodeLocal)
			}

			if tt.fields.pvExist != nil {
				allocator.pvAdd(tt.args.nodeName, tt.fields.pvExist)
			}
			if tt.fields.pvcExist != nil {
				cache.addPVCInfo(tt.fields.pvcExist)
				allocator.pvcAdd(tt.args.nodeName, tt.fields.pvcExist, tt.fields.pvcExist.Spec.VolumeName)
			}

			if tt.fields.pvcDetail != nil {
				_ = allocator.reserve(tt.args.nodeName, &NodeAllocateUnits{LVMPVCAllocateUnits: []*LVMPVAllocated{tt.fields.pvcDetail}})
				assert.NotEmpty(t, cache.pvAllocatedDetails.pvcAllocated, "check reserve result")
			}

			cache.addPVCInfo(tt.args.pvc)
			allocator.pvcUpdate(tt.args.nodeName, nil, tt.args.pvc, tt.args.pvName)

			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_DeleteLVM(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()

	//test delete
	pvcBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithVG().PVCBounding})[0]
	pvBounding := utils.CreateTestPersistentVolume([]utils.TestPVInfo{*utils.GetTestPVCPVWithVG().PVBounding})[0]

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
			name: "test delete success",
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
			allocator := NewLVMCommonPVAllocator(cache)

			cache.AddNodeStorage(tt.fields.nodeLocal)

			if tt.args.pvc != nil {
				cache.addPVCInfo(tt.args.pvc)
			}

			//prepare data
			allocator.pvAdd(tt.args.nodeName, tt.args.pv)

			//check
			allocator.pvDelete(tt.args.nodeName, tt.args.pv)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}
