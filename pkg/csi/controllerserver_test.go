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
	"github.com/alibaba/open-local/pkg/csi/adapter"
	"github.com/alibaba/open-local/pkg/csi/client"
	"github.com/alibaba/open-local/pkg/csi/server"
	fakelocalclientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned/fake"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/container-storage-interface/spec/lib/go/csi"
	fakesnapclientset "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned/fake"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	kubeinformers "k8s.io/client-go/informers"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func init() {
	client.MustRunThisWhenTest()
	server.MustRunThisWhenTest()
	server.StartFake()
}

func Test_controllerServer_CreateVolume(t *testing.T) {
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
	type args struct {
		ctx context.Context
		req *csi.CreateVolumeRequest
	}
	pvcPodSchedulerMap := newPvcPodSchedulerMap()
	pvcTmp := utils.CreateTestPersistentVolumeClaim([]utils.TestPVCInfo{*utils.GetTestPVCPVWithoutVG().PVCPending})[0]
	// pvcWithoutVG_FW
	pvcWithoutVG_FW := pvcTmp.DeepCopy()
	pvcWithoutVG_FW.Name = "pvcWithoutVGFW"
	pvcWithoutVG_FW.SetAnnotations(map[string]string{
		pkg.AnnoSelectedNode: utils.NodeName4,
	})
	pvcPodSchedulerMap.Add(pvcWithoutVG_FW.Namespace, pvcWithoutVG_FW.Name, "ahe-scheduler")
	// pvcWithoutVG_FW_NoNodeName
	pvcWithoutVG_FW_NoNodeName := pvcTmp.DeepCopy()
	pvcWithoutVG_FW_NoNodeName.Name = "pvcWithoutVGFWNoNodeName"
	pvcPodSchedulerMap.Add(pvcWithoutVG_FW_NoNodeName.Namespace, pvcWithoutVG_FW_NoNodeName.Name, "ahe-scheduler")
	// pvcWithoutVG_Extender
	pvcWithoutVG_Extender := pvcTmp.DeepCopy()
	pvcWithoutVG_Extender.Name = "pvcWithoutVGExtender"
	pvcWithoutVG_Extender.SetAnnotations(map[string]string{
		pkg.AnnoSelectedNode: utils.NodeName4,
	})
	pvcPodSchedulerMap.Add(pvcWithoutVG_Extender.Namespace, pvcWithoutVG_Extender.Name, "default")
	// nodeName4
	node := utils.CreateNode(&utils.TestNodeInfo{
		NodeName:  utils.NodeName4,
		IPAddress: "127.0.0.1",
	})

	pvcs := []*v1.PersistentVolumeClaim{
		pvcWithoutVG_FW,
		pvcWithoutVG_FW_NoNodeName,
		pvcWithoutVG_Extender,
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

	for _, pvc := range pvcs {
		if err := pvcInformer.GetIndexer().Add(pvc); err != nil {
			t.Errorf("fail to add pvc: %s", err.Error())
		}
	}
	if err := nodeInformer.GetIndexer().Add(node); err != nil {
		t.Errorf("fail to add node: %s", err.Error())
	}

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
					Name: "test-pv",
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
					Name: "test-pv",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
						pkg.PVName:        "test-pv",
						pkg.PVCNameSpace:  pvcWithoutVG_FW_NoNodeName.Namespace,
						pkg.PVCName:       pvcWithoutVG_FW_NoNodeName.Name,
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "framework success",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: "test-pv",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        "test-pv",
						pkg.PVCNameSpace:  pvcWithoutVG_FW.Namespace,
						pkg.PVCName:       pvcWithoutVG_FW.Name,
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			want: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: int64(150 * 1024 * 1024 * 1024),
					VolumeId:      "test-pv",
					VolumeContext: map[string]string{
						pkg.PVName:           "test-pv",
						pkg.PVCNameSpace:     pvcWithoutVG_FW.Namespace,
						pkg.PVCName:          pvcWithoutVG_FW.Name,
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
			name:   "extender success",
			fields: testfields,
			args: args{
				ctx: context.Background(),
				req: &csi.CreateVolumeRequest{
					Name: "test-pv",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
							AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
						},
					},
					CapacityRange: &csi.CapacityRange{RequiredBytes: int64(150 * 1024 * 1024 * 1024)},
					Parameters: map[string]string{
						pkg.PVName:        "test-pv",
						pkg.PVCNameSpace:  pvcWithoutVG_Extender.Namespace,
						pkg.PVCName:       pvcWithoutVG_Extender.Name,
						pkg.VolumeTypeKey: string(pkg.VolumeTypeLVM),
					},
				},
			},
			want: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: int64(150 * 1024 * 1024 * 1024),
					VolumeId:      "test-pv",
					VolumeContext: map[string]string{
						pkg.PVName:           "test-pv",
						pkg.PVCNameSpace:     pvcWithoutVG_Extender.Namespace,
						pkg.PVCName:          pvcWithoutVG_Extender.Name,
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
