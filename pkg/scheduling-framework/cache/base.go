/*
Copyright 2022/8/17 Alibaba Cloud.

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
package cache

import (
	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"
)

type NodeStorageState struct {
	VGStates     VGStates
	DeviceStates DeviceStates
	InitedByNLS  bool
}

func (n *NodeStorageState) DeepCopy() *NodeStorageState {
	if n == nil {
		return nil
	}
	copy := NewNodeStorageState()
	copy.VGStates = n.VGStates.DeepCopy()
	copy.DeviceStates = n.DeviceStates.DeepCopy()
	copy.InitedByNLS = n.InitedByNLS
	return copy
}

func NewNodeStorageState() *NodeStorageState {
	return &NodeStorageState{VGStates: VGStates{}, DeviceStates: DeviceStates{}}
}

func NewNodeStorageStateFromStorage(nodeLocal *nodelocalstorage.NodeLocalStorage) *NodeStorageState {
	storageState := NewNodeStorageState()
	storageState.VGStates = vgHandler.CreateStatesByNodeLocal(nodeLocal)
	storageState.DeviceStates = deviceHandler.CreateStatesByNodeLocal(nodeLocal)
	storageState.InitedByNLS = true
	return storageState
}

type PVAllocated interface {
	GetBasePVAllocated() *BasePVAllocated
	GetVolumeType() localtype.VolumeType
	DeepCopy() PVAllocated
}

// PV Local allocated details
type BasePVAllocated struct {
	VolumeName   string
	PVCName      string
	PVCNamespace string
	NodeName     string
	Requested    int64 //requested size from pvc
	Allocated    int64 //actual allocated size for the pvc
}

func (b *BasePVAllocated) DeepCopy() *BasePVAllocated {
	if b == nil {
		return nil
	}
	return &BasePVAllocated{
		VolumeName:   b.VolumeName,
		PVCNamespace: b.PVCNamespace,
		PVCName:      b.PVCName,
		NodeName:     b.NodeName,
		Requested:    b.Requested,
		Allocated:    b.Allocated,
	}
}

/*
	allocated details
	if pvc pv bound,then add to pvAllocated and remove pvcAllocated
*/

type PVAllocatedDetails struct {
	pvcAllocated map[string] /*pvc namespace && name*/ PVAllocated
	pvAllocated  map[string] /*volume name*/ PVAllocated
}

func NewPVAllocatedDetails() *PVAllocatedDetails {
	return &PVAllocatedDetails{
		pvcAllocated: map[string]PVAllocated{},
		pvAllocated:  map[string]PVAllocated{},
	}
}

func (l *PVAllocatedDetails) GetByPVC(pvcNamespace, pvcName string) PVAllocated {
	pvcAllocated, ok := l.pvcAllocated[utils.GetPVCKey(pvcNamespace, pvcName)]
	if ok {
		return pvcAllocated
	}
	return nil
}

func (l *PVAllocatedDetails) GetByPV(volumeName string) PVAllocated {
	pvAllocated, ok := l.pvAllocated[volumeName]
	if ok {
		return pvAllocated
	}
	return nil
}

func (l *PVAllocatedDetails) Get(query PVAllocated) PVAllocated {
	baseInfo := query.GetBasePVAllocated()
	if baseInfo.PVCName != "" && baseInfo.PVCNamespace != "" {
		allocated := l.GetByPVC(baseInfo.PVCNamespace, baseInfo.PVCName)
		if allocated != nil {
			return allocated
		}
	}

	if baseInfo.VolumeName != "" {
		return l.GetByPV(baseInfo.VolumeName)
	}
	return nil
}

func (l *PVAllocatedDetails) DeleteByPVC(pvcKey string) {
	delete(l.pvcAllocated, pvcKey)
}

func (l *PVAllocatedDetails) DeleteByPV(remove PVAllocated) {
	if remove == nil {
		return
	}
	baseInfo := remove.GetBasePVAllocated()
	if baseInfo.PVCName != "" && baseInfo.PVCNamespace != "" {
		delete(l.pvcAllocated, utils.GetPVCKey(baseInfo.PVCNamespace, baseInfo.PVCName))
	}
	if baseInfo.VolumeName != "" {
		delete(l.pvAllocated, baseInfo.VolumeName)
	}
}

/*

 */
func (l *PVAllocatedDetails) AssumeByPVC(newAllocated PVAllocated) {
	baseInfo := newAllocated.GetBasePVAllocated()
	if baseInfo.VolumeName != "" {
		l.pvAllocated[baseInfo.VolumeName] = newAllocated
		if baseInfo.PVCName != "" && baseInfo.PVCNamespace != "" {
			delete(l.pvcAllocated, utils.GetPVCKey(baseInfo.PVCNamespace, baseInfo.PVCName))
		}
		return
	}
	if baseInfo.PVCName != "" && baseInfo.PVCNamespace != "" {
		l.pvcAllocated[utils.GetPVCKey(baseInfo.PVCNamespace, baseInfo.PVCName)] = newAllocated
	}

}

/*
	assumeByPV will remove pvcAssumeInfo
*/
func (l *PVAllocatedDetails) AssumeByPV(newAllocated PVAllocated) bool {
	baseInfo := newAllocated.GetBasePVAllocated()
	if baseInfo.VolumeName == "" {
		return false
	}

	l.pvAllocated[baseInfo.VolumeName] = newAllocated

	if baseInfo.PVCName != "" && baseInfo.PVCNamespace != "" {
		delete(l.pvcAllocated, utils.GetPVCKey(baseInfo.PVCNamespace, baseInfo.PVCName))
	}
	return true
}
