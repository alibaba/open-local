package apis

// import (
// 	"testing"

// 	corev1 "k8s.io/api/core/v1"
// 	"k8s.io/apimachinery/pkg/api/resource"
// 	"github.com/oecp/open-local-storage-service/pkg/scheduler/algorithm"
// 	"github.com/oecp/open-local-storage-service/test/framework"
// )

// func TestExpandPVC(t *testing.T) {
// 	type args struct {
// 		ctx *algorithm.SchedulingContext
// 		pvc *corev1.PersistentVolumeClaim
// 	}
// 	newPVC := framework.DefaultLVMPVC.DeepCopy()
// 	req := newPVC.Spec.Resources.Requests
// 	req[corev1.ResourceStorage] = resource.MustParse("30G")
// 	newPVC.Spec.Resources.Requests = req

// 	newMPPVC := framework.DefaultMPPVC.DeepCopy()
// 	newMPPVC.Spec.Resources.Requests = req
// 	tests := []struct {
// 		name    string
// 		args    args
// 		wantErr bool
// 	}{
// 		{name: "test-expand-1", args: struct {
// 			ctx *algorithm.SchedulingContext
// 			pvc *corev1.PersistentVolumeClaim
// 		}{ctx: algorithm.MakeSchedulingContext(), pvc: newPVC}},
// 		{name: "test-expand-already-expanded", args: struct {
// 			ctx *algorithm.SchedulingContext
// 			pvc *corev1.PersistentVolumeClaim
// 		}{ctx: algorithm.MakeSchedulingContext(), pvc: framework.DefaultLVMPVC}},
// 		{name: "test-expand-unsupported-mountpoint", args: struct {
// 			ctx *algorithm.SchedulingContext
// 			pvc *corev1.PersistentVolumeClaim
// 		}{ctx: algorithm.MakeSchedulingContext(), pvc: newMPPVC}, wantErr: true},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if err := ExpandPVC(tt.args.ctx, tt.args.pvc); (err != nil) != tt.wantErr {
// 				t.Errorf("ExpandPVC() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }
