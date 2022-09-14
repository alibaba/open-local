/*
Copyright 2022/9/6 Alibaba Cloud.

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
package utils

import (
	"context"
	"encoding/json"
	"fmt"

	localtype "github.com/alibaba/open-local/pkg"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

func PatchAllocateInfoToPV(kubeclientset kubernetes.Interface, originPV *corev1.PersistentVolume, pvAllocatedInfo *localtype.PVAllocatedInfo) error {

	newPV := originPV.DeepCopy()
	if newPV.Annotations == nil {
		newPV.Annotations = map[string]string{}
	}

	infoJson, err := json.Marshal(pvAllocatedInfo)
	if err != nil {
		return fmt.Errorf("patchAllocateInfoToPV to PV fail: pv(%s),error: %s", originPV.Name, err.Error())
	}

	newPV.Annotations[localtype.AnnotationPVAllocatedInfoKey] = string(infoJson)
	patchBytes, err := GeneratePVPatch(originPV, newPV)
	if err != nil {
		return fmt.Errorf("GeneratePVPatch fail: allocateInfo(%+v), pv(%s), error: %s", pvAllocatedInfo, originPV.Name, err.Error())
	}
	if string(patchBytes) == "{}" {
		return nil
	}
	_, err = kubeclientset.CoreV1().PersistentVolumes().Patch(context.Background(), originPV.Name, apimachinerytypes.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patch vgName to PV fail: allocateInfo(%+v), pv(%s), error: %s", pvAllocatedInfo, originPV.Name, err.Error())
	}
	return nil
}
