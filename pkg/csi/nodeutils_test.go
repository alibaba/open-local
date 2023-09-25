/*
Copyright © 2023 Alibaba Group Holding Ltd.

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

package csi

import (
	"testing"
)

func Test_requireThrottleIO(t *testing.T) {
	type args struct {
		volumeContext map[string]string
	}
	tests := []struct {
		name          string
		args          args
		wantR         bool
		wantBpsValue  int64
		wantIopsValue int64
		wantErr       bool
	}{
		{name: "test empty throttle value",
			args:  args{volumeContext: map[string]string{}},
			wantR: false, wantBpsValue: 0, wantIopsValue: 0, wantErr: false},
		{name: "test bps throttle value only",
			args:  args{volumeContext: map[string]string{"bps": "1024"}},
			wantR: true, wantBpsValue: 1024, wantIopsValue: 0, wantErr: false},
		{name: "test iops throttle value only",
			args:  args{volumeContext: map[string]string{"iops": "100"}},
			wantR: true, wantBpsValue: 0, wantIopsValue: 100, wantErr: false},
		{name: "test bps and iops throttle value",
			args:  args{volumeContext: map[string]string{"bps": "10240", "iops": "110"}},
			wantR: true, wantBpsValue: 10240, wantIopsValue: 110, wantErr: false},
		{name: "test bps quantity throttle value",
			args:  args{volumeContext: map[string]string{"bps": "1Mi", "iops": "110"}},
			wantR: true, wantBpsValue: 1048576, wantIopsValue: 110, wantErr: false},
		{name: "test invalid bps throttle value",
			args:  args{volumeContext: map[string]string{"bps": "abc"}},
			wantR: false, wantBpsValue: 0, wantIopsValue: 0, wantErr: true},
		{name: "test invalid iops throttle value",
			args:  args{volumeContext: map[string]string{"bps": "10240", "iops": "11b"}},
			wantR: false, wantBpsValue: 0, wantIopsValue: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, gotBpsValue, gotIopsValue, err := requireThrottleIO(tt.args.volumeContext)
			if (err != nil) != tt.wantErr {
				t.Errorf("requireThrottleIO() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotR != tt.wantR {
				t.Errorf("requireThrottleIO() gotR = %v, want %v", gotR, tt.wantR)
			}
			if gotBpsValue != tt.wantBpsValue {
				t.Errorf("requireThrottleIO() gotBpsValue = %v, want %v", gotBpsValue, tt.wantBpsValue)
			}
			if gotIopsValue != tt.wantIopsValue {
				t.Errorf("requireThrottleIO() gotIopsValue = %v, want %v", gotIopsValue, tt.wantIopsValue)
			}
		})
	}
}
