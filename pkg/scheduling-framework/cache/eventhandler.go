package cache

import (
	"fmt"

	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

func (nodeLocalStorageCache *NodeLocalStorageCache) OnNodeLocalStorageAdd(obj interface{}) {
	// check
	nodeLocal, ok := obj.(*nodelocalstorage.NodeLocalStorage)
	if !ok {
		log.Errorf("[OnNodeLocalStorageAdd]cannot convert to *NodeLocalStorage: %v", obj)
		return
	}

	// nodename
	nodeName := nodeLocal.Name
	log.Debugf("[OnNodeLocalStorageAdd]nls %s added", nodeName)

	// localStorageCache
	localStorageCache := nodeLocalStorageCache.GetLocalStorageCache(nodeName)
	if localStorageCache == nil {
		localStorageCache = NewLocalStorageCache()
	}

	// LVM
	lvmPools := localStorageCache.GetStoragePools(PoolKindLVM)
	if lvmPools == nil {
		lvmPools = NewStoragePools()
	}
	vgInfoMap := make(map[string]nodelocalstorage.VolumeGroup, len(nodeLocal.Status.FilteredStorageInfo.VolumeGroups))
	for _, vg := range nodeLocal.Status.NodeStorageInfo.VolumeGroups {
		vgInfoMap[vg.Name] = vg
	}
	for _, vgName := range nodeLocal.Status.FilteredStorageInfo.VolumeGroups {
		lvmPool := lvmPools.GetStoragePool(vgName)
		if lvmPool == nil {
			log.Infof("[OnNodeLocalStorageAdd]adding new volume group %s to node cache %s: total %d, allocatable: %d", vgName, nodeName, vgInfoMap[vgName].Total, vgInfoMap[vgName].Allocatable)
			lvmPool = &StoragePool{
				Name:        vgName,
				Total:       vgInfoMap[vgName].Total,
				Allocatable: vgInfoMap[vgName].Allocatable,
				Requested:   0,
				ExtraInfo:   make(map[string]string),
			}
		} else {
			log.Infof("[OnNodeLocalStorageAdd]volume group %s on node cache %s exists: total %d, allocatable: %d, requested %d", vgName, nodeName, lvmPool.Total, lvmPool.Allocatable, lvmPool.Requested)
		}
		lvmPools.SetStoragePool(vgName, lvmPool)
	}
	localStorageCache.SetStoragePools(PoolKindLVM, lvmPools)

	nodeLocalStorageCache.SetLocalStorageCache(nodeName, localStorageCache)
	log.Infof("[OnNodeLocalStorageAdd]node %s is handled", nodeName)
}

func (nodeLocalStorageCache *NodeLocalStorageCache) OnNodeLocalStorageUpdate(oldObj, newObj interface{}) {
	// check
	nodeLocal, ok := newObj.(*nodelocalstorage.NodeLocalStorage)
	if !ok {
		log.Errorf("[OnNodeLocalStorageUpdate]cannot convert newObj to *NodeLocalStorage: %v", newObj)
		return
	}
	old, ok := oldObj.(*nodelocalstorage.NodeLocalStorage)
	if !ok {
		log.Errorf("[OnNodeLocalStorageUpdate]cannot convert oldObj to *NodeLocalStorage: %v", oldObj)
		return
	}
	if utils.HashWithoutState(old) == utils.HashWithoutState(nodeLocal) {
		return
	}
	log.Debugf("[OnNodeLocalStorageUpdate]spec of node local storage %s get changed, try to update", nodeLocal.Name)

	// nodename
	nodeName := nodeLocal.Name
	log.Debugf("[OnNodeLocalStorageUpdate]nls %s updated", nodeName)

	// localStorageCache
	localStorageCache := nodeLocalStorageCache.GetLocalStorageCache(nodeName)
	if localStorageCache == nil {
		localStorageCache = NewLocalStorageCache()
	}

	// LVM
	lvmPools := localStorageCache.GetStoragePools(PoolKindLVM)
	if lvmPools == nil {
		lvmPools = NewStoragePools()
	}
	vgInfoMap := make(map[string]nodelocalstorage.VolumeGroup, len(nodeLocal.Status.FilteredStorageInfo.VolumeGroups))
	for _, vg := range nodeLocal.Status.NodeStorageInfo.VolumeGroups {
		vgInfoMap[vg.Name] = vg
	}
	for _, vgName := range nodeLocal.Status.FilteredStorageInfo.VolumeGroups {
		lvmPool := lvmPools.GetStoragePool(vgName)
		if lvmPool == nil {
			log.Infof("[OnNodeLocalStorageUpdate]adding new volume group %s to node cache %s: total %d, allocatable: %d", vgName, nodeName, vgInfoMap[vgName].Total, vgInfoMap[vgName].Allocatable)
			lvmPool = &StoragePool{
				Name:        vgName,
				Total:       vgInfoMap[vgName].Total,
				Allocatable: vgInfoMap[vgName].Allocatable,
				Requested:   0,
				ExtraInfo:   make(map[string]string),
			}
		} else {
			// vg 更新
			lvmPool.Total = vgInfoMap[vgName].Total
			lvmPool.Allocatable = vgInfoMap[vgName].Allocatable
		}
		lvmPools.SetStoragePool(vgName, lvmPool)
	}
	localStorageCache.SetStoragePools(PoolKindLVM, lvmPools)

	nodeLocalStorageCache.SetLocalStorageCache(nodeName, localStorageCache)
	log.Infof("[OnNodeLocalStorageUpdate]node %s is handled", nodeName)
}

func (nodeLocalStorageCache *NodeLocalStorageCache) OnPVAdd(obj interface{}) {
	// check
	pv, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		log.Errorf("[OnPVAdd]cannot convert to *v1.PersistentVolume: %v", obj)
		return
	}

	// 判断是否是 open-local pv
	if !(pv.Spec.CSI != nil && utils.ContainsProvisioner(pv.Spec.CSI.Driver)) {
		return
	}

	// 若 source 是快照则退出
	attributes := pv.Spec.CSI.VolumeAttributes
	if value, exist := attributes[localtype.ParamSnapshotName]; exist && value != "" {
		return
	}

	// node name
	_, nodeName := utils.IsLocalPV(pv)
	if nodeName == "" {
		return
	}

	// check if pv phase is bound
	localStorageCache := nodeLocalStorageCache.GetLocalStorageCache(nodeName)
	if localStorageCache == nil {
		// 可能的存在情况
		// nls被删除且未被重建（比如节点下线），同时kube-scheduler重启，且环境中有对应节点的pv
		log.Errorf("[OnPVAdd]local storage pool in node %s is empty, existing...", nodeName)
		return
	}

	pvSize := utils.GetPVSize(pv)
	vgName := utils.GetVGNameFromCsiPV(pv)
	storageKind := pv.Spec.CSI.VolumeAttributes[localtype.VolumeTypeKey]
	storagePools := localStorageCache.GetStoragePools(storageKind)
	if storagePools == nil {
		// 可能的存在情况
		// nls被删除重建（agent还没有及时更新），同时kube-scheduler重启，那么 cache 中此时并没有 vg 信息
		// 一种稳的处理方式是
		// cache中记录pv详细的信息，在nls更新时进行cache检查，如有错误及时更新
		log.Errorf("[OnPVAdd]no storage pool kind found %s in node %s, existing...", storageKind, nodeName)
		return
	}
	storagePool := storagePools.GetStoragePool(vgName)
	if storagePool == nil {
		log.Errorf("[OnPVAdd]storage pool %s in node %s is empty, existing...", vgName, nodeName)
		return
	}
	storagePool.ExtraInfo[pv.Name] = VolumeTypePersistent
	if pv.Status.Phase == corev1.VolumeBound {
		// 已有 pv
		storagePool.Requested += uint64(pvSize)
	} else if pv.Status.Phase == corev1.VolumePending {
		// 新 PV
		// do nothing
	} else {
		log.Errorf("[OnPVAdd]unsupported pv status %s", pv.Status.Phase)
		return
	}
	localStorageCache.SetStoragePools(storageKind, storagePools)
	nodeLocalStorageCache.SetLocalStorageCache(nodeName, localStorageCache)
	log.Infof("[OnPVAdd]pv %s is handled", pv.Name)
}

