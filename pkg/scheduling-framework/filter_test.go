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
	"testing"

	"github.com/stretchr/testify/assert"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduling-framework/cache"
	"github.com/alibaba/open-local/pkg/utils"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

type filterResult struct {
	nodeStatuses map[string]*framework.Status
	stateData    *stateData
}

func Test_PreFilter(t *testing.T) {

	//simplePod: return success
	simplePod := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "simple-pod",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending})

	//common pvc pod: return success
	commonPVCPod := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "simple-pod",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos:     []*utils.TestPVCInfo{utils.GetTestPVCPVNotLocal().PVCPending},
	})

	//complex pod
	complexPod, complexPVCPVInfos := createPodComplex()

	//pvc,pv pending
	pvcsPending := createPVC(complexPVCPVInfos.GetTestPVCPending())
	pvcsBounding := createPVC(complexPVCPVInfos.GetTestPVCBounding())
	pvsBounding := createPV(complexPVCPVInfos.GetTestPVBounding())

	type args struct {
		pod *corev1.Pod
	}
	type fields struct {
		pvcs map[string]*corev1.PersistentVolumeClaim
		pvs  map[string]*corev1.PersistentVolume
	}

	tests := []struct {
		name            string
		args            args
		fields          fields
		expectPreFilter *scheduleResult
	}{
		{
			name: "test no pvc defined in pod, expect success",
			args: args{
				pod: simplePod,
			},
			fields: fields{},
			expectPreFilter: &scheduleResult{
				status: framework.NewStatus(framework.Success),
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
			name: "test noLocal pvc defined in pod, expect success",
			args: args{
				pod: commonPVCPod,
			},
			fields: fields{
				pvcs: pvcsPending,
			},
			expectPreFilter: &scheduleResult{
				status: framework.NewStatus(framework.Success),
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
			name: "test complex pod with pvc pending, expect success",
			args: args{
				pod: complexPod,
			},
			fields: fields{
				pvcs: pvcsPending,
			},
			expectPreFilter: &scheduleResult{
				status: framework.NewStatus(framework.Success),
				stateData: &stateData{
					podVolumeInfo: &PodLocalVolumeInfo{
						lvmPVCsWithVgNameNotAllocated: []*LVMPVCInfo{
							{
								vgName:  utils.GetTestPVCPVWithVG().PVBounding.VgName,
								request: getSize(utils.GetTestPVCPVWithVG().PVCPending.Size),
								pvc:     pvcsPending[utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithVG)],
							},
						},
						lvmPVCsSnapshot: []*LVMPVCInfo{
							{
								request: getSize(utils.GetTestPVCPVSnapshot().PVCPending.Size),
								pvc:     pvcsPending[utils.GetPVCKey(utils.LocalNameSpace, utils.PVCSnapshot)],
							},
						},
						lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{
							{
								request: getSize(utils.GetTestPVCPVWithoutVG().PVCPending.Size),
								pvc:     pvcsPending[utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithoutVG)],
							},
						},
						ssdDevicePVCsNotAllocated: []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated: []*DevicePVCInfo{
							{
								mediaType: localtype.MediaTypeHDD,
								request:   getSize(utils.GetTestPVCPVDevice().PVCPending.Size),
								pvc:       pvcsPending[utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithDevice)],
							},
						},
						inlineVolumes: []*cache.InlineVolumeAllocated{
							{
								VolumeName:   "test_inline_volume",
								VolumeSize:   getSize("10Gi"),
								VgName:       utils.VGSSD,
								PodName:      complexPod.Name,
								PodNamespace: complexPod.Namespace,
							},
						},
					},
					allocateStateByNode: map[string]*cache.NodeAllocateState{},
				},
			},
		},
		{
			name: "test complex pod with pvc all bound, expect success",
			args: args{
				pod: complexPod,
			},
			fields: fields{
				pvcs: pvcsBounding,
				pvs:  pvsBounding,
			},
			expectPreFilter: &scheduleResult{
				status: framework.NewStatus(framework.Success),
				stateData: &stateData{
					podVolumeInfo: &PodLocalVolumeInfo{
						lvmPVCsWithVgNameNotAllocated:    []*LVMPVCInfo{},
						lvmPVCsSnapshot:                  []*LVMPVCInfo{},
						lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{},
						ssdDevicePVCsNotAllocated:        []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated:        []*DevicePVCInfo{},
						inlineVolumes: []*cache.InlineVolumeAllocated{
							{
								VolumeName:   "test_inline_volume",
								VolumeSize:   getSize("10Gi"),
								VgName:       utils.VGSSD,
								PodName:      complexPod.Name,
								PodNamespace: complexPod.Namespace,
							},
						},
					},
					allocateStateByNode: map[string]*cache.NodeAllocateState{},
				},
			},
		},
		{
			name: "test complex pod with pvc pending but pvc not found, expect error",
			args: args{
				pod: complexPod,
			},
			fields: fields{},
			expectPreFilter: &scheduleResult{
				status: framework.NewStatus(framework.UnschedulableAndUnresolvable),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			prepare(plugin)
			for _, pvc := range tt.fields.pvcs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			for _, pv := range tt.fields.pvs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumes().Informer().GetIndexer().Add(pv)
			}

			cycleState := framework.NewCycleState()

			gotStatus := plugin.PreFilter(context.Background(), cycleState, tt.args.pod)

			gotDataState, err := plugin.getState(cycleState)
			if tt.expectPreFilter.stateData != nil {
				assert.Equal(t, tt.expectPreFilter.stateData.podVolumeInfo, gotDataState.podVolumeInfo)
				assert.Equal(t, tt.expectPreFilter.stateData.allocateStateByNode, gotDataState.allocateStateByNode)
			} else {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.expectPreFilter.status.Code(), gotStatus.Code())
		})
	}
}

