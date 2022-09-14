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
	"github.com/alibaba/open-local/pkg/utils"
	"time"

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

func CreateTestCache() (*NodeStorageAllocatedCache, kubeclientset.Interface) {
	kubeclient := k8sfake.NewSimpleClientset()

	k8sInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclient, noResyncPeriodFunc())

	k8sInformerFactory.Start(context.Background().Done())

	k8sInformerFactory.WaitForCacheSync(context.Background().Done())

	return NewNodeStorageAllocatedCache(k8sInformerFactory.Core().V1()), kubeclient
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
