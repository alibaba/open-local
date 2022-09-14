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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

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
	status       *framework.Status
	reserveState *cache.NodeAllocateState
	storage      *cache.NodeStorageState
}

func CreateTestPlugin() *LocalPlugin {

	// cache
	ctx := context.Background()

	nodeAntiAffinityWeight, _ := utils.ParseWeight("")

	kubeclient := k8sfake.NewSimpleClientset()
	localclient := localfake.NewSimpleClientset()
	snapshotclient := volumesnapshotfake.NewSimpleClientset()

	k8sInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclient, 0)
	localInformerFactory := localinformers.NewSharedInformerFactory(localclient, 0)
	snapshotInformerFactory := volumesnapshotinformersfactory.NewSharedInformerFactory(snapshotclient, 0)

	k8sInformerFactory.Start(ctx.Done())
	localInformerFactory.Start(ctx.Done())

	k8sInformerFactory.WaitForCacheSync(ctx.Done())
	localInformerFactory.WaitForCacheSync(ctx.Done())

	snapshotInformerFactory.Start(ctx.Done())
	snapshotInformerFactory.WaitForCacheSync(ctx.Done())

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

		_, _ = plugin.localClientSet.CsiV1alpha1().NodeLocalStorages().Create(context.Background(), nls, metav1.CreateOptions{})
		_ = plugin.localInformers.NodeLocalStorages().Informer().GetIndexer().Add(nls)

		plugin.OnNodeLocalStorageAdd(nls)
		nodeInfo := &framework.NodeInfo{}
		_ = nodeInfo.SetNode(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nls.Name,
			},
		})
		nodeInfos = append(nodeInfos, nodeInfo)
	}

	scs := utils.CreateTestStorageClass()
	for _, sc := range scs {
		_, _ = plugin.kubeClientSet.StorageV1().StorageClasses().Create(context.Background(), sc, metav1.CreateOptions{})
		_ = plugin.storageV1Informers.StorageClasses().Informer().GetIndexer().Add(sc)
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

	return createPod(pvcPVInfos), pvcPVInfos
}

// func createPodConplexWithoutSnapshot() (*corev1.Pod, utils.TestPVCPVInfoList) {
// 	pvcPVInfos := []*utils.TestPVCPVInfo{
// 		utils.GetTestPVCPVWithVG(),
// 		utils.GetTestPVCPVDevice(),
// 		utils.GetTestPVCPVWithoutVG(),
// 		utils.GetTestPVCPVNotLocal(), //not local pv
// 	}

// 	return createPod(pvcPVInfos), pvcPVInfos
// }

