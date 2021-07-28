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

package statussyncer

import (
	"reflect"
	"testing"
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
			if got := FilterInfo(tt.args.in, tt.args.include, tt.args.exclude); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSameStringSliceIgnoreOrder(t *testing.T) {
	type args struct {
		x []string
		y []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test1",
			args: args{
				x: []string{"share", "paas"},
				y: []string{"paas", "share"},
			},
			want: true,
		},
		{
			name: "test2",
			args: args{
				x: []string{"share", "paas"},
				y: []string{"paas", "share1"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SameStringSliceIgnoreOrder(tt.args.x, tt.args.y); got != tt.want {
				t.Errorf("SameStringSliceIgnoreOrder() = %v, want %v", got, tt.want)
			}
		})
	}
}
