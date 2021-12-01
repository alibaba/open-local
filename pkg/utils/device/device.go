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

package device

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	localtype "github.com/alibaba/open-local/pkg"
)

func GetBlockInfo(sysPath, blockName string) (Device, error) {
	var device Device
	var media string
	var ro bool
	var total uint64

	blockPath := filepath.Join(sysPath, "/block", blockName)

	// MediaType
	mediaPath := filepath.Join(blockPath, "queue/rotational")
	data, err := getFileContext(mediaPath)
	if err != nil {
		return device, err
	}
	if data == "1" {
		media = string(localtype.MediaTypeHDD)
	} else {
		media = string(localtype.MediaTypeSSD)
	}

	// ReadOnly
	roPath := filepath.Join(blockPath, "ro")
	data, err = getFileContext(roPath)
	if err != nil {
		return device, err
	}
	if data == "1" {
		ro = true
	} else {
		ro = false
	}

	// Total
	totalPath := filepath.Join(blockPath, "size")
	data, err = getFileContext(totalPath)
	if err != nil {
		return device, err
	}
	datatmp, err := strconv.Atoi(data)
	if err != nil {
		return device, err
	}
	total = uint64(datatmp) * 512

	device.Name = fmt.Sprintf("/dev/%s", blockName)
	device.IsPartition = false
	device.MediaType = media
	device.Total = total
	device.ReadOnly = ro

	return device, nil
}

func GetPartitionsInfo(sysPath, blockName string) ([]Device, error) {
	var devices []Device

	blockPath := filepath.Join(sysPath, "/block", blockName)
	dirs, err := ioutil.ReadDir(blockPath)
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		if strings.HasPrefix(dir.Name(), blockName) {
			var device Device
			var media string
			var ro bool
			var total uint64

			partName := dir.Name()
			// MediaType
			mediaPath := filepath.Join(blockPath, "queue/rotational")
			data, err := getFileContext(mediaPath)
			if err != nil {
				return nil, err
			}
			if data == "1" {
				media = string(localtype.MediaTypeHDD)
			} else {
				media = string(localtype.MediaTypeSSD)
			}
			// ReadOnly
			roPath := filepath.Join(blockPath, partName, "ro")
			data, err = getFileContext(roPath)
			if err != nil {
				return nil, err
			}
			if data == "1" {
				ro = true
			} else {
				ro = false
			}
			// Total
			totalPath := filepath.Join(blockPath, partName, "size")
			data, err = getFileContext(totalPath)
			if err != nil {
				return nil, err
			}
			datatmp, err := strconv.Atoi(data)
			if err != nil {
				return nil, err
			}
			total = uint64(datatmp) * 512

			device.Name = fmt.Sprintf("/dev/%s", partName)
			device.IsPartition = true
			device.MediaType = media
			device.Total = total
			device.ReadOnly = ro
			devices = append(devices, device)
		}
	}

	return devices, nil
}

func getFileContext(filePath string) (string, error) {
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file %s error: %s", filePath, err.Error())
	}
	data := string(b)
	data = strings.TrimRight(data, "\n")
	return data, nil
}