func createPod(pvcPVInfos []*utils.TestPVCPVInfo) *corev1.Pod {
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
	return utils.CreatePod(podInfo)
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
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
			}

			for _, pv := range tt.fields.pvs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumes().Informer().GetIndexer().Add(pv)
			}

			if tt.fields.snapshot != nil {
				_, _ = plugin.snapClientSet.SnapshotV1().VolumeSnapshots(tt.fields.snapshot.Namespace).Create(context.Background(), tt.fields.snapshot, metav1.CreateOptions{})
				_ = plugin.snapshotInformers.VolumeSnapshots().Informer().GetIndexer().Add(tt.fields.snapshot)
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

	//pvc without vg
	pvcWithoutVG := utils.GetTestPVCPVWithoutVG()
	pvcWithoutVG.SetSize("100Gi")
	pvcWithoutVG.PVBounding.VgName = utils.VGSSD
	pvcWithoutVGPending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcWithoutVG.PVCPending})[0]
	pvcWithoutVGBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcWithoutVG.PVCBounding})[0]
	pvwithoutVGBounding := utils.CreateTestPersistentVolume([]utils.TestPVInfo{*pvcWithoutVG.PVBounding})[0]

	//large pvc/pv bound, allocate by pv first ,may cost preAllocate fail
	pvcWithoutVGLarge := utils.GetTestPVCPVWithoutVG()
	pvcWithoutVGLarge.SetSize("200Gi")
	pvcWithoutVGLarge.PVBounding.VgName = utils.VGSSD
	pvcWithoutVGLargePending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcWithoutVGLarge.PVCPending})[0]
	pvcWithoutVGLargeBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcWithoutVGLarge.PVCBounding})[0]
	pvwithoutVGLargeBounding := utils.CreateTestPersistentVolume([]utils.TestPVInfo{*pvcWithoutVGLarge.PVBounding})[0]

	type args struct {
		pod      *corev1.Pod
		nodeName string
	}
	type fields struct {
		pvcs map[string]*corev1.PersistentVolumeClaim
		// pvs                   map[string]*corev1.PersistentVolume
		pvcBoundBeforeReserve *corev1.PersistentVolumeClaim
		pvBoundBeforeReserve  *corev1.PersistentVolume
		stateData             *stateData
	}

	tests := []struct {
		name          string
		args          args
		fields        fields
		expectReserve *reserveResult
	}{
		{
			name: "reserve success: test pod with pvc use sc have vg",
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
				},
			},
			expectReserve: &reserveResult{
				status: framework.NewStatus(framework.Success),
				reserveState: &cache.NodeAllocateState{
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
				},
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
			name: "reserve success: test pod with pvc use sc without vg",
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
								request: utils.GetPVCRequested(pvcWithoutVGPending),
								pvc:     pvcWithoutVGPending,
							},
						},
						ssdDevicePVCsNotAllocated: []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated: []*DevicePVCInfo{},
						inlineVolumes:             []*cache.InlineVolumeAllocated{},
					},
				},
			},
			expectReserve: &reserveResult{
				status: framework.NewStatus(framework.Success),
				reserveState: &cache.NodeAllocateState{
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
				},
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
		{
			name: "reserve success: pod with pvc use sc without vg, but pvc staticBinding after preFilter, test nodeStorage space enough",
			args: args{
				pod:      podWithoutVG,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithoutVGPending): pvcWithoutVGPending,
				},
				pvcBoundBeforeReserve: pvcWithoutVGBounding,
				pvBoundBeforeReserve:  pvwithoutVGBounding,
				stateData: &stateData{
					podVolumeInfo: &PodLocalVolumeInfo{
						lvmPVCsWithVgNameNotAllocated: []*LVMPVCInfo{},
						lvmPVCsSnapshot:               []*LVMPVCInfo{},
						lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{
							{
								request: utils.GetPVCRequested(pvcWithoutVGPending),
								pvc:     pvcWithoutVGPending,
							},
						},
						ssdDevicePVCsNotAllocated: []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated: []*DevicePVCInfo{},
						inlineVolumes:             []*cache.InlineVolumeAllocated{},
					},
				},
			},
			expectReserve: &reserveResult{
				status: framework.NewStatus(framework.Success),
				reserveState: &cache.NodeAllocateState{
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
									Allocated:    0,
								},
							},
						},
						DevicePVCAllocateUnits:    []*cache.DeviceTypePVAllocated{},
						InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
					},
				},
				storage: &cache.NodeStorageState{
					VGStates: map[string]*cache.VGStoragePool{
						utils.VGSSD: {
							Name:        utils.VGSSD,
							Total:       int64(300 * utils.LocalGi),
							Allocatable: int64(300 * utils.LocalGi),
							Requested:   utils.GetPVSize(pvwithoutVGBounding),
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
			name: "reserve fail: pod with pvc use sc without vg, but pvc staticBinding after preFilter, test nodeStorage space not enough",
			args: args{
				pod:      podWithoutVG,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcWithoutVGLargePending): pvcWithoutVGLargePending,
				},
				pvcBoundBeforeReserve: pvcWithoutVGLargeBounding,
				pvBoundBeforeReserve:  pvwithoutVGLargeBounding,
				stateData: &stateData{
					podVolumeInfo: &PodLocalVolumeInfo{
						lvmPVCsWithVgNameNotAllocated: []*LVMPVCInfo{},
						lvmPVCsSnapshot:               []*LVMPVCInfo{},
						lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{
							{
								request: utils.GetPVCRequested(pvcWithoutVGLargePending),
								pvc:     pvcWithoutVGLargePending,
							},
						},
						ssdDevicePVCsNotAllocated: []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated: []*DevicePVCInfo{},
						inlineVolumes:             []*cache.InlineVolumeAllocated{},
					},
				},
			},
			expectReserve: &reserveResult{
				status:       framework.NewStatus(framework.Unschedulable),
				reserveState: nil,
				storage: &cache.NodeStorageState{
					VGStates: map[string]*cache.VGStoragePool{
						utils.VGSSD: {
							Name:        utils.VGSSD,
							Total:       int64(300 * utils.LocalGi),
							Allocatable: int64(300 * utils.LocalGi),
							Requested:   utils.GetPVSize(pvwithoutVGLargeBounding),
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
			plugin.OnPodAdd(tt.args.pod)
			for _, pvc := range tt.fields.pvcs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
				plugin.OnPVCAdd(pvc)
			}

			if tt.fields.pvcBoundBeforeReserve != nil {
				//update pvc
				oldPVC, _ := plugin.coreV1Informers.PersistentVolumeClaims().Lister().PersistentVolumeClaims(tt.fields.pvcBoundBeforeReserve.Namespace).Get(tt.fields.pvcBoundBeforeReserve.Name)
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(tt.fields.pvcBoundBeforeReserve.Namespace).Update(context.Background(), tt.fields.pvcBoundBeforeReserve, metav1.UpdateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Update(tt.fields.pvcBoundBeforeReserve)
				plugin.OnPVCUpdate(oldPVC, tt.fields.pvcBoundBeforeReserve)
				//update pv
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumes().Create(context.Background(), tt.fields.pvBoundBeforeReserve, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumes().Informer().GetIndexer().Add(tt.fields.pvBoundBeforeReserve)
				plugin.OnPVAdd(tt.fields.pvBoundBeforeReserve)
			}

			cycleState := framework.NewCycleState()
			cycleState.Write(stateKey, tt.fields.stateData)

			gotStatus := plugin.Reserve(context.Background(), cycleState, tt.args.pod, tt.args.nodeName)
			assert.Equal(t, tt.expectReserve.status.Code(), gotStatus.Code(), "check reserve status")

			gotReserveState, _ := plugin.getReserveState(cycleState, tt.args.nodeName)

			if tt.expectReserve.status.IsSuccess() {

				assert.Equal(t, tt.expectReserve.reserveState.Units, gotReserveState.Units, "check reserve state")
				for _, unit := range tt.expectReserve.reserveState.Units.LVMPVCAllocateUnits {
					var cacheAllocated int64
					if tt.fields.pvcBoundBeforeReserve != nil && tt.fields.pvcBoundBeforeReserve.Name == unit.PVCName {
						assert.True(t, unit.Allocated == 0, "bounding lvm pvc(%s/%s) should not allocate", unit.PVCNamespace, unit.PVCName)
						cacheAllocated = plugin.cache.GetPVAllocatedDetailCopy(tt.fields.pvBoundBeforeReserve.Name).GetBasePVAllocated().Allocated
					} else {
						assert.True(t, unit.Allocated > 0, "unBound pvc(%s/%s) should allocate > 0", unit.PVCNamespace, unit.PVCName)
						cacheAllocated = plugin.cache.GetPVCAllocatedDetailCopy(unit.PVCNamespace, unit.PVCName).GetBasePVAllocated().Allocated
					}
					assert.Equal(t, unit.Requested, cacheAllocated, "lvm pvc(%s/%s)  cache should allocate", unit.PVCNamespace, unit.PVCName)
				}
			} else {
				assert.Equal(t, tt.expectReserve.reserveState, gotReserveState)
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

	pvcDeviceInfo := utils.GetTestPVCPVDevice()
	pvcDevicePending := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcDeviceInfo.PVCPending})[0]
	pvcDeviceBounding := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*pvcDeviceInfo.PVCBounding})[0]
	pvDeviceBounding := utils.CreateTestPersistentVolume([]utils.TestPVInfo{*pvcDeviceInfo.PVBounding})[0]

	type args struct {
		pod      *corev1.Pod
		nodeName string
	}
	type fields struct {
		pvcs map[string]*corev1.PersistentVolumeClaim
		// pvs                   map[string]*corev1.PersistentVolume
		pvcBoundBeforeReserve *corev1.PersistentVolumeClaim
		pvBoundBeforeReserve  *corev1.PersistentVolume
		deviceIncludes        []string
		stateData             *stateData
	}

	tests := []struct {
		name          string
		args          args
		fields        fields
		expectReserve *reserveResult
	}{
		{
			name: "reserve success : test pod with device",
			args: args{
				pod:      podWithDevice,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcDevicePending): pvcDevicePending,
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
								request:   utils.GetPVCRequested(pvcDevicePending),
								pvc:       pvcDevicePending,
							},
						},
						inlineVolumes: []*cache.InlineVolumeAllocated{},
					},
				},
			},
			expectReserve: &reserveResult{
				status: framework.NewStatus(framework.Success),
				reserveState: &cache.NodeAllocateState{
					NodeName: utils.NodeName3,
					PodUid:   string(podWithDevice.UID),
					Units: &cache.NodeAllocateUnits{
						LVMPVCAllocateUnits: []*cache.LVMPVAllocated{},
						DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{
							{
								DeviceName: "/dev/sdc",
								BasePVAllocated: cache.BasePVAllocated{
									PVCName:      pvcDevicePending.Name,
									PVCNamespace: pvcDevicePending.Namespace,
									NodeName:     utils.NodeName3,
									Requested:    utils.GetPVCRequested(pvcDevicePending),
									Allocated:    int64(150 * utils.LocalGi),
								},
							},
						},
						InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
					},
				},
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
		{
			name: "reserve fail : test pod with device, pvc staticBinding after PreFilter, and device not enough for duplicate allocate",
			args: args{
				pod:      podWithDevice,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcDevicePending): pvcDevicePending,
				},
				pvcBoundBeforeReserve: pvcDeviceBounding,
				pvBoundBeforeReserve:  pvDeviceBounding,
				stateData: &stateData{
					podVolumeInfo: &PodLocalVolumeInfo{
						lvmPVCsWithVgNameNotAllocated:    []*LVMPVCInfo{},
						lvmPVCsSnapshot:                  []*LVMPVCInfo{},
						lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{},
						ssdDevicePVCsNotAllocated:        []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated: []*DevicePVCInfo{
							{
								mediaType: localtype.MediaTypeHDD,
								request:   utils.GetPVCRequested(pvcDevicePending),
								pvc:       pvcDevicePending,
							},
						},
						inlineVolumes: []*cache.InlineVolumeAllocated{},
					},
				},
			},
			expectReserve: &reserveResult{
				status:       framework.NewStatus(framework.Unschedulable),
				reserveState: nil,
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
		{
			name: "reserve success : test pod with device, pvc staticBinding after PreFilter, and device enough for duplicate allocate",
			args: args{
				pod:      podWithDevice,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs: map[string]*corev1.PersistentVolumeClaim{
					utils.PVCName(pvcDevicePending): pvcDevicePending,
				},
				pvcBoundBeforeReserve: pvcDeviceBounding,
				pvBoundBeforeReserve:  pvDeviceBounding,
				deviceIncludes:        []string{"/dev/sdb", "/dev/sdc"},
				stateData: &stateData{
					podVolumeInfo: &PodLocalVolumeInfo{
						lvmPVCsWithVgNameNotAllocated:    []*LVMPVCInfo{},
						lvmPVCsSnapshot:                  []*LVMPVCInfo{},
						lvmPVCsWithoutVgNameNotAllocated: []*LVMPVCInfo{},
						ssdDevicePVCsNotAllocated:        []*DevicePVCInfo{},
						hddDevicePVCsNotAllocated: []*DevicePVCInfo{
							{
								mediaType: localtype.MediaTypeHDD,
								request:   utils.GetPVCRequested(pvcDevicePending),
								pvc:       pvcDevicePending,
							},
						},
						inlineVolumes: []*cache.InlineVolumeAllocated{},
					},
				},
			},
			expectReserve: &reserveResult{
				status: framework.NewStatus(framework.Success),
				reserveState: &cache.NodeAllocateState{
					NodeName: utils.NodeName3,
					PodUid:   string(podWithDevice.UID),
					Units: &cache.NodeAllocateUnits{
						LVMPVCAllocateUnits: []*cache.LVMPVAllocated{},
						DevicePVCAllocateUnits: []*cache.DeviceTypePVAllocated{
							{
								DeviceName: "/dev/sdb",
								BasePVAllocated: cache.BasePVAllocated{
									PVCName:      pvcDevicePending.Name,
									PVCNamespace: pvcDevicePending.Namespace,
									NodeName:     utils.NodeName3,
									Requested:    utils.GetPVCRequested(pvcDevicePending),
									Allocated:    int64(0),
								},
							},
						},
						InlineVolumeAllocateUnits: []*cache.InlineVolumeAllocated{},
					},
				},
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
						"/dev/sdb": {
							Name:        "/dev/sdb",
							Total:       int64(200 * utils.LocalGi),
							Allocatable: int64(200 * utils.LocalGi),
							Requested:   int64(0),
							MediaType:   localtype.MediaTypeHDD,
							IsAllocated: false,
						},
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
			nodeInfos := prepare(plugin)

			plugin.OnPodAdd(tt.args.pod)
			for _, pvc := range tt.fields.pvcs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
				plugin.OnPVCAdd(pvc)
			}

			if len(tt.fields.deviceIncludes) > 0 {
				//need more device
				for _, node := range nodeInfos {
					if node.Node().Name != utils.NodeName4 {
						// update nls device white list
						nls, _ := plugin.localInformers.NodeLocalStorages().Lister().Get(node.Node().Name)
						nlsNew := nls.DeepCopy()
						nlsNew.Spec.ListConfig.Devices.Include = tt.fields.deviceIncludes
						nlsNew.Status.FilteredStorageInfo.Devices = tt.fields.deviceIncludes
						_, _ = plugin.localClientSet.CsiV1alpha1().NodeLocalStorages().Update(context.Background(), nlsNew, metav1.UpdateOptions{})
						_ = plugin.localInformers.NodeLocalStorages().Informer().GetIndexer().Update(nlsNew)
						//update nls
						plugin.OnNodeLocalStorageUpdate(nls, nlsNew)
					}
				}
			}

			if tt.fields.pvcBoundBeforeReserve != nil {
				//update pvc
				oldPVC, _ := plugin.coreV1Informers.PersistentVolumeClaims().Lister().PersistentVolumeClaims(tt.fields.pvcBoundBeforeReserve.Namespace).Get(tt.fields.pvcBoundBeforeReserve.Name)
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(tt.fields.pvcBoundBeforeReserve.Namespace).Update(context.Background(), tt.fields.pvcBoundBeforeReserve, metav1.UpdateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Update(tt.fields.pvcBoundBeforeReserve)
				plugin.OnPVCUpdate(oldPVC, tt.fields.pvcBoundBeforeReserve)
				//update pv
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumes().Create(context.Background(), tt.fields.pvBoundBeforeReserve, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumes().Informer().GetIndexer().Add(tt.fields.pvBoundBeforeReserve)
				plugin.OnPVAdd(tt.fields.pvBoundBeforeReserve)
			}

			cycleState := framework.NewCycleState()
			cycleState.Write(stateKey, tt.fields.stateData)

			gotStatus := plugin.Reserve(context.Background(), cycleState, tt.args.pod, tt.args.nodeName)
			assert.Equal(t, tt.expectReserve.status.Code(), gotStatus.Code(), "check reserve status")

			gotReserveState, _ := plugin.getReserveState(cycleState, tt.args.nodeName)

			if tt.expectReserve.status.IsSuccess() {

				assert.Equal(t, tt.expectReserve.reserveState.Units, gotReserveState.Units, "check reserve state")
				for _, unit := range tt.expectReserve.reserveState.Units.DevicePVCAllocateUnits {
					var cacheAllocated int64
					if tt.fields.pvcBoundBeforeReserve != nil && tt.fields.pvcBoundBeforeReserve.Name == unit.PVCName {
						assert.True(t, unit.Allocated == 0, "bounding lvm pvc(%s/%s) should not allocate", unit.PVCNamespace, unit.PVCName)
						cacheAllocated = plugin.cache.GetPVAllocatedDetailCopy(tt.fields.pvBoundBeforeReserve.Name).GetBasePVAllocated().Allocated
					} else {
						assert.True(t, unit.Allocated > 0, "unBound pvc(%s/%s) should allocate > 0", unit.PVCNamespace, unit.PVCName)
						cacheAllocated = plugin.cache.GetPVCAllocatedDetailCopy(unit.PVCNamespace, unit.PVCName).GetBasePVAllocated().Allocated
					}
					assert.True(t, cacheAllocated > 0, "lvm pvc(%s/%s)  cache should allocate", unit.PVCNamespace, unit.PVCName)
				}
			} else {
				assert.Equal(t, tt.expectReserve.reserveState, gotReserveState)
			}

			gotNodeStorage := plugin.cache.GetNodeStorageStateCopy(tt.args.nodeName)
			assert.Equal(t, tt.expectReserve.storage, gotNodeStorage, "check storage pool")
		})
	}
}

