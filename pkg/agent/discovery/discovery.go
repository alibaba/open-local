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

package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/agent/common"
	localv1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	clientset "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/alibaba/open-local/pkg/utils/lvm"
	"github.com/alibaba/open-local/pkg/utils/spdk"
	units "github.com/docker/go-units"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	log "k8s.io/klog/v2"
	"k8s.io/utils/mount"
)

// Discoverer update block device, VG and mountpoint info to Cache
type Discoverer struct {
	*common.Configuration
	// kubeclientset is a standard kubernetes clientset
	kubeclientset  kubernetes.Interface
	localclientset clientset.Interface
	snapclient     snapshot.Interface
	// K8sMounter used to verify mountpoints
	K8sMounter mount.Interface
	recorder   record.EventRecorder
	// 'spdk' indicate if use SPDK storage backend
	spdk       bool
	spdkclient *spdk.SpdkClient
}

type ReservedVGInfo struct {
	vgName          string
	reservedPercent float64
	reservedSize    uint64
}

const (
	DefaultFS          = "ext4"
	AnnoStorageReserve = "csi.aliyun.com/storage-reserved"
)

// NewDiscoverer return Discoverer
func NewDiscoverer(config *common.Configuration, kubeclientset kubernetes.Interface, localclientset clientset.Interface, snapclient snapshot.Interface, recorder record.EventRecorder) *Discoverer {
	return &Discoverer{
		Configuration:  config,
		localclientset: localclientset,
		kubeclientset:  kubeclientset,
		snapclient:     snapclient,
		K8sMounter:     mount.New("" /* default mount path */),
		recorder:       recorder,
		spdk:           false,
	}
}

func (d *Discoverer) getNodeLocalStorage() (*localv1alpha1.NodeLocalStorage, error) {
	nls, err := d.localclientset.CsiV1alpha1().NodeLocalStorages().Get(context.Background(), d.Nodename, metav1.GetOptions{})
	if err != nil {
		if k8serr.IsNotFound(err) {
			log.Infof("node local storage %s not found, waiting for the controller to create the resource", d.Nodename)
		} else {
			log.Errorf("get NodeLocalStorages failed: %s", err.Error())
		}
	} else {
		if nls.Spec.SpdkConfig.DeviceType != "" {
			if d.spdkclient == nil {
				if d.spdkclient = spdk.NewSpdkClient(nls.Spec.SpdkConfig.RpcSocket); d.spdkclient != nil {
					d.spdk = true
				}
			}
		}
	}
	return nls, err
}

// Discover update local storage periodically
func (d *Discoverer) Discover() {
	if nls, err := d.getNodeLocalStorage(); err != nil {
		return
	} else {
		log.V(4).Infof("update node local storage %s status", d.Nodename)
		nlsCopy := nls.DeepCopy()
		// get anno
		reservedVGInfos := make(map[string]ReservedVGInfo)
		if anno, exist := nlsCopy.Annotations[AnnoStorageReserve]; exist {
			if reservedVGInfos, err = getReservedVGInfo(anno); err != nil {
				log.Errorf("get reserved vg info failed: %s, but we ignore...", err.Error())
				return
			}
		}
		// get status first, for we need support regexp
		newStatus := new(localv1alpha1.NodeLocalStorageStatus)
		if err := d.discoverVGs(newStatus, reservedVGInfos); err != nil {
			log.Errorf("discover VG error: %s", err.Error())
			return
		}
		if err := d.discoverDevices(newStatus); err != nil {
			log.Errorf("discover Device error: %s", err.Error())
			return
		}
		if err := d.discoverMountPoints(newStatus); err != nil {
			log.Errorf("discover MountPoint error: %s", err.Error())
			return
		}
		newStatus.NodeStorageInfo.Phase = localv1alpha1.NodeStorageRunning
		newStatus.NodeStorageInfo.State.Status = localv1alpha1.ConditionTrue
		newStatus.NodeStorageInfo.State.Type = localv1alpha1.StorageReady
		lastHeartbeatTime := metav1.Now()
		newStatus.NodeStorageInfo.State.LastHeartbeatTime = &lastHeartbeatTime
		nlsCopy.Status.NodeStorageInfo = newStatus.NodeStorageInfo
		nlsCopy.Status.FilteredStorageInfo.VolumeGroups = FilterVGInfo(nlsCopy)
		nlsCopy.Status.FilteredStorageInfo.MountPoints = FilterMPInfo(nlsCopy)
		nlsCopy.Status.FilteredStorageInfo.Devices = FilterDeviceInfo(nlsCopy)
		nlsCopy.Status.FilteredStorageInfo.UpdateStatus.Status = localv1alpha1.UpdateStatusAccepted
		lastUpdateTime := metav1.Now()
		nlsCopy.Status.FilteredStorageInfo.UpdateStatus.LastUpdateTime = &lastUpdateTime
		nlsCopy.Status.FilteredStorageInfo.UpdateStatus.Reason = ""

		if d.spdk {
			d.createSpdkBdevs(&nlsCopy.Status.FilteredStorageInfo.Devices)
		}

		// only update status
		log.Infof("update nls %s", nlsCopy.Name)
		_, err = d.localclientset.CsiV1alpha1().NodeLocalStorages().UpdateStatus(context.Background(), nlsCopy, metav1.UpdateOptions{})
		if err != nil {
			log.Errorf("local storage CRD updateStatus error: %s", err.Error())
			return
		}
	}
}

