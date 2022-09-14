/*
Copyright 2022/8/31 Alibaba Cloud.

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
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduling-framework/cache"
	"github.com/alibaba/open-local/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

type scoreResult struct {
	nodeStatuses map[string]*framework.Status
	nodeScores   map[string]int64
	stateData    *stateData
}

func Test_score_volumeGroup_binPack(t *testing.T) {

	podNoLocal := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "simple-pod",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos:     []*utils.TestPVCInfo{utils.GetTestPVCPVNotLocal().PVCPending},
	})

	pvcInfo := utils.GetTestPVCPVWithoutVG().PVCPending
	pvcInfo.Size = "100Gi"

	podWithLVMPVC := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podWithLVMPVC",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos: []*utils.TestPVCInfo{
			pvcInfo,
		},
	})

	podWithInlineVolume := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podWithInlineVolume",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		InlineVolumeInfos: []*utils.TestInlineVolumeInfo{
			{
				VolumeName: "test_inline_volume",
				VolumeSize: "150Gi",
				VgName:     utils.VGSSD,
			},
		},
	})

	podBoth := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podBoth",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos: []*utils.TestPVCInfo{
			pvcInfo,
		},
		InlineVolumeInfos: []*utils.TestInlineVolumeInfo{
			{
				VolumeName: "test_inline_volume",
				VolumeSize: "150Gi",
				VgName:     utils.VGSSD,
			},
		},
	})

	//prepare pvc
	pvcWithoutVG := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcInfo})[0]
	pvcNoLocal := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVNotLocal().PVCPending})[0]

	type args struct {
		pod *corev1.Pod
	}
	type fields struct {
		pvcs map[string]*corev1.PersistentVolumeClaim
	}

	tests := []struct {
		name        string
		args        args
		fields      fields
		expectScore *scoreResult
	}{
		{
			name: "test pod with common pvc",
			args: args{
				pod: podNoLocal,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{utils.PVCName(pvcNoLocal): pvcNoLocal},
			},
			expectScore: &scoreResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Success),
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
					utils.NodeName4: framework.NewStatus(framework.Success),
				},
				nodeScores: map[string]int64{
					utils.NodeName1: int64(utils.MinScore),
					utils.NodeName2: int64(utils.MinScore),
					utils.NodeName3: int64(utils.MinScore),
					utils.NodeName4: int64(utils.MaxScore),
				},
				stateData: &stateData{
					podVolumeInfo: &PodLocalVolumeInfo{
						lvmPVCsWithVgNameNotAllocated:    []*LVMPVCInfo{},
						lvmPVCsSnapshot:                  []*LVMPVCInfo{},
						lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{},
						ssdDevicePVCsNotAllocated:        []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated:        []*DevicePVCInfo{},
						inlineVolumes:                    []*cache.InlineVolumeAllocated{},
					},
					allocateStateByNode: map[string]*cache.NodeAllocateState{},
				},
			},
		},
		{
			name: "test pod with pvc lvm without vg",
			args: args{
				pod: podWithLVMPVC,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithoutVG): pvcWithoutVG,
				},
			},
			expectScore: &scoreResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Success),
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
				},
				nodeScores: map[string]int64{
					utils.NodeName1: int64(10),
					utils.NodeName2: int64(5),
					utils.NodeName3: int64(math.Floor(float64(100) / float64(300) * float64(utils.MaxScore))),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName1: {
							NodeName: utils.NodeName1,
							PodUid:   string(podWithLVMPVC.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGSSD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVG.Name,
											PVCNamespace: pvcWithoutVG.Namespace,
											NodeName:     utils.NodeName1,
											Requested:    utils.GetPVCRequested(pvcWithoutVG),
											Allocated:    utils.GetPVCRequested(pvcWithoutVG),
										},
									},
								},
								DevicePVCAllocateUnits:    []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(100 * utils.LocalGi),
										Allocatable: int64(100 * utils.LocalGi),
										Requested:   utils.GetPVCRequested(pvcWithoutVG),
									},
									utils.VGHDD: {
										Name:        utils.VGHDD,
										Total:       int64(500 * utils.LocalGi),
										Allocatable: int64(500 * utils.LocalGi),
										Requested:   0,
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{},
								InitedByNLS:  true,
							},
						},
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podWithLVMPVC.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGSSD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVG.Name,
											PVCNamespace: pvcWithoutVG.Namespace,
											NodeName:     utils.NodeName2,
											Requested:    utils.GetPVCRequested(pvcWithoutVG),
											Allocated:    utils.GetPVCRequested(pvcWithoutVG),
										},
									},
								},
								DevicePVCAllocateUnits:    []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(200 * utils.LocalGi),
										Allocatable: int64(200 * utils.LocalGi),
										Requested:   utils.GetPVCRequested(pvcWithoutVG),
									},
									utils.VGHDD: {
										Name:        utils.VGHDD,
										Total:       int64(750 * utils.LocalGi),
										Allocatable: int64(750 * utils.LocalGi),
										Requested:   0,
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{},
								InitedByNLS:  true,
							},
						},
						utils.NodeName3: {
							NodeName: utils.NodeName3,
							PodUid:   string(podWithLVMPVC.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGSSD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVG.Name,
											PVCNamespace: pvcWithoutVG.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvcWithoutVG),
											Allocated:    utils.GetPVCRequested(pvcWithoutVG),
										},
									},
								},
								DevicePVCAllocateUnits:    []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(300 * utils.LocalGi),
										Allocatable: int64(300 * utils.LocalGi),
										Requested:   utils.GetPVCRequested(pvcWithoutVG),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{
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
			},
		},
		{
			name: "test pod with inlineVolume",
			args: args{
				pod: podWithInlineVolume,
			},
			fields: fields{},
			expectScore: &scoreResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
				},
				nodeScores: map[string]int64{
					utils.NodeName2: int64(math.Floor(float64(150) / float64(200) * float64(utils.MaxScore))),
					utils.NodeName3: int64(5),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podWithInlineVolume.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits:    []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{
									{
										VgName:       utils.VGSSD,
										PodNamespace: podWithInlineVolume.Namespace,
										PodName:      podWithInlineVolume.Name,
										VolumeName:   "test_inline_volume",
										VolumeSize:   getSize("150Gi"),
										Allocated:    getSize("150Gi"),
									},
								},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(200 * utils.LocalGi),
										Allocatable: int64(200 * utils.LocalGi),
										Requested:   getSize("150Gi"),
									},
									utils.VGHDD: {
										Name:        utils.VGHDD,
										Total:       int64(750 * utils.LocalGi),
										Allocatable: int64(750 * utils.LocalGi),
										Requested:   0,
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{},
								InitedByNLS:  true,
							},
						},
						utils.NodeName3: {
							NodeName: utils.NodeName3,
							PodUid:   string(podWithInlineVolume.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits:    []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{
									{
										VgName:       utils.VGSSD,
										PodNamespace: podWithInlineVolume.Namespace,
										PodName:      podWithInlineVolume.Name,
										VolumeName:   "test_inline_volume",
										VolumeSize:   getSize("150Gi"),
										Allocated:    getSize("150Gi"),
									},
								},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(300 * utils.LocalGi),
										Allocatable: int64(300 * utils.LocalGi),
										Requested:   getSize("150Gi"),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{
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
			},
		},
		{
			name: "test pod with both inlineVolume and pvc without vg",
			args: args{
				pod: podBoth,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithoutVG): pvcWithoutVG,
				},
			},
			expectScore: &scoreResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
				},
				nodeScores: map[string]int64{
					utils.NodeName2: int64(math.Floor((float64(100)/float64(750) + 0.75) / float64(2) * float64(utils.MaxScore))),
					utils.NodeName3: int64(math.Floor(float64(250) / float64(300) * float64(utils.MaxScore))),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podBoth.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGHDD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVG.Name,
											PVCNamespace: pvcWithoutVG.Namespace,
											NodeName:     utils.NodeName2,
											Requested:    utils.GetPVCRequested(pvcWithoutVG),
											Allocated:    utils.GetPVCRequested(pvcWithoutVG),
										},
									},
								},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{
									{
										VgName:       utils.VGSSD,
										PodNamespace: podBoth.Namespace,
										PodName:      podBoth.Name,
										VolumeName:   "test_inline_volume",
										VolumeSize:   getSize("150Gi"),
										Allocated:    getSize("150Gi"),
									},
								},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(200 * utils.LocalGi),
										Allocatable: int64(200 * utils.LocalGi),
										Requested:   getSize("150Gi"),
									},
									utils.VGHDD: {
										Name:        utils.VGHDD,
										Total:       int64(750 * utils.LocalGi),
										Allocatable: int64(750 * utils.LocalGi),
										Requested:   utils.GetPVCRequested(pvcWithoutVG),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{},
								InitedByNLS:  true,
							},
						},
						utils.NodeName3: {
							NodeName: utils.NodeName3,
							PodUid:   string(podBoth.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGSSD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVG.Name,
											PVCNamespace: pvcWithoutVG.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvcWithoutVG),
											Allocated:    utils.GetPVCRequested(pvcWithoutVG),
										},
									},
								},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{
									{
										VgName:       utils.VGSSD,
										PodNamespace: podBoth.Namespace,
										PodName:      podBoth.Name,
										VolumeName:   "test_inline_volume",
										VolumeSize:   getSize("150Gi"),
										Allocated:    getSize("150Gi"),
									},
								},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(300 * utils.LocalGi),
										Allocatable: int64(300 * utils.LocalGi),
										Requested:   getSize("250Gi"),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			nodeInfos := prepare(plugin)
			for _, pvc := range tt.fields.pvcs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			cycleState := framework.NewCycleState()
			plugin.PreFilter(context.Background(), cycleState, tt.args.pod)

			var filterSuccessNodes []*framework.NodeInfo
			for _, node := range nodeInfos {
				gotStatus := plugin.Filter(context.Background(), cycleState, tt.args.pod, node)
				if gotStatus.IsSuccess() {
					filterSuccessNodes = append(filterSuccessNodes, node)
				}
			}

			assert.Equal(t, len(filterSuccessNodes), len(tt.expectScore.nodeStatuses), "check filter success num")

			for _, node := range filterSuccessNodes {
				gotScore, gotStatus := plugin.Score(context.Background(), cycleState, tt.args.pod, node.Node().Name)
				assert.Equal(t, tt.expectScore.nodeStatuses[node.Node().Name].Code(), gotStatus.Code())
				assert.Equal(t, tt.expectScore.nodeScores[node.Node().Name], gotScore)
			}

			gotDataState, err := plugin.getState(cycleState)
			if tt.expectScore.stateData != nil {
				assert.Equal(t, tt.expectScore.stateData.allocateStateByNode, gotDataState.allocateStateByNode)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func Test_score_volumeGroup_spread(t *testing.T) {

	pvcInfo := utils.GetTestPVCPVWithoutVG().PVCPending
	pvcInfo.Size = "100Gi"

	podWithLVMPVC := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podWithLVMPVC",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos: []*utils.TestPVCInfo{
			pvcInfo,
		},
	})

	podWithInlineVolume := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podWithInlineVolume",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		InlineVolumeInfos: []*utils.TestInlineVolumeInfo{
			{
				VolumeName: "test_inline_volume",
				VolumeSize: "150Gi",
				VgName:     utils.VGSSD,
			},
		},
	})

	podBoth := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podBoth",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos: []*utils.TestPVCInfo{
			pvcInfo,
		},
		InlineVolumeInfos: []*utils.TestInlineVolumeInfo{
			{
				VolumeName: "test_inline_volume",
				VolumeSize: "150Gi",
				VgName:     utils.VGSSD,
			},
		},
	})

	//prepare pvc
	pvcWithoutVG := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcInfo})[0]

	type args struct {
		pod *corev1.Pod
	}
	type fields struct {
		pvcs map[string]*corev1.PersistentVolumeClaim
	}

	tests := []struct {
		name        string
		args        args
		fields      fields
		expectScore *scoreResult
	}{
		{
			name: "test pod with pvc lvm without vg",
			args: args{
				pod: podWithLVMPVC,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithoutVG): pvcWithoutVG,
				},
			},
			expectScore: &scoreResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Success),
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
				},
				nodeScores: map[string]int64{
					utils.NodeName1: int64(8),
					utils.NodeName2: int64(math.Floor((1.0 - float64(100)/float64(750)) * float64(utils.MaxScore))),
					utils.NodeName3: int64(math.Floor((1.0 - float64(100)/float64(300)) * float64(utils.MaxScore))),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName1: {
							NodeName: utils.NodeName1,
							PodUid:   string(podWithLVMPVC.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGHDD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVG.Name,
											PVCNamespace: pvcWithoutVG.Namespace,
											NodeName:     utils.NodeName1,
											Requested:    utils.GetPVCRequested(pvcWithoutVG),
											Allocated:    utils.GetPVCRequested(pvcWithoutVG),
										},
									},
								},
								DevicePVCAllocateUnits:    []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(100 * utils.LocalGi),
										Allocatable: int64(100 * utils.LocalGi),
										Requested:   0,
									},
									utils.VGHDD: {
										Name:        utils.VGHDD,
										Total:       int64(500 * utils.LocalGi),
										Allocatable: int64(500 * utils.LocalGi),
										Requested:   utils.GetPVCRequested(pvcWithoutVG),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{},
								InitedByNLS:  true,
							},
						},
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podWithLVMPVC.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGHDD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVG.Name,
											PVCNamespace: pvcWithoutVG.Namespace,
											NodeName:     utils.NodeName2,
											Requested:    utils.GetPVCRequested(pvcWithoutVG),
											Allocated:    utils.GetPVCRequested(pvcWithoutVG),
										},
									},
								},
								DevicePVCAllocateUnits:    []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(200 * utils.LocalGi),
										Allocatable: int64(200 * utils.LocalGi),
										Requested:   0,
									},
									utils.VGHDD: {
										Name:        utils.VGHDD,
										Total:       int64(750 * utils.LocalGi),
										Allocatable: int64(750 * utils.LocalGi),
										Requested:   utils.GetPVCRequested(pvcWithoutVG),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{},
								InitedByNLS:  true,
							},
						},
						utils.NodeName3: {
							NodeName: utils.NodeName3,
							PodUid:   string(podWithLVMPVC.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGSSD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVG.Name,
											PVCNamespace: pvcWithoutVG.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvcWithoutVG),
											Allocated:    utils.GetPVCRequested(pvcWithoutVG),
										},
									},
								},
								DevicePVCAllocateUnits:    []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(300 * utils.LocalGi),
										Allocatable: int64(300 * utils.LocalGi),
										Requested:   utils.GetPVCRequested(pvcWithoutVG),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{
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
			},
		},
		{
			name: "test pod with inlineVolume",
			args: args{
				pod: podWithInlineVolume,
			},
			fields: fields{},
			expectScore: &scoreResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
				},
				nodeScores: map[string]int64{
					utils.NodeName2: int64(math.Floor((1.0 - float64(150)/float64(200)) * float64(utils.MaxScore))),
					utils.NodeName3: int64(5),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podWithInlineVolume.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits:    []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{
									{
										VgName:       utils.VGSSD,
										PodNamespace: podWithInlineVolume.Namespace,
										PodName:      podWithInlineVolume.Name,
										VolumeName:   "test_inline_volume",
										VolumeSize:   getSize("150Gi"),
										Allocated:    getSize("150Gi"),
									},
								},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(200 * utils.LocalGi),
										Allocatable: int64(200 * utils.LocalGi),
										Requested:   getSize("150Gi"),
									},
									utils.VGHDD: {
										Name:        utils.VGHDD,
										Total:       int64(750 * utils.LocalGi),
										Allocatable: int64(750 * utils.LocalGi),
										Requested:   0,
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{},
								InitedByNLS:  true,
							},
						},
						utils.NodeName3: {
							NodeName: utils.NodeName3,
							PodUid:   string(podWithInlineVolume.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits:    []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{
									{
										VgName:       utils.VGSSD,
										PodNamespace: podWithInlineVolume.Namespace,
										PodName:      podWithInlineVolume.Name,
										VolumeName:   "test_inline_volume",
										VolumeSize:   getSize("150Gi"),
										Allocated:    getSize("150Gi"),
									},
								},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(300 * utils.LocalGi),
										Allocatable: int64(300 * utils.LocalGi),
										Requested:   getSize("150Gi"),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{
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
			},
		},
		{
			name: "test pod with both inlineVolume and pvc without vg",
			args: args{
				pod: podBoth,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithoutVG): pvcWithoutVG,
				},
			},
			expectScore: &scoreResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
				},
				nodeScores: map[string]int64{
					utils.NodeName2: int64(math.Floor((1.0 - (float64(100) / float64(750)) + (1.0 - 0.75)) / float64(2) * float64(utils.MaxScore))),
					utils.NodeName3: int64(math.Floor((1.0 - float64(250)/float64(300)) * float64(utils.MaxScore))),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podBoth.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGHDD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVG.Name,
											PVCNamespace: pvcWithoutVG.Namespace,
											NodeName:     utils.NodeName2,
											Requested:    utils.GetPVCRequested(pvcWithoutVG),
											Allocated:    utils.GetPVCRequested(pvcWithoutVG),
										},
									},
								},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{
									{
										VgName:       utils.VGSSD,
										PodNamespace: podBoth.Namespace,
										PodName:      podBoth.Name,
										VolumeName:   "test_inline_volume",
										VolumeSize:   getSize("150Gi"),
										Allocated:    getSize("150Gi"),
									},
								},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(200 * utils.LocalGi),
										Allocatable: int64(200 * utils.LocalGi),
										Requested:   getSize("150Gi"),
									},
									utils.VGHDD: {
										Name:        utils.VGHDD,
										Total:       int64(750 * utils.LocalGi),
										Allocatable: int64(750 * utils.LocalGi),
										Requested:   utils.GetPVCRequested(pvcWithoutVG),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{},
								InitedByNLS:  true,
							},
						},
						utils.NodeName3: {
							NodeName: utils.NodeName3,
							PodUid:   string(podBoth.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGSSD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVG.Name,
											PVCNamespace: pvcWithoutVG.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvcWithoutVG),
											Allocated:    utils.GetPVCRequested(pvcWithoutVG),
										},
									},
								},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{
									{
										VgName:       utils.VGSSD,
										PodNamespace: podBoth.Namespace,
										PodName:      podBoth.Name,
										VolumeName:   "test_inline_volume",
										VolumeSize:   getSize("150Gi"),
										Allocated:    getSize("150Gi"),
									},
								},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(300 * utils.LocalGi),
										Allocatable: int64(300 * utils.LocalGi),
										Requested:   getSize("250Gi"),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			plugin.allocateStrategy = NewSpreadStrategy()
			nodeInfos := prepare(plugin)
			for _, pvc := range tt.fields.pvcs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			cycleState := framework.NewCycleState()
			plugin.PreFilter(context.Background(), cycleState, tt.args.pod)

			var filterSuccessNodes []*framework.NodeInfo
			for _, node := range nodeInfos {
				gotStatus := plugin.Filter(context.Background(), cycleState, tt.args.pod, node)
				if gotStatus.IsSuccess() {
					filterSuccessNodes = append(filterSuccessNodes, node)
				}
			}

			assert.Equal(t, len(filterSuccessNodes), len(tt.expectScore.nodeStatuses), "check filter success num")

			for _, node := range filterSuccessNodes {
				gotScore, gotStatus := plugin.Score(context.Background(), cycleState, tt.args.pod, node.Node().Name)
				assert.Equal(t, tt.expectScore.nodeStatuses[node.Node().Name].Code(), gotStatus.Code(), node.Node().Name)
				assert.Equal(t, tt.expectScore.nodeScores[node.Node().Name], gotScore, node.Node().Name)
			}

			gotDataState, err := plugin.getState(cycleState)
			if tt.expectScore.stateData != nil {
				assert.Equal(t, tt.expectScore.stateData.allocateStateByNode, gotDataState.allocateStateByNode)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func Test_score_device(t *testing.T) {

	pvc100gInfo := utils.GetTestPVCPVDevice().PVCPending
	pvc100gInfo.PVCName = "pvc100g"
	pvc200gInfo := utils.GetTestPVCPVDevice().PVCPending
	pvc200gInfo.PVCName = "pvc200g"
	pvc200gInfo.Size = "200Gi"

	podDevice := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podDevice",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos: []*utils.TestPVCInfo{
			pvc100gInfo,
			pvc200gInfo,
		},
	})

	//prepare pvc
	pvc100g := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvc100gInfo})[0]
	pvc200g := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvc200gInfo})[0]

	type args struct {
		pod *corev1.Pod
	}
	type fields struct {
		pvcs     map[string]*corev1.PersistentVolumeClaim
		strategy AllocateStrategy
	}

	tests := []struct {
		name        string
		args        args
		fields      fields
		expectScore *scoreResult
	}{
		{
			name: "test score pod device binPack",
			args: args{
				pod: podDevice,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvc100g): pvc100g,
					utils.PVCName(pvc200g): pvc200g,
				},
				strategy: NewBinPackStrategy(),
			},
			expectScore: &scoreResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
				},
				nodeScores: map[string]int64{
					utils.NodeName2: int64(math.Floor((1.0+float64(200)/300)/2*float64(utils.MaxScore)) /*capacity*/) +
						int64(math.Floor(float64(2)/3*float64(utils.MaxScore))), /*count*/
					utils.NodeName3: int64(math.Floor((1.0+float64(200)/200)/2*float64(utils.MaxScore)) /*capacity*/) +
						int64(math.Floor(float64(2)/3*float64(utils.MaxScore))), /*count*/
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podDevice.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{
									{
										DeviceName: "/dev/sda",
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvc100g.Name,
											PVCNamespace: pvc100g.Namespace,
											NodeName:     utils.NodeName2,
											Requested:    utils.GetPVCRequested(pvc100g),
											Allocated:    int64(100 * utils.LocalGi),
										},
									},
									{
										DeviceName: "/dev/sdb",
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvc200g.Name,
											PVCNamespace: pvc200g.Namespace,
											NodeName:     utils.NodeName2,
											Requested:    utils.GetPVCRequested(pvc200g),
											Allocated:    int64(300 * utils.LocalGi),
										},
									},
								},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(200 * utils.LocalGi),
										Allocatable: int64(200 * utils.LocalGi),
										Requested:   0,
									},
									utils.VGHDD: {
										Name:        utils.VGHDD,
										Total:       int64(750 * utils.LocalGi),
										Allocatable: int64(750 * utils.LocalGi),
										Requested:   0,
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{
									"/dev/sda": {
										Name:        "/dev/sda",
										Total:       int64(100 * utils.LocalGi),
										Allocatable: int64(100 * utils.LocalGi),
										Requested:   int64(100 * utils.LocalGi),
										MediaType:   localtype.MediaTypeHDD,
										IsAllocated: true,
									},
									"/dev/sdb": {
										Name:        "/dev/sdb",
										Total:       int64(300 * utils.LocalGi),
										Allocatable: int64(300 * utils.LocalGi),
										Requested:   int64(300 * utils.LocalGi),
										MediaType:   localtype.MediaTypeHDD,
										IsAllocated: true,
									},
									"/dev/sdc": {
										Name:        "/dev/sdc",
										Total:       int64(400 * utils.LocalGi),
										Allocatable: int64(400 * utils.LocalGi),
										Requested:   0,
										MediaType:   localtype.MediaTypeHDD,
										IsAllocated: false,
									},
								},
								InitedByNLS: true,
							},
						},
						utils.NodeName3: {
							NodeName: utils.NodeName3,
							PodUid:   string(podDevice.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{
									{
										DeviceName: "/dev/sda",
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvc100g.Name,
											PVCNamespace: pvc100g.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvc100g),
											Allocated:    int64(100 * utils.LocalGi),
										},
									},
									{
										DeviceName: "/dev/sdb",
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvc200g.Name,
											PVCNamespace: pvc200g.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvc200g),
											Allocated:    int64(200 * utils.LocalGi),
										},
									},
								},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(300 * utils.LocalGi),
										Allocatable: int64(300 * utils.LocalGi),
										Requested:   0,
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{
									"/dev/sda": {
										Name:        "/dev/sda",
										Total:       int64(100 * utils.LocalGi),
										Allocatable: int64(100 * utils.LocalGi),
										Requested:   int64(100 * utils.LocalGi),
										MediaType:   localtype.MediaTypeHDD,
										IsAllocated: true,
									},
									"/dev/sdb": {
										Name:        "/dev/sdb",
										Total:       int64(200 * utils.LocalGi),
										Allocatable: int64(200 * utils.LocalGi),
										Requested:   int64(200 * utils.LocalGi),
										MediaType:   localtype.MediaTypeHDD,
										IsAllocated: true,
									},
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
			},
		},
		{
			name: "test score pod device spread",
			args: args{
				pod: podDevice,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvc100g): pvc100g,
					utils.PVCName(pvc200g): pvc200g,
				},
				strategy: NewSpreadStrategy(),
			},
			expectScore: &scoreResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
				},
				nodeScores: map[string]int64{
					utils.NodeName2: int64(math.Floor((1.0+float64(200)/300)/2*float64(utils.MaxScore)) /*capacity*/) +
						int64(math.Floor((1.0-float64(2)/3)*float64(utils.MaxScore))), /*count*/
					utils.NodeName3: int64(math.Floor((1.0+float64(200)/200)/2*float64(utils.MaxScore)) /*capacity*/) +
						int64(math.Floor((1.0-float64(2)/3)*float64(utils.MaxScore))), /*count*/
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podDevice.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{
									{
										DeviceName: "/dev/sda",
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvc100g.Name,
											PVCNamespace: pvc100g.Namespace,
											NodeName:     utils.NodeName2,
											Requested:    utils.GetPVCRequested(pvc100g),
											Allocated:    int64(100 * utils.LocalGi),
										},
									},
									{
										DeviceName: "/dev/sdb",
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvc200g.Name,
											PVCNamespace: pvc200g.Namespace,
											NodeName:     utils.NodeName2,
											Requested:    utils.GetPVCRequested(pvc200g),
											Allocated:    int64(300 * utils.LocalGi),
										},
									},
								},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(200 * utils.LocalGi),
										Allocatable: int64(200 * utils.LocalGi),
										Requested:   0,
									},
									utils.VGHDD: {
										Name:        utils.VGHDD,
										Total:       int64(750 * utils.LocalGi),
										Allocatable: int64(750 * utils.LocalGi),
										Requested:   0,
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{
									"/dev/sda": {
										Name:        "/dev/sda",
										Total:       int64(100 * utils.LocalGi),
										Allocatable: int64(100 * utils.LocalGi),
										Requested:   int64(100 * utils.LocalGi),
										MediaType:   localtype.MediaTypeHDD,
										IsAllocated: true,
									},
									"/dev/sdb": {
										Name:        "/dev/sdb",
										Total:       int64(300 * utils.LocalGi),
										Allocatable: int64(300 * utils.LocalGi),
										Requested:   int64(300 * utils.LocalGi),
										MediaType:   localtype.MediaTypeHDD,
										IsAllocated: true,
									},
									"/dev/sdc": {
										Name:        "/dev/sdc",
										Total:       int64(400 * utils.LocalGi),
										Allocatable: int64(400 * utils.LocalGi),
										Requested:   0,
										MediaType:   localtype.MediaTypeHDD,
										IsAllocated: false,
									},
								},
								InitedByNLS: true,
							},
						},
						utils.NodeName3: {
							NodeName: utils.NodeName3,
							PodUid:   string(podDevice.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{
									{
										DeviceName: "/dev/sda",
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvc100g.Name,
											PVCNamespace: pvc100g.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvc100g),
											Allocated:    int64(100 * utils.LocalGi),
										},
									},
									{
										DeviceName: "/dev/sdb",
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvc200g.Name,
											PVCNamespace: pvc200g.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvc200g),
											Allocated:    int64(200 * utils.LocalGi),
										},
									},
								},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
							},
							NodeStorageAllocatedByUnits: &cache.NodeStorageState{
								VGStates: map[string]*cache.VGStoragePool{
									utils.VGSSD: {
										Name:        utils.VGSSD,
										Total:       int64(300 * utils.LocalGi),
										Allocatable: int64(300 * utils.LocalGi),
										Requested:   0,
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{
									"/dev/sda": {
										Name:        "/dev/sda",
										Total:       int64(100 * utils.LocalGi),
										Allocatable: int64(100 * utils.LocalGi),
										Requested:   int64(100 * utils.LocalGi),
										MediaType:   localtype.MediaTypeHDD,
										IsAllocated: true,
									},
									"/dev/sdb": {
										Name:        "/dev/sdb",
										Total:       int64(200 * utils.LocalGi),
										Allocatable: int64(200 * utils.LocalGi),
										Requested:   int64(200 * utils.LocalGi),
										MediaType:   localtype.MediaTypeHDD,
										IsAllocated: true,
									},
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			nodeInfos := prepare(plugin)
			plugin.allocateStrategy = tt.fields.strategy

			for _, pvc := range tt.fields.pvcs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}
			//need more device
			for _, node := range nodeInfos {
				if node.Node().Name != utils.NodeName4 {
					// update nls device white list
					nls, _ := plugin.localInformers.NodeLocalStorages().Lister().Get(node.Node().Name)
					nlsNew := nls.DeepCopy()
					nlsNew.Spec.ListConfig.Devices.Include = []string{"/dev/sda", "/dev/sdb", "/dev/sdc"}
					nlsNew.Status.FilteredStorageInfo.Devices = []string{"/dev/sda", "/dev/sdb", "/dev/sdc"}
					_, _ = plugin.localClientSet.CsiV1alpha1().NodeLocalStorages().Update(context.Background(), nlsNew, metav1.UpdateOptions{})
					_ = plugin.localInformers.NodeLocalStorages().Informer().GetIndexer().Update(nlsNew)
					//update nls
					plugin.OnNodeLocalStorageUpdate(nls, nlsNew)
				}
			}

			cycleState := framework.NewCycleState()
			plugin.PreFilter(context.Background(), cycleState, tt.args.pod)

			var filterSuccessNodes []*framework.NodeInfo
			for _, node := range nodeInfos {
				gotStatus := plugin.Filter(context.Background(), cycleState, tt.args.pod, node)
				if gotStatus.IsSuccess() {
					filterSuccessNodes = append(filterSuccessNodes, node)
				}
			}

			assert.Equal(t, len(filterSuccessNodes), len(tt.expectScore.nodeStatuses), "check filter success num")

			for _, node := range filterSuccessNodes {
				gotScore, gotStatus := plugin.Score(context.Background(), cycleState, tt.args.pod, node.Node().Name)
				assert.Equal(t, tt.expectScore.nodeStatuses[node.Node().Name].Code(), gotStatus.Code())
				assert.Equal(t, tt.expectScore.nodeScores[node.Node().Name], gotScore)
			}

			gotDataState, err := plugin.getState(cycleState)
			if tt.expectScore.stateData != nil {
				assert.Equal(t, tt.expectScore.stateData.allocateStateByNode, gotDataState.allocateStateByNode)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
