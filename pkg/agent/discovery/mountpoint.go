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
	"io/ioutil"
	"path/filepath"
	"reflect"

	lssv1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/ricochet2200/go-disk-usage/du"
	log "github.com/sirupsen/logrus"
	"k8s.io/utils/mount"
)

func (d *Discoverer) discoverMountPoints(newStatus *lssv1alpha1.NodeLocalStorageStatus) error {
	mountPoints, err := d.K8sMounter.List()
	if err != nil {
		return fmt.Errorf("List mountpoint error: %s", err.Error())
	}

	files, err := ioutil.ReadDir(d.MountPath)
	if err != nil {
		return fmt.Errorf("Read mount path error: %s", err.Error())
	}

	if len(files) == 0 {
		log.Debugf("No dir in mount path: %s", d.MountPath)
		return nil
	}

	// Put mount moints into set for faster checks below
	mountPointMap := make(map[string]mount.MountPoint)
	for _, mp := range mountPoints {
		mountPointMap[mp.Path] = mp
	}

	for _, file := range files {
		filePath := filepath.Join(d.MountPath, file.Name())

		// Validate that this path is an actual mountpoint
		if _, isMntPnt := mountPointMap[filePath]; !isMntPnt {
			log.Warningf("Path %q is not an actual mountpoint", filePath)
			continue
		}

		diskUsage := du.NewDiskUsage(mountPointMap[filePath].Path)
		var mpinfo lssv1alpha1.MountPoint
		mpinfo.Condition = lssv1alpha1.StorageReady
		mpinfo.Name = mountPointMap[filePath].Path
		mpinfo.Device = mountPointMap[filePath].Device
		mpinfo.FsType = mountPointMap[filePath].Type
		mpinfo.Total = diskUsage.Size()
		mpinfo.Available = diskUsage.Available()
		if mpinfo.Available == 0 {
			mpinfo.Condition = lssv1alpha1.StorageFull
		}
		// TODO(huizhi.szh): IsBind
		mpinfo.IsBind = false
		mpinfo.Options = mountPointMap[filePath].Opts
		if utils.ContainsString(mpinfo.Options, "ro") {
			mpinfo.ReadOnly = true
		} else {
			mpinfo.ReadOnly = false
		}

		newStatus.NodeStorageInfo.MountPoints = append(newStatus.NodeStorageInfo.MountPoints, mpinfo)
	}

	return nil
}

// checkIfMPStatusTransition check if VG Status Transition
func checkIfMPStatusTransition(old, new *lssv1alpha1.NodeLocalStorageStatus) (transition bool) {

	transition = false

	newMPMap := make(map[string]lssv1alpha1.MountPoint)
	newMPID := make(map[string]int)
	for i, mp := range new.NodeStorageInfo.MountPoints {
		newMPMap[mp.Name] = mp
		newMPID[mp.Name] = i
	}

	if len(old.NodeStorageInfo.MountPoints) != 0 {
		for _, mp := range old.NodeStorageInfo.MountPoints {
			_, isExist := newMPMap[mp.Name]
			if isExist {
				if newMPMap[mp.Name].Available == mp.Available &&
					newMPMap[mp.Name].Total == mp.Total &&
					newMPMap[mp.Name].FsType == mp.FsType &&
					newMPMap[mp.Name].Device == mp.Device &&
					reflect.DeepEqual(newMPMap[mp.Name].Options, mp.Options) {
					continue
				} else {
					transition = true
					break
				}
			}
		}
	} else {
		if len(new.NodeStorageInfo.MountPoints) != 0 {
			transition = true
		}
	}

	return
}
