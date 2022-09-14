package cache

import (
	"fmt"
	"sync"

	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	corev1informers "k8s.io/client-go/informers/core/v1"
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
		units.LVMPVCAllocateUnits[i].Allocated = 0
	}

	for i := range units.DevicePVCAllocateUnits {
		units.DevicePVCAllocateUnits[i].Allocated = 0
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
	4. 开启volume_binding prebind 阶段，更新PVC（node-seleted）；开始openlocal prebind，更新PVC调度信息到NLS ##注意，这里不清楚哪个prebind先执行
	5. external_provisional create Volume and PV(pending状态)
	6. 调度器Watch PV创建（pending状态），不处理
	7. pv_controller：bind PVC/PV
	8. 调度器watch到PV update：PV bound状态 此时PVC已经调度过，故向cache写入Volume信息，并删除pvc的调度信息，防止重复扣减
	9. nls-controller: 1)watch pv update: 获取NLS信息，得到VG信息，并更新PV的VG信息 2)watch到NLS update,如果PV中无VG信息，且bound状态，更新VG信息到PV中
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
	states                       map[string] /*nodeName*/ *NodeStorageState
	inlineVolumeAllocatedDetails map[string] /*nodeName*/ NodeInlineVolumeAllocatedDetails
	pvAllocatedDetails           *PVAllocatedDetails
	pvcInfosMap                  map[string] /*pvcKey*/ *PVCInfo
	sync.RWMutex

	coreV1Informers corev1informers.Interface
}

func NewNodeStorageAllocatedCache(coreV1Informers corev1informers.Interface) *NodeStorageAllocatedCache {

	return &NodeStorageAllocatedCache{
		states:                       map[string]*NodeStorageState{},
		inlineVolumeAllocatedDetails: map[string]NodeInlineVolumeAllocatedDetails{},
		pvcInfosMap:                  map[string]*PVCInfo{},
		pvAllocatedDetails:           NewPVAllocatedDetails(),
		coreV1Informers:              coreV1Informers,
	}
}

func (c *NodeStorageAllocatedCache) GetNodeStorageStateCopy(nodeName string) *NodeStorageState {
	c.Lock()
	defer c.Unlock()
	if state, ok := c.states[nodeName]; ok {
		return state.DeepCopy()
	}
	return nil
}

func (c *NodeStorageAllocatedCache) GetPVCAllocatedDetailCopy(pvcNameSpace, pvcName string) PVAllocated {
	c.Lock()
	defer c.Unlock()
	detail := c.pvAllocatedDetails.GetByPVC(pvcNameSpace, pvcName)
	if detail != nil {
		return detail.DeepCopy()
	}
	return nil
}

func (c *NodeStorageAllocatedCache) GetPVAllocatedDetailCopy(volumeName string) PVAllocated {
	c.Lock()
	defer c.Unlock()
	detail := c.pvAllocatedDetails.GetByPV(volumeName)
	if detail != nil {
		return detail.DeepCopy()
	}
	return nil
}

func (c *NodeStorageAllocatedCache) GetPodInlineVolumeDetailsCopy(nodeName, podUid string) *PodInlineVolumeAllocatedDetails {
	c.Lock()
	defer c.Unlock()
	podDetails := c.getPodInlineVolumeDetails(nodeName, podUid)
	if podDetails == nil {
		return nil
	}
	return podDetails.DeepCopy()
}

