/*
Copyright 2021 OECP Authors.

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
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	volumesnapshotfake "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned/fake"
	volumesnapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v3/informers/externalversions"
	localtype "github.com/oecp/open-local/pkg"
	lssv1alpha1 "github.com/oecp/open-local/pkg/apis/storage/v1alpha1"
	lssfake "github.com/oecp/open-local/pkg/generated/clientset/versioned/fake"
	lssinformers "github.com/oecp/open-local/pkg/generated/informers/externalversions"
	"github.com/oecp/open-local/pkg/scheduler/server"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	log "k8s.io/klog"
	schedulerapi "k8s.io/kube-scheduler/extender/v1"
)

var (
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

const (
	// General
	LSSGi        uint64 = 1024 * 1024 * 1024
	LSSMi        uint64 = 1024 * 1024
	TestPort     int32  = 23000
	LSSNameSpace string = "default"
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
	SCNoLSS        string = "sc-nolss"
	// PVC
	PVCWithVG         string = "pvc-vg"
	PVCWithoutVG      string = "pvc-novg"
	PVCWithVGError    string = "pvc-vg-error"
	PVCWithMountPoint string = "pvc-mp"
	PVCWithDevice     string = "pvc-device"
	PVCNoLSS          string = "pvc-nolss"
	// Pod
	PodName string = "testpod"
)

var NodeNamesAll []string = []string{NodeName1, NodeName2, NodeName3, NodeName4}

type fixture struct {
	t *testing.T

	kubeclient *k8sfake.Clientset
	lssclient  *lssfake.Clientset
	snapclient *volumesnapshotfake.Clientset

	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object
	lssobjects  []runtime.Object
	snapobjects []runtime.Object
}

var f *fixture

func init() {
	f = newFixture(nil)

	nodes := newNode()
	crds := newNodeLocalStorage()
	scs := newStorageClass()
	pvcs := newPersistentVolumeClaim()

	for _, crd := range crds {
		f.lssobjects = append(f.lssobjects, crd)
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

	var scores []int = []int{0, 7, 5, 0}
	for i, priScore := range hostPriorityList {
		if priScore.Score != int64(scores[i]) {
			f.t.Fatalf("Priority Result is wrong!")
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

// 测试使用非LSS PVC的Pod是否调度到非LSS节点上
func TestNoLSS(t *testing.T) {
	f.setT(t)

	var extenderFilterResult schedulerapi.ExtenderFilterResult
	var hostPriorityList schedulerapi.HostPriorityList
	pod := getTestPod(PVCNoLSS)
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

	updateCR := &lssv1alpha1.NodeLocalStorage{
		TypeMeta: metav1.TypeMeta{APIVersion: lssv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: NodeName1,
		},
		Spec: lssv1alpha1.NodeLocalStorageSpec{
			NodeName: NodeName1,
			ListConfig: lssv1alpha1.ListConfig{
				VGs: lssv1alpha1.VGList{
					Include: []string{VGHDD, VGSSD},
				},
			},
		},
		Status: lssv1alpha1.NodeLocalStorageStatus{
			NodeStorageInfo: lssv1alpha1.NodeStorageInfo{
				VolumeGroups: []lssv1alpha1.VolumeGroup{
					{
						Name:            VGSSD,
						PhysicalVolumes: []string{},
						LogicalVolumes:  []lssv1alpha1.LogicalVolume{},
						Total:           100 * LSSGi,
						Available:       100 * LSSGi,
						Allocatable:     100 * LSSGi,
					},
					{
						Name:            VGHDD,
						PhysicalVolumes: []string{},
						LogicalVolumes:  []lssv1alpha1.LogicalVolume{},
						Total:           500 * LSSGi,
						Available:       500 * LSSGi,
						Allocatable:     500 * LSSGi,
					},
				},
				MountPoints: []lssv1alpha1.MountPoint{
					{
						Name:      "/mnt/lss/testmnt-node1-a",
						Total:     200 * LSSGi,
						Available: 200 * LSSGi,
						FsType:    "ext4",
						Options:   []string{"rw", "ordered"},
						Device:    "/dev/sdb",
						ReadOnly:  false,
					},
					{
						Name:      "/mnt/lss/testmnt-node1-b",
						Total:     150 * LSSGi,
						Available: 150 * LSSGi,
						FsType:    "ext4",
						Options:   []string{"rw", "ordered"},
						Device:    "/dev/sdc",
						ReadOnly:  false,
					},
				},
				DeviceInfos: []lssv1alpha1.DeviceInfo{
					{
						Name:      "/dev/sda",
						MediaType: "hdd",
						Total:     100 * LSSGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdb",
						MediaType: string(localtype.MediaTypeSSD),
						Total:     200 * LSSGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdc",
						MediaType: string(localtype.MediaTypeHHD),
						Total:     150 * LSSGi,
						ReadOnly:  false,
					},
				},
			},
			FilteredStorageInfo: lssv1alpha1.FilteredStorageInfo{
				VolumeGroups: []string{
					VGSSD,
					VGHDD,
				},
			},
		},
	}

	// TODO(huizhi): don't know why this does not trigger scheduler onNodeLocalStorageAdd function
	if _, err := f.lssclient.StorageV1alpha1().NodeLocalStorages().Update(context.TODO(), updateCR, metav1.UpdateOptions{}); err != nil {
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
	f.lssobjects = []runtime.Object{}
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

type VGInfo struct {
	Name  string
	Total uint64
}
type MPInfo struct {
	Name     string
	Total    uint64
	FsType   string
	Options  []string
	Device   string
	ReadOnly bool
}
type DeviceInfo struct {
	Name      string
	MediaType localtype.MediaType
	Total     uint64
	ReadOnly  bool
}
type NodeInfo struct {
	Name             string
	WhitelistVGs     []string
	WhitelistDevices []string
	BlacklistMPs     []string
	VGInfos          []VGInfo
	MPInfos          []MPInfo
	DeviceInfos      []DeviceInfo
}

func newNodeLocalStorage() (crds []*lssv1alpha1.NodeLocalStorage) {
	node1 := &lssv1alpha1.NodeLocalStorage{
		TypeMeta: metav1.TypeMeta{APIVersion: lssv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: NodeName1,
		},
		Spec: lssv1alpha1.NodeLocalStorageSpec{
			NodeName: NodeName1,
			ListConfig: lssv1alpha1.ListConfig{
				VGs: lssv1alpha1.VGList{
					Include: []string{VGHDD, VGSSD},
				},
				MountPoints: lssv1alpha1.MountPointList{
					Include: []string{"/mnt/lss/testmnt-*"},
				},
			},
		},
		Status: lssv1alpha1.NodeLocalStorageStatus{
			NodeStorageInfo: lssv1alpha1.NodeStorageInfo{
				VolumeGroups: []lssv1alpha1.VolumeGroup{
					{
						Name:            VGSSD,
						PhysicalVolumes: []string{},
						LogicalVolumes:  []lssv1alpha1.LogicalVolume{},
						Total:           100 * LSSGi,
						Available:       100 * LSSGi,
						Allocatable:     100 * LSSGi,
					},
					{
						Name:            VGHDD,
						PhysicalVolumes: []string{},
						LogicalVolumes:  []lssv1alpha1.LogicalVolume{},
						Total:           500 * LSSGi,
						Available:       500 * LSSGi,
						Allocatable:     500 * LSSGi,
					},
				},
				MountPoints: []lssv1alpha1.MountPoint{
					{
						Name:      "/mnt/lss/testmnt-node1-a",
						Total:     500 * LSSGi,
						Available: 500 * LSSGi,
						FsType:    "ext4",
						Options:   []string{"rw", "ordered"},
						Device:    "/dev/sdb",
						ReadOnly:  false,
					},
				},
				DeviceInfos: []lssv1alpha1.DeviceInfo{
					{
						Name:      "/dev/sda",
						MediaType: string(localtype.MediaTypeHHD),
						Total:     100 * LSSGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdb",
						MediaType: string(localtype.MediaTypeSSD),
						Total:     500 * LSSGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdc",
						MediaType: string(localtype.MediaTypeHHD),
						Total:     150 * LSSGi,
						ReadOnly:  false,
					},
				},
			},
			FilteredStorageInfo: lssv1alpha1.FilteredStorageInfo{
				VolumeGroups: []string{VGHDD, VGSSD},
				MountPoints:  []string{"/mnt/lss/testmnt-node1-a"},
			},
		},
	}
	node2 := &lssv1alpha1.NodeLocalStorage{
		TypeMeta: metav1.TypeMeta{APIVersion: lssv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: NodeName2,
		},
		Spec: lssv1alpha1.NodeLocalStorageSpec{
			NodeName: NodeName2,
			ListConfig: lssv1alpha1.ListConfig{
				VGs: lssv1alpha1.VGList{
					Include: []string{VGHDD, VGSSD},
				},
				MountPoints: lssv1alpha1.MountPointList{
					Include: []string{"/mnt/lss/testmnt-*"},
					Exclude: []string{"/mnt/lss/testmnt-node1-a"},
				},
			},
		},
		Status: lssv1alpha1.NodeLocalStorageStatus{
			NodeStorageInfo: lssv1alpha1.NodeStorageInfo{
				VolumeGroups: []lssv1alpha1.VolumeGroup{
					{
						Name:            VGSSD,
						PhysicalVolumes: []string{},
						LogicalVolumes:  []lssv1alpha1.LogicalVolume{},
						Total:           200 * LSSGi,
						Available:       200 * LSSGi,
						Allocatable:     200 * LSSGi,
					},
					{
						Name:            VGHDD,
						PhysicalVolumes: []string{},
						LogicalVolumes:  []lssv1alpha1.LogicalVolume{},
						Total:           750 * LSSGi,
						Available:       750 * LSSGi,
						Allocatable:     750 * LSSGi,
					},
				},
				MountPoints: []lssv1alpha1.MountPoint{
					{
						Name:      "/mnt/lss/testmnt-node1-a",
						Total:     750 * LSSGi,
						Available: 750 * LSSGi,
						FsType:    "ext4",
						Options:   []string{"rw", "ordered"},
						Device:    "/dev/sdb",
						ReadOnly:  false,
					},
				},
				DeviceInfos: []lssv1alpha1.DeviceInfo{
					{
						Name:      "/dev/sda",
						MediaType: string(localtype.MediaTypeHHD),
						Total:     100 * LSSGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdb",
						MediaType: string(localtype.MediaTypeHHD),
						Total:     200 * LSSGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdc",
						MediaType: string(localtype.MediaTypeHHD),
						Total:     150 * LSSGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdd",
						MediaType: string(localtype.MediaTypeHHD),
						Total:     100 * LSSGi,
						ReadOnly:  false,
					},
				},
			},
			FilteredStorageInfo: lssv1alpha1.FilteredStorageInfo{
				VolumeGroups: []string{VGHDD, VGSSD},
			},
		},
	}
	node3 := &lssv1alpha1.NodeLocalStorage{
		TypeMeta: metav1.TypeMeta{APIVersion: lssv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: NodeName3,
		},
		Spec: lssv1alpha1.NodeLocalStorageSpec{
			NodeName: NodeName3,
			ListConfig: lssv1alpha1.ListConfig{
				VGs: lssv1alpha1.VGList{
					Include: []string{VGSSD},
				},
				MountPoints: lssv1alpha1.MountPointList{
					Include: []string{"/mnt/lss/testmnt-*"},
				},
				Devices: lssv1alpha1.DeviceList{
					Include: []string{"/dev/sdc"},
				},
			},
		},
		Status: lssv1alpha1.NodeLocalStorageStatus{
			NodeStorageInfo: lssv1alpha1.NodeStorageInfo{
				VolumeGroups: []lssv1alpha1.VolumeGroup{
					{
						Name:            VGSSD,
						PhysicalVolumes: []string{},
						LogicalVolumes:  []lssv1alpha1.LogicalVolume{},
						Total:           300 * LSSGi,
						Available:       300 * LSSGi,
						Allocatable:     300 * LSSGi,
					},
				},
				MountPoints: []lssv1alpha1.MountPoint{
					{
						Name:      "/mnt/lss/testmnt-node1-a",
						Total:     1000 * LSSGi,
						Available: 1000 * LSSGi,
						FsType:    "ext4",
						Options:   []string{"rw", "ordered"},
						Device:    "/dev/sdb",
						ReadOnly:  false,
					},
				},
				DeviceInfos: []lssv1alpha1.DeviceInfo{
					{
						Name:      "/dev/sda",
						MediaType: string(localtype.MediaTypeHHD),
						Total:     100 * LSSGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdb",
						MediaType: string(localtype.MediaTypeHHD),
						Total:     200 * LSSGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdc",
						MediaType: string(localtype.MediaTypeHHD),
						Total:     150 * LSSGi,
						ReadOnly:  false,
					},
				},
			},
			FilteredStorageInfo: lssv1alpha1.FilteredStorageInfo{
				VolumeGroups: []string{VGHDD, VGSSD},
				MountPoints:  []string{"/mnt/lss/testmnt-node1-a"},
				Devices:      []string{"/dev/sdc"},
			},
		},
	}
	node4 := &lssv1alpha1.NodeLocalStorage{
		TypeMeta: metav1.TypeMeta{APIVersion: lssv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: NodeName4,
		},
		Spec: lssv1alpha1.NodeLocalStorageSpec{
			NodeName: NodeName4,
			ListConfig: lssv1alpha1.ListConfig{
				VGs: lssv1alpha1.VGList{
					Include: []string{VGSSD},
				},
			},
		},
		Status: lssv1alpha1.NodeLocalStorageStatus{},
	}
	crds = append(crds, node1, node2, node3, node4)
	return crds
}

type PVCInfo struct {
	pvcName string
	size    string
	scName  string
}

func newPersistentVolumeClaim() (pvcs []*corev1.PersistentVolumeClaim) {
	var pvcInfos []PVCInfo = []PVCInfo{
		{
			pvcName: PVCWithVG,
			size:    "150Gi",
			scName:  SCLVMWithVG,
		},
		{
			pvcName: PVCWithoutVG,
			size:    "400Gi",
			scName:  SCLVMWithoutVG,
		},
		{
			pvcName: PVCWithMountPoint,
			size:    "500Gi",
			scName:  SCWithMP,
		},
		{
			pvcName: PVCWithDevice,
			size:    "100Gi",
			scName:  SCWithDevice,
		},
		{
			pvcName: PVCNoLSS,
			size:    "100Gi",
			scName:  SCNoLSS,
		},
	}

	for i, pvcInfo := range pvcInfos {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcInfo.pvcName,
				Namespace: LSSNameSpace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: &pvcInfos[i].scName,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(pvcInfo.size),
					},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimPending,
			},
		}
		pvcs = append(pvcs, pvc)
	}
	return pvcs
}

func newStorageClass() (scs []*storagev1.StorageClass) {
	// storage class: special vg
	var param1 map[string]string
	param1 = make(map[string]string, 0)
	param1["fs"] = "ext4"
	param1["vgName"] = VGSSD
	param1["volumeType"] = string(localtype.VolumeTypeLVM)
	scLVMWithVG := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: SCLVMWithVG,
		},
		Provisioner: localtype.ProvisionerNameYoda,
		Parameters:  param1,
	}
	// storage class: no vg
	var param2 map[string]string
	param2 = make(map[string]string, 0)
	param2["fs"] = "ext4"
	param2["volumeType"] = string(localtype.VolumeTypeLVM)
	scLVMWithoutVG := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: SCLVMWithoutVG,
		},
		Provisioner: localtype.ProvisionerNameYoda,
		Parameters:  param2,
	}

	// storage class: mount point
	var param3 map[string]string
	param3 = make(map[string]string, 0)
	param3["volumeType"] = string(localtype.VolumeTypeMountPoint)
	param3["mediaType"] = "hdd"
	scWithMP := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: SCWithMP,
		},
		Provisioner: localtype.ProvisionerNameYoda,
		Parameters:  param3,
	}

	// storage class: device
	var param4 map[string]string
	param4 = make(map[string]string, 0)
	param4["volumeType"] = string(localtype.VolumeTypeDevice)
	param4["mediaType"] = "hdd"
	scWithDevice := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: SCWithDevice,
		},
		Provisioner: localtype.ProvisionerNameYoda,
		Parameters:  param4,
	}

	// storage class: device
	scWithNoLSS := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: SCNoLSS,
		},
		Provisioner: "kubernetes.io/no-provisioner",
	}

	scs = append(scs, scLVMWithVG, scLVMWithoutVG, scWithMP, scWithDevice, scWithNoLSS)

	return scs
}

func (f *fixture) newExtender() (*server.ExtenderServer, kubeinformers.SharedInformerFactory, lssinformers.SharedInformerFactory, volumesnapshotinformers.SharedInformerFactory) {
	f.lssclient = lssfake.NewSimpleClientset(f.lssobjects...)
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)
	f.snapclient = volumesnapshotfake.NewSimpleClientset(f.snapobjects...)

	k8sInformer := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())
	lssInformer := lssinformers.NewSharedInformerFactory(f.lssclient, noResyncPeriodFunc())
	snapInforer := volumesnapshotinformers.NewSharedInformerFactory(f.snapclient, noResyncPeriodFunc())

	extenderServer := server.NewExtenderServer(f.kubeclient, f.lssclient, f.snapclient, k8sInformer, lssInformer, snapInforer, TestPort, localtype.NewNodeAntiAffinityWeight())

	return extenderServer, k8sInformer, lssInformer, snapInforer
}

func (f *fixture) runExtender() {
	// Init extender
	extenderServer, k8sInformer, lssInformer, snapInformer := f.newExtender()
	stopCh := make(chan struct{})
	defer close(stopCh)

	k8sInformer.Start(stopCh)
	lssInformer.Start(stopCh)
	snapInformer.Start(stopCh)
	extenderServer.InitRouter()
	extenderServer.WaitForCacheSync(stopCh)
}

func (f *fixture) httpGet(url string) []byte {
	resp, err := http.Get(url)
	if err != nil {
		f.t.Fatalf(err.Error())
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		f.t.Fatalf(err.Error())
	}

	return body
}

func (f *fixture) httpPost(url string) []byte {
	resp, err := http.Post(url, "application/x-www-form-urlencoded", strings.NewReader("name=cjb"))
	if err != nil {
		f.t.Fatalf(err.Error())
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		f.t.Fatalf(err.Error())
	}
	return body
}

func getTestPod(pvcName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PodName,
			Namespace: LSSNameSpace,
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
