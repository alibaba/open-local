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

package controller

import (
	"reflect"
	"testing"
	"time"

	"github.com/alibaba/open-local/pkg/agent/common"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	lssfake "github.com/alibaba/open-local/pkg/generated/clientset/versioned/fake"
	volumesnapshot "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned"
	volumesnapshotfake "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned/fake"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

var (
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

func TestNewAgent(t *testing.T) {
	type args struct {
		config        *common.Configuration
		kubeclientset kubernetes.Interface
		lssclientset  clientset.Interface
		snapclientset volumesnapshot.Interface
	}

	lssclient := lssfake.NewSimpleClientset()
	kubeclient := k8sfake.NewSimpleClientset()
	snapclient := volumesnapshotfake.NewSimpleClientset()
	tmpargs := args{
		config: &common.Configuration{
			Nodename:         "test-node",
			SysPath:          "",
			MountPath:        "",
			DiscoverInterval: common.DefaultInterval,
			CRDSpec:          nil,
			CRDStatus:        nil,
		},
		kubeclientset: kubeclient,
		lssclientset:  lssclient,
		snapclientset: snapclient,
	}

	tests := []struct {
		name string
		args args
		want *Agent
	}{
		{
			name: "test",
			args: tmpargs,
			want: &Agent{
				Configuration: tmpargs.config,
				kubeclientset: kubeclient,
				lssclientset:  lssclient,
				snapclientset: snapclient,
				recorder:      nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewAgent(tt.args.config, tt.args.kubeclientset, tt.args.lssclientset, tt.args.snapclientset); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewAgent() = %v, want %v", got, tt.want)
			}
		})
	}
}