func (c *NodeStorageAllocatedCache) IsLocalNode(nodeName string) bool {
	c.Lock()
	defer c.Unlock()
	nodeStorage, ok := c.states[nodeName]
	if !ok {
		return false
	}
	if len(nodeStorage.DeviceStates) != 0 || len(nodeStorage.VGStates) != 0 {
		return true
	}
	return false
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
		c.unreserveInlineVolumesDirect(preAllocateState.NodeName, reservationPodUid, nodeState)
	}

	err := c.reserveLVMPVC(preAllocateState.NodeName, preAllocateState.Units, nodeState)
	if err != nil {
		return err
	}

	err = c.reserveInlineVolumes(preAllocateState.NodeName, preAllocateState.PodUid, preAllocateState.Units.InlineVolumeAllocateUnits, nodeState)
	if err != nil {
		return err
	}

	err = c.reserveDevicePVC(preAllocateState.NodeName, preAllocateState.Units, nodeState)
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
	c.unreserveLVMPVCs(reservedAllocateState.NodeName, reservedAllocateState.Units)
	c.unreserveInlineVolumes(reservedAllocateState.NodeName, reservedAllocateState.PodUid, reservedAllocateState.Units, nodeState)
	c.unreserveDevicePVCs(reservedAllocateState.NodeName, reservedAllocateState.Units)
	if reservationPodUid != "" {
		err := c.reserveInlineVolumes(reservedAllocateState.NodeName, reservationPodUid, reservationPodUnits, nodeState)
		if err != nil {
			klog.Errorf("fail to reserve inline volome: %s", err.Error())
		}
	}
}

func (c *NodeStorageAllocatedCache) reserveLVMPVC(nodeName string, units *NodeAllocateUnits, currentStorageState *NodeStorageState) error {
	if len(units.LVMPVCAllocateUnits) == 0 {
		return nil
	}

	for i, unit := range units.LVMPVCAllocateUnits {

		allocateExist := c.pvAllocatedDetails.GetByPVC(unit.PVCNamespace, unit.PVCName)
		if allocateExist != nil { //add by eventhandler for pvcBound
			continue
		}
		pvc, err := c.coreV1Informers.PersistentVolumeClaims().Lister().PersistentVolumeClaims(unit.PVCNamespace).Get(unit.PVCName)
		if err != nil {
			return err
		}
		if pvc.Status.Phase == corev1.ClaimBound {
			klog.Infof("skip reserveLVMPVC for bound pvc %s/%s", pvc.Namespace, pvc.Name)
			continue
		}

		if currentStorageState.VGStates == nil {
			err := fmt.Errorf("reserveLVMPVC fail, no VG found on node %s", nodeName)
			return err
		}

		vgState, ok := currentStorageState.VGStates[unit.VGName]
		if !ok {
			err := fmt.Errorf("reserveLVMPVC fail, volumeGroup(%s) have not found for pvc(%s) on node %s", unit.VGName, utils.GetPVCKey(unit.PVCNamespace, unit.PVCName), nodeName)
			return err
		}

		if vgState.Allocatable < vgState.Requested+unit.Requested {
			err := fmt.Errorf("reserveLVMPVC fail, volumeGroup(%s) have not enough space for pvc(%s) on node %s", unit.VGName, utils.GetPVCKey(unit.PVCNamespace, unit.PVCName), nodeName)
			return err
		}

		vgState.Requested = vgState.Requested + unit.Requested
		units.LVMPVCAllocateUnits[i].Allocated = unit.Requested
		c.pvAllocatedDetails.AssumeByPVC(units.LVMPVCAllocateUnits[i].DeepCopy())
		klog.V(6).Infof("reserve for lvm pvc unit (%#v) success, current vgState: %#v", unit, vgState)
	}
	return nil
}

func (c *NodeStorageAllocatedCache) unreserveLVMPVCs(nodeName string, units *NodeAllocateUnits) {
	if len(units.LVMPVCAllocateUnits) == 0 {
		return
	}

	for i, unit := range units.LVMPVCAllocateUnits {

		if unit.Allocated == 0 {
			continue
		}

		c.revertLVMPVCIfNeed(nodeName, unit.PVCNamespace, unit.PVCName)
		units.LVMPVCAllocateUnits[i].Allocated = 0
	}
}

