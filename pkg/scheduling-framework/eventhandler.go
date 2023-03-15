package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
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

	plugin.updatePV(pv)

	klog.V(4).Infof("[OnPVUpdate]pv %s is handled", pv.Name)
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

	plugin.deleteByPV(pv)
	klog.V(4).Infof("[OnPVDelete]pv %s is handled", pv.Name)
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
		klog.Errorf("failed to get PV for pvc %s", utils.GetName(pvc.ObjectMeta))
		return
	}

	nodeName := utils.NodeNameFromPVC(pvc)
	if nodeName == "" {
		return
	}

	plugin.cache.AddOrUpdatePVC(pvc, nodeName, pvName)

	klog.V(4).Infof("[OnPVCUpdate]pvc %s is handled", utils.GetName(pvc.ObjectMeta))
}

func (plugin *LocalPlugin) OnPVCDelete(obj interface{}) {
	// check
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		klog.Errorf("[OnPVCDelete]cannot convert to *v1.PersistentVolumeClaim: %v", obj)
		return
	}

	plugin.cache.DeletePVC(pvc)
	klog.V(4).Infof("[OnPVCDelete]pvc %s is handled", utils.GetName(pvc.ObjectMeta))
}

func (plugin *LocalPlugin) OnPodAdd(obj interface{}) {
	// check
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		klog.Errorf("[OnPodAdd]cannot convert to *v1.Pod: %v", obj)
		return
	}

	plugin.cache.AddPod(pod)

	klog.V(4).Infof("[OnPodAdd]pod %s is handled", utils.GetName(pod.ObjectMeta))
}

func (plugin *LocalPlugin) OnPodUpdate(oldObj, newObj interface{}) {
	// check
	pod, ok := newObj.(*corev1.Pod)
	if !ok {
		klog.Errorf("[OnPodUpdate]cannot convert to *v1.Pod: %v", newObj)
		return
	}

	plugin.cache.UpdatePod(pod)

	klog.V(4).Infof("[OnPodUpdate]pod %s is handled", utils.GetName(pod.ObjectMeta))
}

func (plugin *LocalPlugin) OnPodDelete(obj interface{}) {
	// check
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		klog.Errorf("[OnPodDelete]cannot convert to *v1.Pod: %v", obj)
		return
	}
	plugin.cache.DeletePod(pod)

	klog.V(4).Infof("[OnPodDelete]pod %s is handled", utils.GetName(pod.ObjectMeta))
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
	plugin.cache.AddOrUpdatePV(pv, nodeName)

}

func (plugin *LocalPlugin) deleteByPV(pv *corev1.PersistentVolume) {
	nodeName := utils.NodeNameFromPV(pv)
	if nodeName == "" {
		klog.Infof("pv %s is not a valid open-local local pv, skipped", pv.Name)
		return
	}
	plugin.cache.DeletePV(pv, nodeName)
}

func (plugin *LocalPlugin) patchAllocatedNeedMigrateToPod(ctx context.Context, originPod *corev1.Pod, pvcInfos map[string]localtype.PVCAllocateInfo) error {

	if len(pvcInfos) == 0 {
		return nil
	}

	newPod := originPod.DeepCopy()

	if newPod.Annotations == nil {
		newPod.Annotations = map[string]string{}
	}

	allocateInfo := localtype.PodPVCAllocateInfo{PvcAllocates: map[string]localtype.PVCAllocateInfo{}}

	for _, pvcInfo := range pvcInfos {
		allocateInfo.PvcAllocates[utils.GetNameKey(pvcInfo.PVCNameSpace, pvcInfo.PVCName)] = pvcInfo
	}

	infoJsonBytes, err := json.Marshal(allocateInfo)
	if err != nil {
		klog.Errorf("patch pvc allocateInfo(%#+v) to pod(%s) fail, marshal allocate info error : %s", pvcInfos, utils.GetName(newPod.ObjectMeta), err.Error())
		return err
	}
	newPod.Annotations[localtype.AnnotationPodPVCAllocatedNeedMigrateKey] = string(infoJsonBytes)

	patchBytes, err := utils.GeneratePodPatch(originPod, newPod)
	if err != nil {
		return fmt.Errorf("GeneratePVPatch fail: allocateInfo(%#+v) to pod(%s) ! error: %s", allocateInfo, utils.GetName(newPod.ObjectMeta), err.Error())
	}
	if string(patchBytes) == "{}" {
		return nil
	}

	err = retry.OnError(
		retry.DefaultRetry,
		errors.IsTooManyRequests,
		func() error {
			_, err := plugin.kubeClientSet.CoreV1().Pods(newPod.Namespace).
				Patch(ctx, newPod.Name, apimachinerytypes.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
			if err != nil {
				klog.Error("Failed to patch Pod %s, patch: %v, err: %v", utils.GetName(newPod.ObjectMeta), string(patchBytes), err)
			}
			return err
		})

	if err != nil {
		klog.Errorf("patch pvc allocateInfo(%#+v) to pod(%s) fail after retry ,error : %s", pvcInfos, utils.GetName(newPod.ObjectMeta), err.Error())
		return err
	}

	klog.V(4).Infof("patch pvc allocateInfo(%#+v) to pod(%s) success", pvcInfos, utils.GetName(newPod.ObjectMeta))
	return nil
}
