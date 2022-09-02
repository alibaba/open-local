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
	"context"
	"os"
	"strconv"
	"strings"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils/lvm"
	units "github.com/docker/go-units"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	log "k8s.io/klog/v2"
)

func (d *Discoverer) ExpandSnapshotLVIfNeeded() {
	// It's unnecessary for SPDK snapshot. SPDK snapshot size is fixed.
	// In SPDK, when creating snapshot original volume becomes thin provisioned
	// and saves only incremental differences from its underlying snapshot.
	if !d.spdk {
		d.expandSnapshotLvmLVIfNeeded()
	}
}

func (d *Discoverer) expandSnapshotLvmLVIfNeeded() {
	// Step 0: get prefix of snapshot lv
	prefix := os.Getenv(localtype.EnvSnapshotPrefix)
	if prefix == "" {
		prefix = localtype.DefaultSnapshotPrefix
	}

	// Step 1: get all snapshot lv
	lvs, err := getAllLocalSnapshotLV()
	if err != nil {
		log.Errorf("[ExpandSnapshotLVIfNeeded]get open-local snapshot lv failed: %s", err.Error())
		return
	}
	// Step 2: handle every snapshot lv(for)
	for _, lv := range lvs {
		// step 1: get threshold and increase size from snapshotClass
		snapContent, err := d.snapclient.SnapshotV1().VolumeSnapshotContents().Get(context.TODO(), strings.Replace(lv.Name(), prefix, "snapcontent", 1), metav1.GetOptions{})
		if err != nil {
			log.Errorf("[ExpandSnapshotLVIfNeeded]get snapContent %s error: %s", lv.Name(), err.Error())
			return
		}
		snapClass, err := d.snapclient.SnapshotV1().VolumeSnapshotClasses().Get(context.TODO(), *snapContent.Spec.VolumeSnapshotClassName, metav1.GetOptions{})
		if err != nil {
			log.Errorf("[ExpandSnapshotLVIfNeeded]get snapClass %s error: %s", *snapContent.Spec.VolumeSnapshotClassName, err.Error())
			return
		}
		initialSize, threshold, expansionSize := getSnapshotInitialInfo(snapClass.Parameters)
		// step 2: expand snapshot lv if necessary
		if lv.Usage() > threshold {
			log.Infof("[ExpandSnapshotLVIfNeeded]expand snapshot lv %s", lv.Name())
			log.Infof("[getSnapshotInitialInfo]initialSize(%d), threshold(%f), expansionSize(%d)", initialSize, threshold, expansionSize)
			if err := lv.Expand(expansionSize); err != nil {
				log.Errorf("[ExpandSnapshotLVIfNeeded]expand lv %s failed: %s", lv.Name(), err.Error())
				return
			}
			log.Infof("[ExpandSnapshotLVIfNeeded]expand snapshot lv %s successfully", lv.Name())
		}
	}
}

func getSnapshotInitialInfo(param map[string]string) (initialSize uint64, threshold float64, increaseSize uint64) {
	initialSize = localtype.DefaultSnapshotInitialSize
	threshold = localtype.DefaultSnapshotThreshold
	increaseSize = localtype.DefaultSnapshotExpansionSize

	// Step 1: get snapshot initial size
	if str, exist := param[localtype.ParamSnapshotInitialSize]; exist {
		size, err := units.RAMInBytes(str)
		if err != nil {
			log.Error("[getSnapshotInitialInfo]get initialSize from snapshot annotation failed")
		}
		initialSize = uint64(size)
	}
	// Step 2: get snapshot expand threshold
	if str, exist := param[localtype.ParamSnapshotThreshold]; exist {
		str = strings.ReplaceAll(str, "%", "")
		thr, err := strconv.ParseFloat(str, 64)
		if err != nil {
			log.Error("[getSnapshotInitialInfo]parse float failed")
		}
		threshold = thr / 100
	}
	// Step 3: get snapshot increase size
	if str, exist := param[localtype.ParamSnapshotExpansionSize]; exist {
		size, err := units.RAMInBytes(str)
		if err != nil {
			log.Error("[getSnapshotInitialInfo]get increase size from snapshot annotation failed")
		}
		increaseSize = uint64(size)
	}
	return
}

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
