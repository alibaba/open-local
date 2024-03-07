package cache

import (
	"testing"

	"sync"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	localtype "github.com/alibaba/open-local/pkg"
	nodelocalstorage "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
)

func TestUpdateNodeInfo_NodeLocalStorageEmpty(t *testing.T) {
	nc := &NodeCache{
		rwLock: sync.RWMutex{},
		NodeInfo: NodeInfo{
			NodeName:     "test-node",
			SupportSPDK:  false,
			VGs:          make(map[ResourceName]SharedResource),
			MountPoints:  make(map[ResourceName]ExclusiveResource),
			Devices:      make(map[ResourceName]ExclusiveResource),
			AllocatedNum: 0, // 已分配数量
			// TODO(yuzhi.wx) using pv name may conflict, use pv uid later
			LocalPVs:            make(map[string]corev1.PersistentVolume),
			PVCRecordsByExtend:  make(map[string]AllocatedUnit),
			PodInlineVolumeInfo: make(map[string][]InlineVolumeInfo)},
	}
	nodeLocal := &nodelocalstorage.NodeLocalStorage{
		Spec: nodelocalstorage.NodeLocalStorageSpec{
			SpdkConfig: nodelocalstorage.SpdkConfig{},
			ListConfig: nodelocalstorage.ListConfig{
				VGs:         nodelocalstorage.VGList{},
				MountPoints: nodelocalstorage.MountPointList{},
				Devices:     nodelocalstorage.DeviceList{},
			},
		},
		Status: nodelocalstorage.NodeLocalStorageStatus{
			FilteredStorageInfo: nodelocalstorage.FilteredStorageInfo{
				VolumeGroups: []string{},
				Devices:      []string{},
				MountPoints:  []string{},
			},
		},
	}

	result := nc.UpdateNodeInfo(nodeLocal)

	assert.Equal(t, nc, result)
	assert.Equal(t, false, result.SupportSPDK)
	assert.Equal(t, 0, len(result.VGs))
	assert.Equal(t, 0, len(result.Devices))
	assert.Equal(t, 0, len(result.MountPoints))
}

func TestUpdateNodeInfo_NodeLocalStorageNotEmpty(t *testing.T) {
	nc := &NodeCache{
		NodeInfo: NodeInfo{
			NodeName:            "node-1",
			VGs:                 make(map[ResourceName]SharedResource),
			Devices:             make(map[ResourceName]ExclusiveResource),
			MountPoints:         make(map[ResourceName]ExclusiveResource),
			LocalPVs:            make(map[string]corev1.PersistentVolume),
			PodInlineVolumeInfo: make(map[string][]InlineVolumeInfo),
			PVCRecordsByExtend:  make(map[string]AllocatedUnit),
		},
	}

	nodeLocal := &nodelocalstorage.NodeLocalStorage{
		Spec: nodelocalstorage.NodeLocalStorageSpec{
			ListConfig: nodelocalstorage.ListConfig{
				VGs: nodelocalstorage.VGList{
					Include: []string{
						"vg-1",
						"vg-2", // exact match and multi regex
					},	
				},
				MountPoints: nodelocalstorage.MountPointList{
					Include: []string{
						"/mnt/open-local/mnt*", // match with star
					},
				},
				Devices: nodelocalstorage.DeviceList{
					Include: []string{
						"/dev/device-1", // exact match
					},
				},
			},
			SpdkConfig: nodelocalstorage.SpdkConfig{},
		},
		Status: nodelocalstorage.NodeLocalStorageStatus{
			NodeStorageInfo: nodelocalstorage.NodeStorageInfo{
				VolumeGroups: []nodelocalstorage.VolumeGroup{
					{
						Name:        "vg-1",
						Total:       100,
						Allocatable: 90,
						Available:   10,
					},
					{
						Name:        "vg-2",
						Total:       200,
						Allocatable: 190,
						Available:   10,

					},
				},
				DeviceInfos: []nodelocalstorage.DeviceInfo{
					{
						Name:      "/dev/device-1",
						Total:     100,
						MediaType: "ssd",
					},
					{
						Name:      "/dev/device-2",
						Total:     200,
						MediaType: "hdd",
					},
				},
				MountPoints: []nodelocalstorage.MountPoint{
					{
						Name: "/mnt/open-local/mnt1",
						Total: 100,
						Available: 90,
					},
					{
						Name: "/mnt/open-local/mnt2",
						Total: 200,
						Available: 95,
					},
					{
						Name: "/mnt/open-local/mnt3",
						Total: 300,
						Available: 96,
						FsType: "ext4",
					},
				},
			},
			FilteredStorageInfo: nodelocalstorage.FilteredStorageInfo{
				VolumeGroups: []string{
					"vg-1",
					"vg-2",
				},
				Devices: []string{
					"/dev/device-1",
					"/dev/device-2",
				},
				MountPoints: []string{
					"/mnt/open-local/mnt3",
				},
			},
		},
	}

	updatedNodeCache := nc.UpdateNodeInfo(nodeLocal)

	// Test VGs
	assert.Equal(t, 2, len(updatedNodeCache.VGs))

	vg1, found := updatedNodeCache.VGs[ResourceName("vg-1")]
	assert.True(t, found)
	assert.Equal(t, "vg-1", vg1.Name)
	assert.Equal(t, int64(90), vg1.Capacity)
	// assert.Equal(t, int64(10), vg1.Requested)

	vg2, found := updatedNodeCache.VGs[ResourceName("vg-2")]
	assert.True(t, found)
	assert.Equal(t, "vg-2", vg2.Name)
	assert.Equal(t, int64(190), vg2.Capacity)
	// assert.Equal(t, int64(10), vg2.Requested)

	// Test Devices
	assert.Equal(t, 2, len(updatedNodeCache.Devices))

	device1, found := updatedNodeCache.Devices[ResourceName("/dev/device-1")]
	assert.True(t, found)
	assert.Equal(t, device1.Name, "/dev/device-1")
	assert.Equal(t, "/dev/device-1", device1.Device)
	assert.Equal(t, int64(100), device1.Capacity)
	assert.Equal(t, localtype.MediaType("ssd"), device1.MediaType)
	// assert.True(t, device1.IsAllocated)

	device2, found := updatedNodeCache.Devices[ResourceName("/dev/device-2")]
	assert.True(t, found)
	assert.Equal(t, "/dev/device-2", device2.Name)
	assert.Equal(t, "/dev/device-2", device2.Device)
	assert.Equal(t, int64(200), device2.Capacity)
	assert.Equal(t, localtype.MediaType("hdd"), device2.MediaType)
	// assert.True(t, device2.IsAllocated)

	// Test MountPoints
	assert.Equal(t, 1, len(updatedNodeCache.MountPoints))

	mountPoint1, found := updatedNodeCache.MountPoints[ResourceName("/mnt/open-local/mnt3")]
	assert.True(t, found)
	assert.Equal(t, "/mnt/open-local/mnt3", mountPoint1.Name)

}

