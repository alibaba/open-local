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

package framework

import (
	"github.com/oecp/open-local/pkg"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// node releated
	DefaultNode = MakeNode("open-local-node")
	// lvm related
	DefaultLVMSC = MakeDefaultLVMStorageClass("open-local-lvm-sc", "share")

	DefaultLVMPVC = MakeLVMPVC("open-local-lvm-pvc", "default", DefaultLVMSC)

	DefaultLVMPV = MakeLVMPV("open-local-lvm-pv", DefaultNode.Name)
	// mp related
	DefaultMPSC  = MakeDefaultMPStorageClass("open-local-mp-sc")
	DefaultMPPVC = MakeMPPVC("open-local-mp-pvc", "default", DefaultMPSC)
	DefaultMPPV  = MakeMPPV("open-local-mp-pv", DefaultNode.Name)

	// device related
	DefaultDeviceSC  = MakeDefaultDeviceStorageClass("open-local-device-sc")
	DefaultDevicePVC = MakeDevicePVC("open-local-device-pvc", "default", DefaultDeviceSC)
	DefaultDeivcePV  = MakeDevicePV("open-local-device-pv", DefaultNode.Name)

	// vg related
	DefaultVGName = "share"
)

func MakeDefaultLVMStorageClass(name string, vgName string) *storagev1.StorageClass {
	params := make(map[string]string)
	params[pkg.VolumeTypeKey] = string(pkg.VolumeTypeLVM)
	if vgName != "" {
		params[pkg.VGName] = vgName
	}
	params[pkg.VolumeFSTypeKey] = pkg.VolumeFSTypeExt4

	var sc = &storagev1.StorageClass{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner:          pkg.ProvisionerNameYoda,
		Parameters:           params,
		AllowVolumeExpansion: nil,
		VolumeBindingMode:    nil,
	}
	return sc
}

func MakeDefaultMPStorageClass(name string) *storagev1.StorageClass {
	sc := &storagev1.StorageClass{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner: pkg.ProvisionerNameYoda,
		Parameters: map[string]string{
			pkg.VolumeTypeKey:   string(pkg.VolumeTypeMountPoint),
			pkg.VolumeMediaType: string(pkg.MediaTypeSSD),
		},
		ReclaimPolicy:        nil,
		MountOptions:         nil,
		AllowVolumeExpansion: nil,
		VolumeBindingMode:    nil,
		AllowedTopologies:    nil,
	}
	return sc
}

func MakeDefaultDeviceStorageClass(name string) *storagev1.StorageClass {
	sc := &storagev1.StorageClass{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner: pkg.ProvisionerNameYoda,
		Parameters: map[string]string{
			pkg.VolumeTypeKey:   string(pkg.VolumeTypeDevice),
			pkg.VolumeMediaType: string(pkg.MediaTypeSSD),
		},
		ReclaimPolicy:        nil,
		MountOptions:         nil,
		AllowVolumeExpansion: nil,
		VolumeBindingMode:    nil,
		AllowedTopologies:    nil,
	}
	return sc
}