func (nodeLocalStorageCache *NodeLocalStorageCache) OnPVUpdate(oldObj, newObj interface{}) {
	// check
	pv, ok := newObj.(*corev1.PersistentVolume)
	if !ok {
		log.Errorf("[OnPVUpdate]cannot convert to *v1.PersistentVolume: %v", newObj)
		return
	}

	// 判断是否是 open-local pv
	if !(pv.Spec.CSI != nil && utils.ContainsProvisioner(pv.Spec.CSI.Driver)) {
		return
	}

	// 若 source 是快照则退出
	attributes := pv.Spec.CSI.VolumeAttributes
	if value, exist := attributes[localtype.ParamSnapshotName]; exist && value != "" {
		return
	}

	// node name
	_, nodeName := utils.IsLocalPV(pv)
	if nodeName == "" {
		return
	}

	// check if pv phase is bound
	oldPV, _ := oldObj.(*corev1.PersistentVolume)
	localStorageCache := nodeLocalStorageCache.GetLocalStorageCache(nodeName)
	if localStorageCache == nil {
		log.Errorf("[OnPVUpdate]local storage pool in node %s is empty, existing...", nodeName)
		return
	}
	if pv.Status.Phase == corev1.VolumeBound {
		addedSize := utils.GetPVSize(pv) - utils.GetPVSize(oldPV)
		vgName := utils.GetVGNameFromCsiPV(pv)
		storagePools := localStorageCache.GetStoragePools(PoolKindLVM)
		if storagePools == nil {
			log.Errorf("[OnPVUpdate]storage pools LVM in node %s is empty, existing...", nodeName)
			return
		}
		storagePool := storagePools.GetStoragePool(vgName)
		if storagePool == nil {
			log.Errorf("[OnPVUpdate]storage pool %s in node %s is empty, existing...", vgName, nodeName)
			return
		}
		storagePool.Requested += uint64(addedSize)
		storagePools.SetStoragePool(vgName, storagePool)
		localStorageCache.SetStoragePools(PoolKindLVM, storagePools)
	} else {
		log.Errorf("[OnPVUpdate]unsupported pv status %s", pv.Status.Phase)
		return
	}

	nodeLocalStorageCache.SetLocalStorageCache(nodeName, localStorageCache)
	log.Infof("[OnPVUpdate]pv %s is handled", pv.Name)
}

