/*
Copyright © 2021 Alibaba Group Holding Ltd.

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

package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/alibaba/open-local/pkg/utils"
	"net/http"
	"testing"
	"time"

	localtype "github.com/alibaba/open-local/pkg"
	localv1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	localfake "github.com/alibaba/open-local/pkg/generated/clientset/versioned/fake"
	localinformers "github.com/alibaba/open-local/pkg/generated/informers/externalversions"
	"github.com/alibaba/open-local/pkg/scheduler/server"
	volumesnapshotfake "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned/fake"
	volumesnapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v4/informers/externalversions"
	"k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	log "k8s.io/klog/v2"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
)

var (
	noResyncPeriodFunc = func() time.Duration {
		log.Info("test noResyncPeriodFunc")
		return 0
	}
)

const (
	// General
	LocalGi        uint64 = 1024 * 1024 * 1024
	LocalMi        uint64 = 1024 * 1024
	TestPort       int32  = 23000
	LocalNameSpace string = "default"
	// Node
	NodeName1 string = "node-192.168.0.1"
	NodeName2 string = "node-192.168.0.2"
	NodeName3 string = "node-192.168.0.3"
	NodeName4 string = "node-192.168.0.4"
	// VG
	VGSSD string = "ssd"
	VGHDD string = "hdd"
	// StorageClass
	SCLVMWithVG    string = "sc-vg"
	SCLVMWithoutVG string = "sc-novg"
	SCWithMP       string = "sc-mp"
	SCWithDevice   string = "sc-device"
	SCNoLocal      string = "sc-nolocal"
	// PVC
	PVCWithVG         string = "pvc-vg"
	PVCWithoutVG      string = "pvc-novg"
	PVCWithVGError    string = "pvc-vg-error"
	PVCWithMountPoint string = "pvc-mp"
	PVCWithDevice     string = "pvc-device"
	PVCNoLocal        string = "pvc-nolocal"
	// Pod
	PodName string = "testpod"
)

var NodeNamesAll []string = []string{NodeName1, NodeName2, NodeName3, NodeName4}

type fixture struct {
	t *testing.T

	kubeclient  *k8sfake.Clientset
	localclient *localfake.Clientset
	snapclient  *volumesnapshotfake.Clientset

	// Objects from here preloaded into NewSimpleFake.
	kubeobjects  []runtime.Object
	localobjects []runtime.Object
	snapobjects  []runtime.Object
}

var f *fixture

func init() {
	f = newFixture(nil)

	nodes := newNode()
	crds := newNodeLocalStorage()
	scs := newStorageClass()
	pvcs := newPersistentVolumeClaim()

	for _, crd := range crds {
		f.localobjects = append(f.localobjects, crd)
	}
	for _, sc := range scs {
		f.kubeobjects = append(f.kubeobjects, sc)
	}
	for _, pvc := range pvcs {
		f.kubeobjects = append(f.kubeobjects, pvc)
	}
	for _, node := range nodes {
		f.kubeobjects = append(f.kubeobjects, node)
	}

	f.runExtender()
}

func TestVGWithName(t *testing.T) {
	f.setT(t)

	var extenderFilterResult schedulerapi.ExtenderFilterResult
	var hostPriorityList schedulerapi.HostPriorityList
	pod := getTestPod(PVCWithVG)
	nodeNamesForPredicate := NodeNamesAll
	nodeNamesForPriority := NodeNamesAll

	extenderFilterResult = predicateFunc(pod, nodeNamesForPredicate)
	hostPriorityList = priorityFunc(pod, nodeNamesForPriority)

	if len(*extenderFilterResult.NodeNames) != 2 {
		f.t.Fatalf("Filter Result is wrong!")
	}

	var expectScores []int = []int{0, 7, 5, 0}
	for i, actualScore := range hostPriorityList {
		if actualScore.Score != int64(expectScores[i]) {
			f.t.Fatalf("Priority Result is wrong, expect %d, actual %d", expectScores[i], actualScore.Score)
		}
	}
}

func TestVGWithNoName(t *testing.T) {
	f.setT(t)

	var extenderFilterResult schedulerapi.ExtenderFilterResult
	var hostPriorityList schedulerapi.HostPriorityList
	pod := getTestPod(PVCWithoutVG)
	nodeNames := NodeNamesAll

	extenderFilterResult = predicateFunc(pod, nodeNames)
	hostPriorityList = priorityFunc(pod, nodeNames)

	if len(*extenderFilterResult.NodeNames) != 2 {
		f.t.Fatalf("Filter Result is wrong!")
	}
	var scores []int = []int{8, 5, 0, 0}
	for i, priScore := range hostPriorityList {
		if priScore.Score != int64(scores[i]) {
			f.t.Fatalf("Priority Result is wrong!")
		}
	}
}

func TestMountPoint(t *testing.T) {
	f.setT(t)

	var extenderFilterResult schedulerapi.ExtenderFilterResult
	var hostPriorityList schedulerapi.HostPriorityList
	pod := getTestPod(PVCWithMountPoint)
	nodeNames := NodeNamesAll

	extenderFilterResult = predicateFunc(pod, nodeNames)
	hostPriorityList = priorityFunc(pod, nodeNames)

	if len(*extenderFilterResult.NodeNames) != 1 {
		f.t.Fatalf("Filter Result is wrong!")
	}
	var scores []int = []int{5, 0, 10, 0}
	log.Infof("hostPriorityList: %#v", hostPriorityList)

	for i, priScore := range hostPriorityList {
		if priScore.Score != int64(scores[i]) {
			f.t.Fatalf("Priority Result is wrong(index=%d)! expect %d, actual %d", i, scores[i], priScore.Score)
		}
	}
}

func TestDevice(t *testing.T) {
	f.setT(t)

	var extenderFilterResult schedulerapi.ExtenderFilterResult
	var hostPriorityList schedulerapi.HostPriorityList
	pod := getTestPod(PVCWithDevice)
	nodeNames := NodeNamesAll

	extenderFilterResult = predicateFunc(pod, nodeNames)
	hostPriorityList = priorityFunc(pod, nodeNames)

	if len(*extenderFilterResult.NodeNames) != 1 {
		f.t.Fatalf("Filter Result is wrong!")
	}
	var scores []int = []int{0, 0, 11, 0}
	log.Infof("hostPriorityList: %#v", hostPriorityList)
	for i, priScore := range hostPriorityList {
		if priScore.Score != int64(scores[i]) {
			f.t.Fatalf("Priority Result is wrong(index=%d)! expect %d, actual %d", i, scores[i], priScore.Score)
		}
	}
}

// 测试使用非Open-Local PVC的Pod是否调度到非Open-Local节点上
func TestNoLocal(t *testing.T) {
	f.setT(t)

	var extenderFilterResult schedulerapi.ExtenderFilterResult
	var hostPriorityList schedulerapi.HostPriorityList
	pod := getTestPod(PVCNoLocal)
	nodeNames := NodeNamesAll

	extenderFilterResult = predicateFunc(pod, nodeNames)
	hostPriorityList = priorityFunc(pod, nodeNames)

	if len(*extenderFilterResult.NodeNames) != 4 {
		f.t.Fatalf("Filter Result is wrong!")
	}
	var scores []int = []int{0, 0, 0, 10}
	for i, priScore := range hostPriorityList {
		if priScore.Score != int64(scores[i]) {
			f.t.Fatalf("Priority Result is wrong!")
		}
	}
}

func TestUpdateCR(t *testing.T) {
	f.setT(t)

	updateCR := &localv1alpha1.NodeLocalStorage{
		TypeMeta: metav1.TypeMeta{APIVersion: localv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: NodeName1,
		},
		Spec: localv1alpha1.NodeLocalStorageSpec{
			NodeName: NodeName1,
			ListConfig: localv1alpha1.ListConfig{
				VGs: localv1alpha1.VGList{
					Include: []string{VGHDD, VGSSD},
				},
			},
		},
		Status: localv1alpha1.NodeLocalStorageStatus{
			NodeStorageInfo: localv1alpha1.NodeStorageInfo{
				VolumeGroups: []localv1alpha1.VolumeGroup{
					{
						Name:            VGSSD,
						PhysicalVolumes: []string{},
						LogicalVolumes:  []localv1alpha1.LogicalVolume{},
						Total:           100 * LocalGi,
						Available:       100 * LocalGi,
						Allocatable:     100 * LocalGi,
					},
					{
						Name:            VGHDD,
						PhysicalVolumes: []string{},
						LogicalVolumes:  []localv1alpha1.LogicalVolume{},
						Total:           500 * LocalGi,
						Available:       500 * LocalGi,
						Allocatable:     500 * LocalGi,
					},
				},
				MountPoints: []localv1alpha1.MountPoint{
					{
						Name:      "/mnt/open-local/testmnt-node1-a",
						Total:     200 * LocalGi,
						Available: 200 * LocalGi,
						FsType:    "ext4",
						Options:   []string{"rw", "ordered"},
						Device:    "/dev/sdb",
						ReadOnly:  false,
					},
					{
						Name:      "/mnt/open-local/testmnt-node1-b",
						Total:     150 * LocalGi,
						Available: 150 * LocalGi,
						FsType:    "ext4",
						Options:   []string{"rw", "ordered"},
						Device:    "/dev/sdc",
						ReadOnly:  false,
					},
				},
				DeviceInfos: []localv1alpha1.DeviceInfo{
					{
						Name:      "/dev/sda",
						MediaType: "hdd",
						Total:     100 * LocalGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdb",
						MediaType: string(localtype.MediaTypeSSD),
						Total:     200 * LocalGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdc",
						MediaType: string(localtype.MediaTypeHDD),
						Total:     150 * LocalGi,
						ReadOnly:  false,
					},
				},
			},
			FilteredStorageInfo: localv1alpha1.FilteredStorageInfo{
				VolumeGroups: []string{
					VGSSD,
					VGHDD,
				},
			},
		},
	}

	// TODO(huizhi): don't know why this does not trigger scheduler onNodeLocalStorageAdd function
	if _, err := f.localclient.CsiV1alpha1().NodeLocalStorages().Update(context.TODO(), updateCR, metav1.UpdateOptions{}); err != nil {
		f.t.Errorf(err.Error())
	}
	time.Sleep(2 * time.Second)
}

func predicateFunc(pod *corev1.Pod, nodeNames []string) (extenderFilterResult schedulerapi.ExtenderFilterResult) {
	var extenderArgs schedulerapi.ExtenderArgs

	extenderArgs.NodeNames = &nodeNames
	extenderArgs.Pod = pod

	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(extenderArgs)
	if err != nil {
		f.t.Fatal(err)
	}

	url := fmt.Sprintf("http://localhost:%d/scheduler/predicates", TestPort)
	resp, err := http.Post(url, "application/json", b)
	if err != nil {
		f.t.Fatal(err.Error())
	}

	err = json.NewDecoder(resp.Body).Decode(&extenderFilterResult)
	if err != nil {
		f.t.Fatal(err)
	}

	return
}

func priorityFunc(pod *corev1.Pod, nodeNames []string) (hostPriorityList schedulerapi.HostPriorityList) {
	var extenderArgs schedulerapi.ExtenderArgs

	extenderArgs.NodeNames = &nodeNames
	extenderArgs.Pod = pod

	url := fmt.Sprintf("http://localhost:%d/scheduler/priorities", TestPort)
	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(extenderArgs)
	if err != nil {
		f.t.Fatal(err)
	}
	resp, err := http.Post(url, "application/json", b)
	if err != nil {
		f.t.Skip(err)
	}
	err = json.NewDecoder(resp.Body).Decode(&hostPriorityList)
	if err != nil {
		f.t.Fatal(err)
	}
	return
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.localobjects = []runtime.Object{}
	f.kubeobjects = []runtime.Object{}
	f.snapobjects = []runtime.Object{}
	return f
}

func (f *fixture) setT(t *testing.T) {
	if f == nil {
		return
	}
	f.t = t
}

func newNode() (nodes []*corev1.Node) {
	nodeNames := NodeNamesAll
	for _, nodeName := range nodeNames {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
		}
		nodes = append(nodes, node)
	}

	return nodes
}

func newNodeLocalStorage() (crds []*localv1alpha1.NodeLocalStorage) {
	return utils.CreateTestNodeLocalStorage()
}

func newPersistentVolumeClaim() (pvcs []*corev1.PersistentVolumeClaim) {
	var pvcInfos = []utils.TestPVCInfo{
		{
			PVCName:      PVCWithVG,
			PVCNameSpace: LocalNameSpace,
			Size:         "150Gi",
			SCName:       SCLVMWithVG,
			PVCStatus:    corev1.ClaimPending,
		},
		{
			PVCName:      PVCWithoutVG,
			PVCNameSpace: LocalNameSpace,
			Size:         "400Gi",
			SCName:       SCLVMWithoutVG,
			PVCStatus:    corev1.ClaimPending,
		},
		{
			PVCName:      PVCWithMountPoint,
			PVCNameSpace: LocalNameSpace,
			Size:         "500Gi",
			SCName:       SCWithMP,
			PVCStatus:    corev1.ClaimPending,
		},
		{
			PVCName:      PVCWithDevice,
			PVCNameSpace: LocalNameSpace,
			Size:         "100Gi",
			SCName:       SCWithDevice,
			PVCStatus:    corev1.ClaimPending,
		},
		{
			PVCName:      PVCNoLocal,
			PVCNameSpace: LocalNameSpace,
			Size:         "100Gi",
			SCName:       SCNoLocal,
			PVCStatus:    corev1.ClaimPending,
		},
	}
	return utils.CreateTestPersistentVolumeClaim(pvcInfos)
}

func newStorageClass() (scs []*storagev1.StorageClass) {
	return utils.CreateTestStorageClass()
}

func (f *fixture) newExtender() (*server.ExtenderServer, kubeinformers.SharedInformerFactory, localinformers.SharedInformerFactory, volumesnapshotinformers.SharedInformerFactory) {
	f.localclient = localfake.NewSimpleClientset(f.localobjects...)
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)
	f.snapclient = volumesnapshotfake.NewSimpleClientset(f.snapobjects...)

	k8sInformer := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())
	localInformer := localinformers.NewSharedInformerFactory(f.localclient, noResyncPeriodFunc())
	snapInforer := volumesnapshotinformers.NewSharedInformerFactory(f.snapclient, noResyncPeriodFunc())

	extenderServer := server.NewExtenderServer(f.kubeclient, f.localclient, f.snapclient, k8sInformer, localInformer, snapInforer, TestPort, localtype.NewNodeAntiAffinityWeight())

	return extenderServer, k8sInformer, localInformer, snapInforer
}

func (f *fixture) runExtender() {
	// Init extender
	extenderServer, k8sInformer, localInformer, snapInformer := f.newExtender()
	stopCh := make(chan struct{})
	defer close(stopCh)

	k8sInformer.Start(stopCh)
	localInformer.Start(stopCh)
	snapInformer.Start(stopCh)
	extenderServer.InitRouter()
	extenderServer.WaitForCacheSync(stopCh)
}

func getTestPod(pvcName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PodName,
			Namespace: LocalNameSpace,
		},
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "testpvc",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}
}
