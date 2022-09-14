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
package utils

import (
	"encoding/json"

	localtype "github.com/alibaba/open-local/pkg"
	localv1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	//Pod
	InvolumePodName string = "involume_pod"

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
	PVCSnapshot       string = "pvc-snapshot"
	PVCWithVGError    string = "pvc-vg-error"
	PVCWithMountPoint string = "pvc-mp"
	PVCWithDevice     string = "pvc-device"
	PVCNoLocal        string = "pvc-nolocal"
	// Pod
	TestPodName string = "testpod"
)

var NodeNamesAll = []string{NodeName1, NodeName2, NodeName3, NodeName4}

func CreateTestNodeLocalStorage() (crds []*localv1alpha1.NodeLocalStorage) {
	node1 := CreateTestNodeLocalStorage1()
	node2 := CreateTestNodeLocalStorage2()
	node3 := CreateTestNodeLocalStorage3()
	node4 := CreateTestNodeLocalStorage4()
	crds = append(crds, node1, node2, node3, node4)
	return crds
}

func CreateTestNodeLocalStorage1() *localv1alpha1.NodeLocalStorage {
	return &localv1alpha1.NodeLocalStorage{
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
				MountPoints: localv1alpha1.MountPointList{
					Include: []string{"/mnt/open-local/testmnt-*"},
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
						Total:     500 * LocalGi,
						Available: 500 * LocalGi,
						FsType:    "ext4",
						Options:   []string{"rw", "ordered"},
						Device:    "/dev/sdb",
						ReadOnly:  false,
					},
				},
				DeviceInfos: []localv1alpha1.DeviceInfo{
					{
						Name:      "/dev/sda",
						MediaType: string(localtype.MediaTypeHDD),
						Total:     100 * LocalGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdb",
						MediaType: string(localtype.MediaTypeSSD),
						Total:     500 * LocalGi,
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
				VolumeGroups: []string{VGHDD, VGSSD},
				MountPoints:  []string{"/mnt/open-local/testmnt-node1-a"},
			},
		},
	}
}

func CreateTestNodeLocalStorage2() *localv1alpha1.NodeLocalStorage {
	return &localv1alpha1.NodeLocalStorage{
		TypeMeta: metav1.TypeMeta{APIVersion: localv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: NodeName2,
		},
		Spec: localv1alpha1.NodeLocalStorageSpec{
			NodeName: NodeName2,
			ListConfig: localv1alpha1.ListConfig{
				VGs: localv1alpha1.VGList{
					Include: []string{VGHDD, VGSSD},
				},
				MountPoints: localv1alpha1.MountPointList{
					Include: []string{"/mnt/open-local/testmnt-*"},
					Exclude: []string{"/mnt/open-local/testmnt-node1-a"},
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
						Total:           200 * LocalGi,
						Available:       200 * LocalGi,
						Allocatable:     200 * LocalGi,
					},
					{
						Name:            VGHDD,
						PhysicalVolumes: []string{},
						LogicalVolumes:  []localv1alpha1.LogicalVolume{},
						Total:           750 * LocalGi,
						Available:       750 * LocalGi,
						Allocatable:     750 * LocalGi,
					},
				},
				MountPoints: []localv1alpha1.MountPoint{
					{
						Name:      "/mnt/open-local/testmnt-node1-a",
						Total:     750 * LocalGi,
						Available: 750 * LocalGi,
						FsType:    "ext4",
						Options:   []string{"rw", "ordered"},
						Device:    "/dev/sdb",
						ReadOnly:  false,
					},
				},
				DeviceInfos: []localv1alpha1.DeviceInfo{
					{
						Name:      "/dev/sda",
						MediaType: string(localtype.MediaTypeHDD),
						Total:     100 * LocalGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdb",
						MediaType: string(localtype.MediaTypeHDD),
						Total:     300 * LocalGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdc",
						MediaType: string(localtype.MediaTypeHDD),
						Total:     400 * LocalGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdd",
						MediaType: string(localtype.MediaTypeHDD),
						Total:     500 * LocalGi,
						ReadOnly:  false,
					},
				},
			},
			FilteredStorageInfo: localv1alpha1.FilteredStorageInfo{
				VolumeGroups: []string{VGHDD, VGSSD},
			},
		},
	}
}

