/*
Copyright 2022/8/30 Alibaba Cloud.

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
	"github.com/stretchr/testify/assert"
	"testing"

	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	localfake "github.com/alibaba/open-local/pkg/generated/clientset/versioned/fake"
	localinformers "github.com/alibaba/open-local/pkg/generated/informers/externalversions"
	"github.com/alibaba/open-local/pkg/scheduling-framework/cache"
	"github.com/alibaba/open-local/pkg/utils"
	"k8s.io/apimachinery/pkg/api/resource"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	volumesnapshotfake "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned/fake"
	volumesnapshotinformersfactory "github.com/kubernetes-csi/external-snapshotter/client/v4/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgocache "k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	PvcPodName     string = "pvc_pod"
	ComplexPodName string = "complex_pod"
)

type scheduleResult struct {
	status    *framework.Status
	stateData *stateData
}

type reserveResult struct {
	status  *framework.Status
	storage *cache.NodeStorageState
}

func CreateTestPlugin() *LocalPlugin {

	// cache
	cxt := context.Background()

	nodeAntiAffinityWeight, _ := utils.ParseWeight("")

	kubeclient := k8sfake.NewSimpleClientset()
	localclient := localfake.NewSimpleClientset()
	snapshotclient := volumesnapshotfake.NewSimpleClientset()

	k8sInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclient, 0)
	localInformerFactory := localinformers.NewSharedInformerFactory(localclient, 0)
	snapshotInformerFactory := volumesnapshotinformersfactory.NewSharedInformerFactory(snapshotclient, 0)

	k8sInformerFactory.Start(cxt.Done())
	localInformerFactory.Start(cxt.Done())

	k8sInformerFactory.WaitForCacheSync(cxt.Done())
	localInformerFactory.WaitForCacheSync(cxt.Done())

	snapshotInformerFactory.Start(cxt.Done())
	snapshotInformerFactory.WaitForCacheSync(cxt.Done())

	nodeCache := cache.NewNodeStorageAllocatedCache(k8sInformerFactory.Core().V1())

	localPlugin := &LocalPlugin{
		allocateStrategy:       GetAllocateStrategy(nil),
		nodeAntiAffinityWeight: nodeAntiAffinityWeight,

		cache:              nodeCache,
		coreV1Informers:    k8sInformerFactory.Core().V1(),
		scLister:           k8sInformerFactory.Storage().V1().StorageClasses().Lister(),
		storageV1Informers: k8sInformerFactory.Storage().V1(),
		localInformers:     localInformerFactory.Csi().V1alpha1(),
		snapshotInformers:  snapshotInformerFactory.Snapshot().V1(),

		kubeClientSet:  kubeclient,
		localClientSet: localclient,
		snapClientSet:  snapshotclient,
	}

	localStorageInformer := localInformerFactory.Csi().V1alpha1().NodeLocalStorages().Informer()
	localStorageInformer.AddEventHandler(clientgocache.ResourceEventHandlerFuncs{
		AddFunc:    localPlugin.OnNodeLocalStorageAdd,
		UpdateFunc: localPlugin.OnNodeLocalStorageUpdate,
	})

	pvInformer := k8sInformerFactory.Core().V1().PersistentVolumes().Informer()
	pvInformer.AddEventHandler(clientgocache.ResourceEventHandlerFuncs{
		AddFunc:    localPlugin.OnPVAdd,
		UpdateFunc: localPlugin.OnPVUpdate,
		DeleteFunc: localPlugin.OnPVDelete,
	})
	pvcInformer := k8sInformerFactory.Core().V1().PersistentVolumeClaims().Informer()
	pvcInformer.AddEventHandler(clientgocache.ResourceEventHandlerFuncs{
		AddFunc:    localPlugin.OnPVCAdd,
		UpdateFunc: localPlugin.OnPVCUpdate,
		DeleteFunc: localPlugin.OnPVCDelete,
	})
	podInformer := k8sInformerFactory.Core().V1().Pods().Informer()
	podInformer.AddEventHandler(clientgocache.ResourceEventHandlerFuncs{
		AddFunc:    localPlugin.OnPodAdd,
		UpdateFunc: localPlugin.OnPodUpdate,
		DeleteFunc: localPlugin.OnPodDelete,
	})

	return localPlugin
}

func prepare(plugin *LocalPlugin) []*framework.NodeInfo {
	var nodeInfos []*framework.NodeInfo

	nlses := utils.CreateTestNodeLocalStorage()
	for _, nls := range nlses {

		plugin.localClientSet.CsiV1alpha1().NodeLocalStorages().Create(context.Background(), nls, metav1.CreateOptions{})
		plugin.localInformers.NodeLocalStorages().Informer().GetIndexer().Add(nls)

		plugin.OnNodeLocalStorageAdd(nls)
		nodeInfo := &framework.NodeInfo{}
		nodeInfo.SetNode(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nls.Name,
			},
		})
		nodeInfos = append(nodeInfos, nodeInfo)
	}

	scs := utils.CreateTestStorageClass()
	for _, sc := range scs {
		plugin.kubeClientSet.StorageV1().StorageClasses().Create(context.Background(), sc, metav1.CreateOptions{})
		plugin.storageV1Informers.StorageClasses().Informer().GetIndexer().Add(sc)
	}

	return nodeInfos
}

func createPodComplex() (*corev1.Pod, utils.TestPVCPVInfoList) {
	pvcPVInfos := []*utils.TestPVCPVInfo{
		utils.GetTestPVCPVWithVG(),
		utils.GetTestPVCPVWithoutVG(),
		utils.GetTestPVCPVSnapshot(),
		utils.GetTestPVCPVDevice(),
		utils.GetTestPVCPVNotLocal(), //not local pv
	}

	var pvcInfos []*utils.TestPVCInfo
	for _, pvcPVInfo := range pvcPVInfos {
		pvcInfos = append(pvcInfos, pvcPVInfo.PVCPending)
	}

	podInfo := &utils.TestPodInfo{
		PodName:      ComplexPodName,
		PodNameSpace: utils.LocalNameSpace,
		PodStatus:    corev1.PodPending,
		PVCInfos:     pvcInfos,
		InlineVolumeInfos: []*utils.TestInlineVolumeInfo{
			{
				VolumeName: "test_inline_volume",
				VolumeSize: "10Gi",
				VgName:     utils.VGSSD,
			},
		},
	}
	return utils.CreatePod(podInfo), pvcPVInfos
}

func createPVC(pvcInfos []utils.TestPVCInfo) map[string]*corev1.PersistentVolumeClaim {
	pvcs := utils.CreateTestPersistentVolumeClaim(pvcInfos)
	pvcMap := map[string]*corev1.PersistentVolumeClaim{}
	for _, pvc := range pvcs {
		pvcMap[utils.PVCName(pvc)] = pvc
	}
	return pvcMap
}

func createPV(pvInfos []utils.TestPVInfo) map[string]*corev1.PersistentVolume {
	pvs := utils.CreateTestPersistentVolume(pvInfos)
	pvMap := map[string]*corev1.PersistentVolume{}
	for _, pv := range pvs {
		pvMap[pv.Name] = pv
	}
	return pvMap
}

func getSize(size string) int64 {
	q := resource.MustParse(size)
	return q.Value()
}

func Test_Reserve_PodHaveNoLocalPVC(t *testing.T) {
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

	//prepare pvc
	_, complexPVCPVInfos := createPodComplex()
	pvcsPending := createPVC(complexPVCPVInfos.GetTestPVCPending())

	pvcsPending[utils.PVCName(snapshotPVC)] = snapshotPVC

	type args struct {
		pod      *corev1.Pod
		nodeName string
	}
	type fields struct {
		node     *nodelocalstorage.NodeLocalStorage
		pvcs     map[string]*corev1.PersistentVolumeClaim
		pvs      map[string]*corev1.PersistentVolume
		snapshot *volumesnapshotv1.VolumeSnapshot
	}

	tests := []struct {
		name         string
		args         args
		fields       fields
		expectResult *reserveResult
	}{
		{
			name: "test no pvc defined in pod, expect success",
			args: args{
				pod:      simplePod,
				nodeName: utils.NodeName3,
			},
			fields: fields{},
			expectResult: &reserveResult{
				status: framework.NewStatus(framework.Success),
			},
		},
		{
			name: "test noLocal pvc defined in pod, expect success",
			args: args{
				pod:      commonPVCPod,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs: pvcsPending,
			},
			expectResult: &reserveResult{
				status: framework.NewStatus(framework.Success),
			},
		},
		{
			name: "test snapshot pvc defined in pod, expect success",
			args: args{
				pod:      podWithSnapshot,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs:     pvcsPending,
				snapshot: snapshot,
			},
			expectResult: &reserveResult{
				status: framework.NewStatus(framework.Success),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			nodeInfos := prepare(plugin)
			for _, pvc := range tt.fields.pvcs {
				plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			for _, pv := range tt.fields.pvs {
				plugin.kubeClientSet.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
				plugin.coreV1Informers.PersistentVolumes().Informer().GetIndexer().Add(pv)
			}

			if tt.fields.snapshot != nil {
				plugin.snapClientSet.SnapshotV1().VolumeSnapshots(tt.fields.snapshot.Namespace).Create(context.Background(), tt.fields.snapshot, metav1.CreateOptions{})
				plugin.snapshotInformers.VolumeSnapshots().Informer().GetIndexer().Add(tt.fields.snapshot)
			}

			cycleState := framework.NewCycleState()
			plugin.PreFilter(context.Background(), cycleState, tt.args.pod)

			for _, node := range nodeInfos {
				plugin.Filter(context.Background(), cycleState, tt.args.pod, node)
			}
			for _, node := range nodeInfos {
				plugin.Score(context.Background(), cycleState, tt.args.pod, node.Node().Name)
			}

			gotStatus := plugin.Reserve(context.Background(), cycleState, tt.args.pod, tt.args.nodeName)
			assert.Equal(t, tt.expectResult.status, gotStatus)
		})
	}
}

func Test_Reserve_LVMPVC_NotSnapshot(t *testing.T) {
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

	type args struct {
		pod      *corev1.Pod
		nodeName string
	}
	type fields struct {
		pvcs      map[string]*corev1.PersistentVolumeClaim
		pvs       map[string]*corev1.PersistentVolume
		stateData *stateData
	}

	tests := []struct {
		name          string
		args          args
		fields        fields
		expectReserve *reserveResult
	}{
		{
			name: "test pod with pvc use sc have vg",
			args: args{
				pod:      podWithVG,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithVGPending): pvcWithVGPending,
				},
				stateData: &stateData{
					podVolumeInfo: &PodLocalVolumeInfo{
						lvmPVCsWithVgNameNotAllocated: []*LVMPVCInfo{
							{
								vgName:  utils.GetTestPVCPVWithVG().PVBounding.VgName,
								request: utils.GetPVCRequested(pvcWithVGPending),
								pvc:     pvcWithVGPending,
							},
						},
						lvmPVCsSnapshot:                  []*LVMPVCInfo{},
						lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{},
						ssdDevicePVCsNotAllocated:        []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated:        []*DevicePVCInfo{},
						inlineVolumes:                    []*cache.InlineVolumeAllocated{},
					},
					allocateStateByNode: map[string]*cache.NodeAllocateState{
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
			expectReserve: &reserveResult{
				status: framework.NewStatus(framework.Success),
				storage: &cache.NodeStorageState{
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
		{
			name: "test pod with pvc use sc without vg",
			args: args{
				pod:      podWithoutVG,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithoutVGPending): pvcWithoutVGPending,
				},
				stateData: &stateData{
					podVolumeInfo: &PodLocalVolumeInfo{
						lvmPVCsWithVgNameNotAllocated: []*LVMPVCInfo{},
						lvmPVCsSnapshot:               []*LVMPVCInfo{},
						lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{
							{
								vgName:  utils.GetTestPVCPVWithoutVG().PVBounding.VgName,
								request: utils.GetPVCRequested(pvcWithoutVGPending),
								pvc:     pvcWithoutVGPending,
							},
						},
						ssdDevicePVCsNotAllocated: []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated: []*DevicePVCInfo{},
						inlineVolumes:             []*cache.InlineVolumeAllocated{},
					},
					allocateStateByNode: map[string]*cache.NodeAllocateState{
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
			expectReserve: &reserveResult{
				status: framework.NewStatus(framework.Success),
				storage: &cache.NodeStorageState{
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			prepare(plugin)
			for _, pvc := range tt.fields.pvcs {
				plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			cycleState := framework.NewCycleState()
			cycleState.Write(stateKey, tt.fields.stateData)

			gotStatus := plugin.Reserve(context.Background(), cycleState, tt.args.pod, tt.args.nodeName)
			assert.Equal(t, tt.expectReserve.status, gotStatus, "check reserve status")

			for _, detail := range tt.fields.stateData.reservedState.Units.LVMPVCAllocateUnits {
				gotDetail := plugin.cache.GetPVCAllocatedDetailCopy(detail.PVCNamespace, detail.PVCName)
				assert.Equal(t, detail, gotDetail, detail.PVCNamespace, detail.PVCName)
			}
			gotNodeStorage := plugin.cache.GetNodeStorageStateCopy(tt.args.nodeName)
			assert.Equal(t, tt.expectReserve.storage, gotNodeStorage, "check storage pool")
		})
	}
}

func Test_Reserve_inlineVolume(t *testing.T) {
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

	type args struct {
		pod      *corev1.Pod
		nodeName string
	}
	type fields struct {
		stateData *stateData
	}

	tests := []struct {
		name          string
		args          args
		fields        fields
		expectReserve *reserveResult
	}{
		{
			name: "test pod inline volume",
			args: args{
				pod:      podInlineVolume,
				nodeName: utils.NodeName3,
			},
			fields: fields{
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
								VolumeSize:   getSize("150Gi"),
								VgName:       utils.VGSSD,
								PodName:      podInlineVolume.Name,
								PodNamespace: podInlineVolume.Namespace,
							},
						},
					},
					allocateStateByNode: map[string]*cache.NodeAllocateState{
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
			expectReserve: &reserveResult{
				status: framework.NewStatus(framework.Success),
				storage: &cache.NodeStorageState{
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			prepare(plugin)

			cycleState := framework.NewCycleState()
			cycleState.Write(stateKey, tt.fields.stateData)

			gotStatus := plugin.Reserve(context.Background(), cycleState, tt.args.pod, tt.args.nodeName)
			assert.Equal(t, tt.expectReserve.status, gotStatus, "check reserve status")

			gotDetail := plugin.cache.GetPodInlineVolumeDetailsCopy(tt.args.nodeName, string(tt.args.pod.UID))
			assert.Equal(t, cache.PodInlineVolumeAllocatedDetails(tt.fields.stateData.reservedState.Units.InlineVolumeAllocateUnits), *gotDetail, tt.args.nodeName, string(tt.args.pod.UID))
			gotNodeStorage := plugin.cache.GetNodeStorageStateCopy(tt.args.nodeName)
			assert.Equal(t, tt.expectReserve.storage, gotNodeStorage, "check storage pool")
		})
	}
}

func Test_Reserve_DevicePVC(t *testing.T) {
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

	type args struct {
		pod      *corev1.Pod
		nodeName string
	}
	type fields struct {
		pvcs      map[string]*corev1.PersistentVolumeClaim
		pvs       map[string]*corev1.PersistentVolume
		stateData *stateData
	}

	tests := []struct {
		name          string
		args          args
		fields        fields
		expectReserve *reserveResult
	}{
		{
			name: "test pod with device",
			args: args{
				pod:      podWithDevice,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcDevice): pvcDevice,
				},
				stateData: &stateData{
					podVolumeInfo: &PodLocalVolumeInfo{
						lvmPVCsWithVgNameNotAllocated:    []*LVMPVCInfo{},
						lvmPVCsSnapshot:                  []*LVMPVCInfo{},
						lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{},
						ssdDevicePVCsNotAllocated:        []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated: []*DevicePVCInfo{
							{
								mediaType: localtype.MediaTypeHDD,
								request:   utils.GetPVCRequested(pvcDevice),
								pvc:       pvcDevice,
							},
						},
						inlineVolumes: []*cache.InlineVolumeAllocated{},
					},
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
			expectReserve: &reserveResult{
				status: framework.NewStatus(framework.Success),
				storage: &cache.NodeStorageState{
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			prepare(plugin)

			for _, pvc := range tt.fields.pvcs {
				plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			cycleState := framework.NewCycleState()
			cycleState.Write(stateKey, tt.fields.stateData)

			gotStatus := plugin.Reserve(context.Background(), cycleState, tt.args.pod, tt.args.nodeName)
			assert.Equal(t, tt.expectReserve.status, gotStatus, "check reserve status")

			for _, detail := range tt.fields.stateData.reservedState.Units.DevicePVCAllocateUnits {
				gotDetail := plugin.cache.GetPVCAllocatedDetailCopy(detail.PVCNamespace, detail.PVCName)
				assert.Equal(t, detail, gotDetail, detail.PVCNamespace, detail.PVCName)
			}
			gotNodeStorage := plugin.cache.GetNodeStorageStateCopy(tt.args.nodeName)
			assert.Equal(t, tt.expectReserve.storage, gotNodeStorage, "check storage pool")
		})
	}
}

func Test_Prebind(t *testing.T) {

	complexPod, _ := createPodComplex()

	type args struct {
		nodeName string
		pod      *corev1.Pod
		state    *stateData
	}

	type fields struct {
		nodeLocal *nodelocalstorage.NodeLocalStorage
	}

	type expect struct {
		pvcInfos map[string]localtype.NodeStoragePVCAllocateInfo
		status   *framework.Status
	}

	tests := []struct {
		name   string
		args   args
		fields fields
		expect expect
	}{
		{
			name: "test allocate info",
			args: args{
				nodeName: utils.NodeName3,
				pod:      complexPod,
				state: &stateData{
					reservedState: &cache.NodeAllocateState{
						NodeName: utils.NodeName3,
						PodUid:   string(complexPod.UID),
						Units: &cache.NodeAllocateUnits{
							LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
								{
									VGName: utils.VGSSD,
									BasePVAllocated: cache.BasePVAllocated{
										PVCName:      utils.PVCWithVG,
										PVCNamespace: utils.LocalNameSpace,
										NodeName:     utils.NodeName3,
										Requested:    int64(10 * utils.LocalGi),
										Allocated:    int64(10 * utils.LocalGi),
									},
								},
							},
							DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{
								{
									DeviceName: "/dev/sdc",
									BasePVAllocated: cache.BasePVAllocated{
										PVCName:      utils.PVCWithDevice,
										PVCNamespace: utils.LocalNameSpace,
										NodeName:     utils.NodeName3,
										Requested:    int64(10 * utils.LocalGi),
										Allocated:    int64(150 * utils.LocalGi),
									},
								},
							},
							InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
						},
					},
				},
			},
			fields: fields{
				nodeLocal: utils.CreateTestNodeLocalStorage3(),
			},
			expect: expect{
				status: framework.NewStatus(framework.Success),
				pvcInfos: map[string]localtype.NodeStoragePVCAllocateInfo{
					utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithVG): localtype.NodeStoragePVCAllocateInfo{
						PVCName:      utils.PVCWithVG,
						PVCNameSpace: utils.LocalNameSpace,
						PVAllocatedInfo: localtype.PVAllocatedInfo{
							VGName:     utils.VGSSD,
							DeviceName: "",
							VolumeType: string(localtype.VolumeTypeLVM),
						},
					},
					utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithDevice): localtype.NodeStoragePVCAllocateInfo{
						PVCName:      utils.PVCWithDevice,
						PVCNameSpace: utils.LocalNameSpace,
						PVAllocatedInfo: localtype.PVAllocatedInfo{
							VGName:     "",
							DeviceName: "/dev/sdc",
							VolumeType: string(localtype.VolumeTypeDevice),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			prepare(plugin)

			plugin.localClientSet.CsiV1alpha1().NodeLocalStorages().Create(context.Background(), tt.fields.nodeLocal, metav1.CreateOptions{})
			plugin.localInformers.NodeLocalStorages().Informer().GetIndexer().Add(tt.fields.nodeLocal)

			cycleState := framework.NewCycleState()
			cycleState.Write(stateKey, tt.args.state)

			gotStatus := plugin.PreBind(context.Background(), cycleState, tt.args.pod, tt.args.nodeName)
			assert.Equal(t, tt.expect.status, gotStatus, "check prebind status")

			newLocal, err := plugin.localClientSet.CsiV1alpha1().NodeLocalStorages().Get(context.Background(), tt.args.nodeName, metav1.GetOptions{})
			assert.NoError(t, err)

			gotInfos, _ := localtype.GetAllocateInfoFromNLS(newLocal)
			assert.Equal(t, tt.expect.pvcInfos, gotInfos.PvcAllocates)

		})
	}
}

func Test_Unreserve(t *testing.T) {
	type args struct {
		pod *corev1.Pod
	}
}
