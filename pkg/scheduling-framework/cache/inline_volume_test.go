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
)

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
			cache := CreateTestCache()
			allocator := NewInlineVolumeAllocator(cache)

			for _, nodeLocal := range tt.fields.nodeLocals {
				cache.AddNodeStorage(nodeLocal)
			}

			cache.inlineVolumeAllocatedDetails = tt.fields.inlineVolumeAllocatedDetails
			err := allocator.reserve(tt.args.nodeName, tt.args.podUid, tt.args.units.InlineVolumeAllocateUnits)
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
			cache := CreateTestCache()
			allocator := NewInlineVolumeAllocator(cache)

			for _, nodeLocal := range tt.fields.nodeLocals {
				cache.AddNodeStorage(nodeLocal)
			}

			_ = allocator.reserve(tt.args.nodeName, tt.args.podUid, tt.args.units.InlineVolumeAllocateUnits)
			nodeDetails := cache.inlineVolumeAllocatedDetails[tt.args.nodeName]
			assert.NotEmpty(t, nodeDetails)
			podDetails := nodeDetails[tt.args.podUid]
			assert.NotEmpty(t, podDetails)

			allocator.unreserve(tt.args.nodeName, tt.args.podUid, tt.args.units.InlineVolumeAllocateUnits)
			assert.Equal(t, tt.expect.units, tt.args.units, "check units")
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails)
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}