func CreateTestNodeLocalStorage3() *localv1alpha1.NodeLocalStorage {
	return &localv1alpha1.NodeLocalStorage{
		TypeMeta: metav1.TypeMeta{APIVersion: localv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: NodeName3,
		},
		Spec: localv1alpha1.NodeLocalStorageSpec{
			NodeName: NodeName3,
			ListConfig: localv1alpha1.ListConfig{
				VGs: localv1alpha1.VGList{
					Include: []string{VGSSD},
				},
				MountPoints: localv1alpha1.MountPointList{
					Include: []string{"/mnt/open-local/testmnt-*"},
				},
				Devices: localv1alpha1.DeviceList{
					Include: []string{"/dev/sdc"},
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
						Total:           300 * LocalGi,
						Available:       300 * LocalGi,
						Allocatable:     300 * LocalGi,
					},
				},
				MountPoints: []localv1alpha1.MountPoint{
					{
						Name:      "/mnt/open-local/testmnt-node1-a",
						Total:     1000 * LocalGi,
						Available: 1000 * LocalGi,
						FsType:    "ext4",
						Options:   []string{"rw", "ordered"},
						Device:    "/dev/sdb",
						ReadOnly:  false,
					},
				},
				DeviceInfos: []localv1alpha1.DeviceInfo{
					{
						Name:      "/dev/sda",
						MediaType: string(localtype.MediaTypeHDD),
						Total:     100 * LocalGi,
						ReadOnly:  false,
					},
					{
						Name:      "/dev/sdb",
						MediaType: string(localtype.MediaTypeHDD),
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
				VolumeGroups: []string{VGHDD, VGSSD},
				MountPoints:  []string{"/mnt/open-local/testmnt-node1-a"},
				Devices:      []string{"/dev/sdc"},
			},
		},
	}
}

func CreateTestNodeLocalStorage4() *localv1alpha1.NodeLocalStorage {
	return &localv1alpha1.NodeLocalStorage{
		TypeMeta: metav1.TypeMeta{APIVersion: localv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name: NodeName4,
		},
		Spec: localv1alpha1.NodeLocalStorageSpec{
			NodeName: NodeName4,
			ListConfig: localv1alpha1.ListConfig{
				VGs: localv1alpha1.VGList{
					Include: []string{VGSSD},
				},
			},
		},
		Status: localv1alpha1.NodeLocalStorageStatus{},
	}
}

type TestPVCInfo struct {
	PVCName      string
	PVCNameSpace string
	Size         string
	SCName       string
	VolumeName   string
	NodeName     string
	PVCStatus    corev1.PersistentVolumeClaimPhase

	IsSnapshot    bool
	SnapName      string
	SourcePVCName string
}

type TestPVInfo struct {
	VolumeName string
	VolumeSize string
	VolumeType string
	VgName     string
	DeviceName string
	NodeName   string
	ClaimRef   *corev1.ObjectReference
	PVStatus   corev1.PersistentVolumePhase
	IsSnapshot bool
	IsLocalPV  bool
}

type TestPVCPVInfo struct {
	PVCPending  *TestPVCInfo
	PVCBounding *TestPVCInfo
	PVBounding  *TestPVInfo
}

func (info *TestPVCPVInfo) SetSize(size string) {
	info.PVCPending.Size = size
	info.PVCBounding.Size = size
	info.PVBounding.VolumeSize = size
}

type TestPVCPVInfoList []*TestPVCPVInfo

func (list TestPVCPVInfoList) GetTestPVCPending() []TestPVCInfo {
	var pvcs []TestPVCInfo
	for _, info := range list {
		pvcs = append(pvcs, *info.PVCPending)
	}
	return pvcs
}
func (list TestPVCPVInfoList) GetTestPVCBounding() []TestPVCInfo {
	var pvcs []TestPVCInfo
	for _, info := range list {
		pvcs = append(pvcs, *info.PVCBounding)
	}
	return pvcs
}
func (list TestPVCPVInfoList) GetTestPVBounding() []TestPVInfo {
	var pvs []TestPVInfo
	for _, info := range list {
		pvs = append(pvs, *info.PVBounding)
	}
	return pvs
}

func GetTestPVCPVWithVG() *TestPVCPVInfo {
	return &TestPVCPVInfo{
		PVCPending: &TestPVCInfo{
			PVCName:      PVCWithVG,
			PVCNameSpace: LocalNameSpace,
			Size:         "150Gi",
			SCName:       SCLVMWithVG,
			PVCStatus:    corev1.ClaimPending,
			IsSnapshot:   false,
		},
		PVCBounding: &TestPVCInfo{
			PVCName:      PVCWithVG,
			PVCNameSpace: LocalNameSpace,
			Size:         "150Gi",
			SCName:       SCLVMWithVG,
			VolumeName:   "pv-" + PVCWithVG,
			NodeName:     NodeName3,
			PVCStatus:    corev1.ClaimBound,
			IsSnapshot:   false,
		},
		PVBounding: &TestPVInfo{
			VolumeName: "pv-" + PVCWithVG,
			VolumeSize: "150Gi",
			VolumeType: string(localtype.VolumeTypeLVM),
			VgName:     VGSSD,
			NodeName:   NodeName3,
			ClaimRef: &corev1.ObjectReference{
				Name:      PVCWithVG,
				Namespace: LocalNameSpace,
			},
			PVStatus:   corev1.VolumeBound,
			IsLocalPV:  true,
			IsSnapshot: false,
		},
	}
}

func GetTestPVCPVWithoutVG() *TestPVCPVInfo {
	return &TestPVCPVInfo{
		PVCPending: &TestPVCInfo{
			PVCName:      PVCWithoutVG,
			PVCNameSpace: LocalNameSpace,
			Size:         "40Gi",
			SCName:       SCLVMWithoutVG,
			PVCStatus:    corev1.ClaimPending,
			IsSnapshot:   false,
		},
		PVCBounding: &TestPVCInfo{
			PVCName:      PVCWithoutVG,
			PVCNameSpace: LocalNameSpace,
			VolumeName:   "pv-" + PVCWithoutVG,
			Size:         "40Gi",
			SCName:       SCLVMWithoutVG,
			NodeName:     NodeName3,
			PVCStatus:    corev1.ClaimBound,
			IsSnapshot:   false,
		},
		PVBounding: &TestPVInfo{
			VolumeName: "pv-" + PVCWithoutVG,
			VolumeSize: "40Gi",
			VolumeType: string(localtype.VolumeTypeLVM),
			VgName:     VGHDD,
			NodeName:   NodeName3,
			ClaimRef: &corev1.ObjectReference{
				Name:      PVCWithoutVG,
				Namespace: LocalNameSpace,
			},
			PVStatus:   corev1.VolumeBound,
			IsLocalPV:  true,
			IsSnapshot: false,
		},
	}
}

func GetTestPVCPVSnapshot() *TestPVCPVInfo {
	return &TestPVCPVInfo{
		PVCPending: &TestPVCInfo{
			PVCName:       PVCSnapshot,
			PVCNameSpace:  LocalNameSpace,
			Size:          "40Gi",
			SCName:        SCLVMWithoutVG,
			PVCStatus:     corev1.ClaimPending,
			IsSnapshot:    true,
			SnapName:      "snapshot-" + PVCSnapshot,
			SourcePVCName: PVCWithVG,
		},
		PVCBounding: &TestPVCInfo{
			PVCName:       PVCSnapshot,
			PVCNameSpace:  LocalNameSpace,
			VolumeName:    "pv-" + PVCWithDevice,
			Size:          "40Gi",
			SCName:        SCLVMWithoutVG,
			NodeName:      NodeName3,
			PVCStatus:     corev1.ClaimBound,
			IsSnapshot:    true,
			SnapName:      "snapshot-" + PVCSnapshot,
			SourcePVCName: PVCWithVG,
		},
		PVBounding: &TestPVInfo{
			VolumeName: "pv-" + PVCSnapshot,
			VolumeSize: "40Gi",
			VolumeType: string(localtype.VolumeTypeLVM),
			NodeName:   NodeName3,
			ClaimRef: &corev1.ObjectReference{
				Name:      PVCSnapshot,
				Namespace: LocalNameSpace,
			},
			PVStatus:   corev1.VolumeBound,
			IsLocalPV:  true,
			IsSnapshot: true,
		},
	}
}

func GetTestPVCPVNotLocal() *TestPVCPVInfo {
	return &TestPVCPVInfo{
		PVCPending: &TestPVCInfo{
			PVCName:      PVCNoLocal,
			PVCNameSpace: LocalNameSpace,
			Size:         "100Gi",
			SCName:       SCNoLocal,
			PVCStatus:    corev1.ClaimPending,
			IsSnapshot:   false,
		},
		PVCBounding: &TestPVCInfo{
			PVCName:      PVCNoLocal,
			PVCNameSpace: LocalNameSpace,
			VolumeName:   "pv-" + PVCWithDevice,
			Size:         "100Gi",
			SCName:       SCNoLocal,
			NodeName:     NodeName3,
			PVCStatus:    corev1.ClaimBound,
			IsSnapshot:   false,
		},
		PVBounding: &TestPVInfo{
			VolumeName: "pv-" + PVCNoLocal,
			VolumeSize: "100Gi",
			NodeName:   NodeName3,
			ClaimRef: &corev1.ObjectReference{
				Name:      PVCNoLocal,
				Namespace: LocalNameSpace,
			},
			PVStatus:   corev1.VolumeBound,
			IsLocalPV:  false,
			IsSnapshot: false,
		},
	}
}

func GetTestPVCPVDevice() *TestPVCPVInfo {
	return &TestPVCPVInfo{
		PVCPending: &TestPVCInfo{
			PVCName:      PVCWithDevice,
			PVCNameSpace: LocalNameSpace,
			Size:         "100Gi",
			SCName:       SCWithDevice,
			PVCStatus:    corev1.ClaimPending,
			IsSnapshot:   false,
		},
		PVCBounding: &TestPVCInfo{
			PVCName:      PVCWithDevice,
			PVCNameSpace: LocalNameSpace,
			VolumeName:   "pv-" + PVCWithDevice,
			Size:         "100Gi",
			SCName:       SCWithDevice,
			NodeName:     NodeName3,
			PVCStatus:    corev1.ClaimBound,
			IsSnapshot:   false,
		},
		PVBounding: &TestPVInfo{
			VolumeName: "pv-" + PVCWithDevice,
			VolumeSize: "100Gi",
			VolumeType: string(localtype.VolumeTypeDevice),
			DeviceName: "/dev/sdc",
			NodeName:   NodeName3,
			ClaimRef: &corev1.ObjectReference{
				Name:      PVCWithDevice,
				Namespace: LocalNameSpace,
			},
			PVStatus:   corev1.VolumeBound,
			IsLocalPV:  true,
			IsSnapshot: false,
		},
	}
}

func CreateTestPersistentVolumeClaim(pvcInfos []TestPVCInfo) (pvcs []*corev1.PersistentVolumeClaim) {
	for i, pvcInfo := range pvcInfos {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcInfo.PVCName,
				Namespace: LocalNameSpace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: &pvcInfos[i].SCName,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(pvcInfo.Size),
					},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: pvcInfo.PVCStatus,
			},
		}
		if pvcInfo.NodeName != "" {
			pvc.Annotations = map[string]string{
				localtype.AnnoSelectedNode: pvcInfo.NodeName,
			}
		}

		if pvcInfo.VolumeName != "" {
			pvc.Spec.VolumeName = pvcInfo.VolumeName
		}

		if pvcInfo.IsSnapshot {
			pvc.Spec.DataSource = &corev1.TypedLocalObjectReference{
				Kind: "VolumeSnapshot",
				Name: pvcInfo.SnapName,
			}
		}

		pvcs = append(pvcs, pvc)
	}
	return pvcs
}

