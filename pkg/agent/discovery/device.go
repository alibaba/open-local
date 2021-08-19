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
	"io/ioutil"
	"path/filepath"
	"regexp"

	lssv1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	deviceutil "github.com/alibaba/open-local/pkg/utils/device"
)

func (d *Discoverer) discoverDevices(newStatus *lssv1alpha1.NodeLocalStorageStatus) error {
	sysBlockPath := filepath.Join(d.SysPath, "/block")
	blockRegExp := regexp.MustCompile(d.RegExp)
	blockDirs, err := ioutil.ReadDir(sysBlockPath)
	if err != nil {
		return err
	}
	for _, blockName := range blockDirs {
		if blockRegExp.MatchString(blockName.Name()) {
			device, err := deviceutil.GetBlockInfo(d.SysPath, blockName.Name())
			if err != nil {
				return err
			}

			devices, err := deviceutil.GetPartitionsInfo(d.SysPath, blockName.Name())
			if err != nil {
				return err
			}
			devices = append(devices, device)

			for _, device := range devices {
				var deviceInfo lssv1alpha1.DeviceInfo
				deviceInfo.Name = device.Name
				deviceInfo.MediaType = device.MediaType
				deviceInfo.ReadOnly = device.ReadOnly
				deviceInfo.Total = device.Total
				deviceInfo.Condition = lssv1alpha1.StorageReady
				newStatus.NodeStorageInfo.DeviceInfos = append(newStatus.NodeStorageInfo.DeviceInfos, deviceInfo)
			}
		}
	}

	return nil
}

// checkIfDeviceStatusTransition check if Device status transition
func checkIfDeviceStatusTransition(old, new *lssv1alpha1.NodeLocalStorageStatus) (transition bool) {

	transition = false

	newDeviceMap := make(map[string]lssv1alpha1.DeviceInfo)
	newDeviceID := make(map[string]int)
	for i, device := range new.NodeStorageInfo.DeviceInfos {
		newDeviceMap[device.Name] = device
		newDeviceID[device.Name] = i
	}

	if len(old.NodeStorageInfo.DeviceInfos) != 0 {
		for _, deivce := range old.NodeStorageInfo.DeviceInfos {
			_, isExist := newDeviceMap[deivce.Name]
			if isExist {
				if newDeviceMap[deivce.Name].MediaType == deivce.MediaType &&
					newDeviceMap[deivce.Name].Name == deivce.Name &&
					newDeviceMap[deivce.Name].Total == deivce.Total &&
					newDeviceMap[deivce.Name].ReadOnly == deivce.ReadOnly {
					continue
				} else {
					transition = true
					break
				}
			}
		}
	} else {
		if len(new.NodeStorageInfo.DeviceInfos) != 0 {
			transition = true
		}
	}

	return
}
