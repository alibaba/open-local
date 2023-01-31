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
	"fmt"
	"reflect"
	"testing"

	"github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/csi/adapter"
	"github.com/alibaba/open-local/pkg/csi/client"
	"github.com/alibaba/open-local/pkg/csi/server"
	fakelocalclientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned/fake"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/container-storage-interface/spec/lib/go/csi"
	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	fakesnapclientset "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned/fake"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type fields struct {
	inFlight           *InFlight
	pvcPodSchedulerMap *PvcPodSchedulerMap
	schedulerArchMap   *SchedulerArchMap
	adapter            adapter.Adapter
	nodeLister         corelisters.NodeLister
	podLister          corelisters.PodLister
	pvcLister          corelisters.PersistentVolumeClaimLister
	pvLister           corelisters.PersistentVolumeLister
	options            *driverOptions
}

func init() {
	client.MustRunThisWhenTest()
	server.MustRunThisWhenTest()
	server.StartFake()
}

func Test_controllerServer_CreateVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.CreateVolumeRequest
	}

	pvcPodSchedulerMap := newPvcPodSchedulerMap()
	pvcTemplate := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithoutVG().PVCPending})[0]
	// pvcForFW
	pvcForFW := pvcTemplate.DeepCopy()
	pvcForFW.Name = "pvcForFW"
	pvcForFW.SetAnnotations(map[string]string{
		pkg.AnnoSelectedNode: utils.NodeName4,
	})
	pvcPodSchedulerMap.Add(pvcForFW.Namespace, pvcForFW.Name, "ahe-scheduler")
	// pvcWithouNodeNameForFW
	pvcWithouNodeNameForFW := pvcTemplate.DeepCopy()
	pvcWithouNodeNameForFW.Name = "pvcWithouNodeNameForFW"
	pvcPodSchedulerMap.Add(pvcWithouNodeNameForFW.Namespace, pvcWithouNodeNameForFW.Name, "ahe-scheduler")
	// pvcForExtender
	pvcForExtender := pvcTemplate.DeepCopy()
	pvcForExtender.Name = "pvcForExtender"
	pvcForExtender.SetAnnotations(map[string]string{
		pkg.AnnoSelectedNode: utils.NodeName4,
	})
	pvcPodSchedulerMap.Add(pvcForExtender.Namespace, pvcForExtender.Name, "default")
	// pvcUnknown
	pvcUnknown := pvcTemplate.DeepCopy()
	pvcUnknown.Name = "pvcUnknown"
	pvcUnknown.SetAnnotations(map[string]string{
		pkg.AnnoSelectedNode: utils.NodeName4,
	})
	// pvcSnapshotForExtender
	pvcSnapshotForExtender := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVSnapshot().PVCPending})[0]
	pvcSnapshotForExtender.Name = "pvcSnapshotForExtender"
	pvcSnapshotForExtender.SetAnnotations(map[string]string{
		pkg.AnnoSelectedNode: utils.NodeName4,
	})
	pvcs := []*corev1.PersistentVolumeClaim{
		pvcForFW,
		pvcWithouNodeNameForFW,
		pvcForExtender,
		pvcUnknown,
		pvcSnapshotForExtender,
	}
	pvName := "test-pv"
	pvNameForSnapshot := "test-pv-snapshot"
	snapshotName := "test-snapshot"
	snapshotContentName := "test-content"
	snapshotClassName := "test-snapshotclass"
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      pvcForExtender.Name,
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
	// node
	node := utils.CreateNode(&utils.TestNodeInfo{
		NodeName:  utils.NodeName4,
		IPAddress: "127.0.0.1",
	})
	// snapshot
	volumesnapshot := &volumesnapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: "default",
		},
		Spec: volumesnapshotv1.VolumeSnapshotSpec{
			Source: volumesnapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: &pvcForExtender.Name,
			},
			VolumeSnapshotClassName: &snapshotClassName,
		},
	}
	volumesnapshotcontent := &volumesnapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: snapshotContentName,
		},
		Spec: volumesnapshotv1.VolumeSnapshotContentSpec{
			Source: volumesnapshotv1.VolumeSnapshotContentSource{
				VolumeHandle: &pvName,
			},
			VolumeSnapshotClassName: &snapshotClassName,
			DeletionPolicy:          volumesnapshotv1.VolumeSnapshotContentDelete,
			Driver:                  pkg.ProvisionerName,
			VolumeSnapshotRef: corev1.ObjectReference{
				Namespace: "default",
				Name:      snapshotName,
			},
		},
	}
	volumesnapshotclass := &volumesnapshotv1.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: snapshotClassName,
		},
		Parameters: map[string]string{
			pkg.ParamSnapshotReadonly: "true",
		},
		DeletionPolicy: volumesnapshotv1.VolumeSnapshotContentDelete,
		Driver:         pkg.ProvisionerName,
	}

	ctx := context.Background()
	fakeKubeClient := fakekubeclientset.NewSimpleClientset()
	fakeLocalClient := fakelocalclientset.NewSimpleClientset()
	fakeSnapClient := fakesnapclientset.NewSimpleClientset()

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakeKubeClient, 0)
	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes().Informer()
	kubeInformerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced, podInformer.HasSynced, pvcInformer.HasSynced, pvInformer.HasSynced)

	// client
	for _, pvc := range pvcs {
		if err := pvcInformer.GetIndexer().Add(pvc); err != nil {
			t.Errorf("fail to add pvc: %s", err.Error())
		}
	}
	if err := pvInformer.GetIndexer().Add(pv); err != nil {
		t.Errorf("fail to add pvc: %s", err.Error())
	}
	if err := nodeInformer.GetIndexer().Add(node); err != nil {
		t.Errorf("fail to add node: %s", err.Error())
	}
	_, _ = fakeSnapClient.SnapshotV1().VolumeSnapshots("default").Create(context.Background(), volumesnapshot, metav1.CreateOptions{})
	_, _ = fakeSnapClient.SnapshotV1().VolumeSnapshotContents().Create(context.Background(), volumesnapshotcontent, metav1.CreateOptions{})
	_, _ = fakeSnapClient.SnapshotV1().VolumeSnapshotClasses().Create(context.Background(), volumesnapshotclass, metav1.CreateOptions{})

	// pvcPodSchedulerMap
	testfields := fields{
		inFlight:           NewInFlight(),
		pvcPodSchedulerMap: pvcPodSchedulerMap,
		schedulerArchMap:   newSchedulerArchMap([]string{"default"}, []string{"ahe-scheduler"}),
		nodeLister:         kubeInformerFactory.Core().V1().Nodes().Lister(),
		podLister:          kubeInformerFactory.Core().V1().Pods().Lister(),
		pvcLister:          kubeInformerFactory.Core().V1().PersistentVolumeClaims().Lister(),
		pvLister:           kubeInformerFactory.Core().V1().PersistentVolumes().Lister(),
		adapter:            adapter.NewFakeAdapter(),
		options: &driverOptions{
			kubeclient:  fakeKubeClient,
			snapclient:  fakeSnapClient,
			localclient: fakeLocalClient,
		},
	}

	// CreateVolume: called with args {Name:yoda-a5c8ea42-9a10-4a0b-a399-8e41ba447b91 CapacityRange:required_bytes:10737418240  VolumeCapabilities:[mount:<fs_type:"ext4" > access_mode:<mode:SINGLE_NODE_WRITER > ] Parameters:map[csi.storage.k8s.io/pv/name:yoda-a5c8ea42-9a10-4a0b-a399-8e41ba447b91 csi.storage.k8s.io/pvc/name:minio-data-minio-1 csi.storage.k8s.io/pvc/namespace:default volumeType:LVM] Secrets:map[] VolumeContentSource:<nil> AccessibilityRequirements:requisite:<segments:<key:"kubernetes.io/hostname" value:"izrj91f4skdnkpv2z2grhcz" > > preferred:<segments:<key:"kubernetes.io/hostname" value:"izrj91f4skdnkpv2z2grhcz" > >  XXX_NoUnkeyedLiteral:{} XXX_unrecognized:[] XXX_sizecache:0}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *csi.CreateVolumeResponse
		wantErr bool
	}{
		{
			name:   "empty args",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid volume capabilities",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
						},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "no selected node in pvc anno",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
						pkg.PVName:        pvName,
						pkg.PVCNameSpace:  pvcWithouNodeNameForFW.Namespace,
						pkg.PVCName:       pvcWithouNodeNameForFW.Name,
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "no pvc info",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        pvName,
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "no pvc found",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        pvName,
						pkg.PVCNameSpace:  "non-existent-pvc-namespace",
						pkg.PVCName:       "non-existent-pvc-name",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "framework success for lvm",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        pvName,
						pkg.PVCNameSpace:  pvcForFW.Namespace,
						pkg.PVCName:       pvcForFW.Name,
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			want: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: int64(150 * 1024 * 1024 * 1024),
					VolumeId:      pvName,
					VolumeContext: map[string]string{
						pkg.PVName:           pvName,
						pkg.PVCNameSpace:     pvcForFW.Namespace,
						pkg.PVCName:          pvcForFW.Name,
						pkg.VolumeTypeKey:    string(pkg.VolumeTypeLVM),
						pkg.AnnoSelectedNode: utils.NodeName4,
					},
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								pkg.KubernetesNodeIdentityKey: utils.NodeName4,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:   "extender success for lvm: no volume created before",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: fmt.Sprintf("new-%s", pvName),
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        fmt.Sprintf("new-%s", pvName),
						pkg.PVCNameSpace:  pvcForExtender.Namespace,
						pkg.PVCName:       pvcForExtender.Name,
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			want: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: int64(150 * 1024 * 1024 * 1024),
					VolumeId:      fmt.Sprintf("new-%s", pvName),
					VolumeContext: map[string]string{
						pkg.PVName:           fmt.Sprintf("new-%s", pvName),
						pkg.PVCNameSpace:     pvcForExtender.Namespace,
						pkg.PVCName:          pvcForExtender.Name,
						pkg.VolumeTypeKey:    string(pkg.VolumeTypeLVM),
						pkg.AnnoSelectedNode: utils.NodeName4,
						pkg.VGName:           "newVG",
					},
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								pkg.KubernetesNodeIdentityKey: utils.NodeName4,
							},
						},
					},
				},
			},
		},
		{
			name:   "extender success for lvm: volume is already created",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        pvName,
						pkg.PVCNameSpace:  pvcForExtender.Namespace,
						pkg.PVCName:       pvcForExtender.Name,
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			want: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: int64(150 * 1024 * 1024 * 1024),
					VolumeId:      pvName,
					VolumeContext: map[string]string{
						pkg.PVName:           pvName,
						pkg.PVCNameSpace:     pvcForExtender.Namespace,
						pkg.PVCName:          pvcForExtender.Name,
						pkg.VolumeTypeKey:    string(pkg.VolumeTypeLVM),
						pkg.AnnoSelectedNode: utils.NodeName4,
						pkg.VGName:           "newVG",
					},
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								pkg.KubernetesNodeIdentityKey: utils.NodeName4,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:   "extender success for mountpoint",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        pvName,
						pkg.PVCNameSpace:  pvcForExtender.Namespace,
						pkg.PVCName:       pvcForExtender.Name,
						pkg.VolumeTypeKey: string(pkg.VolumeTypeMountPoint),
					},
				},
			},
			want: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: int64(150 * 1024 * 1024 * 1024),
					VolumeId:      pvName,
					VolumeContext: map[string]string{
						pkg.PVName:                       pvName,
						pkg.PVCNameSpace:                 pvcForExtender.Namespace,
						pkg.PVCName:                      pvcForExtender.Name,
						pkg.VolumeTypeKey:                string(pkg.VolumeTypeMountPoint),
						pkg.AnnoSelectedNode:             utils.NodeName4,
						string(pkg.VolumeTypeMountPoint): "/mnt/data/data-0",
					},
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								pkg.KubernetesNodeIdentityKey: utils.NodeName4,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:   "extender success for device",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        pvName,
						pkg.PVCNameSpace:  pvcForExtender.Namespace,
						pkg.PVCName:       pvcForExtender.Name,
						pkg.VolumeTypeKey: string(pkg.VolumeTypeDevice),
					},
				},
			},
			want: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: int64(150 * 1024 * 1024 * 1024),
					VolumeId:      pvName,
					VolumeContext: map[string]string{
						pkg.PVName:                   pvName,
						pkg.PVCNameSpace:             pvcForExtender.Namespace,
						pkg.PVCName:                  pvcForExtender.Name,
						pkg.VolumeTypeKey:            string(pkg.VolumeTypeDevice),
						pkg.AnnoSelectedNode:         utils.NodeName4,
						string(pkg.VolumeTypeDevice): "/dev/sdd",
					},
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								pkg.KubernetesNodeIdentityKey: utils.NodeName4,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:   "unknown scheduler name pvc",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        pvName,
						pkg.PVCNameSpace:  pvcUnknown.Namespace,
						pkg.PVCName:       pvcUnknown.Name,
						pkg.VolumeTypeKey: string(pkg.VolumeTypeDevice),
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "unknown type pvc",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        pvName,
						pkg.PVCNameSpace:  pvcForExtender.Namespace,
						pkg.PVCName:       pvcForExtender.Name,
						pkg.VolumeTypeKey: "unknown",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "extender success for snapshot",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: pvNameForSnapshot,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        pvNameForSnapshot,
						pkg.PVCNameSpace:  pvcSnapshotForExtender.Namespace,
						pkg.PVCName:       pvcSnapshotForExtender.Name,
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
					VolumeContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Snapshot{
							Snapshot: &csi.VolumeContentSource_SnapshotSource{
								SnapshotId: snapshotContentName,
							},
						},
					},
				},
			},
			want: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: int64(150 * 1024 * 1024 * 1024),
					VolumeId:      pvNameForSnapshot,
					VolumeContext: map[string]string{
						pkg.PVName:                pvNameForSnapshot,
						pkg.PVCNameSpace:          pvcSnapshotForExtender.Namespace,
						pkg.PVCName:               pvcSnapshotForExtender.Name,
						pkg.VolumeTypeKey:         string(pkg.VolumeTypeLVM),
						pkg.AnnoSelectedNode:      utils.NodeName4,
						pkg.VGName:                "newVG",
						pkg.ParamSnapshotReadonly: "true",
						pkg.ParamSnapshotName:     snapshotContentName,
					},
					AccessibleTopology: []*csi.Topology{
						{
							Segments: map[string]string{
								pkg.KubernetesNodeIdentityKey: utils.NodeName4,
							},
						},
					},
					ContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Snapshot{
							Snapshot: &csi.VolumeContentSource_SnapshotSource{
								SnapshotId: snapshotContentName,
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &controllerServer{
				inFlight:           tt.fields.inFlight,
				pvcPodSchedulerMap: tt.fields.pvcPodSchedulerMap,
				schedulerArchMap:   tt.fields.schedulerArchMap,
				nodeLister:         tt.fields.nodeLister,
				podLister:          tt.fields.podLister,
				pvcLister:          tt.fields.pvcLister,
				pvLister:           tt.fields.pvLister,
				adapter:            tt.fields.adapter,
				options:            tt.fields.options,
			}
			got, err := cs.CreateVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("controllerServer.CreateVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controllerServer.CreateVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_controllerServer_DeleteVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.DeleteVolumeRequest
	}

	pvNameLVM := "test-pv"
	pvNameLVMForSnapshot := "test-pv-snapshot"
	pvNameMountPoint := "test-pv-mp"
	pvNameDevice := "test-pv-device"
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvNameLVM,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "testpvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      pkg.KubernetesNodeIdentityKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{utils.NodeName4},
								},
							},
						},
					},
				},
			},
		},
	}
	pvSnapshot := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvNameLVMForSnapshot,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "testpvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.ParamVGName:           "newVG",
						pkg.VolumeTypeKey:         string(pkg.VolumeTypeLVM),
						pkg.ParamSnapshotName:     "snapshotName",
						pkg.ParamSnapshotReadonly: "true",
					},
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      pkg.KubernetesNodeIdentityKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{utils.NodeName4},
								},
							},
						},
					},
				},
			},
		},
	}
	pvMountPoint := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvNameMountPoint,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "testpvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						string(pkg.VolumeTypeMountPoint): "/mnt/data/data-0",
						pkg.VolumeTypeKey:                string(pkg.VolumeTypeMountPoint),
					},
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      pkg.KubernetesNodeIdentityKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{utils.NodeName4},
								},
							},
						},
					},
				},
			},
		},
	}
	pvDevice := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvNameDevice,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "testpvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						string(pkg.VolumeTypeDevice): "/dev/sdd",
						pkg.VolumeTypeKey:            string(pkg.VolumeTypeDevice),
					},
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      pkg.KubernetesNodeIdentityKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{utils.NodeName4},
								},
							},
						},
					},
				},
			},
		},
	}
	pvs := []*corev1.PersistentVolume{
		pv,
		pvSnapshot,
		pvMountPoint,
		pvDevice,
	}
	// node
	node := utils.CreateNode(&utils.TestNodeInfo{
		NodeName:  utils.NodeName4,
		IPAddress: "127.0.0.1",
	})

	ctx := context.Background()
	fakeKubeClient := fakekubeclientset.NewSimpleClientset()
	fakeLocalClient := fakelocalclientset.NewSimpleClientset()
	fakeSnapClient := fakesnapclientset.NewSimpleClientset()

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakeKubeClient, 0)
	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes().Informer()
	kubeInformerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced, podInformer.HasSynced, pvcInformer.HasSynced, pvInformer.HasSynced)

	// client
	for _, pv := range pvs {
		if err := pvInformer.GetIndexer().Add(pv); err != nil {
			t.Errorf("fail to add pvc: %s", err.Error())
		}
	}
	if err := nodeInformer.GetIndexer().Add(node); err != nil {
		t.Errorf("fail to add node: %s", err.Error())
	}

	// pvcPodSchedulerMap
	testfields := fields{
		inFlight:           NewInFlight(),
		pvcPodSchedulerMap: newPvcPodSchedulerMap(),
		schedulerArchMap:   newSchedulerArchMap([]string{"default"}, []string{"ahe-scheduler"}),
		nodeLister:         kubeInformerFactory.Core().V1().Nodes().Lister(),
		podLister:          kubeInformerFactory.Core().V1().Pods().Lister(),
		pvcLister:          kubeInformerFactory.Core().V1().PersistentVolumeClaims().Lister(),
		pvLister:           kubeInformerFactory.Core().V1().PersistentVolumes().Lister(),
		adapter:            adapter.NewFakeAdapter(),
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
		want    *csi.DeleteVolumeResponse
		wantErr bool
	}{
		{
			name:   "empty args",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.DeleteVolumeRequest{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "delete volume for lvm",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.DeleteVolumeRequest{
					VolumeId: pvNameLVM,
				},
			},
			want:    &csi.DeleteVolumeResponse{},
			wantErr: false,
		},
		{
			name:   "delete volume for snapshot",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.DeleteVolumeRequest{
					VolumeId: pvNameLVMForSnapshot,
				},
			},
			want:    &csi.DeleteVolumeResponse{},
			wantErr: false,
		},
		{
			name:   "delete volume for mountpoint",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.DeleteVolumeRequest{
					VolumeId: pvNameMountPoint,
				},
			},
			want:    &csi.DeleteVolumeResponse{},
			wantErr: false,
		},
		{
			name:   "delete volume for device",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.DeleteVolumeRequest{
					VolumeId: pvNameDevice,
				},
			},
			want:    &csi.DeleteVolumeResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &controllerServer{
				inFlight:           tt.fields.inFlight,
				pvcPodSchedulerMap: tt.fields.pvcPodSchedulerMap,
				schedulerArchMap:   tt.fields.schedulerArchMap,
				adapter:            tt.fields.adapter,
				nodeLister:         tt.fields.nodeLister,
				podLister:          tt.fields.podLister,
				pvcLister:          tt.fields.pvcLister,
				pvLister:           tt.fields.pvLister,
				options:            tt.fields.options,
			}
			got, err := cs.DeleteVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("controllerServer.DeleteVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controllerServer.DeleteVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_controllerServer_CreateSnapshot(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.CreateSnapshotRequest
	}

	pvName := "test-pv"
	snapshotContentName := "test-content"
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "testpvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      pkg.KubernetesNodeIdentityKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{utils.NodeName4},
								},
							},
						},
					},
				},
			},
		},
	}
	// node
	node := utils.CreateNode(&utils.TestNodeInfo{
		NodeName:  utils.NodeName4,
		IPAddress: "127.0.0.1",
	})

	ctx := context.Background()
	fakeKubeClient := fakekubeclientset.NewSimpleClientset()
	fakeLocalClient := fakelocalclientset.NewSimpleClientset()
	fakeSnapClient := fakesnapclientset.NewSimpleClientset()

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakeKubeClient, 0)
	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes().Informer()
	kubeInformerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced, podInformer.HasSynced, pvcInformer.HasSynced, pvInformer.HasSynced)

	// client
	if err := pvInformer.GetIndexer().Add(pv); err != nil {
		t.Errorf("fail to add pvc: %s", err.Error())
	}
	if err := nodeInformer.GetIndexer().Add(node); err != nil {
		t.Errorf("fail to add node: %s", err.Error())
	}

	// pvcPodSchedulerMap
	testfields := fields{
		inFlight:           NewInFlight(),
		pvcPodSchedulerMap: newPvcPodSchedulerMap(),
		schedulerArchMap:   newSchedulerArchMap([]string{"default"}, []string{"ahe-scheduler"}),
		nodeLister:         kubeInformerFactory.Core().V1().Nodes().Lister(),
		podLister:          kubeInformerFactory.Core().V1().Pods().Lister(),
		pvcLister:          kubeInformerFactory.Core().V1().PersistentVolumeClaims().Lister(),
		pvLister:           kubeInformerFactory.Core().V1().PersistentVolumes().Lister(),
		adapter:            adapter.NewFakeAdapter(),
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
		want    *csi.CreateSnapshotResponse
		wantErr bool
	}{
		{
			name:   "invalid args: empty snapshot name",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateSnapshotRequest{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid args: empty src volume id",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateSnapshotRequest{
					Name: snapshotContentName,
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid args: param",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: pvName,
					Name:           snapshotContentName,
					Parameters: map[string]string{
						pkg.ParamSnapshotReadonly: "true",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid args: param",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: pvName,
					Name:           snapshotContentName,
					Parameters: map[string]string{
						pkg.ParamSnapshotReadonly: "true",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid args: param",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: pvName,
					Name:           snapshotContentName,
					Parameters: map[string]string{
						pkg.ParamSnapshotReadonly: "true",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "create snapshot success",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: pvName,
					Name:           snapshotContentName,
					Parameters: map[string]string{
						pkg.ParamSnapshotReadonly: "true",
					},
				},
			},
			want: &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SizeBytes:      4294967296,
					SnapshotId:     snapshotContentName,
					SourceVolumeId: pvName,
					ReadyToUse:     true,
				},
			},
			wantErr: false,
		},
		{
			name:   "create snapshot success: snapshot size is larger than pv size",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: pvName,
					Name:           snapshotContentName,
					Parameters: map[string]string{
						pkg.ParamSnapshotReadonly: "true",
					},
				},
			},
			want: &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SizeBytes:      161061273600,
					SnapshotId:     snapshotContentName,
					SourceVolumeId: pvName,
					ReadyToUse:     true,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &controllerServer{
				inFlight:           tt.fields.inFlight,
				pvcPodSchedulerMap: tt.fields.pvcPodSchedulerMap,
				schedulerArchMap:   tt.fields.schedulerArchMap,
				adapter:            tt.fields.adapter,
				nodeLister:         tt.fields.nodeLister,
				podLister:          tt.fields.podLister,
				pvcLister:          tt.fields.pvcLister,
				pvLister:           tt.fields.pvLister,
				options:            tt.fields.options,
			}
			got, err := cs.CreateSnapshot(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("controllerServer.CreateSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != nil && got.Snapshot != nil {
				got.Snapshot.CreationTime = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controllerServer.CreateSnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_controllerServer_DeleteSnapshot(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.DeleteSnapshotRequest
	}

	pvName := "test-pv"
	snapshotContentName := "test-content"
	snapshotClassName := "test-snapshotclass"
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "testpvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      pkg.KubernetesNodeIdentityKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{utils.NodeName4},
								},
							},
						},
					},
				},
			},
		},
	}
	// node
	node := utils.CreateNode(&utils.TestNodeInfo{
		NodeName:  utils.NodeName4,
		IPAddress: "127.0.0.1",
	})
	volumesnapshotcontent := &volumesnapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: snapshotContentName,
		},
		Spec: volumesnapshotv1.VolumeSnapshotContentSpec{
			Source: volumesnapshotv1.VolumeSnapshotContentSource{
				VolumeHandle: &pvName,
			},
			VolumeSnapshotClassName: &snapshotClassName,
			DeletionPolicy:          volumesnapshotv1.VolumeSnapshotContentDelete,
			Driver:                  pkg.ProvisionerName,
			VolumeSnapshotRef: corev1.ObjectReference{
				Namespace: "default",
				Name:      "testsnap",
			},
		},
	}

	ctx := context.Background()
	fakeKubeClient := fakekubeclientset.NewSimpleClientset()
	fakeLocalClient := fakelocalclientset.NewSimpleClientset()
	fakeSnapClient := fakesnapclientset.NewSimpleClientset()

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakeKubeClient, 0)
	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes().Informer()
	kubeInformerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced, podInformer.HasSynced, pvcInformer.HasSynced, pvInformer.HasSynced)

	// client
	if err := pvInformer.GetIndexer().Add(pv); err != nil {
		t.Errorf("fail to add pvc: %s", err.Error())
	}
	if err := nodeInformer.GetIndexer().Add(node); err != nil {
		t.Errorf("fail to add node: %s", err.Error())
	}
	_, _ = fakeSnapClient.SnapshotV1().VolumeSnapshotContents().Create(context.Background(), volumesnapshotcontent, metav1.CreateOptions{})

	// pvcPodSchedulerMap
	testfields := fields{
		inFlight:           NewInFlight(),
		pvcPodSchedulerMap: newPvcPodSchedulerMap(),
		schedulerArchMap:   newSchedulerArchMap([]string{"default"}, []string{"ahe-scheduler"}),
		nodeLister:         kubeInformerFactory.Core().V1().Nodes().Lister(),
		podLister:          kubeInformerFactory.Core().V1().Pods().Lister(),
		pvcLister:          kubeInformerFactory.Core().V1().PersistentVolumeClaims().Lister(),
		pvLister:           kubeInformerFactory.Core().V1().PersistentVolumes().Lister(),
		adapter:            adapter.NewFakeAdapter(),
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
		want    *csi.DeleteSnapshotResponse
		wantErr bool
	}{
		{
			name:   "empty args",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.DeleteSnapshotRequest{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "delete snapshot successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.DeleteSnapshotRequest{
					SnapshotId: snapshotContentName,
				},
			},
			want:    &csi.DeleteSnapshotResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &controllerServer{
				inFlight:           tt.fields.inFlight,
				pvcPodSchedulerMap: tt.fields.pvcPodSchedulerMap,
				schedulerArchMap:   tt.fields.schedulerArchMap,
				adapter:            tt.fields.adapter,
				nodeLister:         tt.fields.nodeLister,
				podLister:          tt.fields.podLister,
				pvcLister:          tt.fields.pvcLister,
				pvLister:           tt.fields.pvLister,
				options:            tt.fields.options,
			}
			got, err := cs.DeleteSnapshot(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("controllerServer.DeleteSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controllerServer.DeleteSnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_controllerServer_ControllerExpandVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.ControllerExpandVolumeRequest
	}

	pvName := "test-pv"
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("150Gi"),
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "testpvc",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					VolumeAttributes: map[string]string{
						pkg.ParamVGName:   "newVG",
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      pkg.KubernetesNodeIdentityKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{utils.NodeName4},
								},
							},
						},
					},
				},
			},
		},
	}
	// node
	node := utils.CreateNode(&utils.TestNodeInfo{
		NodeName:  utils.NodeName4,
		IPAddress: "127.0.0.1",
	})

	ctx := context.Background()
	fakeKubeClient := fakekubeclientset.NewSimpleClientset()
	fakeLocalClient := fakelocalclientset.NewSimpleClientset()
	fakeSnapClient := fakesnapclientset.NewSimpleClientset()

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakeKubeClient, 0)
	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes().Informer()
	kubeInformerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced, podInformer.HasSynced, pvcInformer.HasSynced, pvInformer.HasSynced)

	// client
	if err := pvInformer.GetIndexer().Add(pv); err != nil {
		t.Errorf("fail to add pvc: %s", err.Error())
	}
	if err := nodeInformer.GetIndexer().Add(node); err != nil {
		t.Errorf("fail to add node: %s", err.Error())
	}

	// pvcPodSchedulerMap
	testfields := fields{
		inFlight:           NewInFlight(),
		pvcPodSchedulerMap: newPvcPodSchedulerMap(),
		schedulerArchMap:   newSchedulerArchMap([]string{"default"}, []string{"ahe-scheduler"}),
		nodeLister:         kubeInformerFactory.Core().V1().Nodes().Lister(),
		podLister:          kubeInformerFactory.Core().V1().Pods().Lister(),
		pvcLister:          kubeInformerFactory.Core().V1().PersistentVolumeClaims().Lister(),
		pvLister:           kubeInformerFactory.Core().V1().PersistentVolumes().Lister(),
		adapter:            adapter.NewFakeAdapter(),
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
		want    *csi.ControllerExpandVolumeResponse
		wantErr bool
	}{
		{
			name:   "empty args",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.ControllerExpandVolumeRequest{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "expand volume successfully",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.ControllerExpandVolumeRequest{
					VolumeId: pvName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: 268435456000,
					},
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
				},
			},
			want: &csi.ControllerExpandVolumeResponse{
				CapacityBytes:         268435456000,
				NodeExpansionRequired: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &controllerServer{
				inFlight:           tt.fields.inFlight,
				pvcPodSchedulerMap: tt.fields.pvcPodSchedulerMap,
				schedulerArchMap:   tt.fields.schedulerArchMap,
				adapter:            tt.fields.adapter,
				nodeLister:         tt.fields.nodeLister,
				podLister:          tt.fields.podLister,
				pvcLister:          tt.fields.pvcLister,
				pvLister:           tt.fields.pvLister,
				options:            tt.fields.options,
			}
			got, err := cs.ControllerExpandVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("controllerServer.ControllerExpandVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controllerServer.ControllerExpandVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_controllerServer_ValidateVolumeCapabilities(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.ValidateVolumeCapabilitiesRequest
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *csi.ValidateVolumeCapabilitiesResponse
		wantErr bool
	}{
		{
			name:   "unsupported volume capabilities",
			fields: fields{},
			args: args{
				ctx: context.Background(),
				req: &csi.ValidateVolumeCapabilitiesRequest{
					VolumeId: "testpv",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
						},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "validate volume capabilities successfully",
			fields: fields{},
			args: args{
				ctx: context.Background(),
				req: &csi.ValidateVolumeCapabilitiesRequest{
					VolumeId: "testpv",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
				},
			},
			want: &csi.ValidateVolumeCapabilitiesResponse{
				Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &controllerServer{
				inFlight:           tt.fields.inFlight,
				pvcPodSchedulerMap: tt.fields.pvcPodSchedulerMap,
				schedulerArchMap:   tt.fields.schedulerArchMap,
				adapter:            tt.fields.adapter,
				nodeLister:         tt.fields.nodeLister,
				podLister:          tt.fields.podLister,
				pvcLister:          tt.fields.pvcLister,
				pvLister:           tt.fields.pvLister,
				options:            tt.fields.options,
			}
			got, err := cs.ValidateVolumeCapabilities(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("controllerServer.ValidateVolumeCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controllerServer.ValidateVolumeCapabilities() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_controllerServer_ControllerGetCapabilities(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.ControllerGetCapabilitiesRequest
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *csi.ControllerGetCapabilitiesResponse
		wantErr bool
	}{
		{
			name:   "success",
			fields: fields{},
			args: args{
				ctx: context.Background(),
				req: &csi.ControllerGetCapabilitiesRequest{},
			},
			want: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
							},
						},
					},
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
							},
						},
					},
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &controllerServer{
				inFlight:           tt.fields.inFlight,
				pvcPodSchedulerMap: tt.fields.pvcPodSchedulerMap,
				schedulerArchMap:   tt.fields.schedulerArchMap,
				adapter:            tt.fields.adapter,
				nodeLister:         tt.fields.nodeLister,
				podLister:          tt.fields.podLister,
				pvcLister:          tt.fields.pvcLister,
				pvLister:           tt.fields.pvLister,
				options:            tt.fields.options,
			}
			got, err := cs.ControllerGetCapabilities(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("controllerServer.ControllerGetCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controllerServer.ControllerGetCapabilities() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newControllerServer(t *testing.T) {
	type args struct {
		options *driverOptions
	}

	ctx := context.Background()
	fakeKubeClient := fakekubeclientset.NewSimpleClientset()
	fakeLocalClient := fakelocalclientset.NewSimpleClientset()
	fakeSnapClient := fakesnapclientset.NewSimpleClientset()

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(fakeKubeClient, 0)
	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	podInformer := kubeInformerFactory.Core().V1().Pods().Informer()
	pvcInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims().Informer()
	pvInformer := kubeInformerFactory.Core().V1().PersistentVolumes().Informer()
	kubeInformerFactory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced, podInformer.HasSynced, pvcInformer.HasSynced, pvInformer.HasSynced)

	tests := []struct {
		name string
		args args
	}{
		{
			name: "new controller server",
			args: args{
				options: &driverOptions{
					driverName:              pkg.ProvisionerName,
					nodeID:                  utils.NodeName4,
					endpoint:                "endpoint",
					sysPath:                 "syspath",
					cgroupDriver:            "systemd",
					grpcConnectionTimeout:   3000,
					extenderSchedulerNames:  []string{"default-scheduler"},
					frameworkSchedulerNames: []string{},
					kubeclient:              fakeKubeClient,
					localclient:             fakeLocalClient,
					snapclient:              fakeSnapClient,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newControllerServer(tt.args.options); got == nil {
				t.Errorf("newControllerServer() = nil")
			}
		})
	}
}
