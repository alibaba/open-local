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

package csi

import (
	"reflect"
	"testing"

	"github.com/alibaba/open-local/pkg"
	fakelocalclientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned/fake"
	spdk "github.com/alibaba/open-local/pkg/utils/spdk"
	"github.com/container-storage-interface/spec/lib/go/csi"
	fakesnapclientset "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned/fake"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	mountutils "k8s.io/mount-utils"
	testingexec "k8s.io/utils/exec/testing"
)

func Test_nodeServer_NodePublishVolume(t *testing.T) {
	type fields struct {
		ephemeralVolumeStore Store
		inFlight             *InFlight
		spdkSupported        bool
		spdkclient           *spdk.SpdkClient
		osTool               OSTool
		options              *driverOptions
	}
	type args struct {
		ctx context.Context
		req *csi.NodePublishVolumeRequest
	}

	findmntAction := func() ([]byte, []byte, error) {
		return []byte("TYPE=ext4"), []byte{}, nil
	}
	blkidAction := func() ([]byte, []byte, error) {
		return []byte("DEVICE=/dev/newVG/test-pv\nTYPE=ext4"), []byte{}, nil
	}
	// resize2fsAction := func() ([]byte, []byte, error) {
	// 	return []byte{}, []byte{}, nil
	// }

	ctx := context.Background()
	fakeKubeClient := fakekubeclientset.NewSimpleClientset()
	fakeLocalClient := fakelocalclientset.NewSimpleClientset()
	fakeSnapClient := fakesnapclientset.NewSimpleClientset()

	pvLVMName := "test-pv"
	pvMPName := "test-mp-pv"
	pvDeviceName := "test-device-pv"
	lvmPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvLVMName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-pvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
		},
	}
	mpPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvMPName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-pvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.MPName:        "/mnt/yoda/disk-0",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeMountPoint),
					},
				},
			},
		},
	}
	devicePV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvDeviceName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-pvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.DeviceName:    "/dev/sdd",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeDevice),
					},
				},
			},
		},
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakeKubeClient, 0)
	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes().Informer()
	kubeInformerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced, podInformer.HasSynced, pvcInformer.HasSynced, pvInformer.HasSynced)

	_, _ = fakeKubeClient.CoreV1().PersistentVolumes().Create(context.Background(), lvmPV, metav1.CreateOptions{})
	_, _ = fakeKubeClient.CoreV1().PersistentVolumes().Create(context.Background(), devicePV, metav1.CreateOptions{})
	_, _ = fakeKubeClient.CoreV1().PersistentVolumes().Create(context.Background(), mpPV, metav1.CreateOptions{})

	testfields := fields{
		ephemeralVolumeStore: NewMockVolumeStore(""),
		inFlight:             NewInFlight(),
		spdkSupported:        false,
		spdkclient:           nil,
		osTool:               NewFakeOSTool(),
		options: &driverOptions{
			kubeclient:  fakeKubeClient,
			snapclient:  fakeSnapClient,
			localclient: fakeLocalClient,
		},
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		scripts []testingexec.FakeAction
		want    *csi.NodePublishVolumeResponse
		wantErr bool
	}{
		{
			name:   "empty reqs",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{},
			},
			scripts: []testingexec.FakeAction{},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid reqs: empty targetPath",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId: pvLVMName,
				},
			},
			scripts: []testingexec.FakeAction{},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "mount fs lvm successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:   pvLVMName,
					TargetPath: "targetpath",
					VolumeContext: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
						pkg.PVName:        pvLVMName,
					},
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
				},
			},
			scripts: []testingexec.FakeAction{findmntAction, blkidAction},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: false,
		},
		{
			name:   "mount block lvm successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:   pvLVMName,
					TargetPath: "targetpath",
					VolumeContext: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
						pkg.PVName:        pvLVMName,
					},
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}},
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
				},
			},
			scripts: []testingexec.FakeAction{findmntAction, blkidAction},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: false,
		},
		{
			name:   "mount readonly fs lvm successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:   pvLVMName,
					TargetPath: "targetpath",
					VolumeContext: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
						pkg.PVName:        pvLVMName,
					},
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
					Readonly: true,
				},
			},
			scripts: []testingexec.FakeAction{findmntAction, blkidAction},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: false,
		},
		{
			name:   "mount snapshot lvm successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:   pvLVMName,
					TargetPath: "targetpath",
					VolumeContext: map[string]string{
						pkg.ParamVGName:       "newVG",
						pkg.VolumeTypeKey:     string(pkg.VolumeTypeLVM),
						pkg.ParamSnapshotName: "snapshot",
						pkg.ParamReadonly:     "true",
						pkg.PVName:            pvLVMName,
					},
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
				},
			},
			scripts: []testingexec.FakeAction{findmntAction, blkidAction},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: false,
		},
		{
			name:   "mount snapshot lvm failed: no readonly",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:   pvLVMName,
					TargetPath: "targetpath",
					VolumeContext: map[string]string{
						pkg.ParamVGName:       "newVG",
						pkg.VolumeTypeKey:     string(pkg.VolumeTypeLVM),
						pkg.ParamSnapshotName: "snapshot",
						pkg.PVName:            pvLVMName,
					},
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
				},
			},
			scripts: []testingexec.FakeAction{findmntAction, blkidAction},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "mount ephemeral lvm successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:   pvLVMName,
					TargetPath: "targetpath",
					VolumeContext: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
						pkg.Ephemeral:     "true",
					},
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
				},
			},
			scripts: []testingexec.FakeAction{findmntAction, blkidAction},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: false,
		},
		// todo: 未来重构 CSI 接口再测试 striping
		// {
		// 	name:   "mount ephemeral striping lvm successfully",
		// 	fields: testfields,
		// 	args: args{
		// 		ctx: context.Background(),
		// 		req: &csi.NodePublishVolumeRequest{
		// 			VolumeId:   pvName,
		// 			TargetPath: "targetpath",
		// 			VolumeContext: map[string]string{
		// 				pkg.ParamVGName:   "newVG",
		// 				pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
		// 				pkg.Ephemeral:     "true",
		// 				LvmTypeTag:        StripingType,
		// 			},
		// 			VolumeCapability: &csi.VolumeCapability{
		// 				AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
		// 				AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		// 			},
		// 		},
		// 	},
		// 	want:    &csi.NodePublishVolumeResponse{},
		// 	wantErr: false,
		// },
		{
			name:   "mount mountpoint successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:   pvMPName,
					TargetPath: "targetpath",
					VolumeContext: map[string]string{
						string(pkg.MPName): "/mnt/open-local/disk0",
						pkg.VolumeTypeKey:  string(pkg.VolumeTypeMountPoint),
						pkg.PVName:         pvMPName,
					},
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
				},
			},
			scripts: []testingexec.FakeAction{findmntAction, blkidAction},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: false,
		},
		{
			name:   "mount device successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:   pvDeviceName,
					TargetPath: "targetpath",
					VolumeContext: map[string]string{
						string(pkg.DeviceName): "/dev/sdd",
						pkg.VolumeTypeKey:      string(pkg.VolumeTypeDevice),
						pkg.PVName:             pvDeviceName,
					},
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
				},
			},
			scripts: []testingexec.FakeAction{findmntAction, blkidAction},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: false,
		},
		{
			name:   "mount block device successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:   pvDeviceName,
					TargetPath: "targetpath",
					VolumeContext: map[string]string{
						string(pkg.DeviceName): "/dev/sdd",
						pkg.VolumeTypeKey:      string(pkg.VolumeTypeDevice),
						pkg.PVName:             pvDeviceName,
					},
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}},
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
				},
			},
			scripts: []testingexec.FakeAction{findmntAction, blkidAction},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &nodeServer{
				k8smounter:           NewFakeSafeMounter(tt.scripts...),
				ephemeralVolumeStore: tt.fields.ephemeralVolumeStore,
				inFlight:             tt.fields.inFlight,
				spdkSupported:        tt.fields.spdkSupported,
				spdkclient:           tt.fields.spdkclient,
				osTool:               tt.fields.osTool,
				options:              tt.fields.options,
			}
			got, err := ns.NodePublishVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("nodeServer.NodePublishVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("nodeServer.NodePublishVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_nodeServer_NodeUnpublishVolume(t *testing.T) {
	type fields struct {
		ephemeralVolumeStore Store
		inFlight             *InFlight
		spdkSupported        bool
		spdkclient           *spdk.SpdkClient
		osTool               OSTool
		options              *driverOptions
	}
	type args struct {
		ctx context.Context
		req *csi.NodeUnpublishVolumeRequest
	}

	ctx := context.Background()
	fakeKubeClient := fakekubeclientset.NewSimpleClientset()
	fakeLocalClient := fakelocalclientset.NewSimpleClientset()
	fakeSnapClient := fakesnapclientset.NewSimpleClientset()

	pvLVMName := "test-pv"
	pvMPName := "test-mp-pv"
	pvDeviceName := "test-device-pv"
	lvmPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvLVMName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-pvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
		},
	}
	mpPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvMPName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-pvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.MPName:        "/mnt/yoda/disk-0",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeMountPoint),
					},
				},
			},
		},
	}
	devicePV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvDeviceName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-pvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.DeviceName:    "/dev/sdd",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeDevice),
					},
				},
			},
		},
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakeKubeClient, 0)
	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes().Informer()
	kubeInformerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced, podInformer.HasSynced, pvcInformer.HasSynced, pvInformer.HasSynced)

	_, _ = fakeKubeClient.CoreV1().PersistentVolumes().Create(context.Background(), lvmPV, metav1.CreateOptions{})
	_, _ = fakeKubeClient.CoreV1().PersistentVolumes().Create(context.Background(), devicePV, metav1.CreateOptions{})
	_, _ = fakeKubeClient.CoreV1().PersistentVolumes().Create(context.Background(), mpPV, metav1.CreateOptions{})

	testfields := fields{
		ephemeralVolumeStore: NewMockVolumeStore(""),
		inFlight:             NewInFlight(),
		spdkSupported:        false,
		spdkclient:           nil,
		osTool:               NewFakeOSTool(),
		options: &driverOptions{
			kubeclient:  fakeKubeClient,
			snapclient:  fakeSnapClient,
			localclient: fakeLocalClient,
		},
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		scripts []testingexec.FakeAction
		want    *csi.NodeUnpublishVolumeResponse
		wantErr bool
	}{
		{
			name:   "empty reqs",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodeUnpublishVolumeRequest{},
			},
			scripts: []testingexec.FakeAction{},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid reqs: empty targetPath",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodeUnpublishVolumeRequest{
					VolumeId: pvLVMName,
				},
			},
			scripts: []testingexec.FakeAction{},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "umount pv successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodeUnpublishVolumeRequest{
					VolumeId:   pvLVMName,
					TargetPath: "targetpath",
				},
			},
			scripts: []testingexec.FakeAction{},
			want:    &csi.NodeUnpublishVolumeResponse{},
			wantErr: false,
		},
		{
			name:   "umount ephemeral volume successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodeUnpublishVolumeRequest{
					VolumeId:   "test-ephemeral",
					TargetPath: "targetpath",
				},
			},
			scripts: []testingexec.FakeAction{},
			want:    &csi.NodeUnpublishVolumeResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &nodeServer{
				k8smounter:           NewFakeSafeMounter(tt.scripts...),
				ephemeralVolumeStore: tt.fields.ephemeralVolumeStore,
				inFlight:             tt.fields.inFlight,
				spdkSupported:        tt.fields.spdkSupported,
				spdkclient:           tt.fields.spdkclient,
				osTool:               tt.fields.osTool,
				options:              tt.fields.options,
			}
			got, err := ns.NodeUnpublishVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("nodeServer.NodeUnpublishVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("nodeServer.NodeUnpublishVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_nodeServer_NodeExpandVolume(t *testing.T) {
	type fields struct {
		k8smounter           *mountutils.SafeFormatAndMount
		ephemeralVolumeStore Store
		inFlight             *InFlight
		spdkSupported        bool
		spdkclient           *spdk.SpdkClient
		osTool               OSTool
		options              *driverOptions
	}
	type args struct {
		ctx context.Context
		req *csi.NodeExpandVolumeRequest
	}

	ctx := context.Background()
	fakeKubeClient := fakekubeclientset.NewSimpleClientset()
	fakeLocalClient := fakelocalclientset.NewSimpleClientset()
	fakeSnapClient := fakesnapclientset.NewSimpleClientset()

	pvLVMName := "test-pv"
	pvMPName := "test-mp-pv"
	pvDeviceName := "test-device-pv"
	lvmPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvLVMName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-pvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
		},
	}
	mpPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvMPName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-pvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.MPName:        "/mnt/yoda/disk-0",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeMountPoint),
					},
				},
			},
		},
	}
	devicePV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvDeviceName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-pvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.DeviceName:    "/dev/sdd",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeDevice),
					},
				},
			},
		},
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakeKubeClient, 0)
	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes().Informer()
	kubeInformerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced, podInformer.HasSynced, pvcInformer.HasSynced, pvInformer.HasSynced)

	_, _ = fakeKubeClient.CoreV1().PersistentVolumes().Create(context.Background(), lvmPV, metav1.CreateOptions{})
	_, _ = fakeKubeClient.CoreV1().PersistentVolumes().Create(context.Background(), devicePV, metav1.CreateOptions{})
	_, _ = fakeKubeClient.CoreV1().PersistentVolumes().Create(context.Background(), mpPV, metav1.CreateOptions{})

	testfields := fields{
		ephemeralVolumeStore: NewMockVolumeStore(""),
		inFlight:             NewInFlight(),
		spdkSupported:        false,
		spdkclient:           nil,
		osTool:               NewFakeOSTool(),
		options: &driverOptions{
			kubeclient:  fakeKubeClient,
			snapclient:  fakeSnapClient,
			localclient: fakeLocalClient,
		},
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *csi.NodeExpandVolumeResponse
		wantErr bool
	}{
		{
			name:   "empty reqs",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodeExpandVolumeRequest{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid reqs: empty targetPath",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodeExpandVolumeRequest{
					VolumeId: pvLVMName,
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "resize fs successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.NodeExpandVolumeRequest{
					VolumeId:   pvLVMName,
					VolumePath: "targetpath",
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: int64(150 * 1024 * 1024 * 1024),
					},
				},
			},
			want:    &csi.NodeExpandVolumeResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &nodeServer{
				k8smounter:           tt.fields.k8smounter,
				ephemeralVolumeStore: tt.fields.ephemeralVolumeStore,
				inFlight:             tt.fields.inFlight,
				spdkSupported:        tt.fields.spdkSupported,
				spdkclient:           tt.fields.spdkclient,
				osTool:               tt.fields.osTool,
				options:              tt.fields.options,
			}
			got, err := ns.NodeExpandVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("nodeServer.NodeExpandVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("nodeServer.NodeExpandVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}