func Test_Filter_PodHaveNoLocalPVC(t *testing.T) {
	//simplePod: all node ok
	simplePod := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "simple-pod",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending})

	//common pvc pod: all node ok
	commonPVCPod := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "simple-pod",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos:     []*utils.TestPVCInfo{utils.GetTestPVCPVNotLocal().PVCPending},
	})

	//prepare pvc
	_, complexPVCPVInfos := createPodComplex()
	pvcsPending := createPVC(complexPVCPVInfos.GetTestPVCPending())

	type args struct {
		pod *corev1.Pod
	}
	type fields struct {
		pvcs map[string]*corev1.PersistentVolumeClaim
		pvs  map[string]*corev1.PersistentVolume
	}

	tests := []struct {
		name         string
		args         args
		fields       fields
		expectFilter *filterResult
	}{
		{
			name: "test no pvc defined in pod, expect success",
			args: args{
				pod: simplePod,
			},
			fields: fields{},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Success),
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
					utils.NodeName4: framework.NewStatus(framework.Success),
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
			name: "test noLocal pvc defined in pod, expect success",
			args: args{
				pod: commonPVCPod,
			},
			fields: fields{
				pvcs: pvcsPending,
			},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Success),
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
					utils.NodeName4: framework.NewStatus(framework.Success),
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			nodeInfos := prepare(plugin)
			for _, pvc := range tt.fields.pvcs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			for _, pv := range tt.fields.pvs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumes().Informer().GetIndexer().Add(pv)
			}

			cycleState := framework.NewCycleState()
			plugin.PreFilter(context.Background(), cycleState, tt.args.pod)

			for _, node := range nodeInfos {
				gotStatus := plugin.Filter(context.Background(), cycleState, tt.args.pod, node)
				assert.Equal(t, tt.expectFilter.nodeStatuses[node.Node().Name].Code(), gotStatus.Code())
			}

			gotDataState, err := plugin.getState(cycleState)
			if tt.expectFilter.stateData != nil {
				assert.Equal(t, tt.expectFilter.stateData.podVolumeInfo, gotDataState.podVolumeInfo)
				assert.Equal(t, tt.expectFilter.stateData.allocateStateByNode, gotDataState.allocateStateByNode)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func Test_Filter_LVMPVC_NotSnapshot(t *testing.T) {
	podWithVG := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podWithVG",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos: []*utils.TestPVCInfo{
			utils.GetTestPVCPVWithVG().PVCPending,
		},
	})

	podWithoutVG := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podWithoutVG",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos: []*utils.TestPVCInfo{
			utils.GetTestPVCPVWithoutVG().PVCPending,
		},
	})

	//normal pvc/pv
	pvcWithVGPending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithVG().PVCPending})[0]
	pvcWithoutVG := utils.GetTestPVCPVWithoutVG()
	pvcWithoutVG.PVCPending.Size = "160Gi"
	pvcWithoutVGPending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcWithoutVG.PVCPending})[0]

	//large pvc/pv
	pvcWithVGLarge := utils.GetTestPVCPVWithVG()
	pvcWithVGLarge.PVCPending.Size = "1000Gi"
	pvcWithVGLargePending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcWithVGLarge.PVCPending})[0]
	pvcWithoutVGLarge := utils.GetTestPVCPVWithoutVG()
	pvcWithoutVGLarge.PVCPending.Size = "1000Gi"
	pvcWithOUTVGLargePending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcWithoutVGLarge.PVCPending})[0]

	type args struct {
		pod *corev1.Pod
	}
	type fields struct {
		pvcs map[string]*corev1.PersistentVolumeClaim
	}

	tests := []struct {
		name         string
		args         args
		fields       fields
		expectFilter *filterResult
	}{
		{
			name: "test pod with pvc use sc have vg",
			args: args{
				pod: podWithVG,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithVGPending): pvcWithVGPending,
				},
			},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Unschedulable),
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
					utils.NodeName4: framework.NewStatus(framework.Unschedulable),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podWithVG.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGSSD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithVGPending.Name,
											PVCNamespace: pvcWithVGPending.Namespace,
											NodeName:     utils.NodeName2,
											Requested:    utils.GetPVCRequested(pvcWithVGPending),
											Allocated:    utils.GetPVCRequested(pvcWithVGPending),
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
										Requested:   utils.GetPVCRequested(pvcWithVGPending),
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
							PodUid:   string(podWithVG.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGSSD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithVGPending.Name,
											PVCNamespace: pvcWithVGPending.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvcWithVGPending),
											Allocated:    utils.GetPVCRequested(pvcWithVGPending),
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
										Requested:   utils.GetPVCRequested(pvcWithVGPending),
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
			name: "test pod with pvc too large use sc have vg",
			args: args{
				pod: podWithVG,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithVGLargePending): pvcWithVGLargePending,
				},
			},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Unschedulable),
					utils.NodeName2: framework.NewStatus(framework.Unschedulable),
					utils.NodeName3: framework.NewStatus(framework.Unschedulable),
					utils.NodeName4: framework.NewStatus(framework.Unschedulable),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{},
				},
			},
		},
		{
			name: "test pod with pvc use sc without vg",
			args: args{
				pod: podWithoutVG,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithoutVGPending): pvcWithoutVGPending,
				},
			},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Success),
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
					utils.NodeName4: framework.NewStatus(framework.Unschedulable),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName1: {
							NodeName: utils.NodeName1,
							PodUid:   string(podWithoutVG.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGHDD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVGPending.Name,
											PVCNamespace: pvcWithoutVGPending.Namespace,
											NodeName:     utils.NodeName1,
											Requested:    utils.GetPVCRequested(pvcWithoutVGPending),
											Allocated:    utils.GetPVCRequested(pvcWithoutVGPending),
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
										Requested:   utils.GetPVCRequested(pvcWithoutVGPending),
									},
								},
								DeviceStates: map[string]*cache.DeviceResourcePool{},
								InitedByNLS:  true,
							},
						},
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podWithoutVG.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGSSD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVGPending.Name,
											PVCNamespace: pvcWithoutVGPending.Namespace,
											NodeName:     utils.NodeName2,
											Requested:    utils.GetPVCRequested(pvcWithoutVGPending),
											Allocated:    utils.GetPVCRequested(pvcWithoutVGPending),
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
										Requested:   utils.GetPVCRequested(pvcWithoutVGPending),
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
							PodUid:   string(podWithoutVG.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
									{
										VGName: utils.VGSSD,
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcWithoutVGPending.Name,
											PVCNamespace: pvcWithoutVGPending.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvcWithoutVGPending),
											Allocated:    utils.GetPVCRequested(pvcWithoutVGPending),
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
										Requested:   utils.GetPVCRequested(pvcWithoutVGPending),
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
			name: "test pod with pvc too large use sc without vg",
			args: args{
				pod: podWithoutVG,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithOUTVGLargePending): pvcWithOUTVGLargePending,
				},
			},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Unschedulable),
					utils.NodeName2: framework.NewStatus(framework.Unschedulable),
					utils.NodeName3: framework.NewStatus(framework.Unschedulable),
					utils.NodeName4: framework.NewStatus(framework.Unschedulable),
				},
				stateData: &stateData{allocateStateByNode: map[string]*cache.NodeAllocateState{}},
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

			for _, node := range nodeInfos {
				gotStatus := plugin.Filter(context.Background(), cycleState, tt.args.pod, node)
				assert.Equal(t, tt.expectFilter.nodeStatuses[node.Node().Name].Code(), gotStatus.Code())
			}

			gotDataState, err := plugin.getState(cycleState)
			if tt.expectFilter.stateData != nil {
				assert.Equal(t, tt.expectFilter.stateData.allocateStateByNode, gotDataState.allocateStateByNode)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func Test_Filter_LVMPVC_Snapshot(t *testing.T) {
	podWithSnapshot := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podWithVG",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos: []*utils.TestPVCInfo{
			utils.GetTestPVCPVSnapshot().PVCPending,
		},
	})

	snapshot := &volumesnapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.GetTestPVCPVSnapshot().PVCPending.SnapName,
			Namespace: utils.GetTestPVCPVSnapshot().PVCPending.PVCNameSpace,
		},
		Spec: volumesnapshotv1.VolumeSnapshotSpec{
			Source: volumesnapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: &utils.GetTestPVCPVSnapshot().PVCPending.SourcePVCName,
			},
		},
	}

	snapshotPVC := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVSnapshot().PVCPending})[0]
	sourcePVC := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithVG().PVCBounding})[0]

	type args struct {
		pod *corev1.Pod
	}
	type fields struct {
		snapshot    *volumesnapshotv1.VolumeSnapshot
		snapshotPVC *corev1.PersistentVolumeClaim
		sourcePVC   *corev1.PersistentVolumeClaim
	}

	tests := []struct {
		name         string
		args         args
		fields       fields
		expectFilter *filterResult
	}{
		{
			name: "test pod with snapshot pvc but snapshot not found",
			args: args{
				pod: podWithSnapshot,
			},
			fields: fields{
				snapshotPVC: snapshotPVC,
				sourcePVC:   sourcePVC,
			},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Error),
					utils.NodeName2: framework.NewStatus(framework.Error),
					utils.NodeName3: framework.NewStatus(framework.Error),
					utils.NodeName4: framework.NewStatus(framework.Error),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{},
				},
			},
		},
		{
			name: "test pod with snapshot pvc but source pvc not found",
			args: args{
				pod: podWithSnapshot,
			},
			fields: fields{
				snapshot:    snapshot,
				snapshotPVC: snapshotPVC,
			},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Error),
					utils.NodeName2: framework.NewStatus(framework.Error),
					utils.NodeName3: framework.NewStatus(framework.Error),
					utils.NodeName4: framework.NewStatus(framework.Error),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{},
				},
			},
		},
		{
			name: "test pod with snapshot pvc valid",
			args: args{
				pod: podWithSnapshot,
			},
			fields: fields{
				snapshot:    snapshot,
				snapshotPVC: snapshotPVC,
				sourcePVC:   sourcePVC,
			},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Unschedulable),
					utils.NodeName2: framework.NewStatus(framework.Unschedulable),
					utils.NodeName3: framework.NewStatus(framework.Success),
					utils.NodeName4: framework.NewStatus(framework.Unschedulable),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName3: {
							NodeName: utils.NodeName3,
							PodUid:   string(podWithSnapshot.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits:       []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits:    []*cache.DeviceTypePVAllocated{},
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

			if tt.fields.sourcePVC != nil {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(tt.fields.sourcePVC.Namespace).Create(context.Background(), tt.fields.sourcePVC, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(tt.fields.sourcePVC)
			}
			if tt.fields.snapshotPVC != nil {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(tt.fields.snapshotPVC.Namespace).Create(context.Background(), tt.fields.snapshotPVC, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(tt.fields.snapshotPVC)
			}
			if tt.fields.snapshot != nil {
				_, _ = plugin.snapClientSet.SnapshotV1().VolumeSnapshots(tt.fields.snapshot.Namespace).Create(context.Background(), tt.fields.snapshot, metav1.CreateOptions{})
				_ = plugin.snapshotInformers.VolumeSnapshots().Informer().GetIndexer().Add(tt.fields.snapshot)
			}

			cycleState := framework.NewCycleState()
			plugin.PreFilter(context.Background(), cycleState, tt.args.pod)

			for _, node := range nodeInfos {
				gotStatus := plugin.Filter(context.Background(), cycleState, tt.args.pod, node)
				assert.Equal(t, tt.expectFilter.nodeStatuses[node.Node().Name].Code(), gotStatus.Code())
			}

			gotDataState, err := plugin.getState(cycleState)
			if tt.expectFilter.stateData != nil {
				assert.Equal(t, tt.expectFilter.stateData.allocateStateByNode, gotDataState.allocateStateByNode)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func Test_Filter_inlineVolume(t *testing.T) {
	podInlineVolume := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podInlineVolume",
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

	podInlineVolumeLarge := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podInlineVolume",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		InlineVolumeInfos: []*utils.TestInlineVolumeInfo{
			{
				VolumeName: "test_inline_volume",
				VolumeSize: "1000Gi",
				VgName:     utils.VGSSD,
			},
		},
	})

	type args struct {
		pod *corev1.Pod
	}
	type fields struct {
	}

	tests := []struct {
		name         string
		args         args
		fields       fields
		expectFilter *filterResult
	}{
		{
			name: "test pod with inlineVolume",
			args: args{
				pod: podInlineVolume,
			},
			fields: fields{},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Unschedulable),
					utils.NodeName2: framework.NewStatus(framework.Success),
					utils.NodeName3: framework.NewStatus(framework.Success),
					utils.NodeName4: framework.NewStatus(framework.Unschedulable),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName2: {
							NodeName: utils.NodeName2,
							PodUid:   string(podInlineVolume.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits:    []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{
									{
										VgName:       utils.VGSSD,
										PodNamespace: podInlineVolume.Namespace,
										PodName:      podInlineVolume.Name,
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
							PodUid:   string(podInlineVolume.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits:    []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{},
								InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{
									{
										VgName:       utils.VGSSD,
										PodNamespace: podInlineVolume.Namespace,
										PodName:      podInlineVolume.Name,
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
			name: "test pod with inlineVolume too large use sc have vg",
			args: args{
				pod: podInlineVolumeLarge,
			},
			fields: fields{},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Unschedulable),
					utils.NodeName2: framework.NewStatus(framework.Unschedulable),
					utils.NodeName3: framework.NewStatus(framework.Unschedulable),
					utils.NodeName4: framework.NewStatus(framework.Unschedulable),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			nodeInfos := prepare(plugin)

			cycleState := framework.NewCycleState()
			plugin.PreFilter(context.Background(), cycleState, tt.args.pod)

			for _, node := range nodeInfos {
				gotStatus := plugin.Filter(context.Background(), cycleState, tt.args.pod, node)
				assert.Equal(t, tt.expectFilter.nodeStatuses[node.Node().Name].Code(), gotStatus.Code())
			}

			gotDataState, err := plugin.getState(cycleState)
			if tt.expectFilter.stateData != nil {
				assert.Equal(t, tt.expectFilter.stateData.allocateStateByNode, gotDataState.allocateStateByNode)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func Test_Filter_DevicePVC(t *testing.T) {
	podWithDevice := utils.CreatePod(&utils.TestPodInfo{
		PodName:      "podDevice",
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos: []*utils.TestPVCInfo{
			utils.GetTestPVCPVDevice().PVCPending,
		},
	})

	//normal pvc
	pvcDevice := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVDevice().PVCPending})[0]

	//large pvc
	pvcDeviceLargeInfo := utils.GetTestPVCPVDevice()
	pvcDeviceLargeInfo.PVCPending.Size = "1000Gi"
	pvcDeviceLarge := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcDeviceLargeInfo.PVCPending})[0]

	type args struct {
		pod *corev1.Pod
	}
	type fields struct {
		pvcs map[string]*corev1.PersistentVolumeClaim
	}

	tests := []struct {
		name         string
		args         args
		fields       fields
		expectFilter *filterResult
	}{
		{
			name: "test pod with pvc device",
			args: args{
				pod: podWithDevice,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcDevice): pvcDevice,
				},
			},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Unschedulable),
					utils.NodeName2: framework.NewStatus(framework.Unschedulable),
					utils.NodeName3: framework.NewStatus(framework.Success),
					utils.NodeName4: framework.NewStatus(framework.Unschedulable),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{
						utils.NodeName3: {
							NodeName: utils.NodeName3,
							PodUid:   string(podWithDevice.UID),
							Units: &cache.NodeAllocateUnits{
								LVMPVCAllocateUnits: []*cache.LVMPVAllocated{},
								DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{
									{
										DeviceName: "/dev/sdc",
										BasePVAllocated: cache.BasePVAllocated{
											PVCName:      pvcDevice.Name,
											PVCNamespace: pvcDevice.Namespace,
											NodeName:     utils.NodeName3,
											Requested:    utils.GetPVCRequested(pvcDevice),
											Allocated:    int64(150 * utils.LocalGi),
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
			},
		},
		{
			name: "test pod with pvc device large",
			args: args{
				pod: podWithDevice,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcDeviceLarge): pvcDeviceLarge,
				},
			},
			expectFilter: &filterResult{
				nodeStatuses: map[string]*framework.Status{
					utils.NodeName1: framework.NewStatus(framework.Unschedulable),
					utils.NodeName2: framework.NewStatus(framework.Unschedulable),
					utils.NodeName3: framework.NewStatus(framework.Unschedulable),
					utils.NodeName4: framework.NewStatus(framework.Unschedulable),
				},
				stateData: &stateData{
					allocateStateByNode: map[string]*cache.NodeAllocateState{},
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

			for _, node := range nodeInfos {
				gotStatus := plugin.Filter(context.Background(), cycleState, tt.args.pod, node)
				assert.Equal(t, tt.expectFilter.nodeStatuses[node.Node().Name].Code(), gotStatus.Code())
			}

			gotDataState, err := plugin.getState(cycleState)
			if tt.expectFilter.stateData != nil {
				assert.Equal(t, tt.expectFilter.stateData.allocateStateByNode, gotDataState.allocateStateByNode)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
