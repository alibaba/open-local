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
