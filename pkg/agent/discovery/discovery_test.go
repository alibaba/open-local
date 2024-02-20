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

package discovery

import (
	localv1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func TestFilterInfo(t *testing.T) {
	type args struct {
		in      []string
		include []string
		exclude []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "test1",
			args: args{
				in:      []string{"paas1", "paas"},
				include: []string{"paas[0-9]*"},
				exclude: []string{"paas[0-1]+"},
			},
			want: []string{"paas"},
		},
		{
			name: "test2",
			args: args{
				in:      []string{"paas4", "share"},
				include: []string{"paas[0-9]+", "share"},
				exclude: []string{},
			},
			want: []string{"paas4", "share"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FilterInfo(tt.args.in, tt.args.include, tt.args.exclude); !sameStringSlice(got, tt.want) {
				t.Errorf("FilterInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func sameStringSlice(x, y []string) bool {
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

func TestIsNlsStatusChanged(t *testing.T) {
	type args struct {
		oldStatus *localv1alpha1.NodeLocalStorageStatus
		newStatus *localv1alpha1.NodeLocalStorageStatus
	}
	updatedTime := metav1.Now()
	oldStatus := &localv1alpha1.NodeLocalStorageStatus{
		NodeStorageInfo: localv1alpha1.NodeStorageInfo{
			DeviceInfos: []localv1alpha1.DeviceInfo{
				{
					Name: "/dev/sdb",
					MediaType: "hdd",
					Total: 100000000,
				},
			},
			VolumeGroups: []localv1alpha1.VolumeGroup{
				{
					Name: "pool0",
					PhysicalVolumes: []string{
						"/dev/sdb",
					},
					Total: 100000000,
					Allocatable: 100000000,
					Available: 100000000,
				},
			},
			Phase: "Running",
			State: localv1alpha1.StorageState{
				Status: localv1alpha1.ConditionTrue,
				Type: localv1alpha1.StorageReady,
			},
		},
		FilteredStorageInfo: localv1alpha1.FilteredStorageInfo{
			UpdateStatus: localv1alpha1.UpdateStatusInfo{
				LastUpdateTime: &updatedTime,
				Status: "accepted",
			},
			VolumeGroups: []string{"pool0"},
		},
	}

	newStatus := &localv1alpha1.NodeLocalStorageStatus{
		NodeStorageInfo: localv1alpha1.NodeStorageInfo{
			DeviceInfos: []localv1alpha1.DeviceInfo{
				{
					Name: "/dev/sdb",
					MediaType: "hdd",
					Total: 100000000,
				},
			},
			VolumeGroups: []localv1alpha1.VolumeGroup{
				{
					Name: "pool0",
					PhysicalVolumes: []string{
						"/dev/sdb",
					},
					Total: 100000000,
					Allocatable: 50000000,  // changed
					Available: 50000000,    // changed
				},
			},
			Phase: "Running",
			State: localv1alpha1.StorageState{
				Status: localv1alpha1.ConditionTrue,
				Type: localv1alpha1.StorageReady,
			},
		},
		FilteredStorageInfo: localv1alpha1.FilteredStorageInfo{
			UpdateStatus: localv1alpha1.UpdateStatusInfo{
				LastUpdateTime: &updatedTime,
				Status: "accepted",
			},
			VolumeGroups: []string{"pool0"},
		},
	}

	newStatusOnlyTimeChanged := oldStatus
	newUpdateTime := updatedTime.Add(1*time.Minute)
	newStatusOnlyTimeChanged.FilteredStorageInfo.UpdateStatus.LastUpdateTime = &metav1.Time{Time: newUpdateTime}
	newStatusOnlyTimeChanged.NodeStorageInfo.State.LastTransitionTime = &metav1.Time{Time: newUpdateTime}
	newStatusOnlyTimeChanged.NodeStorageInfo.State.LastHeartbeatTime = &metav1.Time{Time: newUpdateTime}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test1",
			args: args{
				oldStatus: oldStatus,
				newStatus: newStatus,
			},
			want: true,
		},
		{
			name: "test2",
			args: args{
				oldStatus: oldStatus,
				newStatus: newStatusOnlyTimeChanged,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNlsStatusChanged(tt.args.oldStatus, tt.args.newStatus); got != tt.want {
				t.Errorf("isNlsStatusChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}