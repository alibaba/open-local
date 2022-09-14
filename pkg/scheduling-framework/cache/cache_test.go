/*
Copyright 2022/8/27 Alibaba Cloud.

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
	"context"
	"fmt"
	"testing"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/stretchr/testify/assert"

	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_reserveLVMPVC(t *testing.T) {

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
			cache, kubeClientSet := CreateTestCache()

			for _, nodeLocal := range tt.fields.nodeLocals {
				cache.AddNodeStorage(nodeLocal)
			}

			for _, pvc := range tt.fields.pvcs {
				_, _ = kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = cache.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			currentStorageState := cache.states[tt.args.nodeName]
			err := cache.reserveLVMPVC(tt.args.nodeName, tt.args.units, currentStorageState)
			assert.Equal(t, tt.expect.resultErr, err != nil, fmt.Sprintf("errInfo:%+v", err))
			assert.Equal(t, tt.expect.units, tt.args.units, "check units")
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_unreserveLVMPVC(t *testing.T) {

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
			cache, kubeClientSet := CreateTestCache()

			for _, nodeLocal := range tt.fields.nodeLocals {
				cache.AddNodeStorage(nodeLocal)
			}

			for _, pvc := range tt.fields.pvcs {
				_, _ = kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = cache.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			cache.states = tt.fields.states
			cache.pvAllocatedDetails = tt.fields.pvAllocatedDetails

			cache.unreserveLVMPVCs(tt.args.nodeName, tt.args.units)
			assert.Equal(t, tt.expect.units, tt.args.units, "check units")
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_reserveDevicePVC(t *testing.T) {

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
			cache, kubeClientSet := CreateTestCache()

			for _, nodeLocal := range tt.fields.nodeLocals {
				cache.AddNodeStorage(nodeLocal)
			}

			for _, pvc := range tt.fields.pvcs {
				_, _ = kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = cache.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			currentStorageState := cache.states[tt.args.nodeName]
			err := cache.reserveDevicePVC(tt.args.nodeName, tt.args.units, currentStorageState)
			assert.Equal(t, tt.expect.resultErr, err != nil, fmt.Sprintf("errInfo:%+v", err))
			assert.Equal(t, tt.expect.units, tt.args.units, "check units")
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_unreserveDevicePVC(t *testing.T) {

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
			cache, kubeClientSet := CreateTestCache()

			for _, nodeLocal := range tt.fields.nodeLocals {
				cache.AddNodeStorage(nodeLocal)
			}

			for _, pvc := range tt.fields.pvcs {
				_, _ = kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = cache.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			cache.states = tt.fields.states
			cache.pvAllocatedDetails = tt.fields.pvAllocatedDetails

			cache.unreserveDevicePVCs(tt.args.nodeName, tt.args.units)
			assert.Equal(t, tt.expect.units, tt.args.units, "check units")
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_reserveInlineVolumes(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()
	podUid := "test_pod"
	inlineVolumeNormal := CreateInlineVolumes(podUid, "test_volume", int64(10*utils.LocalGi))
	expectInlineVolumeNormal := inlineVolumeNormal.DeepCopy()
	expectInlineVolumeNormal.Allocated = inlineVolumeNormal.VolumeSize

	inlineVolumeTooLarge := CreateInlineVolumes(podUid, "test_volume", int64(1000*utils.LocalGi))
	expectInlineVolumeTooLarge := inlineVolumeTooLarge.DeepCopy()

	type args struct {
		nodeName string
		podUid   string
		units    *NodeAllocateUnits
	}

	type fields struct {
		nodeLocals                   []*nodelocalstorage.NodeLocalStorage
		inlineVolumeAllocatedDetails map[string]NodeInlineVolumeAllocatedDetails
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
			name: "test reserve success  inlineVolume normal",
			args: args{
				nodeName: utils.NodeName3,
				podUid:   podUid,
				units: &NodeAllocateUnits{
					InlineVolumeAllocateUnits: []*InlineVolumeAllocated{inlineVolumeNormal},
				},
			},
			fields: fields{
				nodeLocals:                   []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					InlineVolumeAllocateUnits: []*InlineVolumeAllocated{expectInlineVolumeNormal},
				},
				resultErr: false,
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   inlineVolumeNormal.VolumeSize,
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
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{
					utils.NodeName3: {
						podUid: &PodInlineVolumeAllocatedDetails{expectInlineVolumeNormal},
					},
				},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "test reserve fail for inlineVolume too large",
			args: args{
				nodeName: utils.NodeName3,
				podUid:   podUid,
				units: &NodeAllocateUnits{
					InlineVolumeAllocateUnits: []*InlineVolumeAllocated{inlineVolumeTooLarge},
				},
			},
			fields: fields{
				nodeLocals:                   []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					InlineVolumeAllocateUnits: []*InlineVolumeAllocated{expectInlineVolumeTooLarge},
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
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{
					utils.NodeName3: {},
				},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "test inlineVolume normal reserve success, pod cache not nil but empty",
			args: args{
				nodeName: utils.NodeName3,
				podUid:   podUid,
				units: &NodeAllocateUnits{
					InlineVolumeAllocateUnits: []*InlineVolumeAllocated{inlineVolumeNormal},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{
					utils.NodeName3: {
						podUid: &PodInlineVolumeAllocatedDetails{},
					},
				},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					InlineVolumeAllocateUnits: []*InlineVolumeAllocated{expectInlineVolumeNormal},
				},
				resultErr: false,
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   inlineVolumeNormal.VolumeSize,
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
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{
					utils.NodeName3: {
						podUid: &PodInlineVolumeAllocatedDetails{expectInlineVolumeNormal},
					},
				},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, _ := CreateTestCache()

			for _, nodeLocal := range tt.fields.nodeLocals {
				cache.AddNodeStorage(nodeLocal)
			}

			cache.inlineVolumeAllocatedDetails = tt.fields.inlineVolumeAllocatedDetails
			currentStorageState := cache.states[tt.args.nodeName]
			err := cache.reserveInlineVolumes(tt.args.nodeName, tt.args.podUid, tt.args.units.InlineVolumeAllocateUnits, currentStorageState)
			assert.Equal(t, tt.expect.resultErr, err != nil, fmt.Sprintf("errInfo:%+v", err))
			assert.Equal(t, tt.expect.units, tt.args.units, "check units")
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_unreserveInlineVolumes(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()
	podUid := "test_pod"
	inlineVolumeNormal := CreateInlineVolumes(podUid, "test_volume", int64(10*utils.LocalGi))
	expectInlineVolumeNormal := inlineVolumeNormal.DeepCopy()
	expectInlineVolumeNormal.Allocated = 0

	type args struct {
		nodeName string
		podUid   string
		units    *NodeAllocateUnits
	}

	type fields struct {
		nodeLocals []*nodelocalstorage.NodeLocalStorage
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
			name: "test revert success  inlineVolume normal",
			args: args{
				nodeName: utils.NodeName3,
				podUid:   podUid,
				units: &NodeAllocateUnits{
					InlineVolumeAllocateUnits: []*InlineVolumeAllocated{inlineVolumeNormal},
				},
			},
			fields: fields{
				nodeLocals: []*nodelocalstorage.NodeLocalStorage{nodeLocal},
			},
			expect: expect{
				units: &NodeAllocateUnits{
					InlineVolumeAllocateUnits: []*InlineVolumeAllocated{expectInlineVolumeNormal},
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
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{
					utils.NodeName3: {
						podUid: &PodInlineVolumeAllocatedDetails{},
					},
				},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, _ := CreateTestCache()

			for _, nodeLocal := range tt.fields.nodeLocals {
				cache.AddNodeStorage(nodeLocal)
			}

			currentStorageState := cache.states[tt.args.nodeName]
			_ = cache.reserveInlineVolumes(tt.args.nodeName, tt.args.podUid, tt.args.units.InlineVolumeAllocateUnits, currentStorageState)
			nodeDetails := cache.inlineVolumeAllocatedDetails[tt.args.nodeName]
			assert.NotEmpty(t, nodeDetails)
			podDetails := nodeDetails[tt.args.podUid]
			assert.NotEmpty(t, podDetails)

			cache.unreserveInlineVolumes(tt.args.nodeName, tt.args.podUid, tt.args.units, currentStorageState)
			assert.Equal(t, tt.expect.units, tt.args.units, "check units")
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_AllocateLVM_ByPV(t *testing.T) {

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
			cache, kubeClientSet := CreateTestCache()

			if tt.fields.nodeLocal != nil {
				cache.AddNodeStorage(tt.fields.nodeLocal)
			}

			if tt.fields.pvc != nil {
				cache.AddPVCInfo(tt.fields.pvc, tt.args.nodeName, tt.fields.pvc.Spec.VolumeName)
				cache.AllocateLVMByPVCEvent(tt.fields.pvc, tt.args.nodeName, tt.fields.pvc.Spec.VolumeName)
			}

			if tt.fields.pvcDetail != nil {
				pendingPVC := tt.fields.pvc.DeepCopy()
				pendingPVC.Status.Phase = corev1.ClaimPending
				_, _ = kubeClientSet.CoreV1().PersistentVolumeClaims(tt.fields.pvc.Namespace).Create(context.Background(), pendingPVC, metav1.CreateOptions{})
				_ = cache.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pendingPVC)
				err := cache.reserveLVMPVC(tt.args.nodeName, &NodeAllocateUnits{LVMPVCAllocateUnits: []*LVMPVAllocated{tt.fields.pvcDetail}}, cache.states[tt.args.nodeName])
				assert.NoError(t, err)
				assert.NotEmpty(t, cache.pvAllocatedDetails.pvcAllocated, "check reserve result")
			}

			if tt.fields.pvExist != nil {
				cache.AllocateLVMByPV(tt.fields.pvExist, tt.args.nodeName)
			}

			cache.AllocateLVMByPV(tt.args.pv, tt.args.nodeName)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_AllocateLVM_byPVCEvent(t *testing.T) {
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
			cache, kubeClientSet := CreateTestCache()

			if tt.fields.nodeLocal != nil {
				cache.AddNodeStorage(tt.fields.nodeLocal)
			}

			if tt.fields.pvExist != nil {
				cache.AllocateLVMByPV(tt.fields.pvExist, tt.args.nodeName)
			}
			if tt.fields.pvcExist != nil {
				cache.AddPVCInfo(tt.fields.pvcExist, tt.args.nodeName, tt.fields.pvcExist.Spec.VolumeName)
				cache.AllocateLVMByPVCEvent(tt.fields.pvcExist, tt.args.nodeName, tt.fields.pvcExist.Spec.VolumeName)
			}

			if tt.fields.pvcDetail != nil {
				_, _ = kubeClientSet.CoreV1().PersistentVolumeClaims(pvcPending.Namespace).Create(context.Background(), pvcPending, metav1.CreateOptions{})
				_ = cache.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvcPending)
				_ = cache.reserveLVMPVC(tt.args.nodeName, &NodeAllocateUnits{LVMPVCAllocateUnits: []*LVMPVAllocated{tt.fields.pvcDetail}}, cache.states[tt.args.nodeName])
				assert.NotEmpty(t, cache.pvAllocatedDetails.pvcAllocated, "check reserve result")
			}

			cache.AddPVCInfo(tt.args.pvc, tt.args.nodeName, tt.args.pvName)
			cache.AllocateLVMByPVCEvent(tt.args.pvc, tt.args.nodeName, tt.args.pvName)

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
			cache, kubeClientSet := CreateTestCache()

			cache.AddNodeStorage(tt.fields.nodeLocal)

			if tt.args.pvc != nil {
				_, _ = kubeClientSet.CoreV1().PersistentVolumeClaims(tt.args.pvc.Namespace).Create(context.Background(), tt.args.pvc, metav1.CreateOptions{})
				_ = cache.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(tt.args.pvc)
			}

			if tt.args.pv != nil {
				_, _ = kubeClientSet.CoreV1().PersistentVolumes().Create(context.Background(), tt.args.pv, metav1.CreateOptions{})
				_ = cache.coreV1Informers.PersistentVolumes().Informer().GetIndexer().Add(tt.args.pv)
			}

			//prepare data
			cache.AllocateLVMByPV(tt.args.pv, tt.args.nodeName)

			//check
			cache.DeleteLVM(tt.args.pv, tt.args.nodeName)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_AllocateDevice(t *testing.T) {

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
			cache, _ := CreateTestCache()

			if tt.fields.nodeLocal != nil {
				cache.AddNodeStorage(tt.fields.nodeLocal)
			}

			if len(tt.fields.states) > 0 {
				cache.states = tt.fields.states
			}

			cache.AllocateDevice(tt.args.pv, tt.args.nodeName)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_DeleteDevice(t *testing.T) {

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
			cache, kubeClientSet := CreateTestCache()

			cache.AddNodeStorage(tt.fields.nodeLocal)

			if tt.args.pvc != nil {
				_, _ = kubeClientSet.CoreV1().PersistentVolumeClaims(tt.args.pvc.Namespace).Create(context.Background(), tt.args.pvc, metav1.CreateOptions{})
				_ = cache.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(tt.args.pvc)
			}

			if tt.args.pv != nil {
				_, _ = kubeClientSet.CoreV1().PersistentVolumes().Create(context.Background(), tt.args.pv, metav1.CreateOptions{})
				_ = cache.coreV1Informers.PersistentVolumes().Informer().GetIndexer().Add(tt.args.pv)
			}

			cache.AllocateDevice(tt.args.pv, tt.args.nodeName)

			cache.DeleteDevice(tt.args.pv, tt.args.nodeName)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}

func Test_AddNodeStorage(t *testing.T) {
	type args struct {
		nodeLocal *nodelocalstorage.NodeLocalStorage
	}

	type fields struct {
		/*cache*/
		states map[string]*NodeStorageState
	}

	type expect struct {
		/*cache*/
		states map[string]*NodeStorageState
	}

	nodeLocal := utils.CreateTestNodeLocalStorage3()

	tests := []struct {
		name   string
		args   args
		fields fields
		expect expect
	}{
		{
			name: "test_node_state_have_not_found",
			args: args{
				nodeLocal: nodeLocal,
			},
			fields: fields{
				states: map[string]*NodeStorageState{},
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
			},
		},
		{
			name: "test_node_state_have_allocated_first",
			args: args{
				nodeLocal: nodeLocal,
			},
			fields: fields{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       0,
								Allocatable: 0,
								Requested:   int64(10 * utils.LocalGi),
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       0,
								Allocatable: 0,
								Requested:   int64(150 * utils.LocalGi),
								IsAllocated: true,
							},
						},
						InitedByNLS: false,
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
								Requested:   int64(10 * utils.LocalGi),
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
			},
		},
		{
			name: "test_node_state_update",
			args: args{
				nodeLocal: nodeLocal,
			},
			fields: fields{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(200 * utils.LocalGi),
								Requested:   int64(10 * utils.LocalGi),
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(100 * utils.LocalGi),
								Allocatable: int64(100 * utils.LocalGi),
								Requested:   int64(100 * utils.LocalGi),
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: true,
							},
						},
						InitedByNLS: true,
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
								Requested:   int64(10 * utils.LocalGi),
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
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, _ := CreateTestCache()
			cache.states = tt.fields.states
			cache.AddNodeStorage(tt.args.nodeLocal)
			assert.Equal(t, tt.expect.states, cache.states)
		})
	}
}