func CreatePVsBound() []*corev1.PersistentVolume {
	pvInfos := []TestPVInfo{
		*GetTestPVCPVWithVG().PVBounding,
		*GetTestPVCPVWithoutVG().PVBounding,
		*GetTestPVCPVDevice().PVBounding,
		*GetTestPVCPVNotLocal().PVBounding,
	}
	return CreateTestPersistentVolume(pvInfos)
}

func CreateTestPersistentVolume(pvInfos []TestPVInfo) (pvs []*corev1.PersistentVolume) {
	for _, pvInfo := range pvInfos {
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: pvInfo.VolumeName,
			},
			Spec: corev1.PersistentVolumeSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Capacity: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(pvInfo.VolumeSize),
				},
				ClaimRef: pvInfo.ClaimRef,
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						VolumeAttributes: map[string]string{},
					},
				},
			},
			Status: corev1.PersistentVolumeStatus{
				Phase: pvInfo.PVStatus,
			},
		}
		if pvInfo.VgName != "" {
			allocateInfo := localtype.PVAllocatedInfo{VGName: pvInfo.VgName, VolumeType: pvInfo.VolumeType}
			infoJson, _ := json.Marshal(allocateInfo)
			pv.Annotations = map[string]string{
				localtype.AnnotationPVAllocatedInfoKey: string(infoJson),
			}
		}

		if pvInfo.DeviceName != "" {
			allocateInfo := localtype.PVAllocatedInfo{DeviceName: pvInfo.DeviceName, VolumeType: pvInfo.VolumeType}
			infoJson, _ := json.Marshal(allocateInfo)
			pv.Annotations = map[string]string{
				localtype.AnnotationPVAllocatedInfoKey: string(infoJson),
			}
		}

		if pvInfo.IsLocalPV {
			pv.Spec.CSI.Driver = localtype.ProvisionerNameYoda
			pv.Spec.CSI.VolumeAttributes[localtype.VolumeTypeKey] = pvInfo.VolumeType
		}
		if pvInfo.IsSnapshot {
			pv.Spec.CSI.VolumeAttributes[localtype.ParamSnapshotName] = "snapshot"
		}

		if pvInfo.NodeName != "" {
			pv.Spec.NodeAffinity = &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      localtype.KubernetesNodeIdentityKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{pvInfo.NodeName},
								},
							},
						},
					},
				},
			}
		}

		pvs = append(pvs, pv)
	}
	return
}

