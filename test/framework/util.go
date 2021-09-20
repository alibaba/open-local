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

package framework

import (
	"fmt"

	"github.com/alibaba/open-local/pkg"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MakeLVMPVC(name, ns string, sc *storagev1.StorageClass) *corev1.PersistentVolumeClaim {
	var scName *string
	if sc != nil {
		scName = &sc.Name
	} else {
		scName = &DefaultLVMSC.Name
	}
	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "",
					Kind:       "PersistentVolume",
					Name:       DefaultLVMPV.Name,
				},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: nil,
			Selector:    nil,
			Resources: corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("20Gi")},
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("20Gi")},
			},
			VolumeName:       "",
			StorageClassName: scName,
			VolumeMode:       nil,
			DataSource:       nil,
		},

		Status: corev1.PersistentVolumeClaimStatus{
			Phase:    corev1.ClaimBound,
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("20Gi")},
		},
	}
	return pvc
}

func MakeMPPVC(name, ns string, sc *storagev1.StorageClass) *corev1.PersistentVolumeClaim {
	var scName *string
	if sc != nil {
		scName = &sc.Name
	} else {
		scName = &DefaultMPSC.Name
	}
	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: make(map[string]string),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "",
					Kind:       "PersistentVolume",
					Name:       DefaultMPPV.Name,
				},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: nil,
			Selector:    nil,
			Resources: corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("200Gi")},
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("200Gi")},
			},
			VolumeName:       "",
			StorageClassName: scName,
			VolumeMode:       nil,
			DataSource:       nil,
		},

		Status: corev1.PersistentVolumeClaimStatus{
			Phase:    corev1.ClaimBound,
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("200Gi")},
		},
	}
	return pvc
}

func MakeDevicePVC(name, ns string, sc *storagev1.StorageClass) *corev1.PersistentVolumeClaim {
	var scName *string
	if sc != nil {
		scName = &sc.Name
	} else {
		scName = &DefaultDeviceSC.Name
	}
	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "",
					Kind:       "PersistentVolume",
					Name:       DefaultDeivcePV.Name,
				},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: nil,
			Selector:    nil,
			Resources: corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("200Gi")},
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("200Gi")},
			},
			VolumeName:       "",
			StorageClassName: scName,
			VolumeMode:       nil,
			DataSource:       nil,
		},

		Status: corev1.PersistentVolumeClaimStatus{
			Phase:    corev1.ClaimBound,
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("200Gi")},
		},
	}
	return pvc
}

func MakeLVMPV(name string, nodeName string) *corev1.PersistentVolume {
	return MakePV(name, nodeName, pkg.VolumeTypeLVM)
}

func MakeMPPV(name string, nodeName string) *corev1.PersistentVolume {
	return MakePV(name, nodeName, pkg.VolumeTypeMountPoint)
}

func MakeDevicePV(name string, nodeName string) *corev1.PersistentVolume {
	return MakePV(name, nodeName, pkg.VolumeTypeDevice)
}

func MakePV(name string, nodeName string, volumeType pkg.VolumeType) *corev1.PersistentVolume {
	var affinity *corev1.VolumeNodeAffinity
	if nodeName != "" {
		affinity = GenVolumeNodeAffinity(nodeName)
	}
	volumeAttributes := make(map[string]string)

	var scName string
	switch volumeType {
	case pkg.VolumeTypeLVM:
		scName = DefaultLVMSC.Name
		volumeAttributes[pkg.VGName] = DefaultVGName
	case pkg.VolumeTypeDevice:
		scName = DefaultDeviceSC.Name
	case pkg.VolumeTypeMountPoint:
		scName = DefaultMPSC.Name
	default:
		scName = DefaultLVMSC.Name
	}

	volumeAttributes[pkg.VolumeTypeKey] = string(volumeType)

	csi := corev1.PersistentVolumeSource{
		CSI: &corev1.CSIPersistentVolumeSource{
			Driver:               pkg.ProvisionerNameYoda,
			ReadOnly:             false,
			FSType:               pkg.VolumeFSTypeExt4,
			VolumeAttributes:     volumeAttributes,
			NodePublishSecretRef: nil,
		},
	}

	return &corev1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: csi,
			StorageClassName:       scName,
			Capacity:               corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("20Gi")},
			NodeAffinity:           affinity,
		},
		Status: corev1.PersistentVolumeStatus{},
	}
}

func MakePodMP(name, ns string, pvc *corev1.PersistentVolumeClaim) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "test-pvc",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc.Name,
						},
					},
				},
			},
		},
	}
}

// Returns a pod definition based on the namespace. The pod references the PVC's
// name.  A slice of BASH commands can be supplied as args to be run by the pod
func MakePod(name string, ns string, nodeSelector map[string]string, pvclaims []*corev1.PersistentVolumeClaim, isPrivileged bool, command string) *corev1.Pod {
	if len(command) == 0 {
		command = "trap exit TERM; while true; do sleep 1; done"
	}
	podSpec := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "corev1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:         name,
			GenerateName: "pvc-tester-",
			Namespace:    ns,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "write-pod",
					Image:   "BusyBoxImage",
					Command: []string{"/bin/sh"},
					Args:    []string{"-c", command},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &isPrivileged,
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyOnFailure,
		},
	}
	var volumeMounts = make([]corev1.VolumeMount, len(pvclaims))
	var volumes = make([]corev1.Volume, len(pvclaims))
	for index, pvclaim := range pvclaims {
		volumename := fmt.Sprintf("volume%v", index+1)
		volumeMounts[index] = corev1.VolumeMount{Name: volumename, MountPath: "/mnt/" + volumename}
		volumes[index] = corev1.Volume{Name: volumename, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvclaim.Name, ReadOnly: false}}}
	}
	podSpec.Spec.Containers[0].VolumeMounts = volumeMounts
	podSpec.Spec.Volumes = volumes
	if nodeSelector != nil {
		podSpec.Spec.NodeSelector = nodeSelector
	}
	return podSpec
}

func MakeNode(name string) *corev1.Node {
	return &corev1.Node{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec:   corev1.NodeSpec{},
		Status: corev1.NodeStatus{},
	}
}

func GenVolumeNodeAffinity(nodeName string) *corev1.VolumeNodeAffinity {
	return &corev1.VolumeNodeAffinity{
		Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{
			{MatchExpressions: []corev1.NodeSelectorRequirement{
				{Key: pkg.KubernetesNodeIdentityKey, Operator: corev1.NodeSelectorOpIn, Values: []string{nodeName}},
			}},
		}},
	}
}