// Create AIO bdevs
func (d *Discoverer) createSpdkBdevs(devices *[]string) {
	var found bool

	bdevs, err := d.spdkclient.GetBdevs()
	if err != nil {
		log.Error("createSpdkBdevs - GetBdevs:", err.Error())
		return
	}

	for _, dev := range *devices {
		found = false
		for _, bdev := range *bdevs {
			if bdev.GetFilename() == dev {
				found = true
				break
			}
		}
		if !found {
			bdevName := "bdev-aio" + strings.Replace(dev, "/", "_", -1)
			if _, err := d.spdkclient.CreateBdev(bdevName, dev); err != nil {
				log.Error("createSpdkBdevs - CreateBdev failed:", err.Error())
			}
		}
	}
}

// InitResource will create relevant resource
func (d *Discoverer) InitResource() {
	nls, err := d.getNodeLocalStorage()
	if err != nil {
		log.Error("InitResource - getNodeLocalStorage:", err.Error())
		return
	}
	vgs := nls.Spec.ResourceToBeInited.VGs
	mountpoints := nls.Spec.ResourceToBeInited.MountPoints
	if !d.spdk {
		for _, vg := range vgs {
			if _, err := lvm.LookupVolumeGroup(vg.Name); err == lvm.ErrVolumeGroupNotFound {
				err := d.createVG(vg.Name, vg.Devices)
				if err != nil {
					msg := fmt.Sprintf("create vg %s with device %v failed: %s. you can try command \"vgcreate %s %v --force\" manually on this node", vg.Name, vg.Devices, err.Error(), vg.Name, strings.Join(vg.Devices, " "))
					log.Error(msg)
					d.recorder.Event(nls, corev1.EventTypeWarning, localtype.EventCreateVGFailed, msg)
				}
			}
		}
	} else {
		var found bool
		bdevs, err := d.spdkclient.GetBdevs()
		if err != nil {
			log.Error("InitResource - GetBdevs failed:", err.Error())
			return
		}

		lvss, err := d.spdkclient.GetLvStores()
		if err != nil {
			log.Error("InitResource - GetLvStores failed:", err.Error())
			return
		}

		for _, vg := range vgs {
			found = false
			for _, lvs := range *lvss {
				if vg.Name == lvs.Name {
					found = true
					break
				}
			}

			if !found {
				for _, bdev := range *bdevs {
					if bdev.GetFilename() == vg.Devices[0] {
						found = true
						break
					}
				}

				bdevName := "bdev-aio" + strings.Replace(vg.Devices[0], "/", "_", -1)
				if !found {
					if _, err := d.spdkclient.CreateBdev(bdevName, vg.Devices[0]); err != nil {
						log.Error("InitResource - CreateBdev failed:", err.Error())
						return
					}
				}

				if _, err := d.spdkclient.CreateLvstore(bdevName, vg.Name); err != nil {
					log.Error("InitResource - CreateLvstore failed:", err.Error())
				}
			}
		}
	}
	for _, mp := range mountpoints {
		notMounted, err := d.K8sMounter.IsLikelyNotMountPoint(mp.Path)
		if err != nil && strings.Contains(err.Error(), "no such file or directory") {
			if err := os.MkdirAll(mp.Path, 0777); err != nil {
				log.Errorf("mkdir error: %s", err.Error())
				continue
			}
		}
		if notMounted {
			fsType := mp.FsType
			if fsType == "" {
				fsType = DefaultFS
			}
			if err := utils.Format(mp.Device, fsType); err != nil {
				log.Errorf("format error: %s", err.Error())
				continue
			}
			if err := d.K8sMounter.Mount(mp.Device, mp.Path, fsType, mp.Options); err != nil {
				log.Errorf("mount error: %s", err.Error())
				continue
			}
		}
	}
}

