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

package discovery

import (
	"fmt"
	"os"

	localtype "github.com/alibaba/open-local/pkg"
	localv1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils/lvm"
	log "k8s.io/klog/v2"
)

func (d *Discoverer) discoverVGs(newStatus *localv1alpha1.NodeLocalStorageStatus, reservedVGInfo map[string]ReservedVGInfo) error {
	if d.spdk {
		return d.discoverLvstore(newStatus, reservedVGInfo)
	} else {
		return d.discoverLvmVGs(newStatus, reservedVGInfo)
	}
}

func (d *Discoverer) discoverLvstore(newStatus *localv1alpha1.NodeLocalStorageStatus, reservedVGInfo map[string]ReservedVGInfo) error {
	lvss, err := d.spdkclient.GetLvStores()
	if err != nil {
		return fmt.Errorf("List Lvstore error: %s", err.Error())
	}

	bdevs, err := d.spdkclient.GetBdevs()
	if err != nil {
		return fmt.Errorf("Get bdevs error: %s", err.Error())
	}

	for _, lvs := range *lvss {
		var vgCrd localv1alpha1.VolumeGroup
		vgCrd.Condition = localv1alpha1.StorageReady
		vgCrd.Name = lvs.Name

		for _, bdev := range *bdevs {
			if bdev.Name == lvs.BaseBdev {
				vgCrd.PhysicalVolumes = append(vgCrd.PhysicalVolumes, bdev.GetFilename())
				break
			}
		}
		vgCrd.Total = lvs.TotalClusters * lvs.ClusterSize

		vgCrd.Available = lvs.FreeClusters * lvs.ClusterSize
		if vgCrd.Available == 0 {
			vgCrd.Condition = localv1alpha1.StorageFull
		}

		vgCrd.Allocatable = vgCrd.Available
		//vgCrd.LogicalVolumes seems unused, skip it.

		newStatus.NodeStorageInfo.VolumeGroups = append(newStatus.NodeStorageInfo.VolumeGroups, vgCrd)
	}

	return nil
}

func (d *Discoverer) discoverLvmVGs(newStatus *localv1alpha1.NodeLocalStorageStatus, reservedVGInfo map[string]ReservedVGInfo) error {
	vgnames, err := lvm.ListVolumeGroupNames()
	if err != nil {
		return fmt.Errorf("List volume group error: %s", err.Error())
	}

	for _, vgname := range vgnames {
		var vgCrd localv1alpha1.VolumeGroup
		vgCrd.Condition = localv1alpha1.StorageReady
		// Name
		vg, err := lvm.LookupVolumeGroup(vgname)
		if err != nil {
			log.Errorf("Look up volume group %s error: %s", vgname, err.Error())
			continue
		}
		vgCrd.Name = vg.Name()

		// PV
		vgCrd.PhysicalVolumes, err = vg.ListPhysicalVolumeNames()
		if err != nil {
			log.Errorf("List physical volume %s error: %s", vgname, err.Error())
			continue
		}
		// total & available
		vgCrd.Total, _ = vg.BytesTotal()
		vgCrd.Available, _ = vg.BytesFree()
		if vgCrd.Available == 0 {
			vgCrd.Condition = localv1alpha1.StorageFull
		}

		// LogicalVolumes
		logicalVolumeNames, err := vg.ListLogicalVolumeNames()
		if err != nil {
			log.Errorf("List volume group %s error: %s", vgname, err.Error())
			continue
		}
		vgCrd.Allocatable = vgCrd.Total
		for _, lvname := range logicalVolumeNames {
			var lv localv1alpha1.LogicalVolume
			lv.Name = lvname
			lv.VGName = vgname
			tmplv, err := vg.LookupLogicalVolume(lvname)
			if err != nil {
				log.Errorf("List logical volume %s error: %s", lvname, err.Error())
				continue
			}
			lv.Total = tmplv.SizeInBytes()
			if !d.isLocalLV(lvname) {
				vgCrd.Allocatable -= lv.Total
			}
			lv.Condition = localv1alpha1.StorageReady
			vgCrd.LogicalVolumes = append(vgCrd.LogicalVolumes, lv)
		}

		// check if vgCrd.Allocatable is correct
		if info, exist := reservedVGInfo[vg.Name()]; exist {
			// reservedPercent
			var reservedSize uint64
			if info.reservedSize != 0 {
				reservedSize = info.reservedSize
			} else {
				reservedSize = uint64(float64(vgCrd.Total) * info.reservedPercent)
			}

			if vgCrd.Allocatable > vgCrd.Total-reservedSize {
				vgCrd.Allocatable = vgCrd.Total - info.reservedSize
			}
		}

		// Todo(huizhi.szh): vg.Check(): Failed to connect to lvmetad. Falling back to device scanning.
		// if err = vg.Check(); err != nil {
		// 	log.Errorf("volume %s check error: %s", vgname, err.Error())
		// 	vgCrd.Condition = lssv1alpha1.StorageFault
		// }
		vgCrd.Condition = localv1alpha1.StorageReady

		newStatus.NodeStorageInfo.VolumeGroups = append(newStatus.NodeStorageInfo.VolumeGroups, vgCrd)
	}

	return nil
}

func (d *Discoverer) createVG(vgname string, devices []string) error {
	force := false
	forceCreateVG := os.Getenv(localtype.EnvForceCreateVG)
	if forceCreateVG == "true" {
		force = true
	}

	var pvs []*lvm.PhysicalVolume
	for _, dev := range devices {
		pv, err := lvm.CreatePhysicalVolume(dev, force)
		if err != nil {
			log.Errorf("create physical volume %s error: %s", dev, err.Error())
			return err
		}
		pvs = append(pvs, pv)
	}
	_, err := lvm.CreateVolumeGroup(vgname, pvs, nil, force)
	if err != nil {
		log.Errorf("create volume volume %s error: %s", vgname, err.Error())
		return err
	}
	return nil
}

// isLocalLV check if lv is created by open-local according to the lv name
func (d *Discoverer) isLocalLV(lvname string) bool {
	prefixlen := len(d.Configuration.LogicalVolumeNamePrefix)
	ephemeralVolumePrefix := "csi-"

	if len(lvname) >= prefixlen && d.Configuration.LogicalVolumeNamePrefix == lvname[:prefixlen] {
		return true
	} else if len(lvname) >= len(ephemeralVolumePrefix) && lvname[:4] == ephemeralVolumePrefix {
		return true
	}

	return false
}
