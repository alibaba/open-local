/*
Copyright 2022/8/28 Alibaba Cloud.

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
	"context"
	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"
	"time"

	localclientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	localfake "github.com/alibaba/open-local/pkg/generated/clientset/versioned/fake"
	localinformers "github.com/alibaba/open-local/pkg/generated/informers/externalversions"

	corev1 "k8s.io/api/core/v1"
	kubeinformers "k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
)

var (
	noResyncPeriodFunc = func() time.Duration {
		klog.Info("test noResyncPeriodFunc")
		return 0
	}
)

func CreateTestCache() (*NodeStorageAllocatedCache, localclientset.Interface, kubeclientset.Interface) {
	kubeclient := k8sfake.NewSimpleClientset()
	localclient := localfake.NewSimpleClientset()

	k8sInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclient, noResyncPeriodFunc())
	localInformerFactory := localinformers.NewSharedInformerFactory(localclient, noResyncPeriodFunc())

	k8sInformerFactory.Start(context.Background().Done())
	localInformerFactory.Start(context.Background().Done())

	k8sInformerFactory.WaitForCacheSync(context.Background().Done())
	localInformerFactory.WaitForCacheSync(context.Background().Done())

	return NewNodeStorageAllocatedCache(k8sInformerFactory.Core().V1(), localInformerFactory.Csi().V1alpha1()), localclient, kubeclient
}

func CreateTestPVAllocatedDetailsFromPVs(pvsBound []*corev1.PersistentVolume) *PVAllocatedDetails {
	details := NewPVAllocatedDetails()
	for _, pv := range pvsBound {
		isLocalPV, volumeType := utils.IsOpenLocalPV(pv, false)
		if !isLocalPV {
			continue
		}
		_, nodeName := utils.IsLocalPV(pv)
		if nodeName == "" {
			continue
		}

		switch volumeType {
		case pkg.VolumeTypeLVM:
			vgName := utils.GetVGNameFromCsiPV(pv)
			if vgName == "" {
				continue
			}
			details.AssumeByPV(NewLVMPVAllocatedFromPV(pv, vgName, nodeName))
		case pkg.VolumeTypeDevice:
			deviceName := utils.GetDeviceNameFromCsiPV(pv)
			if deviceName == "" {
				continue
			}
			details.AssumeByPV(NewDeviceTypePVAllocatedFromPV(pv, deviceName, nodeName))
		}
	}
	return details
}

func AddPVDetails(details *PVAllocatedDetails, pvDetails []PVAllocated) *PVAllocatedDetails {
	for _, detail := range pvDetails {
		details.AssumeByPV(detail)
	}
	return details
}

func AddPVCDetails(details *PVAllocatedDetails, pvcDetails []PVAllocated) *PVAllocatedDetails {
	for _, detail := range pvcDetails {
		details.AssumeByPVC(detail)
	}
	return details
}

func CreateInlineVolumes(podName, volumeName string, volumeSize int64) *InlineVolumeAllocated {
	return &InlineVolumeAllocated{
		VgName:       utils.VGSSD,
		VolumeName:   volumeName,
		VolumeSize:   volumeSize,
		PodName:      podName,
		PodNamespace: utils.LocalNameSpace,
	}
}
