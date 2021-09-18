/*
Copyright © 2021 Alibaba Group Holding Ltd.

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

package statussyncer

import (
	"context"
	"fmt"
	"regexp"

	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm"
	"github.com/alibaba/open-local/pkg/scheduler/algorithm/cache"
	"github.com/alibaba/open-local/pkg/utils"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

const (
	EventReasonSuccess string = "Success"
	EventReasonFailed  string = "Failed"
)

type StatusSyncer struct {
	recorder record.EventRecorder
	client   clientset.Interface
	ctx      *algorithm.SchedulingContext
}

func NewStatusSyncer(recorder record.EventRecorder, c clientset.Interface, ctx *algorithm.SchedulingContext) *StatusSyncer {
	return &StatusSyncer{
		recorder: recorder,
		client:   c,
		ctx:      ctx,
	}
}

func (syncer *StatusSyncer) OnUpdateInitialized(nls *nodelocalstorage.NodeLocalStorage) {
	if need, err := syncer.isUpdateNeeded(nls); !need {
		log.Debugf("update status skipped")
		if err == nil && nls.Status.FilteredStorageInfo.UpdateStatus.Status == nodelocalstorage.UpdateStatusFailed {
			e := syncer.CleanStatus(nls)
			if e != nil {
				log.Errorf("CleanStatus failed: %s", e)
			}
		}

		return
	}

	nlsCopy := nls.DeepCopy()
	nlsCopy.Status.FilteredStorageInfo.VolumeGroups = FilterVGInfo(nlsCopy)
	nlsCopy.Status.FilteredStorageInfo.MountPoints = FilterMPInfo(nlsCopy)
	nlsCopy.Status.FilteredStorageInfo.Devices = FilterDeviceInfo(nlsCopy)
	nlsCopy.Status.FilteredStorageInfo.UpdateStatus.Status = nodelocalstorage.UpdateStatusAccepted
	nlsCopy.Status.FilteredStorageInfo.UpdateStatus.LastUpdateTime = metav1.Now()
	nlsCopy.Status.FilteredStorageInfo.UpdateStatus.Reason = ""

	// only update status
	_, err := syncer.client.CsiV1alpha1().NodeLocalStorages().UpdateStatus(context.Background(), nlsCopy, metav1.UpdateOptions{})
	if err != nil {
		log.Errorf("local storage CRD update Status FilteredStorageInfo error: %s", err.Error())
	}
	return
}

// isUpdateNeeded will check whether .status.filteredStorageInfo of nls need update
func (syncer *StatusSyncer) isUpdateNeeded(nls *nodelocalstorage.NodeLocalStorage) (bool, error) {
	tmpCache := syncer.ctx.ClusterNodeCache.GetNodeCache(nls.GetName())

	updateVG := false
	updateMP := false
	updateDevice := false

	// VG
	VGFiltered := FilterVGInfo(nls)
	if !SameStringSliceIgnoreOrder(VGFiltered, nls.Status.FilteredStorageInfo.VolumeGroups) {
		var VGFromCache []string
		for _, vg := range tmpCache.VGs {
			VGFromCache = append(VGFromCache, vg.Name)
		}
		addedVGs, unchangedVGs, removedVGs := utils.GetAddedAndRemovedItems(VGFiltered, VGFromCache)
		log.Debugf("added vgs: %#v", addedVGs)
		sum := len(addedVGs) + len(removedVGs)
		if sum != 0 {
			updateVG = true
		} else if sum == 0 && len(unchangedVGs) == len(VGFromCache) {
			updateVG = true
		}
	}

	// MountPoint
	MPFiltered := FilterMPInfo(nls)
	if !SameStringSliceIgnoreOrder(MPFiltered, nls.Status.FilteredStorageInfo.MountPoints) {
		var MPFromCache []string
		for _, mp := range tmpCache.MountPoints {
			MPFromCache = append(MPFromCache, mp.Name)
		}
		addedMountPoints, unchangedMountPoints, removedMountPoints := utils.GetAddedAndRemovedItems(MPFiltered, MPFromCache)
		log.Debugf("removed mountpoints: %#v", removedMountPoints)
		log.Debugf("added mountpoints: %#v", removedMountPoints)
		for _, mp := range removedMountPoints {
			if mpCache, exist := tmpCache.MountPoints[cache.ResourceName(mp)]; exist {
				if mpCache.IsAllocated {
					reason := fmt.Sprintf("update FilteredStorageInfo pre-check failed, mountpoint %s has been allocated", mpCache.Name)
					syncer.recorder.Eventf(nls, corev1.EventTypeNormal, EventReasonFailed,
						"update FilteredStorageInfo pre-check failed, mountpoint %s has been allocated", mpCache.Name)
					err := syncer.UpdateFailedStatus(nls, reason)
					if err != nil {
						log.Errorf("UpdateStatus failed: %s", err)
					}
					return false, fmt.Errorf(reason)
				} else {
					log.Debugf("mountpoint %s in node %s will be deleted", mp, nls.Name)
				}
			}
		}
		sum := len(removedMountPoints) + len(addedMountPoints)
		if sum != 0 {
			updateMP = true
		} else if sum == 0 && len(unchangedMountPoints) == len(MPFromCache) {
			updateMP = true
		}
	}

	// Device
	DeviceFiltered := FilterDeviceInfo(nls)
	if !SameStringSliceIgnoreOrder(DeviceFiltered, nls.Status.FilteredStorageInfo.Devices) {
		var DevFromCache []string
		for _, dev := range tmpCache.Devices {
			DevFromCache = append(DevFromCache, dev.Name)
		}
		addedDeivces, unchangedDeivces, removedDevices := utils.GetAddedAndRemovedItems(DeviceFiltered, DevFromCache)
		log.Debugf("removed devices: %#v", removedDevices)
		log.Debugf("added devices: %#v", addedDeivces)
		for _, dev := range removedDevices {
			if devCache, exist := tmpCache.Devices[cache.ResourceName(dev)]; exist {
				if devCache.IsAllocated {
					reason := fmt.Sprintf("update FilteredStorageInfo pre-check failed, device %s has been allocated", devCache.Name)
					syncer.recorder.Eventf(nls, corev1.EventTypeNormal, EventReasonFailed,
						"update FilteredStorageInfo pre-check failed, device %s has been allocated", devCache.Name)
					err := syncer.UpdateFailedStatus(nls, reason)
					if err != nil {
						log.Errorf("UpdateStatus failed: %s", err)
					}
					return false, fmt.Errorf(reason)
				} else {
					log.Debugf("device %s in node %s will be deleted", dev, nls.Name)
				}
			}
		}
		sum := len(removedDevices) + len(addedDeivces)
		if sum != 0 {
			updateDevice = true
		} else if sum == 0 && len(unchangedDeivces) == len(DevFromCache) {
			updateDevice = true
		}
	}

	return (updateVG || updateMP || updateDevice), nil
}

func (syncer *StatusSyncer) UpdateFailedStatus(nls *nodelocalstorage.NodeLocalStorage, reason string) error {
	nlsCopy := nls.DeepCopy()
	nlsCopy.Status.FilteredStorageInfo.UpdateStatus.LastUpdateTime = metav1.Now()
	nlsCopy.Status.FilteredStorageInfo.UpdateStatus.Reason = reason
	nlsCopy.Status.FilteredStorageInfo.UpdateStatus.Status = nodelocalstorage.UpdateStatusFailed
	// only update status
	_, err := syncer.client.CsiV1alpha1().NodeLocalStorages().UpdateStatus(context.Background(), nlsCopy, metav1.UpdateOptions{})
	if err != nil {
		log.Errorf("local storage CRD update Status FilteredStorageInfo error: %s", err.Error())
		return err
	}
	return nil
}

func (syncer *StatusSyncer) CleanStatus(nls *nodelocalstorage.NodeLocalStorage) error {
	nlsCopy := nls.DeepCopy()
	nlsCopy.Status.FilteredStorageInfo.UpdateStatus.Status = nodelocalstorage.UpdateStatusAccepted
	nlsCopy.Status.FilteredStorageInfo.UpdateStatus.Reason = ""
	nlsCopy.Status.FilteredStorageInfo.UpdateStatus.LastUpdateTime = metav1.Now()
	// only update status
	_, err := syncer.client.CsiV1alpha1().NodeLocalStorages().UpdateStatus(context.Background(), nlsCopy, metav1.UpdateOptions{})
	if err != nil {
		log.Errorf("local storage CRD update Status FilteredStorageInfo error: %s", err.Error())
		return err
	}
	return nil
}

func FilterVGInfo(nls *nodelocalstorage.NodeLocalStorage) []string {
	var vgSlice []string
	for _, vg := range nls.Status.NodeStorageInfo.VolumeGroups {
		vgSlice = append(vgSlice, vg.Name)
	}

	return FilterInfo(vgSlice, nls.Spec.ListConfig.VGs.Include, nls.Spec.ListConfig.VGs.Exclude)
}

func FilterMPInfo(nls *nodelocalstorage.NodeLocalStorage) []string {
	var mpSlice []string
	for _, mp := range nls.Status.NodeStorageInfo.MountPoints {
		mpSlice = append(mpSlice, mp.Name)
	}

	return FilterInfo(mpSlice, nls.Spec.ListConfig.MountPoints.Include, nls.Spec.ListConfig.MountPoints.Exclude)
}

func FilterDeviceInfo(nls *nodelocalstorage.NodeLocalStorage) []string {
	var devSlice []string
	for _, dev := range nls.Status.NodeStorageInfo.DeviceInfos {
		devSlice = append(devSlice, dev.Name)
	}

	return FilterInfo(devSlice, nls.Spec.ListConfig.Devices.Include, nls.Spec.ListConfig.Devices.Exclude)
}

func FilterInfo(info []string, include []string, exclude []string) []string {
	filterMap := make(map[string]string, len(info))

	for _, inc := range include {
		reg := regexp.MustCompile(inc)
		for _, i := range info {
			if reg.FindString(i) == i {
				filterMap[i] = i
			}
		}
	}
	for _, exc := range exclude {
		reg := regexp.MustCompile(exc)
		for _, i := range info {
			if reg.FindString(i) == i {
				delete(filterMap, i)
			}
		}
	}

	var filterSlice []string
	for vg := range filterMap {
		filterSlice = append(filterSlice, vg)
	}

	return filterSlice
}

func SameStringSliceIgnoreOrder(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}
	// create a map of string -> int
	diff := make(map[string]int, len(x))
	for _, _x := range x {
		// 0 value for int is 0, so just increment a counter for the string
		diff[_x]++
	}
	for _, _y := range y {
		// If the string _y is not in diff bail out early
		if _, ok := diff[_y]; !ok {
			return false
		}
		diff[_y] -= 1
		if diff[_y] == 0 {
			delete(diff, _y)
		}
	}

	return len(diff) == 0
}