func TestAddPodInlineVolumeInfo(t *testing.T) {
	nc := NewNodeCache("test-node")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "test-volume",
					VolumeSource: corev1.VolumeSource{
						CSI: &corev1.CSIVolumeSource{
							Driver: localtype.ProvisionerName,
							VolumeAttributes: map[string]string{
								"vgName": "test-vg",
							},
						},
					},
				},
			},
			NodeName: "test-node",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	err := nc.AddPodInlineVolumeInfo(pod)
	if err != nil {
		t.Errorf("AddPodInlineVolumeInfo failed, err: %v", err)
	}
	if len(nc.PodInlineVolumeInfo) != 1 {
		t.Errorf("AddPodInlineVolumeInfo failed, PodInlineVolumeInfo len: %d", len(nc.PodInlineVolumeInfo))
	}
	if len(nc.PodInlineVolumeInfo["test-uid"]) != 1 {
		t.Errorf("AddPodInlineVolumeInfo failed, PodInlineVolumeInfo len: %d", len(nc.PodInlineVolumeInfo["test-uid"]))
	}
	if nc.PodInlineVolumeInfo["test-uid"][0].VgName != "test-vg" {
		t.Errorf("AddPodInlineVolumeInfo failed, PodInlineVolumeInfo VgName: %s", nc.PodInlineVolumeInfo["test-uid"][0].VgName)
	}
	if nc.PodInlineVolumeInfo["test-uid"][0].VolumeName != "test-volume" {
		t.Errorf("AddPodInlineVolumeInfo failed, PodInlineVolumeInfo VolumeName: %s", nc.PodInlineVolumeInfo["test-uid"][0].VolumeName)
	}
	if nc.PodInlineVolumeInfo["test-uid"][0].PodName != "test-pod" {
		t.Errorf("AddPodInlineVolumeInfo failed, PodInlineVolumeInfo PodName: %s", nc.PodInlineVolumeInfo["test-uid"][0].PodName)
	}
	if nc.PodInlineVolumeInfo["test-uid"][0].PodNamespace != "test-ns" {
		t.Errorf("AddPodInlineVolumeInfo failed, PodInlineVolumeInfo PodNamespace: %s", nc.PodInlineVolumeInfo["test-uid"][0].PodNamespace)
	}
	if nc.PodInlineVolumeInfo["test-uid"][0].Recorded != false { // note: not add new inlinevolume to VG here
		t.Errorf("AddPodInlineVolumeInfo failed, PodInlineVolumeInfo Recorded: %v", nc.PodInlineVolumeInfo["test-uid"][0].Recorded)
	}
	if nc.VGs["test-vg"].Requested != 0 { // note: not add new inlinevolume to VG here
		t.Errorf("AddPodInlineVolumeInfo failed, VGs Requested: %d", nc.VGs["test-vg"].Requested)
	}
}
