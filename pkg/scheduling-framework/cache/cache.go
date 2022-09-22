package cache

import (
	"fmt"
	"sync"

	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/scheduler/errors"

	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"
	storagelisters "k8s.io/client-go/listers/storage/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

var vgHandler = &VGHandler{}
var deviceHandler = &DeviceHandler{}

type NodeAllocateState struct {
	NodeName                    string
	PodUid                      string
	Units                       *NodeAllocateUnits
	NodeStorageAllocatedByUnits *NodeStorageState
}

type NodeAllocateUnits struct {
	LVMPVCAllocateUnits       []*LVMPVAllocated
	DevicePVCAllocateUnits    []*DeviceTypePVAllocated
	InlineVolumeAllocateUnits []*InlineVolumeAllocated
}

func (units *NodeAllocateUnits) Clone() *NodeAllocateUnits {
	if units == nil {
		return units
	}
	copy := &NodeAllocateUnits{
		LVMPVCAllocateUnits:       []*LVMPVAllocated{},
		DevicePVCAllocateUnits:    []*DeviceTypePVAllocated{},
		InlineVolumeAllocateUnits: []*InlineVolumeAllocated{},
	}

	for _, unit := range units.LVMPVCAllocateUnits {
		copy.LVMPVCAllocateUnits = append(copy.LVMPVCAllocateUnits, unit.DeepCopy().(*LVMPVAllocated))
	}
	for _, unit := range units.DevicePVCAllocateUnits {
		copy.DevicePVCAllocateUnits = append(copy.DevicePVCAllocateUnits, unit.DeepCopy().(*DeviceTypePVAllocated))
	}
	for _, unit := range units.InlineVolumeAllocateUnits {
		copy.InlineVolumeAllocateUnits = append(copy.InlineVolumeAllocateUnits, unit.DeepCopy())
	}
	return copy
}

func (units *NodeAllocateUnits) ResetAllocatedSize() {
	if units == nil {
		return
	}
	for i := range units.LVMPVCAllocateUnits {
		units.LVMPVCAllocateUnits[i].GetBasePVAllocated().Allocated = 0
	}

	for i := range units.DevicePVCAllocateUnits {
		units.DevicePVCAllocateUnits[i].GetBasePVAllocated().Allocated = 0
	}

	for i := range units.InlineVolumeAllocateUnits {
		units.InlineVolumeAllocateUnits[i].Allocated = 0
	}
}

func (units *NodeAllocateUnits) HaveLocalUnits() bool {
	if units == nil {
		return false
	}
	return len(units.LVMPVCAllocateUnits)+len(units.DevicePVCAllocateUnits)+len(units.InlineVolumeAllocateUnits) > 0
}

/**

支持的回收策略：delete/retain

case1：正常调度流程-动态绑定
	1. Create PVC 延迟binding（调度器watch到PVC创建，未bind node不处理）
	2. Create Pod，调度器开始调度Pod
	3. reserve阶段：调度PVC，更新cache，
	4. 开启volume_binding prebind 阶段，更新PVC（node-seleted）；开始openlocal prebind，更新PVC调度信息到PV或者Pod ##注意，这里不清楚哪个prebind先执行，如果社区plugin先于openlocal plugin，会直接patch到PV
	5. external_provisional create Volume and PV(pending状态)
	6. 调度器Watch PV创建（pending状态），不处理
	7. pv_controller：bind PVC/PV
	8. 调度器watch到PV update：PV bound状态 此时PVC已经调度过，故向cache写入Volume信息，并删除pvc的调度信息，防止重复扣减
	9. open-local-controller: watch pod 事件，发现pod上有需要迁移的PVC调度allocate信息，则patch到对应PV
	10. prebind结束，bind node


Case2：正常调度流程-静态绑定-（调度之前pv_controller提前已绑定）
	1. onPVAdd/onPVUpdate：1）未bound阶段非pending，创建了PV的调度信息 2）bound，则向cache写入Volume信息，并删除pvc的调度信息，防止重复扣减
	2. onPVCAdd/onPVCUpdate：1）未bound阶段，则不处理 2）bound阶段，则更新PVC info信息
	3.调度器：已bound的PVC跳过



case3：正常调度流程-静态绑定-调度器prebind阶段绑定
	1. onPVAdd/onPVUpdate：1）未bound阶段非pending，创建PV的调度信息 2）bound，则向cache写入Volume信息，并删除pvc的调度信息，防止重复扣减
	2. onPVCAdd/onPVCUpdate：1）未bound阶段，则不处理 2）bound阶段，bound阶段，则更新PVC info信息
	3. 调度器：正常reserve，后续PV收到消息，会自动revert PVC调度记录，并加上pv账本


case4:调度器重建流程以及各类异常调度流程
	onPVDelete： 删除PV allocated信息，删除账本信息
	onPVCDelete： 删除PVC allocated信息，info信息，如果PV还在，则继续保留PV部分，且不扣减账本
	case3.1 已Bound PVC/PV
			onPVAdd: 1)如果没有pv allocated信息，账本扣除并增加pv allocated信息 2)如果有pvc allocate信息，则要revert掉
			onPVUpdate： 同PVAdd
			OnPVCAdd：	1）发现PVC/PV bound，更新pvc info信息，如果有pvc allocateDetail，更新起request 2）校验pvDetail，符合resize逻辑，则做delta更新
			onPVCUpdate： 同PVCAdd
			调度流程：如果一个POD调度含已bound的PVC，则跳过该PVC调度

	case3.2 PV pending状态（上次调度prebind阶段调度器退出？）
			onPVAdd: 不处理，返回
			onPVUpdate： 不处理，返回
			OnPVCAdd：	没bound，则不处理（属于调度器处理流程）
			onPVCUpdate： 没bound，则不处理（属于调度器处理流程）

	case3.3 PV其他状态
			onPVAdd： 根据PV创建allocated信息，更新node账本（此时无PVC信息）
			onPVUpdate： 同add

	case3.4 pod prebind阶段异常或重启，并重调度Pod：
			1）如果prebind阶段，volume_binding prebind阶段部分PVC/PV已bound，该部分不会回滚，会按PVC/PV bound情况重新调度回该Node
			2）如果prebind阶段，volume_binding prebind阶段所有PVC/PV都未执行，则会回滚pvc调度信息


eventHandlers完整流程（PVC/PV）
	onPVCAdd/onPVCUpdate：在是local pvc情况下，记录或者更新pvc info request信息
		1.未bound，返回
		2. 和PV bound,获取spec里的request信息
			2.1. 如果pvc有明细信息，则更新明细request
			2.2. 如果没有pv明细，则返回
			2.3。如果有pv明细，则判断是否是pvc扩容，计算delta，如果是扩容，则更新扩容size

	onPVCDelete： 删除PVC info信息等

	onPVAdd/onPVUpdate：
		1. pending:不处理，返回
		2. 获取capacity，并判断是否已有allocated信息
			2.1. 没有allocated：如果之前内存没有allocated信息，则创建allocated，并更新账本
			2.2. revert PVC allocate信息，避免删除PVC重复扣
			2.3  已allocated：计算是否resize,更新allocated，更新账本
	onPVDelete： 删除（PVC/PV）allocated信息，删除账本信息

	onPodAdd/opPodUpdate:
		1. Pod是否调度
			1.1. pod已调度完成： 更新inlineVolume信息，更新账本
			1.2. Pod未调度：直接返回
	onPodDelete:
		删除inlineVolume信息，账本更新




*/

type NodeStorageAllocatedCache struct {
	strategyType           pkg.StrategyType
	lvmPVAllocator         PVPVCAllocator
	lvmSnapshotPVAllocator PVPVCAllocator
	inlineVolumeAllocator  InlineVolumeAllocator
	deviceAllocator        PVPVCAllocator

	states                       map[string] /*nodeName*/ *NodeStorageState
	inlineVolumeAllocatedDetails map[string] /*nodeName*/ NodeInlineVolumeAllocatedDetails
	pvAllocatedDetails           *PVAllocatedDetails
	pvcInfosMap                  map[string] /*pvcKey*/ *PVCInfo
	sync.RWMutex
}

func NewNodeStorageAllocatedCache(strategyType pkg.StrategyType) *NodeStorageAllocatedCache {

	cache := &NodeStorageAllocatedCache{
		strategyType:                 strategyType,
		states:                       map[string]*NodeStorageState{},
		inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
		pvcInfosMap:                  map[string]*PVCInfo{},
		pvAllocatedDetails:           NewPVAllocatedDetails(),
	}
	cache.lvmPVAllocator = NewLVMCommonPVAllocator(cache)
	cache.inlineVolumeAllocator = NewInlineVolumeAllocator(cache)
	cache.deviceAllocator = NewDevicePVAllocator(cache)
	cache.lvmSnapshotPVAllocator = NewLVMSnapshotPVAllocator(cache)
	return cache
}

func (c *NodeStorageAllocatedCache) GetNodeStorageStateCopy(nodeName string) *NodeStorageState {
	c.RLock()
	defer c.RUnlock()
	if state, ok := c.states[nodeName]; ok {
		return state.DeepCopy()
	}
	return nil
}

func (c *NodeStorageAllocatedCache) GetPVCAllocatedDetailCopy(pvcNameSpace, pvcName string) PVAllocated {
	c.RLock()
	defer c.RUnlock()
	detail := c.pvAllocatedDetails.GetByPVC(pvcNameSpace, pvcName)
	if detail != nil {
		return detail.DeepCopy()
	}
	return nil
}

func (c *NodeStorageAllocatedCache) GetPVAllocatedDetailCopy(volumeName string) PVAllocated {
	c.RLock()
	defer c.RUnlock()
	detail := c.pvAllocatedDetails.GetByPV(volumeName)
	if detail != nil {
		return detail.DeepCopy()
	}
	return nil
}

func (c *NodeStorageAllocatedCache) GetPodInlineVolumeDetailsCopy(nodeName, podUid string) *PodInlineVolumeAllocatedDetails {
	c.RLock()
	defer c.RUnlock()
	podDetails := c.getPodInlineVolumeDetails(nodeName, podUid)
	if podDetails == nil {
		return nil
	}
	return podDetails.DeepCopy()
}

func (c *NodeStorageAllocatedCache) IsLocalNode(nodeName string) bool {
	c.RLock()
	defer c.RUnlock()
	nodeStorage, ok := c.states[nodeName]
	if !ok {
		return false
	}
	return nodeStorage.IsLocal()
}

/*
assume by cache, should record unit.allocated , allocated size will use by plugin Unreserve to revert cache
*/
func (c *NodeStorageAllocatedCache) Reserve(preAllocateState *NodeAllocateState, reservationPodUid string) error {
	if preAllocateState == nil || preAllocateState.Units == nil {
		return nil
	}
	c.Lock()
	defer c.Unlock()

	nodeState, ok := c.states[preAllocateState.NodeName]
	if !ok || !nodeState.InitedByNLS {
		err := fmt.Errorf("assume fail for node(%s), storage not init by NLS", preAllocateState.NodeName)
		klog.Errorf(err.Error())
		return err
	}

	if reservationPodUid != "" {
		c.inlineVolumeAllocator.unreserveDirect(preAllocateState.NodeName, reservationPodUid)
	}

	err := c.lvmPVAllocator.reserve(preAllocateState.NodeName, preAllocateState.Units)
	if err != nil {
		return err
	}

	err = c.inlineVolumeAllocator.reserve(preAllocateState.NodeName, preAllocateState.PodUid, preAllocateState.Units.InlineVolumeAllocateUnits)
	if err != nil {
		return err
	}

	err = c.deviceAllocator.reserve(preAllocateState.NodeName, preAllocateState.Units)
	if err != nil {
		return err
	}

	return nil
}

func (c *NodeStorageAllocatedCache) Unreserve(reservedAllocateState *NodeAllocateState, reservationPodUid string, reservationPodUnits []*InlineVolumeAllocated) {
	if reservedAllocateState == nil || reservedAllocateState.Units == nil {
		return
	}
	c.Lock()
	defer c.Unlock()
	nodeState, ok := c.states[reservedAllocateState.NodeName]
	if !ok || !nodeState.InitedByNLS {
		klog.Errorf("revert fail for node(%s), storage not init by NLS", reservedAllocateState.NodeName)
		return
	}
	c.lvmPVAllocator.unreserve(reservedAllocateState.NodeName, reservedAllocateState.Units)
	c.inlineVolumeAllocator.unreserve(reservedAllocateState.NodeName, reservedAllocateState.PodUid, reservedAllocateState.Units.InlineVolumeAllocateUnits)
	c.deviceAllocator.unreserve(reservedAllocateState.NodeName, reservedAllocateState.Units)
	if reservationPodUid != "" {
		err := c.inlineVolumeAllocator.reserve(reservedAllocateState.NodeName, reservationPodUid, reservationPodUnits)
		klog.Errorf("reserve reservationPod(%s) fail : ", reservationPodUid, err)
	}
}

func (c *NodeStorageAllocatedCache) AddNodeStorage(nodeLocal *nodelocalstorage.NodeLocalStorage) {
	c.Lock()
	defer c.Unlock()
	_, ok := c.states[nodeLocal.Name]
	if ok {
		c.updateNodeStorage(nodeLocal)
		return
	}
	c.states[nodeLocal.Name] = NewNodeStorageStateFromStorage(nodeLocal)
}

func (c *NodeStorageAllocatedCache) UpdateNodeStorage(old, new *nodelocalstorage.NodeLocalStorage) {
	c.Lock()
	defer c.Unlock()
	_, ok := c.states[new.Name]
	if !ok {
		c.states[new.Name] = NewNodeStorageStateFromStorage(new)
	}

	if utils.HashWithoutState(old) != utils.HashWithoutState(new) {
		c.updateNodeStorage(new)
	}
}

func (c *NodeStorageAllocatedCache) updateNodeStorage(nodeLocal *nodelocalstorage.NodeLocalStorage) {
	oldNodeState := c.states[nodeLocal.Name]
	newNodeState := NewNodeStorageStateFromStorage(nodeLocal)
	c.states[nodeLocal.Name].InitedByNLS = true
	c.states[nodeLocal.Name].VGStates = vgHandler.StatesForUpdate(oldNodeState.VGStates, newNodeState.VGStates)
	c.states[nodeLocal.Name].DeviceStates = deviceHandler.StatesForUpdate(oldNodeState.DeviceStates, newNodeState.DeviceStates)
}

func (c *NodeStorageAllocatedCache) AddPod(pod *corev1.Pod) {
	if !utils.IsPodNeedAllocate(pod) {
		return
	}

	c.Lock()
	defer c.Unlock()

	c.inlineVolumeAllocator.podAdd(pod)
}

func (c *NodeStorageAllocatedCache) UpdatePod(pod *corev1.Pod) {
	if utils.IsPodNeedAllocate(pod) { //add pod
		c.AddPod(pod)
		return
	}

	if pod.Spec.NodeName != "" {
		c.DeletePod(pod)
	}

}

func (c *NodeStorageAllocatedCache) DeletePod(pod *corev1.Pod) {
	c.Lock()
	defer c.Unlock()

	c.inlineVolumeAllocator.podDelete(pod)
}

func (c *NodeStorageAllocatedCache) addPVCInfo(pvc *corev1.PersistentVolumeClaim) {
	if pvc.Status.Phase != corev1.ClaimBound {
		return
	}
	pvName := utils.GetPVFromBoundPVC(pvc)
	nodeName := utils.NodeNameFromPVC(pvc)

	c.pvcInfosMap[utils.PVCName(pvc)] = NewPVCInfo(pvc, nodeName, pvName)
}

func (c *NodeStorageAllocatedCache) AddOrUpdatePVC(pvc *corev1.PersistentVolumeClaim, nodeName, volumeName string) {
	if pvc.Status.Phase != corev1.ClaimBound {
		return
	}

	c.Lock()
	defer c.Unlock()

	c.addPVCInfo(pvc)
	pvDetail := c.pvAllocatedDetails.GetByPV(volumeName)
	if pvDetail == nil {
		return
	}
	allocator := c.getAllocatorByVolumeType(pvDetail.GetVolumeType(), volumeName)
	if allocator == nil {
		return
	}
	allocator.pvcAdd(nodeName, pvc, volumeName)
}

func (c *NodeStorageAllocatedCache) DeletePVC(pvc *corev1.PersistentVolumeClaim) {
	c.Lock()
	defer c.Unlock()

	delete(c.pvcInfosMap, utils.PVCName(pvc))
	c.pvAllocatedDetails.DeleteByPVC(utils.PVCName(pvc))
}

func (c *NodeStorageAllocatedCache) AddOrUpdatePV(pv *corev1.PersistentVolume, nodeName string) {

	isOpenLocalPV, pvType := utils.IsOpenLocalPV(pv, false)
	if !isOpenLocalPV {
		return
	}

	c.Lock()
	defer c.Unlock()

	allocator := c.getAllocatorByVolumeType(pvType, pv.Name)
	if allocator == nil {
		return
	}

	allocator.pvAdd(nodeName, pv)
}

func (c *NodeStorageAllocatedCache) DeletePV(pv *corev1.PersistentVolume, nodeName string) {
	isOpenLocalPV, pvType := utils.IsOpenLocalPV(pv, false)
	if !isOpenLocalPV {
		return
	}

	c.Lock()
	defer c.Unlock()

	allocator := c.getAllocatorByVolumeType(pvType, pv.Name)
	if allocator == nil {
		return
	}

	allocator.pvDelete(nodeName, pv)
}

func (c *NodeStorageAllocatedCache) PrefilterPVC(scLister storagelisters.StorageClassLister, localPVC *corev1.PersistentVolumeClaim, podVolumeInfos *PodLocalVolumeInfo) error {
	allocator := c.getAllocatorByPVC(scLister, localPVC)
	if allocator == nil {
		return nil
	}
	return allocator.prefilter(scLister, localPVC, podVolumeInfos)
}

func (c *NodeStorageAllocatedCache) PrefilterInlineVolumes(pod *corev1.Pod, podVolumeInfo *PodLocalVolumeInfo) error {
	return c.inlineVolumeAllocator.prefilter(pod, podVolumeInfo)
}

func (c *NodeStorageAllocatedCache) PreAllocate(pod *corev1.Pod, podVolumeInfo *PodLocalVolumeInfo, reservationPod *corev1.Pod, nodeName string) (*NodeAllocateState, error) {
	nodeStateClone := c.GetNodeStorageStateCopy(nodeName)
	if nodeStateClone == nil {
		return nil, fmt.Errorf("node(%s) have no local storage pool", nodeName)
	}
	if !nodeStateClone.InitedByNLS {
		return nil, fmt.Errorf("node(%s) local storage pool had not been inited", nodeName)
	}
	nodeAllocate := &NodeAllocateState{NodeStorageAllocatedByUnits: nodeStateClone, NodeName: nodeName, PodUid: string(pod.UID)}
	if reservationPod != nil {
		c.inlineVolumeAllocator.preRevert(nodeName, reservationPod, nodeStateClone)
	}
	allocateUnits, err := c.preAllocateByVGs(pod, podVolumeInfo, nodeName, nodeStateClone)
	if allocateUnits != nil {
		nodeAllocate.Units = allocateUnits
	}
	if err != nil {
		return nodeAllocate, err
	}
	allocateUnitsByDevice, err := c.deviceAllocator.preAllocate(nodeName, podVolumeInfo, nodeStateClone)
	if err != nil {
		return nodeAllocate, err
	}
	for _, unit := range allocateUnitsByDevice {
		allocateUnits.DevicePVCAllocateUnits = append(allocateUnits.DevicePVCAllocateUnits, unit.(*DeviceTypePVAllocated))
	}
	return nodeAllocate, nil
}

func (c *NodeStorageAllocatedCache) MakeAllocateInfo(pvc *corev1.PersistentVolumeClaim) *pkg.PVCAllocateInfo {
	detail := c.GetPVCAllocatedDetailCopy(pvc.Namespace, pvc.Name)
	if detail == nil {
		detail = c.GetPVAllocatedDetailCopy(pvc.Spec.VolumeName)
	}
	if detail == nil {
		return nil
	}
	allocator := c.getAllocatorByVolumeType(detail.GetVolumeType(), detail.GetBasePVAllocated().VolumeName)
	if allocator == nil {
		klog.Warningf("Unknown allocate Type %s for pvc(%s)", detail.GetVolumeType(), utils.PVCName(pvc))
		return nil
	}
	return allocator.allocateInfo(detail)
}

func (c *NodeStorageAllocatedCache) IsPVHaveAllocateInfo(pv *corev1.PersistentVolume, volumeType pkg.VolumeType) bool {
	allocator := c.getAllocatorByVolumeType(volumeType, pv.Name)
	if allocator == nil {
		return true
	}
	return allocator.isPVHaveAllocateInfo(pv)
}

func (c *NodeStorageAllocatedCache) preAllocateByVGs(pod *corev1.Pod, podVolumeInfo *PodLocalVolumeInfo, nodeName string, nodeStateClone *NodeStorageState) (*NodeAllocateUnits, error) {
	allocateUnits := &NodeAllocateUnits{LVMPVCAllocateUnits: []*LVMPVAllocated{}, DevicePVCAllocateUnits: []*DeviceTypePVAllocated{}, InlineVolumeAllocateUnits: []*InlineVolumeAllocated{}}
	inlineVolumeAllocate, err := c.inlineVolumeAllocator.preAllocate(nodeName, pod, podVolumeInfo.InlineVolumes, nodeStateClone)
	if inlineVolumeAllocate != nil {
		allocateUnits.InlineVolumeAllocateUnits = inlineVolumeAllocate
	}
	if err != nil {
		return allocateUnits, err
	}

	pvcAllocate, err := c.lvmPVAllocator.preAllocate(nodeName, podVolumeInfo, nodeStateClone)
	if err != nil {
		return allocateUnits, err
	}
	for _, allocate := range pvcAllocate {
		allocateUnits.LVMPVCAllocateUnits = append(allocateUnits.LVMPVCAllocateUnits, allocate.(*LVMPVAllocated))
	}
	return allocateUnits, nil
}

func (c *NodeStorageAllocatedCache) initIfNeedAndGetVGState(nodeName, vgName string) (*VGStoragePool, bool) {
	nodeStoragePool := c.initIfNeedAndGetNodeStoragePool(nodeName)

	if _, ok := nodeStoragePool.VGStates[vgName]; !ok {
		nodeStoragePool.VGStates[vgName] = NewVGState(vgName)
	}
	return nodeStoragePool.VGStates[vgName], nodeStoragePool.InitedByNLS
}

func (c *NodeStorageAllocatedCache) initIfNeedAndGetNodeStoragePool(nodeName string) *NodeStorageState {
	if _, ok := c.states[nodeName]; !ok {
		c.states[nodeName] = NewNodeStorageState()
	}

	return c.states[nodeName]
}

func (c *NodeStorageAllocatedCache) getPodInlineVolumeDetails(nodeName, podUid string) *PodInlineVolumeAllocatedDetails {
	nodeDetails, ok := c.inlineVolumeAllocatedDetails[nodeName]
	if !ok {
		return nil
	}
	podDetails, ok := nodeDetails[podUid]
	if !ok {
		return nil
	}
	return podDetails
}

func (c *NodeStorageAllocatedCache) getRequestFromPVCInfos(pv *corev1.PersistentVolume) int64 {
	pvcName, pvcNamespace := utils.PVCNameFromPV(pv)
	if pvcName != "" {
		pvcInfo := c.pvcInfosMap[utils.GetPVCKey(pvcNamespace, pvcName)]
		if pvcInfo != nil {
			return pvcInfo.Requested
		}
	}
	return int64(0)
}

func allocateVgState(nodeName, vgName string, nodeState *NodeStorageState, size int64) error {
	if nodeState.VGStates == nil {
		err := fmt.Errorf("allocateVG fail, no VG found on node %s", nodeName)
		return err
	}
	vgState, ok := nodeState.VGStates[vgName]
	if !ok {
		err := fmt.Errorf("no vg pool %s in node %s", vgName, nodeName)
		klog.Error(err)
		return err
	}

	// free size
	poolFreeSize := vgState.Allocatable - vgState.Requested
	if poolFreeSize < size {
		return errors.NewInsufficientLVMError(size, int64(vgState.Requested), int64(vgState.Allocatable), vgName, nodeName)
	}
	// 更新临时 cache
	vgState.Requested += size
	return nil
}

func (c *NodeStorageAllocatedCache) getAllocatorByVolumeType(volumeType pkg.VolumeType, volumeName string) PVPVCAllocator {
	switch volumeType {
	case pkg.VolumeTypeLVM:
		return c.lvmPVAllocator
	case pkg.VolumeTypeDevice:
		return c.deviceAllocator
	default:
		klog.V(6).Infof("not a open-local pv %s, type %s, not add to cache", volumeName, volumeType)
	}
	return nil
}

func (c *NodeStorageAllocatedCache) getAllocatorByPVC(scLister storagelisters.StorageClassLister, pvc *corev1.PersistentVolumeClaim) PVPVCAllocator {
	if isLocalPV, pvType := utils.IsLocalPVC(pvc, scLister, true); isLocalPV {
		switch pvType {
		case pkg.VolumeTypeLVM:
			if utils.IsSnapshotPVC(pvc) {
				return c.lvmSnapshotPVAllocator
			} else {
				return c.lvmPVAllocator
			}
		case pkg.VolumeTypeDevice:
			return c.deviceAllocator
		default:
			klog.V(6).Infof("not a open-local pvc %s/%s, type %s, not add to cache", pvc.Namespace, pvc.Name, pvType)
		}
	}
	return nil
}
