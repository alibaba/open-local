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

package version

import (
	"testing"
)

func TestGetFullVersion(t *testing.T) {
	type args struct {
		longFormat bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test",
			args: args{
				longFormat: true,
			},
			want: "-",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetFullVersion(tt.args.longFormat); got != tt.want {
				t.Errorf("GetFullVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNameWithVersion(t *testing.T) {
	type args struct {
		longFormat bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test",
			args: args{
				longFormat: true,
			},
			want: "open-local: -",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NameWithVersion(tt.args.longFormat); got != tt.want {
				t.Errorf("NameWithVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