func (nodeLocalStorageCache *NodeLocalStorageCache) OnPVDelete(obj interface{}) {
	// check
	pv, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		log.Errorf("[OnPVDelete]cannot convert to *v1.PersistentVolume: %v", obj)
		return
	}

	// 判断是否是 open-local pv
	if !(pv.Spec.CSI != nil && utils.ContainsProvisioner(pv.Spec.CSI.Driver)) {
		return
	}

	// 若 source 是快照则退出
	attributes := pv.Spec.CSI.VolumeAttributes
	if value, exist := attributes[localtype.ParamSnapshotName]; exist && value != "" {
		return
	}

	// node name
	_, nodeName := utils.IsLocalPV(pv)
	if nodeName == "" {
		return
	}

	// delete
	pvSize := utils.GetPVSize(pv)
	vgName := utils.GetVGNameFromCsiPV(pv)
	localStorageCache := nodeLocalStorageCache.GetLocalStorageCache(nodeName)
	if localStorageCache == nil {
		log.Errorf("[OnPVDelete]local storage pool in node %s is empty, existing...", nodeName)
		return
	}
	storagePools := localStorageCache.GetStoragePools(PoolKindLVM)
	if storagePools == nil {
		log.Errorf("[OnPVDelete]storage pools LVM in node %s is empty, existing...", nodeName)
		return
	}

	storagePool := storagePools.GetStoragePool(vgName)
	if storagePool == nil {
		log.Errorf("[OnPVDelete]storage pool %s in node %s is empty, existing...", vgName, nodeName)
		return
	}

	storagePool.Requested -= uint64(pvSize)
	delete(storagePool.ExtraInfo, pv.Name)
	storagePools.SetStoragePool(vgName, storagePool)
	localStorageCache.SetStoragePools(PoolKindLVM, storagePools)
	nodeLocalStorageCache.SetLocalStorageCache(nodeName, localStorageCache)

	log.Infof("[OnPVDelete]pv %s is handled", pv.Name)
}

