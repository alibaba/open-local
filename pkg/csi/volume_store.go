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
package csi

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type Store interface {
	AddVolume(volumeID, device string) error
	DeleteVolume(volumeID string) error
	GetDevice(volumeID string) (string, bool)
}

type volumeStore struct {
	rwLock             sync.RWMutex
	volumeDeviceMapper map[string]string
	dataFilePath       string
}

func NewVolumeStore(dataFilePath string) (Store, error) {
	var err error
	var volumeDeviceMapper map[string]string
	if volumeDeviceMapper, err = loadVolumeData(dataFilePath); err != nil {
		if os.IsNotExist(err) {
			volumeDeviceMapper = make(map[string]string)
		} else {
			return nil, err
		}
	}
	return &volumeStore{
		volumeDeviceMapper: volumeDeviceMapper,
		dataFilePath:       dataFilePath,
	}, nil
}

func (store *volumeStore) AddVolume(volumeID, device string) error {
	store.rwLock.Lock()
	defer store.rwLock.Unlock()
	store.volumeDeviceMapper[volumeID] = device
	if err := saveVolumeData(store.dataFilePath, store.volumeDeviceMapper); err != nil {
		return err
	}

	return nil
}

func (store *volumeStore) DeleteVolume(volumeID string) error {
	store.rwLock.Lock()
	defer store.rwLock.Unlock()
	delete(store.volumeDeviceMapper, volumeID)
	if err := saveVolumeData(store.dataFilePath, store.volumeDeviceMapper); err != nil {
		return err
	}
	return nil
}

func (store *volumeStore) GetDevice(volumeID string) (string, bool) {
	store.rwLock.RLock()
	defer store.rwLock.RUnlock()

	device, ok := store.volumeDeviceMapper[volumeID]
	return device, ok
}

// saveVolumeData persists parameter data as json file at the provided location
func saveVolumeData(dataFilePath string, data map[string]string) error {
	file, err := os.Create(dataFilePath)
	if err != nil {
		return fmt.Errorf("failed to save volume data file %s: %v", dataFilePath, err)
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(data); err != nil {
		return fmt.Errorf("failed to save volume data file: %s", err.Error())
	}
	return nil
}

// loadVolumeData loads volume info from specified json file/location
func loadVolumeData(dataFilePath string) (map[string]string, error) {
	file, err := os.Open(dataFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data := map[string]string{}
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse volume data file: %s", err.Error())
	}

	return data, nil
}
