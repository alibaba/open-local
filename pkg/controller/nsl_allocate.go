/*
Copyright 2022/9/1 Alibaba Cloud.

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
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/api/errors"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

func (controller *Controller) addVGInfoToPVsOnNode(nslName string) error {
	//var errors []error
	nls, err := controller.nlsLister.Get(nslName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get nls fail, nls(%s), err: %s", nslName, err.Error())
	}
	info, err := localtype.GetAllocateInfoFromNLS(nls)
	if err != nil {
		return fmt.Errorf("addVGInfoToPV fail, can not get pv allocate info from nls(%s), err: %s", nslName, err.Error())
	}

	if info == nil || info.PvcAllocates == nil {
		return nil
	}

	var errorList []error
	for _, pvcAllocated := range info.PvcAllocates {
		pvc, err := controller.pvcLister.PersistentVolumeClaims(pvcAllocated.PVCNameSpace).Get(pvcAllocated.PVCName)
		if err != nil {
			if !errors.IsNotFound(err) {
				errorList = append(errorList, fmt.Errorf("failed to get pvc %s/%s", pvc.Namespace, pvc.Name))
			}
			continue
		}
		if pvc.Status.Phase != corev1.ClaimBound {
			continue
		}
		name := utils.GetPVFromBoundPVC(pvc)
		if len(name) == 0 {
			errorList = append(errorList, fmt.Errorf("failed to get PVName for pvc %s/%s", pvc.Namespace, pvc.Name))
			continue
		}
		pv, err := controller.pvLister.Get(name)
		if err != nil {
			errorList = append(errorList, fmt.Errorf("failed to get PV(%s) for pvc %s/%s error: %s", name, pvc.Namespace, pvc.Name, err.Error()))
			continue
		}

		controller.patchAllocateInfoToPV(pv, &pvcAllocated.PVAllocatedInfo, nslName)

	}
	if len(errorList) > 0 {
		return fmt.Errorf("errorList:%+v", errorList)
	}
	return nil
}

func (controller *Controller) patchPVByPVEvent(pvName string, pvcNameSpace, pvcName string) error {
	pv, err := controller.pvLister.Get(pvName)
	if err != nil {
		return err
	}

	pvc, err := controller.pvcLister.PersistentVolumeClaims(pvcNameSpace).Get(pvcName)
	if err != nil {
		return err
	}

	nodeName := utils.NodeNameFromPVC(pvc)
	if nodeName == "" {
		return nil
	}

	nls, err := controller.nlsLister.Get(nodeName)
	if err != nil {
		return err
	}

	allocateInfos, err := localtype.GetAllocateInfoFromNLS(nls)
	if err != nil {
		return err
	}
	if len(allocateInfos.PvcAllocates) <= 0 {
		klog.Infof("can not found nls(%s) allocate info", nodeName)
		return nil
	}

	pvcAllocateInfo, ok := allocateInfos.PvcAllocates[utils.GetPVCKey(pvcNameSpace, pvcName)]
	if !ok {
		klog.Infof("can not found pvc(%s) allocate info from nls(%s) ", utils.GetPVCKey(pvcNameSpace, pvcName), nodeName)
		return nil
	}

	return controller.patchAllocateInfoToPV(pv, &pvcAllocateInfo.PVAllocatedInfo, nodeName)
}

func (controller *Controller) patchAllocateInfoToPV(originPV *corev1.PersistentVolume, pvAllocatedInfo *localtype.PVAllocatedInfo, nodeName string) error {
	if originPV == nil {
		return nil
	}

	isLocal, volumeType := utils.IsOpenLocalPV(originPV, false)
	if !isLocal {
		return nil
	}

	//must skip if exist, for pv reuse case and static bound by volumeBinding plugin, nls info may not correct
	switch volumeType {
	case localtype.VolumeTypeLVM:
		vgName := utils.GetVGNameFromCsiPV(originPV)
		if vgName != "" {
			return nil
		}
	case localtype.DeviceName:
		deviceName := utils.GetVGNameFromCsiPV(originPV)
		if deviceName != "" {
			return nil
		}
	}

	newPV := originPV.DeepCopy()
	if newPV.Annotations == nil {
		newPV.Annotations = map[string]string{}
	}

	infoJson, err := json.Marshal(pvAllocatedInfo)
	if err != nil {
		return fmt.Errorf("patchAllocateInfoToPV to PV fail: pv(%s),error: %s", originPV.Name, err.Error())
	}

	newPV.Annotations[localtype.AnnotationPVAllocatedInfoKey] = string(infoJson)
	patchBytes, err := utils.GeneratePVPatch(originPV, newPV)
	if err != nil {
		return fmt.Errorf("GeneratePVPatch fail: allocateInfo(%+v), pv(%s), node %s! error: %s", pvAllocatedInfo, originPV.Name, nodeName, err.Error())
	}
	if string(patchBytes) == "{}" {
		return nil
	}
	controller.kubeclientset.CoreV1().PersistentVolumes().Patch(context.Background(), originPV.Name, apimachinerytypes.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patch vgName to PV fail: allocateInfo(%+v), pv(%s), node %s! error: %s", pvAllocatedInfo, originPV.Name, nodeName, err.Error())
	}
	return nil
}

func (controller *Controller) removePVCAllocatedInfoFromNLS(pvcNameSpace, pvcName string, nodeName string) error {
	nls, err := controller.localclientset.CsiV1alpha1().NodeLocalStorages().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		klog.Errorf("remove pvc(%s/%s) allocateInfo from nls(%s) fail, get nls error : %s", pvcNameSpace, pvcName, nodeName, err.Error())
		return err
	}

	if nls.Annotations == nil {
		klog.V(6).Infof("skip remove pvc(%s/%s) allocateInfo from nls(%s), no annotations!", pvcNameSpace, pvcName, nodeName, nodeName)
		return nil
	}

	nlsAllocateInfo, err := localtype.GetAllocateInfoFromNLS(nls)
	if err != nil {
		klog.Errorf("remove pvc(%s/%s) allocateInfo from nls(%s) fail, get allocate info from nls error : %s", pvcNameSpace, pvcName, nodeName, err.Error())
		return err
	}

	if nlsAllocateInfo == nil || nlsAllocateInfo.PvcAllocates == nil {
		klog.V(6).Infof("skip remove pvc(%s/%s) allocateInfo from nls(%s), no nlsAllocateInfo annotation!", pvcNameSpace, pvcName, nodeName, nodeName)
		return nil
	}

	if _, ok := nlsAllocateInfo.PvcAllocates[utils.GetPVCKey(pvcNameSpace, pvcName)]; !ok {
		klog.V(6).Infof("skip remove pvc(%s/%s) allocateInfo from nls(%s), no pvc allocateInfo exist!", pvcNameSpace, pvcName, nodeName, nodeName)
		return nil
	}

	newNls := nls.DeepCopy()

	delete(nlsAllocateInfo.PvcAllocates, utils.GetPVCKey(pvcNameSpace, pvcName))

	infoJsonBytes, err := json.Marshal(nlsAllocateInfo)
	if err != nil {
		klog.Errorf("remove pvc(%s/%s) allocateInfo from nls(%s) fail, marshal allocate info error : %s", pvcNameSpace, pvcName, nodeName, err.Error())
		return err
	}
	newNls.Annotations[localtype.AnnotationNodeStorageAllocatedInfoKey] = string(infoJsonBytes)
	_, err = controller.localclientset.CsiV1alpha1().NodeLocalStorages().Update(context.Background(), newNls, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("remove pvc(%s/%s) allocateInfo from nls(%s) fail,error : %s", pvcNameSpace, pvcName, nodeName, err.Error())
		return err
	}
	klog.V(4).Infof("remove pvc(%s/%s) allocateInfo from nls(%s) success", pvcNameSpace, pvcName, nodeName, nodeName)
	return nil
}
