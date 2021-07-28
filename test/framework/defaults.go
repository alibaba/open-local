package framework

import (
	"github.com/oecp/open-local-storage-service/pkg"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// node releated
	DefaultNode = MakeNode("open-local-storage-service-node")
	// lvm related
	DefaultLVMSC = MakeDefaultLVMStorageClass("open-local-storage-service-lvm-sc", "share")

	DefaultLVMPVC = MakeLVMPVC("open-local-storage-service-lvm-pvc", "default", DefaultLVMSC)

	DefaultLVMPV = MakeLVMPV("open-local-storage-service-lvm-pv", DefaultNode.Name)
	// mp related
	DefaultMPSC  = MakeDefaultMPStorageClass("open-local-storage-service-mp-sc")
	DefaultMPPVC = MakeMPPVC("open-local-storage-service-mp-pvc", "default", DefaultMPSC)
	DefaultMPPV  = MakeMPPV("open-local-storage-service-mp-pv", DefaultNode.Name)

	// device related
	DefaultDeviceSC  = MakeDefaultDeviceStorageClass("open-local-storage-service-device-sc")
	DefaultDevicePVC = MakeDevicePVC("open-local-storage-service-device-pvc", "default", DefaultDeviceSC)
	DefaultDeivcePV  = MakeDevicePV("open-local-storage-service-device-pv", DefaultNode.Name)

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
