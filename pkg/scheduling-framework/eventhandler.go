package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

func (plugin *LocalPlugin) OnNodeLocalStorageAdd(obj interface{}) {
	// check
	nodeLocal, ok := obj.(*nodelocalstorage.NodeLocalStorage)
	if !ok {
		klog.Errorf("[OnNodeLocalStorageAdd]cannot convert to *NodeLocalStorage: %v", obj)
		return
	}

	// nodename
	nodeName := nodeLocal.Name
	plugin.cache.AddNodeStorage(nodeLocal)
	klog.V(4).Infof("[OnNodeLocalStorageAdd]node %s is handled", nodeName)
}

func (plugin *LocalPlugin) OnNodeLocalStorageUpdate(oldObj, newObj interface{}) {
	// check
	nodeLocal, ok := newObj.(*nodelocalstorage.NodeLocalStorage)
	if !ok {
		klog.Errorf("[OnNodeLocalStorageUpdate]cannot convert newObj to *NodeLocalStorage: %v", newObj)
		return
	}
	old, ok := oldObj.(*nodelocalstorage.NodeLocalStorage)
	if !ok {
		klog.Errorf("[OnNodeLocalStorageUpdate]cannot convert oldObj to *NodeLocalStorage: %v", oldObj)
		return
	}
	// nodename
	nodeName := nodeLocal.Name
	plugin.cache.UpdateNodeStorage(old, nodeLocal)
	klog.V(4).Infof("[OnNodeLocalStorageUpdate]node %s is handled", nodeName)
}

func (plugin *LocalPlugin) OnPVAdd(obj interface{}) {
	plugin.OnPVUpdate(nil, obj)
}