func (c *NodeStorageAllocatedCache) reserveInlineVolumes(nodeName string, podUid string, units []*InlineVolumeAllocated, currentStorageState *NodeStorageState) error {
	if len(units) == 0 {
		return nil
	}

	nodeDetails, exist := c.inlineVolumeAllocatedDetails[nodeName]
	if !exist {
		nodeDetails = NodeInlineVolumeAllocatedDetails{}
		c.inlineVolumeAllocatedDetails[nodeName] = nodeDetails
	}

	//allocated return
	allocateExist, exist := nodeDetails[podUid]
	if exist && len(*allocateExist) > 0 {
		err := fmt.Errorf("reserveInlineVolumes fail, pod(%s) inlineVolume had allocated on node %s", podUid, nodeName)
		return err
	}

	podDetails := PodInlineVolumeAllocatedDetails{}
	for i, unit := range units {

		if currentStorageState.VGStates == nil {
			err := fmt.Errorf("reserveInlineVolumes fail, no VG found on node %s", nodeName)
			return err
		}

		vgState, ok := currentStorageState.VGStates[unit.VgName]
		if !ok {
			err := fmt.Errorf("reserveInlineVolumes fail, volumeGroup(%s) have not found for pod(%s) on node %s", unit.VgName, podUid, nodeName)
			return err
		}

		if vgState.Allocatable < vgState.Requested+unit.VolumeSize {
			err := fmt.Errorf("reserveInlineVolumes fail, volumeGroup(%s) have not enough space for pod(%s) on node %s", unit.VgName, podUid, nodeName)
			return err
		}

		vgState.Requested = vgState.Requested + unit.VolumeSize
		units[i].Allocated = unit.VolumeSize

		podDetails = append(podDetails, units[i].DeepCopy())
		nodeDetails[podUid] = &podDetails
		klog.V(6).Infof("reserve for inlineVolume unit (%#v) success, current vgState: %#v", unit, vgState)
	}
	return nil
}

func (c *NodeStorageAllocatedCache) unreserveInlineVolumes(nodeName, podUid string, units *NodeAllocateUnits, currentStorageState *NodeStorageState) {
	if len(units.InlineVolumeAllocateUnits) == 0 {
		return
	}

	for i := range units.InlineVolumeAllocateUnits {
		units.InlineVolumeAllocateUnits[i].Allocated = 0
	}

	c.unreserveInlineVolumesDirect(nodeName, podUid, currentStorageState)
}

func (c *NodeStorageAllocatedCache) unreserveInlineVolumesDirect(nodeName, podUid string, currentStorageState *NodeStorageState) {
	nodeDetails, exist := c.inlineVolumeAllocatedDetails[nodeName]
	if !exist {
		return
	}

	podInlineDetails, exist := nodeDetails[podUid]
	if !exist || len(*podInlineDetails) <= 0 {
		return
	}

	for _, detail := range *podInlineDetails {

		if currentStorageState.VGStates == nil {
			klog.Errorf("unreserveInlineVolumes fail, no VG found on node %s", nodeName)
			continue
		}

		vgState, ok := currentStorageState.VGStates[detail.VgName]
		if !ok {
			klog.Errorf("unreserveInlineVolumes fail, volumeGroup(%s) have not found for pod(%s) on node %s", detail.VgName, podUid, nodeName)
			continue
		}

		vgState.Requested = vgState.Requested - detail.Allocated
		klog.V(6).Infof("unreserve for inlineVolume (%#v) success, current vgState: %#v", detail, vgState)
	}

	nodeDetails[podUid] = &PodInlineVolumeAllocatedDetails{}
}