func CreateTestStorageClass() (scs []*storagev1.StorageClass) {
	// storage class: special vg
	param1 := make(map[string]string)
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
	param2 := make(map[string]string)
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
	param3 := make(map[string]string)
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
	param4 := make(map[string]string)
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
	scWithNoLocal := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: SCNoLocal,
		},
		Provisioner: "kubernetes.io/no-provisioner",
	}

	scs = append(scs, scLVMWithVG, scLVMWithoutVG, scWithMP, scWithDevice, scWithNoLocal)

	return scs
}

type TestInlineVolumeInfo struct {
	VolumeName string
	VolumeSize string
	VgName     string
}

type TestPodInfo struct {
	PodName      string
	PodNameSpace string
	NodeName     string
	PodStatus    corev1.PodPhase

	PVCInfos          []*TestPVCInfo
	InlineVolumeInfos []*TestInlineVolumeInfo
}

func CreatePod(podInfo *TestPodInfo) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID(podInfo.PodName),
			Name:      podInfo.PodName,
			Namespace: podInfo.PodNameSpace,
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{},
		},
		Status: corev1.PodStatus{
			Phase: podInfo.PodStatus,
		},
	}

	if podInfo.NodeName != "" {
		pod.Spec.NodeName = podInfo.NodeName
	}

	for _, inline := range podInfo.InlineVolumeInfos {
		inlineVolume := corev1.Volume{
			Name: inline.VolumeName,
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					Driver: localtype.ProvisionerNameYoda,
					VolumeAttributes: map[string]string{
						localtype.VGName:      inline.VgName,
						localtype.ParamLVSize: inline.VolumeSize,
					},
				},
			},
		}
		pod.Spec.Volumes = append(pod.Spec.Volumes, inlineVolume)
	}

	for _, pvcInfo := range podInfo.PVCInfos {
		pv := corev1.Volume{
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcInfo.PVCName,
					ReadOnly:  false,
				},
			},
		}
		pod.Spec.Volumes = append(pod.Spec.Volumes, pv)
	}

	return pod
}

type TestNodeInfo struct {
	NodeName  string
	IPAddress string
}

func CreateNode(nodeInfo *TestNodeInfo) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeInfo.NodeName,
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: nodeInfo.IPAddress,
				},
			},
		},
	}
}