func Test_initIfNeedAndGetVGState(t *testing.T) {
	type args struct {
		nodeName string
		vgName   string
	}

	type fields struct {
		/*cache*/
		states map[string]*NodeStorageState
	}

	type expect struct {
		/*cache*/
		states map[string]*NodeStorageState

		//return result
		vgState      *VGStoragePool
		expectInited bool
	}

	tests := []struct {
		name   string
		args   args
		fields fields
		expect expect
	}{
		{
			name: "test_node_local_storage_not_init",
			args: args{
				nodeName: utils.NodeName3,
				vgName:   utils.VGSSD,
			},
			fields: fields{
				states: map[string]*NodeStorageState{},
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					utils.NodeName3: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name: utils.VGSSD,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{},
						InitedByNLS:  false,
					},
				},
				vgState: &VGStoragePool{
					Name: utils.VGSSD,
				},
				expectInited: false,
			},
		},
		{
			name: "test_node_state_inited",
			args: args{
				nodeName: utils.NodeName3,
				vgName:   utils.VGSSD,
			},
			fields: fields{
				states: map[string]*NodeStorageState{
					utils.NodeName3: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(200 * utils.LocalGi),
								Allocatable: int64(200 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(100 * utils.LocalGi),
								Allocatable: int64(100 * utils.LocalGi),
								Requested:   0,
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: false,
							},
						},
						InitedByNLS: true,
					},
				},
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					utils.NodeName3: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(200 * utils.LocalGi),
								Allocatable: int64(200 * utils.LocalGi),
								Requested:   0,
							},
						},
						DeviceStates: map[string]*DeviceResourcePool{
							"/dev/sdc": {
								Name:        "/dev/sdc",
								Total:       int64(100 * utils.LocalGi),
								Allocatable: int64(100 * utils.LocalGi),
								Requested:   0,
								MediaType:   localtype.MediaTypeHDD,
								IsAllocated: false,
							},
						},
						InitedByNLS: true,
					},
				},
				vgState: &VGStoragePool{
					Name:        utils.VGSSD,
					Total:       int64(200 * utils.LocalGi),
					Allocatable: int64(200 * utils.LocalGi),
					Requested:   0,
				},
				expectInited: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, _ := CreateTestCache()
			cache.states = tt.fields.states
			gotVg, gotInited := cache.initIfNeedAndGetVGState(tt.args.nodeName, tt.args.vgName)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.vgState, gotVg, "check vg return")
			assert.Equal(t, tt.expect.expectInited, gotInited, "check init")
		})
	}
}

