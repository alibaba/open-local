/*
Copyright 2022/8/21 Alibaba Cloud.

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
package plugin

import (
	"fmt"

	"github.com/alibaba/open-local/pkg/scheduling-framework/cache"
	"github.com/alibaba/open-local/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

func (plugin *LocalPlugin) getPodLocalVolumeInfos(pod *corev1.Pod) (*cache.PodLocalVolumeInfo, error) {
	volumeInfos := cache.NewPodLocalVolumeInfo()
	//inlineVolume
	err := plugin.cache.PrefilterInlineVolumes(pod, volumeInfos)
	if err != nil {
		return volumeInfos, err
	}
	// pvc
	ns := pod.Namespace
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			name := v.PersistentVolumeClaim.ClaimName
			pvc, err := plugin.coreV1Informers.PersistentVolumeClaims().Lister().PersistentVolumeClaims(ns).Get(name)
			if err != nil {
				klog.Errorf("failed to get pvc by name %s/%s: %s", ns, name, err.Error())
				return volumeInfos, err
			}
			if pvc.Status.Phase == corev1.ClaimBound {
				klog.Infof("skip scheduling bound pvc %s/%s", pvc.Namespace, pvc.Name)
				continue
			}
			scName := pvc.Spec.StorageClassName
			if scName == nil {
				continue
			}
			_, err = plugin.scLister.Get(*scName)
			if err != nil {
				klog.Errorf("failed to get storage class by name %s: %s", *scName, err.Error())
				return volumeInfos, err
			}
			err = plugin.cache.PrefilterPVC(plugin.scLister, pvc, volumeInfos)
			if err != nil {
				return volumeInfos, err
			}
		}
	}
	return volumeInfos, nil
}

func (plugin *LocalPlugin) getInlineVolumeAllocates(pod *corev1.Pod) ([]*cache.InlineVolumeAllocated, error) {
	var inlineVolumeAllocates []*cache.InlineVolumeAllocated

	containInlineVolume, _ := utils.ContainInlineVolumes(pod)
	if !containInlineVolume {
		return nil, nil
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.CSI != nil && utils.ContainsProvisioner(volume.CSI.Driver) {
			vgName, size := utils.GetInlineVolumeInfoFromParam(volume.CSI.VolumeAttributes)
			if vgName == "" {
				return nil, fmt.Errorf("no vgName found in inline volume of Pod %s", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			}

			inlineVolumeAllocates = append(inlineVolumeAllocates, &cache.InlineVolumeAllocated{
				PodNamespace: pod.Namespace,
				PodName:      pod.Name,
				VolumeName:   volume.Name,
				VolumeSize:   size,
				VgName:       vgName,
			})
		}
	}
	return inlineVolumeAllocates, nil
}
