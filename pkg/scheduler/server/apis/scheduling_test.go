package apis

//
//import (
//	"fmt"
//	"reflect"
//	"testing"
//	"github.com/oecp/open-local-storage-service/pkg"
//	"github.com/oecp/open-local-storage-service/pkg/scheduler"
//
//	corev1 "k8s.io/api/core/v1"
//	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm"
//	"github.com/oecp/open-local-storage-service/test/framework"
//)
//
//func TestSchedulingPVC(t *testing.T) {
//	type args struct {
//		ctx  *algorithm.SchedulingContext
//		pvc  *corev1.PersistentVolumeClaim
//		node *corev1.Node
//	}
//
//	pvcLVM := framework.MakeLSSLVMPVC("test-pvc-lvm", "default", nil)
//
//	tests := []struct {
//		name    string
//		args    args
//		want    *scheduler.BindingInfo
//		wantErr bool
//	}{
//		{
//			name: "test",
//			args: args{
//				ctx:  algorithm.MakeSchedulingContext(),
//				pvc:  pvcLVM,
//				node: framework.DefaultLSSNode,
//			},
//			want: &scheduler.BindingInfo{
//				Node:                  framework.DefaultLSSNode.Name,
//				Disk:                  "",
//				VgName:                framework.DefaultVGName,
//				Device:                "",
//				VolumeType:            pkg.LSSVolumeTypeLVM,
//				PersistentVolumeClaim: fmt.Sprintf("%s/%s", pvcLVM.Namespace, pvcLVM.Name),
//			},
//			wantErr: false,
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			got, err := SchedulingPVC(tt.args.ctx, tt.args.pvc, tt.args.node)
//			if (err != nil) != tt.wantErr {
//				t.Errorf("SchedulingPVC() error = %v, wantErr %v", err, tt.wantErr)
//				return
//			}
//			if !reflect.DeepEqual(got, tt.want) {
//				t.Errorf("SchedulingPVC() = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}