func (c *NodeStorageAllocatedCache) reserveDevicePVC(nodeName string, units *NodeAllocateUnits, currentStorageState *NodeStorageState) error {
	if len(units.DevicePVCAllocateUnits) == 0 {
		return nil
	}

	for i, unit := range units.DevicePVCAllocateUnits {

		allocateExist := c.pvAllocatedDetails.GetByPVC(unit.PVCNamespace, unit.PVCName)
		if allocateExist != nil {
			continue
		}
		pvc, err := c.coreV1Informers.PersistentVolumeClaims().Lister().PersistentVolumeClaims(unit.PVCNamespace).Get(unit.PVCName)
		if err != nil {
			return err
		}
		if pvc.Status.Phase == corev1.ClaimBound {
			klog.Infof("skip reserveDevicePVC for bound pvc %s/%s", pvc.Namespace, pvc.Name)
			continue
		}

		deviceState, ok := currentStorageState.DeviceStates[unit.DeviceName]
		if !ok {
			err := fmt.Errorf("reserveDevicePVC fail, device(%s) have not found for pvc(%s) on node %s", unit.DeviceName, utils.GetPVCKey(unit.PVCNamespace, unit.PVCName), nodeName)
			return err
		}

		if deviceState.IsAllocated {
			err := fmt.Errorf("reserveDevicePVC fail, device(%s) have allocated on node %s", unit.DeviceName, nodeName)
			return err
		}

		if deviceState.Allocatable < unit.Requested {
			err := fmt.Errorf("reserveDevicePVC fail, device(%s) allocatable small than pvc(%s) request on node %s", unit.DeviceName, utils.GetPVCKey(unit.PVCNamespace, unit.PVCName), nodeName)
			return err
		}

		deviceState.IsAllocated = true
		deviceState.Requested = deviceState.Allocatable

		units.DevicePVCAllocateUnits[i].Allocated = deviceState.Allocatable
		c.pvAllocatedDetails.AssumeByPVC(units.DevicePVCAllocateUnits[i].DeepCopy())
	}
	klog.V(6).Infof("reserve for device units (%#v) success, current allocated device: %#v", units.DevicePVCAllocateUnits, currentStorageState.DeviceStates)
	return nil
}