func (plugin *LocalPlugin) OnPVUpdate(oldObj, newObj interface{}) {
	// check
	pv, ok := newObj.(*corev1.PersistentVolume)
	if !ok {
		klog.Errorf("[OnPVUpdate] newObj cannot convert to *v1.PersistentVolume: %v", newObj)
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
	plugin.updatePV(pv)

	klog.Infof("[OnPVUpdate]pv %s is handled", pv.Name)
}

func (plugin *LocalPlugin) OnPVDelete(obj interface{}) {
	// check
	pv, ok := obj.(*corev1.PersistentVolume)
	if !ok {
		klog.Errorf("[OnPVDelete]cannot convert to *v1.PersistentVolume: %v", obj)
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

	plugin.deleteByPV(pv)
	klog.Infof("[OnPVDelete]pv %s is handled", pv.Name)
}

func (plugin *LocalPlugin) OnPVCAdd(obj interface{}) {
	plugin.OnPVCUpdate(nil, obj)
}

func (plugin *LocalPlugin) OnPVCUpdate(oldObj, newObj interface{}) {
	// check
	pvc, ok := newObj.(*corev1.PersistentVolumeClaim)
	if !ok {
		klog.Errorf("[OnPVCUpdate]cannot convert to *v1.PersistentVolumeClaim: %v", newObj)
		return
	}
	if pvc.Status.Phase != corev1.ClaimBound {
		return
	}
	pvName := utils.GetPVFromBoundPVC(pvc)
	if len(pvName) == 0 {
		klog.Errorf("failed to get PV for pvc %s/%s", pvc.Namespace, pvc.Name)
		return
	}

	nodeName := utils.NodeNameFromPVC(pvc)
	if nodeName == "" {
		return
	}

	plugin.allocatedByPVCEvent(nodeName, pvc, pvName)

	klog.Infof("[OnPVCUpdate]pvc %s/%s is handled", pvc.Namespace, pvc.Name)
}

func (plugin *LocalPlugin) OnPVCDelete(obj interface{}) {
	// check
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		klog.Errorf("[OnPVCDelete]cannot convert to *v1.PersistentVolumeClaim: %v", obj)
		return
	}

	plugin.cache.DeleteByPVC(pvc)
	klog.Infof("[OnPVCDelete]pvc %s/%s is handled", pvc.Namespace, pvc.Name)
}

func (plugin *LocalPlugin) OnPodAdd(obj interface{}) {
	// check
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		klog.Errorf("[OnPodAdd]cannot convert to *v1.Pod: %v", obj)
		return
	}

	plugin.cache.AddPod(pod)

	klog.Infof("[OnPodAdd]pod %s is handled", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
}

func (plugin *LocalPlugin) OnPodUpdate(oldObj, newObj interface{}) {
	// check
	pod, ok := newObj.(*corev1.Pod)
	if !ok {
		klog.Errorf("[OnPodUpdate]cannot convert to *v1.Pod: %v", newObj)
		return
	}

	plugin.cache.UpdatePod(pod)

	klog.Infof("[OnPodUpdate]pod %s is handled", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
}

func (plugin *LocalPlugin) OnPodDelete(obj interface{}) {
	// check
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		klog.Errorf("[OnPodDelete]cannot convert to *v1.Pod: %v", obj)
		return
	}
	plugin.cache.DeletePod(pod)

	klog.Infof("[OnPodDelete]pod %s is handled", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
}

// for lvm type pvc expand
func (plugin *LocalPlugin) allocatedByPVCEvent(nodeName string, pvc *corev1.PersistentVolumeClaim, volumeName string) error {
	plugin.cache.AddPVCInfo(pvc, nodeName, volumeName)
	pvDetail := plugin.cache.GetPVAllocatedDetailCopy(volumeName)
	if pvDetail == nil {
		return nil
	}
	switch pvDetail.GetVolumeType() {
	case localtype.VolumeTypeLVM:
		plugin.cache.AllocateLVMByPVCEvent(pvc, volumeName, nodeName)
	case localtype.VolumeTypeDevice:
		klog.V(6).Infof("device type pvc %s, type %s, have added by pv", volumeName, pvDetail.GetVolumeType())
		return nil
	default:
		klog.V(6).Infof("not a open-local pv %s, type %s, not add to cache", volumeName, pvDetail.GetVolumeType())
		return nil
	}
	return nil
}

func (plugin *LocalPlugin) updatePV(pv *corev1.PersistentVolume) {
	if pv.Status.Phase == corev1.VolumePending {
		klog.Infof("pv %s is in %s status, skipped", pv.Name, pv.Status.Phase)
		return
	}

	nodeName := utils.NodeNameFromPV(pv)
	if nodeName == "" {
		klog.Infof("pv %s is not a valid open-local local pv, skipped", pv.Name)
		return
	}
	isOpenLocalPV, pvType := utils.IsOpenLocalPV(pv, false)
	if !isOpenLocalPV {
		return
	}

	switch localtype.VolumeType(pvType) {
	case localtype.VolumeTypeLVM:
		plugin.cache.AllocateLVMByPV(pv, nodeName)
	case localtype.VolumeTypeDevice:
		plugin.cache.AllocateDevice(pv, nodeName)
	default:
		klog.V(6).Infof("not a open-local pv %s, type %s, not add to cache", pv.Name, pvType)
		return
	}

}

func (plugin *LocalPlugin) deleteByPV(pv *corev1.PersistentVolume) {
	nodeName := utils.NodeNameFromPV(pv)
	if nodeName == "" {
		klog.Infof("pv %s is not a valid open-local local pv, skipped", pv.Name)
		return
	}
	isOpenLocalPV, pvType := utils.IsOpenLocalPV(pv, false)
	if !isOpenLocalPV {
		return
	}

	switch localtype.VolumeType(pvType) {
	case localtype.VolumeTypeLVM:
		plugin.cache.DeleteLVM(pv, nodeName)
	case localtype.VolumeTypeDevice:
		plugin.cache.DeleteDevice(pv, nodeName)
	default:
		klog.V(6).Infof("not a open-local pv %s, type %s, not add to cache", pv.Name, pvType)
		return
	}
}

func (plugin *LocalPlugin) addAllocatedInfoToNLS(nodeName string, pvcInfos []localtype.NodeStoragePVCAllocateInfo) error {

	if len(pvcInfos) == 0 {
		return nil
	}

	err := retry.RetryOnConflict(
		retry.DefaultRetry, func() error {
			nls, err := plugin.localClientSet.CsiV1alpha1().NodeLocalStorages().Get(context.Background(), nodeName, metav1.GetOptions{ResourceVersion: "0"})
			if err != nil {
				klog.Errorf("update pvc(%+v) allocateInfo to nls(%s) fail, get nls error : %s", pvcInfos, nodeName, err.Error())
				return err
			}
			newNls := nls.DeepCopy()
			if newNls.Annotations == nil {
				newNls.Annotations = map[string]string{}
			}

			nlsAllocateInfo, err := localtype.GetAllocateInfoFromNLS(newNls)
			if err != nil {
				klog.Errorf("update pvc(%+v) allocateInfo to nls(%s) fail, get allocate info from nls error : %s", pvcInfos, nodeName, err.Error())
				return err
			}

			if nlsAllocateInfo == nil {
				nlsAllocateInfo = &localtype.NodeStorageAllocateInfo{}
			}

			if nlsAllocateInfo.PvcAllocates == nil {
				nlsAllocateInfo.PvcAllocates = map[string]localtype.NodeStoragePVCAllocateInfo{}
			}

			for _, pvcInfo := range pvcInfos {
				if oldPvcInfo, ok := nlsAllocateInfo.PvcAllocates[utils.GetPVCKey(pvcInfo.PVCNameSpace, pvcInfo.PVCName)]; ok {
					if oldPvcInfo.VGName == pvcInfo.VGName && oldPvcInfo.DeviceName == pvcInfo.DeviceName {
						klog.V(6).Infof("skip update pvc(%s/%s) allocateInfo to nls(%s), info exist!", pvcInfo.PVCNameSpace, pvcInfo.PVCName, nodeName)
						continue
					}
				}
				nlsAllocateInfo.PvcAllocates[utils.GetPVCKey(pvcInfo.PVCNameSpace, pvcInfo.PVCName)] = pvcInfo
			}

			infoJsonBytes, err := json.Marshal(nlsAllocateInfo)
			if err != nil {
				klog.Errorf("update pvc(%+v) allocateInfo to nls(%s) fail, marshal allocate info error : %s", pvcInfos, nodeName, err.Error())
				return err
			}
			newNls.Annotations[localtype.AnnotationNodeStorageAllocatedInfoKey] = string(infoJsonBytes)
			_, err = plugin.localClientSet.CsiV1alpha1().NodeLocalStorages().Update(context.Background(), newNls, metav1.UpdateOptions{})
			if err != nil {
				klog.Errorf("update pvc(%+v) allocateInfo to nls(%s) fail,error : %s", pvcInfos, nodeName, err.Error())
				return err
			}
			return nil
		})

	if err != nil {
		klog.Errorf("update pvc(%+v) allocateInfo to nls(%s) fail after retry ,error : %s", pvcInfos, nodeName, err.Error())
		return err
	}

	klog.V(4).Infof("update pvc(%+v) allocateInfo to nls(%s) success", pvcInfos, nodeName)
	return nil
}