func Test_Prebind(t *testing.T) {

	//complex pod
	complexPod, complexPVCPVInfos := createPodComplex()

	//pvc,pv pending
	pvcsPending := createPVC(complexPVCPVInfos.GetTestPVCPending())
	pvcsBounding := createPVC(complexPVCPVInfos.GetTestPVCBounding())
	pvsBounding := createPV(complexPVCPVInfos.GetTestPVBounding())

	type args struct {
		nodeName string
		pod      *corev1.Pod
	}

	type fields struct {
		pvcs         map[string]*corev1.PersistentVolumeClaim
		pvs          map[string]*corev1.PersistentVolume
		lastReserved *cache.NodeAllocateState
		nodeLocal    *nodelocalstorage.NodeLocalStorage
	}

	type expect struct {
		pvHaveAllocateInfos map[string]localtype.PVAllocatedInfo
		pvcInfos            map[string]localtype.PVCAllocateInfo
		status              *framework.Status
	}

	tests := []struct {
		name   string
		args   args
		fields fields
		expect expect
	}{
		{
			name: "test openlocal prebind before volumeBinding prebind",
			args: args{
				nodeName: utils.NodeName3,
				pod:      complexPod,
			},
			fields: fields{
				nodeLocal: utils.CreateTestNodeLocalStorage3(),
				pvcs:      pvcsPending,
				lastReserved: &cache.NodeAllocateState{
					NodeName: utils.NodeName3,
					PodUid:   string(complexPod.UID),
					Units: &cache.NodeAllocateUnits{
						LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
							{
								VGName: utils.VGSSD,
								BasePVAllocated: cache.BasePVAllocated{
									PVCName:      utils.PVCWithoutVG,
									PVCNamespace: utils.LocalNameSpace,
									NodeName:     utils.NodeName3,
									Requested:    int64(40 * utils.LocalGi),
									Allocated:    int64(40 * utils.LocalGi),
								},
							},
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
			expect: expect{
				status:              framework.NewStatus(framework.Success),
				pvHaveAllocateInfos: map[string]localtype.PVAllocatedInfo{},
				pvcInfos: map[string]localtype.PVCAllocateInfo{
					utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithVG): {
						PVCName:      utils.PVCWithVG,
						PVCNameSpace: utils.LocalNameSpace,
						PVAllocatedInfo: localtype.PVAllocatedInfo{
							VGName:     utils.VGSSD,
							DeviceName: "",
							VolumeType: string(localtype.VolumeTypeLVM),
						},
					},
					utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithoutVG): {
						PVCName:      utils.PVCWithoutVG,
						PVCNameSpace: utils.LocalNameSpace,
						PVAllocatedInfo: localtype.PVAllocatedInfo{
							VGName:     utils.VGSSD,
							DeviceName: "",
							VolumeType: string(localtype.VolumeTypeLVM),
						},
					},
					utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithDevice): {
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
		{
			name: "test openlocal prebind after volumeBinding prebind",
			args: args{
				nodeName: utils.NodeName3,
				pod:      complexPod,
			},
			fields: fields{
				nodeLocal: utils.CreateTestNodeLocalStorage3(),
				pvcs:      pvcsBounding,
				pvs:       pvsBounding,
				lastReserved: &cache.NodeAllocateState{
					NodeName: utils.NodeName3,
					PodUid:   string(complexPod.UID),
					Units: &cache.NodeAllocateUnits{
						LVMPVCAllocateUnits: []*cache.LVMPVAllocated{
							{
								VGName: utils.VGSSD,
								BasePVAllocated: cache.BasePVAllocated{
									PVCName:      utils.PVCWithoutVG,
									PVCNamespace: utils.LocalNameSpace,
									NodeName:     utils.NodeName3,
									Requested:    int64(40 * utils.LocalGi),
									Allocated:    int64(40 * utils.LocalGi),
								},
							},
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
			expect: expect{
				status: framework.NewStatus(framework.Success),
				pvHaveAllocateInfos: map[string]localtype.PVAllocatedInfo{
					"pv-" + utils.PVCWithVG: {
						VGName:     utils.VGSSD,
						DeviceName: "",
						VolumeType: string(localtype.VolumeTypeLVM),
					},
					"pv-" + utils.PVCWithoutVG: {
						VGName:     utils.VGHDD,
						DeviceName: "",
						VolumeType: string(localtype.VolumeTypeLVM),
					},
					"pv-" + utils.PVCWithDevice: {
						VGName:     "",
						DeviceName: "/dev/sdc",
						VolumeType: string(localtype.VolumeTypeDevice),
					},
				},
				pvcInfos: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := CreateTestPlugin()
			prepare(plugin)

			_, _ = plugin.localClientSet.CsiV1alpha1().NodeLocalStorages().Create(context.Background(), tt.fields.nodeLocal, metav1.CreateOptions{})
			_ = plugin.localInformers.NodeLocalStorages().Informer().GetIndexer().Add(tt.fields.nodeLocal)
			plugin.OnNodeLocalStorageAdd(tt.fields.nodeLocal)

			podCopy := complexPod.DeepCopy()
			_, _ = plugin.kubeClientSet.CoreV1().Pods(complexPod.Namespace).Create(context.Background(), podCopy, metav1.CreateOptions{})
			_ = plugin.coreV1Informers.Pods().Informer().GetIndexer().Add(podCopy)
			plugin.OnPodAdd(podCopy)

			for _, pvc := range pvcsPending {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
				plugin.OnPVCAdd(pvc)
			}

			if tt.fields.lastReserved != nil {
				_ = plugin.cache.Reserve(tt.fields.lastReserved, "")
			}

			for _, pvc := range tt.fields.pvcs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
				plugin.OnPVCAdd(pvc)
			}

			for _, pv := range tt.fields.pvs {
				pvCopy := pv.DeepCopy()
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumes().Create(context.Background(), pvCopy, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumes().Informer().GetIndexer().Add(pvCopy)
				plugin.OnPVAdd(pvCopy)
			}

			cycleState := framework.NewCycleState()

			gotStatus := plugin.PreBind(context.Background(), cycleState, tt.args.pod, tt.args.nodeName)
			assert.Equal(t, tt.expect.status, gotStatus, "check prebind status")

			newPod, err := plugin.kubeClientSet.CoreV1().Pods(tt.args.pod.Namespace).Get(context.Background(), tt.args.pod.Name, metav1.GetOptions{})
			assert.NoError(t, err)

			gotPVFillInfos := map[string]localtype.PVAllocatedInfo{}
			for _, pv := range tt.fields.pvs {
				newPV, _ := plugin.kubeClientSet.CoreV1().PersistentVolumes().Get(context.Background(), pv.Name, metav1.GetOptions{})
				pvAllocateInfo, _ := localtype.GetAllocatedInfoFromPVAnnotation(newPV)
				if pvAllocateInfo != nil {
					gotPVFillInfos[pv.Name] = *pvAllocateInfo
				}
			}
			assert.Equal(t, tt.expect.pvHaveAllocateInfos, gotPVFillInfos, "check pv infos")

			if tt.expect.pvcInfos != nil {
				gotInfos, _ := localtype.GetAllocateInfoFromPod(newPod)
				assert.Equal(t, tt.expect.pvcInfos, gotInfos.PvcAllocates)
			}

		})
	}
}

func Test_Unreserve(t *testing.T) {

	pvcWithoutVG := utils.GetTestPVCPVWithoutVG()
	pvcWithoutVG.PVBounding.VgName = utils.VGSSD

	complexPVCPVInfos := utils.TestPVCPVInfoList{
		utils.GetTestPVCPVWithVG(),
		utils.GetTestPVCPVDevice(),
		pvcWithoutVG,
		utils.GetTestPVCPVNotLocal(), //not local pv
	}

	complexPod := createPod(complexPVCPVInfos)

	//pvc,pv pending
	pvcsPending := createPVC(complexPVCPVInfos.GetTestPVCPending())
	pvcsBounding := createPVC(complexPVCPVInfos.GetTestPVCBounding())
	pvsBounding := createPV(complexPVCPVInfos.GetTestPVBounding())

	type args struct {
		pod      *corev1.Pod
		nodeName string
	}

	type fields struct {
		pvcs           map[string]*corev1.PersistentVolumeClaim
		deviceIncludes []string

		pvcBoundBeforeReserve *corev1.PersistentVolumeClaim
		pvBoundBeforeReserve  *corev1.PersistentVolume
	}

	type expect struct {
		reserveStatus *framework.Status
		nodeStorage   *cache.NodeStorageState
	}

	tests := []struct {
		name   string
		args   args
		fields fields
		expect expect
	}{
		{
			name: "test reserved success and unReserve all allocate",
			args: args{
				pod:      complexPod,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs: pvcsPending,
			},
			expect: expect{
				reserveStatus: framework.NewStatus(framework.Success),
				nodeStorage: &cache.NodeStorageState{
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
		{
			name: "test reserved success with lvm pvc staticBinding between [PreFilter,Reserve] and only unReserve allocate success unit",
			args: args{
				pod:      complexPod,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs:                  pvcsPending,
				pvcBoundBeforeReserve: pvcsBounding[utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithoutVG)],
				pvBoundBeforeReserve:  pvsBounding["pv-"+utils.PVCWithoutVG],
			},
			expect: expect{
				reserveStatus: framework.NewStatus(framework.Success),
				nodeStorage: &cache.NodeStorageState{
					VGStates: map[string]*cache.VGStoragePool{
						utils.VGSSD: {
							Name:        utils.VGSSD,
							Total:       int64(300 * utils.LocalGi),
							Allocatable: int64(300 * utils.LocalGi),
							Requested:   utils.GetPVSize(pvsBounding["pv-"+utils.PVCWithoutVG]),
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
			name: "test reserved fail with device pvc staticBinding between [PreFilter,Reserve] and only unReserve allocate success unit",
			args: args{
				pod:      complexPod,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs:                  pvcsPending,
				pvcBoundBeforeReserve: pvcsBounding[utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithDevice)],
				pvBoundBeforeReserve:  pvsBounding["pv-"+utils.PVCWithDevice],
			},
			expect: expect{
				reserveStatus: framework.NewStatus(framework.Unschedulable),
				nodeStorage: &cache.NodeStorageState{
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
		{
			name: "test reserved success with device pvc staticBinding between [PreFilter,Reserve] and only unReserve allocate success unit",
			args: args{
				pod:      complexPod,
				nodeName: utils.NodeName3,
			},
			fields: fields{
				pvcs:                  pvcsPending,
				deviceIncludes:        []string{"/dev/sdb", "/dev/sdc"},
				pvcBoundBeforeReserve: pvcsBounding[utils.GetPVCKey(utils.LocalNameSpace, utils.PVCWithDevice)],
				pvBoundBeforeReserve:  pvsBounding["pv-"+utils.PVCWithDevice],
			},
			expect: expect{
				reserveStatus: framework.NewStatus(framework.Success),
				nodeStorage: &cache.NodeStorageState{
					VGStates: map[string]*cache.VGStoragePool{
						utils.VGSSD: {
							Name:        utils.VGSSD,
							Total:       int64(300 * utils.LocalGi),
							Allocatable: int64(300 * utils.LocalGi),
							Requested:   0,
						},
					},
					DeviceStates: map[string]*cache.DeviceResourcePool{
						"/dev/sdb": {
							Name:        "/dev/sdb",
							Total:       int64(200 * utils.LocalGi),
							Allocatable: int64(200 * utils.LocalGi),
							Requested:   int64(0),
							MediaType:   localtype.MediaTypeHDD,
							IsAllocated: false,
						},
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
			nodeInfos := prepare(plugin)
			plugin.OnPodAdd(tt.args.pod)

			for _, pvc := range tt.fields.pvcs {
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Add(pvc)
				plugin.OnPVCAdd(pvc)
			}

			if len(tt.fields.deviceIncludes) > 0 {
				//need more device
				for _, node := range nodeInfos {
					if node.Node().Name != utils.NodeName4 {
						// update nls device white list
						nls, _ := plugin.localInformers.NodeLocalStorages().Lister().Get(node.Node().Name)
						nlsNew := nls.DeepCopy()
						nlsNew.Spec.ListConfig.Devices.Include = tt.fields.deviceIncludes
						nlsNew.Status.FilteredStorageInfo.Devices = tt.fields.deviceIncludes
						_, _ = plugin.localClientSet.CsiV1alpha1().NodeLocalStorages().Update(context.Background(), nlsNew, metav1.UpdateOptions{})
						_ = plugin.localInformers.NodeLocalStorages().Informer().GetIndexer().Update(nlsNew)
						//update nls
						plugin.OnNodeLocalStorageUpdate(nls, nlsNew)
					}
				}
			}

			cycleState := framework.NewCycleState()
			//preFilter
			plugin.PreFilter(context.Background(), cycleState, tt.args.pod)

			//Filter
			var filterSuccessNodes []string
			for _, nodeInfo := range nodeInfos {
				status := plugin.Filter(context.Background(), cycleState, tt.args.pod, nodeInfo)
				if status.IsSuccess() {
					filterSuccessNodes = append(filterSuccessNodes, nodeInfo.Node().Name)
				} else {
					fmt.Printf("filter fail for node(%s), info: %s\n", nodeInfo.Node().Name, status.Message())
				}
			}
			fmt.Printf("filterSuccessNodes: %#v\n", filterSuccessNodes)
			//Score
			for _, nodeName := range filterSuccessNodes {
				score, status := plugin.Score(context.Background(), cycleState, tt.args.pod, nodeName)
				if status.IsSuccess() {
					fmt.Printf("score for node(%s) : %d\n", nodeName, score)
				}
			}
			assert.NotEmpty(t, filterSuccessNodes, "tt.args.nodeName should filer success")
			//pvc bound after preFilter and before Reserve
			if tt.fields.pvcBoundBeforeReserve != nil {
				//update pvc
				oldPVC, _ := plugin.coreV1Informers.PersistentVolumeClaims().Lister().PersistentVolumeClaims(tt.fields.pvcBoundBeforeReserve.Namespace).Get(tt.fields.pvcBoundBeforeReserve.Name)
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumeClaims(tt.fields.pvcBoundBeforeReserve.Namespace).Update(context.Background(), tt.fields.pvcBoundBeforeReserve, metav1.UpdateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumeClaims().Informer().GetIndexer().Update(tt.fields.pvcBoundBeforeReserve)
				plugin.OnPVCUpdate(oldPVC, tt.fields.pvcBoundBeforeReserve)
				//update pv
				_, _ = plugin.kubeClientSet.CoreV1().PersistentVolumes().Create(context.Background(), tt.fields.pvBoundBeforeReserve, metav1.CreateOptions{})
				_ = plugin.coreV1Informers.PersistentVolumes().Informer().GetIndexer().Add(tt.fields.pvBoundBeforeReserve)
				plugin.OnPVAdd(tt.fields.pvBoundBeforeReserve)
			}

			gotStatus := plugin.Reserve(context.Background(), cycleState, tt.args.pod, tt.args.nodeName)
			assert.Equal(t, tt.expect.reserveStatus.Code(), gotStatus.Code(), "check reserve result: %#v", gotStatus)
			reserveState, err := plugin.getReserveState(cycleState, tt.args.nodeName)
			fmt.Printf("reserve allocate result: %#v ,err: %v, \n", reserveState, err)

			assert.Equal(t, !gotStatus.IsSuccess(), reserveState == nil, "reserve fail and state nil")
			var reserveUnitsCopy *cache.NodeAllocateUnits
			if reserveState != nil {
				reserveUnitsCopy = reserveState.Units.Clone()
			}

			plugin.Unreserve(context.Background(), cycleState, tt.args.pod, tt.args.nodeName)

			assert.Equal(t, tt.expect.nodeStorage, plugin.cache.GetNodeStorageStateCopy(tt.args.nodeName), "check unreserved node storage cache")
			assert.Empty(t, plugin.cache.GetPodInlineVolumeDetailsCopy(tt.args.nodeName, string(tt.args.pod.UID)), "inlineVolume revert fail")

			if reserveUnitsCopy == nil {
				return
			}
			for _, unit := range reserveUnitsCopy.LVMPVCAllocateUnits {
				if tt.fields.pvcBoundBeforeReserve != nil && tt.fields.pvcBoundBeforeReserve.Name == unit.PVCName {
					assert.NotNil(t, plugin.cache.GetPVAllocatedDetailCopy(tt.fields.pvBoundBeforeReserve.Name), "pv bound must allocated")
				}
				assert.Nil(t, plugin.cache.GetPVCAllocatedDetailCopy(unit.PVCNamespace, unit.PVCName), "pvc should revert")
			}

			for _, unit := range reserveUnitsCopy.DevicePVCAllocateUnits {
				if tt.fields.pvcBoundBeforeReserve != nil && tt.fields.pvcBoundBeforeReserve.Name == unit.PVCName {
					assert.NotNil(t, plugin.cache.GetPVAllocatedDetailCopy(tt.fields.pvBoundBeforeReserve.Name), "pv bound must allocated")
				}
				assert.Nil(t, plugin.cache.GetPVCAllocatedDetailCopy(unit.PVCNamespace, unit.PVCName), "pvc should revert")
			}
		})
	}
}
