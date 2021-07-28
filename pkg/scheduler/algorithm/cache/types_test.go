/*
Copyright 2021 OECP Authors.

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
	"reflect"
	"testing"

	"github.com/oecp/open-local-storage-service/pkg"
	"github.com/oecp/open-local-storage-service/pkg/utils"
	"github.com/oecp/open-local-storage-service/test/framework"
	corev1 "k8s.io/api/core/v1"
)

func MakePodPvcMappingNotReady() *PodPvcMapping {
	mapping := NewPodPvcMapping()
	mapping.PvcPod["default/pod-1-pvc-1"] = "default/pod-1"
	mapping.PvcPod["default/pod-1-pvc-2"] = "default/pod-1"
	mapping.PodPvcInfo["default/pod-1"] = map[string]bool{
		"default/pod-1-pvc-1": true,
		"default/pod-1-pvc-2": false,
	}
	return mapping
}

func MakePodPvcMappingReady() *PodPvcMapping {
	mapping := NewPodPvcMapping()
	mapping.PvcPod["default/pod-1-pvc-1"] = "default/pod-1"
	mapping.PvcPod["default/pod-1-pvc-2"] = "default/pod-1"
	mapping.PodPvcInfo["default/pod-1"] = map[string]bool{
		"default/pod-1-pvc-1": true,
		"default/pod-1-pvc-2": true,
	}
	return mapping
}
func TestIsPodPvcReady(t *testing.T) {
	pvc1 := framework.MakeDevicePVC("pod-1-pvc-1", "default", nil)
	pvc2 := framework.MakeDevicePVC("pod-1-pvc-2", "default", nil)
	nodeName := "testnode"
	pvc1.Annotations[pkg.AnnSelectedNode] = nodeName
	pvc2.Annotations[pkg.AnnSelectedNode] = nodeName
	tests := []struct {
		name    string
		isReady bool
		mapping *PodPvcMapping
		pvc     *corev1.PersistentVolumeClaim
	}{
		{name: "test-ready", isReady: true, mapping: MakePodPvcMappingReady(), pvc: pvc1},
		{name: "test-not-ready", isReady: false, mapping: MakePodPvcMappingNotReady(), pvc: pvc2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mapping.IsPodPvcReady(tt.pvc) != tt.isReady {
				t.Errorf("[%s]IsPodPvcReady is not as expected, expected %t, actual %t", tt.name, tt.isReady, tt.mapping.IsPodPvcReady(pvc1))
			}

		})
	}
}

func TestPodPvcMapping_PutPod(t *testing.T) {
	type args struct {
		podName string
		pvcs    []*corev1.PersistentVolumeClaim
	}

	pvc1 := framework.MakeDevicePVC("pod-1-pvc-1", "default", nil)
	pvc2 := framework.MakeDevicePVC("pod-1-pvc-2", "default", nil)
	pvcs := []*corev1.PersistentVolumeClaim{
		pvc1, pvc2,
	}

	pvcStatusInfo := NewPvcStatusInfo()
	pvcStatusInfo[utils.PVCName(pvc1)] = false
	pvcStatusInfo[utils.PVCName(pvc2)] = false

	tests := []struct {
		name               string
		args               args
		expectedPvcPod     map[string]string
		expectedPodPvcInfo map[string]PvcStatusInfo
	}{
		{name: "test-put-pod-1", args: args{
			podName: "pod-1",
			pvcs:    pvcs,
		}, expectedPvcPod: map[string]string{
			"default/pod-1-pvc-1": "pod-1",
			"default/pod-1-pvc-2": "pod-1",
		}, expectedPodPvcInfo: map[string]PvcStatusInfo{
			"pod-1": map[string]bool{
				"default/pod-1-pvc-1": false,
				"default/pod-1-pvc-2": false,
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPodPvcMapping()
			p.PutPod(tt.args.podName, tt.args.pvcs)
			if !reflect.DeepEqual(p.PvcPod, tt.expectedPvcPod) {
				t.Errorf("failed to check PvcPod, expected: %v, acutal: %v", tt.expectedPvcPod, p.PvcPod)
			}
			if !reflect.DeepEqual(p.PodPvcInfo, tt.expectedPodPvcInfo) {
				t.Errorf("failed to check PodPvcInfo, expected: %v, acutal: %v", tt.expectedPodPvcInfo, p.PodPvcInfo)
			}
		})
	}
}

func TestPodPvcMapping_DeletePod(t *testing.T) {

	mapping := MakePodPvcMappingNotReady()
	mapping2 := MakePodPvcMappingNotReady()
	type args struct {
		p                  *PodPvcMapping
		podName            string
		pvcs               []*corev1.PersistentVolumeClaim
		expectedPvcPod     map[string]string
		expectedPodPvcInfo map[string]PvcStatusInfo
	}

	pvc1 := framework.MakeDevicePVC("pod-1-pvc-1", "default", nil)
	pvc2 := framework.MakeDevicePVC("pod-1-pvc-2", "default", nil)
	pvcs := []*corev1.PersistentVolumeClaim{
		pvc1,
		pvc2,
	}
	tests := []struct {
		name string
		args args
	}{
		{name: "test-delete-pod-exists", args: args{
			p:                  mapping,
			podName:            "default/pod-1",
			pvcs:               pvcs,
			expectedPvcPod:     map[string]string{},
			expectedPodPvcInfo: map[string]PvcStatusInfo{},
		}},
		{name: "test-delete-pod-not-exists", args: args{
			p:                  mapping2,
			podName:            "default/pod-2",
			pvcs:               pvcs,
			expectedPvcPod:     mapping2.PvcPod,
			expectedPodPvcInfo: mapping2.PodPvcInfo,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.p.DeletePod(tt.args.podName, tt.args.pvcs)
			if !reflect.DeepEqual(tt.args.p.PvcPod, tt.args.expectedPvcPod) {
				t.Errorf("failed to check PvcPod, expected: %v, acutal: %v", tt.args.expectedPvcPod, tt.args.p.PvcPod)
			}
			if !reflect.DeepEqual(tt.args.p.PodPvcInfo, tt.args.expectedPodPvcInfo) {
				t.Errorf("failed to check PodPvcInfo, expected: %v, acutal: %v", tt.args.expectedPodPvcInfo, tt.args.p.PodPvcInfo)
			}
		})
	}
}

func TestPodPvcMapping_PutPvc(t *testing.T) {
	type fields struct {
		PvcInfo map[string]PvcStatusInfo
		PvcPod  map[string]string
	}
	type args struct {
		pvc *corev1.PersistentVolumeClaim
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PodPvcMapping{
				PodPvcInfo: tt.fields.PvcInfo,
				PvcPod:     tt.fields.PvcPod,
			}
			p.PutPvc(tt.args.pvc)
			// TODO(yuzhi.wx) test the p status

		})
	}
}

func TestPodPvcMapping_DeletePvc(t *testing.T) {
	type fields struct {
		PvcInfo map[string]PvcStatusInfo
		PvcPod  map[string]string
	}
	type args struct {
		pvc *corev1.PersistentVolumeClaim
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PodPvcMapping{
				PodPvcInfo: tt.fields.PvcInfo,
				PvcPod:     tt.fields.PvcPod,
			}
			p.DeletePvc(tt.args.pvc)
			// TODO(yuzhi.wx) test the p status

		})
	}
}

func TestBindingMap_IsPVCExists(t *testing.T) {
	type args struct {
		pvc string
	}
	tests := []struct {
		name string
		bm   BindingMap
		args args
		want bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.bm.IsPVCExists(tt.args.pvc); got != tt.want {
				t.Errorf("IsPVCExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewPodPvcMapping(t *testing.T) {
	tests := []struct {
		name string
		want *PodPvcMapping
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewPodPvcMapping(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewPodPvcMapping() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewPvcStatusInfo(t *testing.T) {
	tests := []struct {
		name string
		want PvcStatusInfo
	}{
		{
			name: "test-new-pvc-status-info",
			want: NewPvcStatusInfo(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewPvcStatusInfo(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewPvcStatusInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}