func (nodeLocalStorageCache *NodeLocalStorageCache) OnPodAdd(obj interface{}) {
	// check
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		log.Errorf("[OnPodAdd]cannot convert to *v1.Pod: %v", obj)
		return
	}

	if !assignedPod(pod) {
		// reserve阶段会更新cache
		return
	} else {
		containInlineVolumes, nodeName := utils.ContainInlineVolumes(pod)
		if nodeName == "" {
			log.Infof("[OnPodAdd]nodename of pod %s is empty", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			return
		}
		localStorageCache := nodeLocalStorageCache.GetLocalStorageCache(nodeName)
		if localStorageCache == nil {
			log.Warningf("[OnPodAdd]local storage pool in node %s is empty, existing...", nodeName)
			return
		}
		if containInlineVolumes {
			for _, volume := range pod.Spec.Volumes {
				if volume.CSI != nil && utils.ContainsProvisioner(volume.CSI.Driver) {
					vgName, size := utils.GetInlineVolumeInfoFromParam(volume.CSI.VolumeAttributes)
					if vgName == "" {
						log.Errorf("[OnPodAdd]no vgName found in inline volume of Pod %s", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
					}
					storagePools := localStorageCache.GetStoragePools(PoolKindLVM)
					if storagePools == nil {
						log.Errorf("[OnPodAdd]storage pools LVM in node %s is empty, existing...", nodeName)
						return
					}
					storagePool := storagePools.GetStoragePool(vgName)
					if storagePool == nil {
						log.Errorf("[OnPodAdd]storage pool %s in node %s is empty, existing...", vgName, nodeName)
						return
					}
					storagePool.Requested += uint64(size)
					storagePool.ExtraInfo[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)] = VolumeTypeEphemeral
					storagePools.SetStoragePool(vgName, storagePool)
					localStorageCache.SetStoragePools(PoolKindLVM, storagePools)
				}
			}
		}
		nodeLocalStorageCache.SetLocalStorageCache(nodeName, localStorageCache)
	}

	log.Infof("[OnPodAdd]pod %s is handled", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
}

func (nodeLocalStorageCache *NodeLocalStorageCache) OnPodDelete(obj interface{}) {
	// check
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		log.Errorf("[OnPodDelete]cannot convert to *v1.Pod: %v", obj)
		return
	}

	// nodename
	containInlineVolumes, nodeName := utils.ContainInlineVolumes(pod)
	if nodeName == "" {
		log.Infof("[OnPodDelete]nodename of pod %s is empty", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
		return
	}
	localStorageCache := nodeLocalStorageCache.GetLocalStorageCache(nodeName)
	if localStorageCache == nil {
		log.Warningf("[OnPodDelete]local storage pool in node %s is empty, existing...", nodeName)
		return
	}
	if containInlineVolumes {
		for _, volume := range pod.Spec.Volumes {
			if volume.CSI != nil && utils.ContainsProvisioner(volume.CSI.Driver) {
				vgName, size := utils.GetInlineVolumeInfoFromParam(volume.CSI.VolumeAttributes)
				if vgName == "" {
					log.Errorf("[OnPodDelete]no vgName found in inline volume of Pod %s", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
				}
				storagePools := localStorageCache.GetStoragePools(PoolKindLVM)
				if storagePools == nil {
					log.Errorf("[OnPodDelete]storage pools LVM in node %s is empty, existing...", nodeName)
					return
				}
				storagePool := storagePools.GetStoragePool(vgName)
				if storagePool == nil {
					log.Errorf("[OnPodDelete]storage pool %s in node %s is empty, existing...", vgName, nodeName)
					return
				}
				_, exist := storagePool.ExtraInfo[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)]
				if exist {
					storagePool.Requested -= uint64(size)
					delete(storagePool.ExtraInfo, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
				}
				storagePools.SetStoragePool(vgName, storagePool)
				localStorageCache.SetStoragePools(PoolKindLVM, storagePools)
			}
		}
	}
	nodeLocalStorageCache.SetLocalStorageCache(nodeName, localStorageCache)

	log.Infof("[OnPodDelete]pod %s is handled", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
}

// assignedPod selects pods that are assigned (scheduled and running).
func assignedPod(pod *corev1.Pod) bool {
	return len(pod.Spec.NodeName) != 0
}