func Test_UpdatePod(t *testing.T) {

	nodeLocal := utils.CreateTestNodeLocalStorage3()

	podInfo := &utils.TestPodInfo{
		PodName:      utils.InvolumePodName,
		PodNameSpace: utils.LocalNameSpace,
		NodeName:     utils.NodeName3,
		PodStatus:    corev1.PodRunning,
		InlineVolumeInfos: []*utils.TestInlineVolumeInfo{
			{
				VolumeName: "test_inline_volume",
				VolumeSize: "10Gi",
				VgName:     utils.VGSSD,
			},
		},
	}

	inlineVolume := CreateInlineVolumes(podInfo.PodName, "test_inline_volume", int64(10*utils.LocalGi))

	//pod running
	podRunning := utils.CreatePod(podInfo)

	//pod succeeded
	podInfo.PodStatus = corev1.PodSucceeded
	podSucceeded := utils.CreatePod(podInfo)

	//pod failed
	podInfo.PodStatus = corev1.PodFailed
	podFailed := utils.CreatePod(podInfo)

	//pod pending
	podInfo.PodStatus = corev1.PodPending
	podPending := utils.CreatePod(podInfo)

	type args struct {
		pod *corev1.Pod
	}

	type fields struct {
		nodeLocal *nodelocalstorage.NodeLocalStorage
		podBefore *corev1.Pod
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
			name: "update pod running with inlineVolume",
			args: args{
				pod: podRunning,
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
								Requested:   int64(10 * utils.LocalGi),
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
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{
					utils.NodeName3: {
						string(podRunning.UID): &PodInlineVolumeAllocatedDetails{inlineVolume},
					},
				},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "update skip for pod running with inlineVolume, because inlineVolume exist",
			args: args{
				pod: utils.CreatePod(&utils.TestPodInfo{
					PodName:      utils.InvolumePodName,
					PodNameSpace: utils.LocalNameSpace,
					NodeName:     utils.NodeName3,
					PodStatus:    corev1.PodRunning,
					InlineVolumeInfos: []*utils.TestInlineVolumeInfo{
						{
							VolumeName: "test_inline_volume",
							VolumeSize: "20Gi",
							VgName:     utils.VGSSD,
						},
					},
				}),
			},
			fields: fields{
				nodeLocal: nodeLocal,
				podBefore: podRunning,
			},
			expect: expect{
				states: map[string]*NodeStorageState{
					nodeLocal.Name: {
						VGStates: map[string]*VGStoragePool{
							utils.VGSSD: {
								Name:        utils.VGSSD,
								Total:       int64(300 * utils.LocalGi),
								Allocatable: int64(300 * utils.LocalGi),
								Requested:   int64(10 * utils.LocalGi),
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
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{
					utils.NodeName3: {
						string(podRunning.UID): &PodInlineVolumeAllocatedDetails{inlineVolume},
					},
				},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "update pod succeeded with inlineVolume, then remove",
			args: args{
				pod: podSucceeded,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				podBefore: podRunning,
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
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{
					utils.NodeName3: {},
				},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "update pod failed with inlineVolume, then remove",
			args: args{
				pod: podFailed,
			},
			fields: fields{
				nodeLocal: nodeLocal,
				podBefore: podRunning,
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
				inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{
					utils.NodeName3: {},
				},
				pvAllocatedDetails: &PVAllocatedDetails{
					pvcAllocated: map[string]PVAllocated{},
					pvAllocated:  map[string]PVAllocated{},
				},
			},
		},
		{
			name: "update skip for pod pending with inlineVolume",
			args: args{
				pod: podPending,
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

			cache, kubeClientSet := CreateTestCache()
			cache.AddNodeStorage(tt.fields.nodeLocal)

			if tt.fields.podBefore != nil {
				_, _ = kubeClientSet.CoreV1().Pods(tt.fields.podBefore.Namespace).Create(context.Background(), tt.fields.podBefore, metav1.CreateOptions{})
				_ = cache.coreV1Informers.Pods().Informer().GetIndexer().Add(tt.fields.podBefore)
				cache.AddPod(tt.fields.podBefore)
			}

			_, _ = kubeClientSet.CoreV1().Pods(tt.args.pod.Namespace).Create(context.Background(), tt.args.pod, metav1.CreateOptions{})
			_ = cache.coreV1Informers.Pods().Informer().GetIndexer().Add(tt.args.pod)

			cache.UpdatePod(tt.args.pod)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails, "checn inline details")
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}
