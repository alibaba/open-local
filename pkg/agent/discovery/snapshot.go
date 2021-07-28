package discovery

import (
	"context"
	"os"
	"strconv"
	"strings"

	units "github.com/docker/go-units"
	"github.com/oecp/open-local-storage-service/pkg/utils/lvm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

type SnapshotLV struct {
	lvName       string
	originLvName string
	size         uint64
	usage        float64
}

const (
	AnnoSnapshotInitialSize  = "storage.oecp.io/snapshot-initial-size"
	AnnoSnapshotThreshold    = "storage.oecp.io/threshold"
	AnnoSnapshotIncreaseSize = "storage.oecp.io/increase"
	EnvSnapshotPrefix        = "OPENLSS_SNAPSHOT_PREFIX"
	DefaultSnapshotPrefix    = "open-local-storage-service"
	DefaultSnapshotSize      = 4 * 1024 * 1024 * 1024
	DefaultSnapshotThreshold = 0.5
	DefaultIncreaseSize      = 1 * 1024 * 1024 * 1024
)

func (d *Discoverer) ExpandSnapshotLVIfNeeded() {
	// Step 0: get prefix of snapshot lv
	prefix := os.Getenv(EnvSnapshotPrefix)
	if prefix == "" {
		prefix = DefaultSnapshotPrefix
	}

	// Step 1: get all snapshot lv
	lvs, err := getAllLSSSnapshotLV()
	if err != nil {
		klog.Errorf("[ExpandSnapshotLVIfNeeded]get open-local-storage-service snapshot lv failed: %s", err.Error())
		return
	}
	// Step 2: handle every snapshot lv(for)
	for _, lv := range lvs {
		// step 1: get threshold and increase size from snapshot
		snapContent, err := d.snapclient.SnapshotV1beta1().VolumeSnapshotContents().Get(context.TODO(), strings.Replace(lv.Name(), prefix, "snapcontent", 1), metav1.GetOptions{})
		if err != nil {
			klog.Errorf("[ExpandSnapshotLVIfNeeded]get snapContent %s error: %s", lv.Name(), err.Error())
			return
		}
		snap, err := d.snapclient.SnapshotV1beta1().VolumeSnapshots(snapContent.Spec.VolumeSnapshotRef.Namespace).Get(context.TODO(), snapContent.Spec.VolumeSnapshotRef.Name, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("[ExpandSnapshotLVIfNeeded]get snapshot %s/%s error: %s", snapContent.Spec.VolumeSnapshotRef.Namespace, snapContent.Spec.VolumeSnapshotRef.Name, err.Error())
			return
		}
		_, threshold, increaseSize := getSnapshotInitialInfo(snap.Annotations)
		// step 2: expand snapshot lv if necessary
		if lv.Usage() > threshold {
			klog.Infof("[ExpandSnapshotLVIfNeeded]expand snapshot lv %s", lv.Name())
			if err := lv.Expand(increaseSize); err != nil {
				klog.Errorf("[ExpandSnapshotLVIfNeeded]expand lv %s failed: %s", lv.Name(), err.Error())
				return
			}
			klog.Infof("[ExpandSnapshotLVIfNeeded]expand snapshot lv %s successfully", lv.Name())
		}
	}

	// force update status of nls
	d.Discover()

	return
}

func getSnapshotInitialInfo(anno map[string]string) (initialSize uint64, threshold float64, increaseSize uint64) {
	initialSize = DefaultSnapshotSize
	threshold = DefaultSnapshotThreshold
	increaseSize = DefaultIncreaseSize

	// Step 1: get snapshot initial size
	if str, exist := anno[AnnoSnapshotInitialSize]; exist {
		size, err := units.RAMInBytes(str)
		if err != nil {
			klog.Error("[getSnapshotInitialInfo]get initialSize from snapshot annotation failed")
		}
		initialSize = uint64(size)
	}
	// Step 2: get snapshot expand threshold
	if str, exist := anno[AnnoSnapshotThreshold]; exist {
		str = strings.ReplaceAll(str, "%", "")
		thr, err := strconv.ParseFloat(str, 64)
		if err != nil {
			klog.Error("[getSnapshotInitialInfo]parse float failed")
		}
		threshold = thr / 100
	}
	// Step 3: get snapshot increase size
	if str, exist := anno[AnnoSnapshotIncreaseSize]; exist {
		size, err := units.RAMInBytes(str)
		if err != nil {
			klog.Error("[getSnapshotInitialInfo]get increase size from snapshot annotation failed")
		}
		increaseSize = uint64(size)
	}
	klog.Infof("[getSnapshotInitialInfo]initialSize(%d), threshold(%f), increaseSize(%d)", initialSize, threshold, increaseSize)
	return
}

//
func getAllLSSSnapshotLV() (lvs []*lvm.LogicalVolume, err error) {
	// get all vg names
	lvs = make([]*lvm.LogicalVolume, 0)
	vgNames, err := lvm.ListVolumeGroupNames()
	if err != nil {
		klog.Errorf("[getAllLSSSnapshotLV]List volume group names error: %s", err.Error())
		return nil, err
	}
	for _, vgName := range vgNames {
		// step 1: get vg info
		vg, err := lvm.LookupVolumeGroup(vgName)
		if err != nil {
			klog.Errorf("[getAllLSSSnapshotLV]Look up volume group %s error: %s", vgName, err.Error())
			return nil, err
		}
		// step 2: get all lv of the selected vg
		logicalVolumeNames, err := vg.ListLogicalVolumeNames()
		if err != nil {
			klog.Errorf("[getAllLSSSnapshotLV]List volume group %s error: %s", vgName, err.Error())
			return nil, err
		}
		// step 3: update lvs variable
		for _, lvName := range logicalVolumeNames {
			tmplv, err := vg.LookupLogicalVolume(lvName)
			if err != nil {
				klog.Errorf("[getAllLSSSnapshotLV]List logical volume %s error: %s", lvName, err.Error())
				continue
			}
			if tmplv.IsSnapshot() {
				lvs = append(lvs, tmplv)
			}
		}
	}

	return lvs, nil
}