func getReservedVGInfo(reservedAnno string) (infos map[string]ReservedVGInfo, err error) {
	// step 0: var definition
	infos = make(map[string]ReservedVGInfo)
	reservedVGMap := map[string]string{}
	// step 1: unmarshal
	if err := json.Unmarshal([]byte(reservedAnno), &reservedVGMap); err != nil {
		return nil, fmt.Errorf(
			"failed to parse annotation value (%q) err=%v",
			reservedAnno,
			err)
	}
	// step 2: get reserved info from anno
	for k, v := range reservedVGMap {
		var percent float64
		var size int64
		var info ReservedVGInfo
		if strings.HasSuffix(v, "%") {
			// reservedPercent
			v = strings.ReplaceAll(v, "%", "")
			thr, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("[getReservedVGInfo]parse float failed")
			}
			percent = thr / 100
			info = ReservedVGInfo{
				vgName:          k,
				reservedPercent: percent,
			}
		} else {
			// reservedSize
			size, err = units.RAMInBytes(v)
			if err != nil {
				return nil, fmt.Errorf("[getReservedVGInfo]get reserved size failed")
			}
			info = ReservedVGInfo{
				vgName:       k,
				reservedSize: uint64(size),
			}
		}
		infos[k] = info
	}

	return infos, nil
}

func FilterVGInfo(nls *localv1alpha1.NodeLocalStorage) []string {
	var vgSlice []string
	for _, vg := range nls.Status.NodeStorageInfo.VolumeGroups {
		vgSlice = append(vgSlice, vg.Name)
	}

	return FilterInfo(vgSlice, nls.Spec.ListConfig.VGs.Include, nls.Spec.ListConfig.VGs.Exclude)
}

func FilterMPInfo(nls *localv1alpha1.NodeLocalStorage) []string {
	var mpSlice []string
	for _, mp := range nls.Status.NodeStorageInfo.MountPoints {
		mpSlice = append(mpSlice, mp.Name)
	}

	return FilterInfo(mpSlice, nls.Spec.ListConfig.MountPoints.Include, nls.Spec.ListConfig.MountPoints.Exclude)
}

func FilterDeviceInfo(nls *localv1alpha1.NodeLocalStorage) []string {
	var devSlice []string
	for _, dev := range nls.Status.NodeStorageInfo.DeviceInfos {
		devSlice = append(devSlice, dev.Name)
	}

	return FilterInfo(devSlice, nls.Spec.ListConfig.Devices.Include, nls.Spec.ListConfig.Devices.Exclude)
}

func FilterInfo(info []string, include []string, exclude []string) []string {
	filterMap := make(map[string]string, len(info))

	for _, inc := range include {
		reg := regexp.MustCompile(inc)
		for _, i := range info {
			if reg.FindString(i) == i {
				filterMap[i] = i
			}
		}
	}
	for _, exc := range exclude {
		reg := regexp.MustCompile(exc)
		for _, i := range info {
			if reg.FindString(i) == i {
				delete(filterMap, i)
			}
		}
	}

	var filterSlice []string
	for vg := range filterMap {
		filterSlice = append(filterSlice, vg)
	}

	return filterSlice
}