func (c *NodeStorageAllocatedCache) unreserveDevicePVCs(nodeName string, units *NodeAllocateUnits) {
	if len(units.DevicePVCAllocateUnits) == 0 {
		return
	}

	for i, unit := range units.DevicePVCAllocateUnits {

		if unit.Allocated == 0 {
			continue
		}
		c.revertDeviceByPVCIfNeed(nodeName, unit.PVCNamespace, unit.PVCName)
		units.DevicePVCAllocateUnits[i].Allocated = 0
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

func (c *NodeStorageAllocatedCache) DeleteByPVC(pvc *corev1.PersistentVolumeClaim) {
	c.Lock()
	defer c.Unlock()

	delete(c.pvcInfosMap, utils.PVCName(pvc))
	c.pvAllocatedDetails.DeleteByPVC(utils.PVCName(pvc))
}

func (c *NodeStorageAllocatedCache) AddPod(pod *corev1.Pod) {
	if !utils.IsPodNeedAllocate(pod) {
		return
	}
	contain, nodeName := utils.ContainInlineVolumes(pod)
	if !contain {
		return
	}

	c.Lock()
	defer c.Unlock()

	nodeDetails, exist := c.inlineVolumeAllocatedDetails[nodeName]
	if !exist {
		nodeDetails = NodeInlineVolumeAllocatedDetails{}
		c.inlineVolumeAllocatedDetails[nodeName] = nodeDetails
	}

	//allocated return
	podDetailsExist, exist := nodeDetails[string(pod.UID)]
	if exist && len(*podDetailsExist) > 0 {
		klog.V(6).Infof("pod(%s) inlineVolume had allocated on node %s", string(pod.UID), nodeName)
		return
	}

	nodeStorageState := c.initIfNeedAndGetNodeStoragePool(nodeName)

	podDetails := PodInlineVolumeAllocatedDetails{}

	for _, volume := range pod.Spec.Volumes {
		if volume.CSI != nil && utils.ContainsProvisioner(volume.CSI.Driver) {
			vgName, size := utils.GetInlineVolumeInfoFromParam(volume.CSI.VolumeAttributes)
			if vgName == "" {
				klog.Errorf("no vgName found in inline volume of Pod %s", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
				return
			}
			allocateInfo := NewInlineVolumeAllocated(pod.Name, pod.Namespace, vgName, volume.Name, size)

			if _, exist := nodeStorageState.VGStates[vgName]; !exist {
				nodeStorageState.VGStates[vgName] = NewVGState(vgName)
			}
			nodeStorageState.VGStates[vgName].Requested += size
			podDetails = append(podDetails, allocateInfo)
		}
	}
	nodeDetails[string(pod.UID)] = &podDetails
	klog.V(6).Infof("allocate inlineVolumes for pod(%s) success", pod.Name)

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

	contain, nodeName := utils.ContainInlineVolumes(pod)
	if !contain || nodeName == "" {
		return
	}

	nodeDetails, exist := c.inlineVolumeAllocatedDetails[nodeName]
	if !exist {
		return
	}

	podDetails, exist := nodeDetails[string(pod.UID)]
	if !exist || len(*podDetails) == 0 {
		return
	}

	nodeStorageState, ok := c.states[nodeName]
	if !ok {
		delete(nodeDetails, string(pod.UID))
		klog.Infof("no node(%s) state found, only delete inlineVolumes details for pod(%s) finished", nodeName, pod.Name)
		return
	}

	for _, volume := range *podDetails {
		if vgState, exist := nodeStorageState.VGStates[volume.VgName]; exist {
			vgState.Requested = vgState.Requested - volume.VolumeSize
		}
	}
	delete(nodeDetails, string(pod.UID))
	klog.V(6).Infof("allocate inlineVolumes for pod(%s) success", pod.Name)
}

func (c *NodeStorageAllocatedCache) AddPVCInfo(pvc *corev1.PersistentVolumeClaim, nodeName, volumeName string) {
	c.Lock()
	defer c.Unlock()

	c.pvcInfosMap[utils.PVCName(pvc)] = NewPVCInfo(pvc, nodeName, volumeName)
}

//allocateSize must record on pvDetail
func (c *NodeStorageAllocatedCache) AllocateLVMByPVCEvent(pvc *corev1.PersistentVolumeClaim, nodeName, volumeName string) {
	if pvc == nil {
		return
	}
	if pvc.Status.Phase != corev1.ClaimBound {
		klog.Infof("pv %s is in %s status, skipped", utils.PVCName(pvc), pvc.Status.Phase)
		return
	}

	c.Lock()
	defer c.Unlock()

	oldPVCDetail := c.pvAllocatedDetails.GetByPVC(pvc.Namespace, pvc.Name)

	//if local pvc, update request
	if oldPVCDetail != nil {
		oldPVCDetail.GetBasePVAllocated().Requested = utils.GetPVCRequested(pvc)
	}

	oldPVDetail := c.pvAllocatedDetails.GetByPV(volumeName)
	if oldPVDetail == nil {
		//plugin starting and receive pvc event first, so can allocate by pv event later
		return
	}

	maxRequest := utils.GetPVCRequested(pvc)
	//max(pvcRequest,pvRequest)
	if maxRequest < oldPVDetail.GetBasePVAllocated().Requested {
		maxRequest = oldPVDetail.GetBasePVAllocated().Requested
	}

	deltaAllocate := maxRequest - oldPVDetail.GetBasePVAllocated().Allocated
	if deltaAllocate <= 0 {
		return
	}

	lvmAllocated, ok := oldPVDetail.(*LVMPVAllocated)
	if !ok {
		klog.Errorf("can not convert pv(%s) AllocateInfo to LVMPVAllocated", volumeName)
		return
	}

	vgState, initedByNLS := c.initIfNeedAndGetVGState(nodeName, lvmAllocated.VGName)

	if vgState.Allocatable < vgState.Requested+deltaAllocate {
		klog.Warningf("volumeGroup(%s) have not enough space or not init by NLS(init:%v) for pvc(%s) on node %s", lvmAllocated.VGName, initedByNLS, utils.PVCName(pvc), nodeName)
	}

	vgState.Requested = vgState.Requested + deltaAllocate
	c.pvAllocatedDetails.AssumeAllocateSizeToPVDetail(volumeName, maxRequest)
	klog.V(6).Infof("expand for lvm pvc old detail(%#v) to %d success, current vgState: %#v", oldPVDetail, maxRequest, vgState)
}

func (c *NodeStorageAllocatedCache) AllocateLVMByPV(pv *corev1.PersistentVolume, nodeName string) {
	if pv == nil {
		return
	}

	if pv.Status.Phase == corev1.VolumePending {
		klog.Infof("pv %s is in %s status, skipped", pv.Name, pv.Status.Phase)
		return
	}

	c.Lock()
	defer c.Unlock()

	vgName := utils.GetVGNameFromCsiPV(pv)

	if vgName == "" {
		switch pv.Status.Phase {
		case corev1.VolumeBound:
			pvcName, pvcNamespace := utils.PVCNameFromPV(pv)
			if pvcName == "" {
				klog.Errorf("pv(%s) is bound, but not found pvcName on pv", pv.Name)
				return
			}
			pvcDetail := c.pvAllocatedDetails.GetByPVC(pvcNamespace, pvcName)
			if pvcDetail == nil {
				klog.Errorf("can't find pvcDetail for pvc(%s)", utils.GetPVCKey(pvcNamespace, pvcName))
				return
			}
			lvmDetail := pvcDetail.(*LVMPVAllocated)
			vgName = lvmDetail.VGName
		default:
			klog.V(6).Infof("pv %s is not bound to any volume group, skipped", pv.Name)
			return

		}
	}

	if vgName == "" {
		klog.Errorf("AllocateLVMByPV skip : vgName not found", pv.Name)
		return
	}

	new := NewLVMPVAllocatedFromPV(pv, vgName, nodeName)

	pvcRequest := c.getRequestFromPVCInfos(pv)

	maxRequest := pvcRequest //pv may resize small, so can update pv request
	//max(pvcRequest,newPVRequest)
	if maxRequest < new.Requested {
		maxRequest = new.Requested
	}

	old := c.pvAllocatedDetails.GetByPV(pv.Name)
	deltaAllocate := maxRequest
	if old != nil {
		deltaAllocate = maxRequest - old.GetBasePVAllocated().Allocated
	}

	vgState, initedByNLS := c.initIfNeedAndGetVGState(nodeName, vgName)

	// 	resolve allocated duplicate for allocate pvc by scheduler
	c.revertLVMPVCIfNeed(nodeName, new.PVCNamespace, new.PVCName)

	//request:pvc 100Gi pv 200Gi,allocate: 200Gi => request:pvc 100Gi pv 100Gi,allocate: 100Gi can success
	if deltaAllocate == 0 {
		return
	}

	if vgState.Allocatable < vgState.Requested+deltaAllocate {
		klog.Warningf("volumeGroup(%s) have not enough space or not init by NLS(init:%v) for pv(%s) on node %s", vgName, initedByNLS, pv.Name, nodeName)
	}

	vgState.Requested = vgState.Requested + deltaAllocate
	new.Allocated = maxRequest
	c.pvAllocatedDetails.AssumeByPVEvent(new)
	klog.V(6).Infof("allocate for lvm pv (%#v) success, current vgState: %#v", new, vgState)
}

func (c *NodeStorageAllocatedCache) revertLVMPVCIfNeed(nodeName, pvcNameSpace, pvcName string) {
	if pvcNameSpace == "" && pvcName == "" {
		return
	}

	//pv allocated, then remove pvc allocated size
	allocatedByPVC := c.pvAllocatedDetails.GetByPVC(pvcNameSpace, pvcName)
	if allocatedByPVC == nil {
		return
	}

	lvmAllocated, ok := allocatedByPVC.(*LVMPVAllocated)
	if !ok {
		klog.Errorf("can not convert pvc(%s) AllocateInfo to LVMPVAllocated", utils.GetPVCKey(pvcNameSpace, pvcName))
		return
	}
	//add by pvc event, no vgName && allocateSize = 0
	if lvmAllocated.VGName == "" || allocatedByPVC.GetBasePVAllocated().Allocated <= 0 {
		return
	}

	nodeState, ok := c.states[nodeName]
	if !ok || !nodeState.InitedByNLS {
		klog.Errorf("revertLVMByPVCIfNeed fail for node(%s), storage not init by NLS", nodeName)
		return
	}
	if len(nodeState.VGStates) <= 0 {
		klog.Errorf("revertLVMByPVCIfNeed fail, no VG found on node %s", nodeName)
		return
	}

	vgState, ok := nodeState.VGStates[lvmAllocated.VGName]
	if !ok {
		klog.Errorf("revertLVMByPVCIfNeed fail, volumeGroup(%s) have not found for pvc(%s) on node %s", lvmAllocated.VGName, utils.GetPVCKey(pvcNameSpace, pvcName), nodeName)
		return
	}

	vgState.Requested = vgState.Requested - lvmAllocated.Allocated
	c.pvAllocatedDetails.DeleteByPVC(utils.GetPVCKey(pvcNameSpace, pvcName))
	klog.V(6).Infof("revert for pvc (%s) success, current vgState: %#v", utils.GetPVCKey(pvcNameSpace, pvcName), vgState)
}

func (c *NodeStorageAllocatedCache) DeleteLVM(pv *corev1.PersistentVolume, nodeName string) {

	vgName := utils.GetVGNameFromCsiPV(pv)
	if vgName == "" {
		klog.V(6).Infof("pv %s is not bound to any volume group, skipped", pv.Name)
		return
	}

	c.Lock()
	defer c.Unlock()

	old := c.pvAllocatedDetails.GetByPV(pv.Name)

	if old == nil {
		return
	}

	c.updateVGRequestByDelta(nodeName, vgName, -old.GetBasePVAllocated().Allocated)
	c.pvAllocatedDetails.DeleteByPV(old)
}

func (c *NodeStorageAllocatedCache) AllocateDevice(pv *corev1.PersistentVolume, nodeName string) {

	if pv == nil {
		return
	}
	if pv.Status.Phase == corev1.VolumePending {
		klog.Infof("pv %s is in %s status, skipped", pv.Name, pv.Status.Phase)
		return
	}

	deviceName := utils.GetDeviceNameFromCsiPV(pv)

	if deviceName == "" {
		switch pv.Status.Phase {
		case corev1.VolumeBound:
			pvcName, pvcNamespace := utils.PVCNameFromPV(pv)
			if pvcName == "" {
				klog.Errorf("pv(%s) is bound, but not found pvcName on pv", pv.Name)
				return
			}
			pvcDetail := c.pvAllocatedDetails.GetByPVC(pvcNamespace, pvcName)
			if pvcDetail == nil {
				klog.Errorf("can't find pvcDetail for pvc(%s)", utils.GetPVCKey(pvcNamespace, pvcName))
				return
			}
			deviceDetail := pvcDetail.(*DeviceTypePVAllocated)
			deviceName = deviceDetail.DeviceName
		default:
			klog.V(6).Infof("pv %s is not bound to any device, skipped", pv.Name)
			return

		}
	}

	if deviceName == "" {
		klog.Errorf("allocateDevice : pv %s is not a valid open-local pv(device with name)", pv.Name)
		return
	}

	new := NewDeviceTypePVAllocatedFromPV(pv, deviceName, nodeName)

	c.Lock()
	defer c.Unlock()

	deviceState, initNLS := c.initIfNeedAndGetDeviceState(nodeName, deviceName)

	if initNLS {
		new.Allocated = deviceState.Allocatable
	} else {
		klog.Infof("device(%s) have not not init by NLS(inti:%v) for pv(%s) on node %s", deviceName, initNLS, pv.Name, nodeName)
		new.Allocated = new.Requested
	}

	//resolve allocated duplicate for staticBounding by volumeBinding plugin may allocate pvc on on other device,so should revert
	c.revertDeviceByPVCIfNeed(nodeName, new.PVCNamespace, new.PVCName)

	c.allocateDeviceForState(nodeName, deviceName, new.Allocated)
	c.pvAllocatedDetails.AssumeByPVEvent(new)
	klog.V(6).Infof("allocate for device pv (%#v) success, current deviceState: %#v", new, deviceState)
}

func (c *NodeStorageAllocatedCache) revertDeviceByPVCIfNeed(nodeName, pvcNameSpace, pvcName string) {
	if pvcNameSpace == "" && pvcName == "" {
		return
	}
	allocatedByPVC := c.pvAllocatedDetails.GetByPVC(pvcNameSpace, pvcName)
	if allocatedByPVC == nil {
		return
	}
	deviceAllocated, ok := allocatedByPVC.(*DeviceTypePVAllocated)
	if !ok {
		klog.Errorf("can not convert pvc(%s) AllocateInfo to DeviceTypePVAllocated", utils.GetPVCKey(pvcNameSpace, pvcName))
		return
	}

	if deviceAllocated.Allocated == 0 || deviceAllocated.DeviceName == "" {
		return
	}

	c.revertDeviceForState(nodeName, deviceAllocated.DeviceName)
	c.pvAllocatedDetails.DeleteByPVC(utils.GetPVCKey(pvcNameSpace, pvcName))
}

func (c *NodeStorageAllocatedCache) DeleteDevice(pv *corev1.PersistentVolume, nodeName string) {

	deviceName := utils.GetDeviceNameFromCsiPV(pv)
	if deviceName == "" {
		klog.Errorf("deleteDevice: pv %s is not a valid open-local pv(device with name)", pv.Name)
		return
	}

	c.Lock()
	defer c.Unlock()

	old := c.pvAllocatedDetails.GetByPV(pv.Name)
	if old == nil {
		return
	}

	c.revertDeviceForState(nodeName, deviceName)
	c.pvAllocatedDetails.DeleteByPV(old)
}

func (c *NodeStorageAllocatedCache) initIfNeedAndGetVGState(nodeName, vgName string) (*VGStoragePool, bool) {
	nodeStoragePool := c.initIfNeedAndGetNodeStoragePool(nodeName)

	if _, ok := nodeStoragePool.VGStates[vgName]; !ok {
		nodeStoragePool.VGStates[vgName] = NewVGState(vgName)
	}
	return nodeStoragePool.VGStates[vgName], nodeStoragePool.InitedByNLS
}

func (c *NodeStorageAllocatedCache) initIfNeedAndGetDeviceState(nodeName, deviceName string) (*DeviceResourcePool, bool) {
	nodeStoragePool := c.initIfNeedAndGetNodeStoragePool(nodeName)

	if _, ok := nodeStoragePool.DeviceStates[deviceName]; !ok {
		nodeStoragePool.DeviceStates[deviceName] = &DeviceResourcePool{Name: deviceName}
	}
	return nodeStoragePool.DeviceStates[deviceName], nodeStoragePool.InitedByNLS
}

func (c *NodeStorageAllocatedCache) initIfNeedAndGetNodeStoragePool(nodeName string) *NodeStorageState {
	if _, ok := c.states[nodeName]; !ok {
		c.states[nodeName] = NewNodeStorageState()
	}

	return c.states[nodeName]
}

func (c *NodeStorageAllocatedCache) updateVGRequestByDelta(nodeName string, vgName string, delta int64) {
	nodeStoragePool, ok := c.states[nodeName]
	if !ok {
		nodeStoragePool = NewNodeStorageState()
		c.states[nodeName] = nodeStoragePool
	}

	_, ok = nodeStoragePool.VGStates[vgName]
	if !ok {
		nodeStoragePool.VGStates[vgName] = NewVGState(vgName)
	}

	if vgState, ok := nodeStoragePool.VGStates[vgName]; ok {
		vgState.Requested = vgState.Requested + delta
	}

}

func (c *NodeStorageAllocatedCache) allocateDeviceForState(nodeName string, deviceName string, requestSize int64) {
	nodeStoragePool, ok := c.states[nodeName]
	if !ok {
		nodeStoragePool = NewNodeStorageState()
		c.states[nodeName] = nodeStoragePool
	}

	nodeStoragePool.DeviceStates.AllocateDevice(deviceName, requestSize)
}

func (c *NodeStorageAllocatedCache) revertDeviceForState(nodeName string, deviceName string) {
	nodeStoragePool, ok := c.states[nodeName]
	if !ok {
		return
	}
	if nodeStoragePool.DeviceStates == nil {
		return
	}
	nodeStoragePool.DeviceStates.RevertDevice(deviceName)
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
