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
	"github.com/alibaba/open-local/pkg/utils/lvm"
	log "k8s.io/klog/v2"
)

//
func getAllLocalSnapshotLV() (lvs []*lvm.LogicalVolume, err error) {
	// get all vg names
	lvs = make([]*lvm.LogicalVolume, 0)
	vgNames, err := lvm.ListVolumeGroupNames()
	if err != nil {
		log.Errorf("[getAllLocalSnapshotLV]List volume group names error: %s", err.Error())
		return nil, err
	}
	for _, vgName := range vgNames {
		// step 1: get vg info
		vg, err := lvm.LookupVolumeGroup(vgName)
		if err != nil {
			log.Errorf("[getAllLocalSnapshotLV]Look up volume group %s error: %s", vgName, err.Error())
			return nil, err
		}
		// step 2: get all lv of the selected vg
		logicalVolumeNames, err := vg.ListLogicalVolumeNames()
		if err != nil {
			log.Errorf("[getAllLocalSnapshotLV]List volume group %s error: %s", vgName, err.Error())
			return nil, err
		}
		// step 3: update lvs variable
		for _, lvName := range logicalVolumeNames {
			tmplv, err := vg.LookupLogicalVolume(lvName)
			if err != nil {
				log.Errorf("[getAllLocalSnapshotLV]List logical volume %s error: %s", lvName, err.Error())
				continue
			}
			if tmplv.IsSnapshot() {
				lvs = append(lvs, tmplv)
			}
		}
	}

	return lvs, nil
}
