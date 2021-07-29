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

package scheduler

import (
	"reflect"
	"testing"

	"github.com/oecp/open-local/pkg"
)

func TestExtenderOptions_GetWeight(t *testing.T) {
	type fields struct {
		Master                  string
		Kubeconfig              string
		DataDir                 string
		Port                    int32
		EnabledNodeAntiAffinity string
	}
	bareWeight := pkg.NewNodeAntiAffinityWeight()
	bareWeight.Put(pkg.VolumeTypeMountPoint, 5)

	bareWeight_2 := pkg.NewNodeAntiAffinityWeight()
	bareWeight_2.Put(pkg.VolumeTypeMountPoint, 5)
	bareWeight_2.Put(pkg.VolumeTypeDevice, 2)

	tests := []struct {
		name        string
		fields      fields
		wantWeights *pkg.NodeAntiAffinityWeight
		wantErr     bool
	}{
		{
			name:        "test-bare-name",
			fields:      fields{EnabledNodeAntiAffinity: "MountPoint=5"},
			wantWeights: bareWeight,
			wantErr:     false,
		},
		{
			name:        "test-bare-name-2-fields",
			fields:      fields{EnabledNodeAntiAffinity: "MountPoint=5,Device=2"},
			wantWeights: bareWeight_2,
			wantErr:     false,
		},
		{
			name:    "test-invalid-format",
			fields:  fields{EnabledNodeAntiAffinity: "MountPoint=5,=Device=2"},
			wantErr: true,
		},
		{
			name:    "test-invalid-range",
			fields:  fields{EnabledNodeAntiAffinity: "MountPoint=15,Device=2"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			option := &ExtenderOptions{
				Master:                  tt.fields.Master,
				Kubeconfig:              tt.fields.Kubeconfig,
				DataDir:                 tt.fields.DataDir,
				Port:                    tt.fields.Port,
				EnabledNodeAntiAffinity: tt.fields.EnabledNodeAntiAffinity,
			}
			gotWeights, err := option.ParseWeight()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetWeight() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotWeights, tt.wantWeights) {
				t.Errorf("GetWeight() gotWeights = %v, want %v", gotWeights, tt.wantWeights)
			}
		})
	}
}
