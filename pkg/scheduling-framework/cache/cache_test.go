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
	"testing"

	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
)

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
			cache := CreateTestCache()
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
			cache := CreateTestCache()
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

			cache := CreateTestCache()
			cache.AddNodeStorage(tt.fields.nodeLocal)

			if tt.fields.podBefore != nil {
				cache.AddPod(tt.fields.podBefore)
			}

			cache.UpdatePod(tt.args.pod)
			assert.Equal(t, tt.expect.states, cache.states, "check cache states")
			assert.Equal(t, tt.expect.inlineVolumeAllocatedDetails, cache.inlineVolumeAllocatedDetails, "checn inline details")
			assert.Equal(t, tt.expect.pvAllocatedDetails, cache.pvAllocatedDetails, "check pv details")

		})
	}
}
