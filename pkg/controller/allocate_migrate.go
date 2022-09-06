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
	"fmt"
	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

func (controller *Controller) addVGInfoToPVsForPod(podNameSpace, podName string) error {
	//var errors []error
	pod, err := controller.podLister.Pods(podNameSpace).Get(podName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get pod fail, pod(%s/%s), err: %s", pod.Namespace, pod.Name, err.Error())
	}
	info, err := localtype.GetAllocateInfoFromPod(pod)
	if err != nil {
		return fmt.Errorf("addVGInfoToPV fail, can not get pv allocate info from pod(%s/%s), err: %s", pod.Namespace, pod.Name, err.Error())
	}

	if info == nil || info.PvcAllocates == nil {
		return nil
	}

	var errorList []error
	for _, pvcAllocated := range info.PvcAllocates {
		pvc, err := controller.pvcLister.PersistentVolumeClaims(pvcAllocated.PVCNameSpace).Get(pvcAllocated.PVCName)
		if err != nil {
			errorList = append(errorList, fmt.Errorf("failed to get pvc %s/%s for pod(%s/%s), retry after", pvc.Namespace, pvc.Name, pod.Namespace, pod.Name))
			continue
		}
		if pvc.Status.Phase != corev1.ClaimBound {
			errorList = append(errorList, fmt.Errorf("pv, pvc(%s/%s) not bound for pod(%s/%s), status: %s, retry after", pvc.Namespace, pvc.Name, pod.Namespace, pod.Name, pvc.Status.Phase))
			continue
		}
		name := utils.GetPVFromBoundPVC(pvc)
		if len(name) == 0 {
			errorList = append(errorList, fmt.Errorf("failed to get PVName for pvc %s/%s with pod(%s/%s) , retry after", pvc.Namespace, pvc.Name, pod.Namespace, pod.Name))
			continue
		}
		pv, err := controller.pvLister.Get(name)
		if err != nil {
			errorList = append(errorList, fmt.Errorf("failed to get PV(%s) for pvc %s/%s with pod(%s/%s) error: %s, retry after", name, pvc.Namespace, pvc.Name, pod.Namespace, pod.Name, err.Error()))
			continue
		}

		err = controller.patchAllocateInfoToPV(pv, &pvcAllocated.PVAllocatedInfo)
		if err != nil {
			errorList = append(errorList, fmt.Errorf("failed to patch allocateInfo(%#v) to PV(%s) for pod %s/%s error: %s, retry after", pvcAllocated, name, pod.Namespace, pod.Name, err.Error()))
		}

	}
	if len(errorList) > 0 {
		return fmt.Errorf("errorList:%+v", errorList)
	}
	return nil
}

func (controller *Controller) patchAllocateInfoToPV(originPV *corev1.PersistentVolume, pvAllocatedInfo *localtype.PVAllocatedInfo) error {
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

	return utils.PatchAllocateInfoToPV(controller.kubeclientset, originPV, pvAllocatedInfo)
}
