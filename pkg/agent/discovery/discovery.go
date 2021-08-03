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

package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	units "github.com/docker/go-units"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned"
	"github.com/oecp/open-local/pkg/agent/common"
	lssv1alpha1 "github.com/oecp/open-local/pkg/apis/storage/v1alpha1"
	clientset "github.com/oecp/open-local/pkg/generated/clientset/versioned"
	"github.com/oecp/open-local/pkg/utils"
	"github.com/oecp/open-local/pkg/utils/lvm"
	log "github.com/sirupsen/logrus"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
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
	mutex      sync.RWMutex
}

type ReservedVGInfo struct {
	vgName          string
	reservedPercent float64
	reservedSize    uint64
}

const (
	DefaultFS          = "ext4"
	AnnoStorageReserve = "storage.oecp.io/storage-reserved"
)

// NewDiscoverer return Discoverer
func NewDiscoverer(config *common.Configuration, kubeclientset kubernetes.Interface, lssclientset clientset.Interface, snapclient snapshot.Interface) *Discoverer {
	return &Discoverer{
		Configuration:  config,
		localclientset: lssclientset,
		kubeclientset:  kubeclientset,
		snapclient:     snapclient,
		K8sMounter:     mount.New("" /* default mount path */),
		mutex:          sync.RWMutex{},
	}
}

// updateConfigurationFromNLSC will update the common.Configuration.CRDSpec from nodelocalstorage initconfig
func (d *Discoverer) updateConfigurationFromNLSC() error {
	nlsc, err := d.localclientset.StorageV1alpha1().NodeLocalStorageInitConfigs().Get(context.Background(), d.InitConfig, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error updateConfigurationFromNLSC: %s", err.Error())
	}

	node, err := d.kubeclientset.CoreV1().Nodes().Get(context.Background(), d.Nodename, metav1.GetOptions{})
	nodeLabels := node.Labels
	for _, nodeconfig := range nlsc.Spec.NodesConfig {
		selector, err := metav1.LabelSelectorAsSelector(nodeconfig.Selector)
		if err != nil {
			return err
		}
		if !selector.Matches(labels.Set(nodeLabels)) {
			continue
		}
		updateCRDSpec(d.CRDSpec, nodeconfig.ListConfig, nodeconfig.ResourceToBeInited)
		return nil
	}
	updateCRDSpec(d.CRDSpec, nlsc.Spec.GlobalConfig.ListConfig, nlsc.Spec.GlobalConfig.ResourceToBeInited)
	return nil
}

// updateCRDSpec will update the nodelocalstorage spec according to necessary info
func updateCRDSpec(CRDSpec *lssv1alpha1.NodeLocalStorageSpec, listConfig lssv1alpha1.ListConfig, resource lssv1alpha1.ResourceToBeInited) {
	CRDSpec.ListConfig = listConfig
	CRDSpec.ResourceToBeInited = resource
}

// createLocalStorageCRD will create NodeLocalStorage CRD
// Notice: the agent will create only one CRD, this CRD will show
// the local storage information of that node which the agent run at
func (d *Discoverer) newLocalStorageCRD() *lssv1alpha1.NodeLocalStorage {
	nls := new(lssv1alpha1.NodeLocalStorage)

	nls.ObjectMeta.Name = d.Nodename
	nls.Spec.NodeName = d.Nodename
	nls.Spec.ListConfig = d.CRDSpec.ListConfig
	nls.Spec.ResourceToBeInited = d.CRDSpec.ResourceToBeInited

	return nls
}

// Discover update local storage periodically
func (d *Discoverer) Discover() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	// update configuration from NLSC
	if err := d.updateConfigurationFromNLSC(); err != nil {
		log.Errorf("update configuration from NLSC error: %s", err.Error())
		return
	}

	if nls, err := d.localclientset.StorageV1alpha1().NodeLocalStorages().Get(context.Background(), d.Nodename, metav1.GetOptions{}); err != nil {
		if k8serr.IsNotFound(err) {
			log.Infof("creating node local storage %s", d.Nodename)
			nls = d.newLocalStorageCRD()
			// create new nls
			_, err = d.localclientset.StorageV1alpha1().NodeLocalStorages().Create(context.Background(), nls, metav1.CreateOptions{})
			if err != nil {
				log.Errorf("create local storage CRD status failed: %s", err.Error())
				return
			}
		} else {
			log.Errorf("get NodeLocalStorages failed: %s", err.Error())
			return
		}
	} else {
		log.Debugf("update node local storage %s status", d.Nodename)
		nlsCopy := nls.DeepCopy()
		// get anno
		reservedVGInfos := make(map[string]ReservedVGInfo, 0)
		if anno, exist := nlsCopy.Annotations[AnnoStorageReserve]; exist {
			if reservedVGInfos, err = getReservedVGInfo(anno); err != nil {
				log.Errorf("get reserved vg info failed: %s, but we ignore...", err.Error())
				return
			}
		}
		// get status first, for we need support regexp
		newStatus := new(lssv1alpha1.NodeLocalStorageStatus)
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
		newStatus.NodeStorageInfo.Phase = lssv1alpha1.NodeStorageRunning
		newStatus.NodeStorageInfo.State.Status = lssv1alpha1.ConditionTrue
		newStatus.NodeStorageInfo.State.Type = lssv1alpha1.StorageReady
		newStatus.NodeStorageInfo.State.LastHeartbeatTime = metav1.Now()
		if checkIfStatusTransition(d.CRDStatus, newStatus) {
			newStatus.NodeStorageInfo.State.LastTransitionTime = metav1.Now()
		} else {
			newStatus.NodeStorageInfo.State.LastTransitionTime = d.CRDStatus.NodeStorageInfo.State.LastTransitionTime
		}
		nlsCopy.Status.NodeStorageInfo = newStatus.NodeStorageInfo
		if nlsCopy.Status.FilteredStorageInfo.UpdateStatus.LastUpdateTime.Time.IsZero() {
			nlsCopy.Status.FilteredStorageInfo.UpdateStatus.LastUpdateTime = metav1.Now()
		}

		// only update status
		_, err = d.localclientset.StorageV1alpha1().NodeLocalStorages().UpdateStatus(context.Background(), nlsCopy, metav1.UpdateOptions{})
		if err != nil {
			log.Errorf("local storage CRD updateStatus error: %s", err.Error())
			return
		}
	}
}

// InitResource will create relevant resource
func (d *Discoverer) InitResource() {
	nls, err := d.localclientset.StorageV1alpha1().NodeLocalStorages().Get(context.Background(), d.Nodename, metav1.GetOptions{})
	if err != nil {
		log.Errorf("get node local storage %s failed: %s", d.Nodename, err.Error())
		return
	}
	vgs := nls.Spec.ResourceToBeInited.VGs
	mountpoints := nls.Spec.ResourceToBeInited.MountPoints
	for _, vg := range vgs {

		if _, err := lvm.LookupVolumeGroup(vg.Name); err == lvm.ErrVolumeGroupNotFound {
			err := d.createVG(vg.Name, vg.Devices)
			if err != nil {
				log.Errorf("create vg %s failed: %s", vg.Name, err.Error())
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

func checkIfStatusTransition(old, new *lssv1alpha1.NodeLocalStorageStatus) (transition bool) {
	transition = checkIfDeviceStatusTransition(old, new) || checkIfVGStatusTransition(old, new) || checkIfMPStatusTransition(old, new)
	return
}

func getReservedVGInfo(reservedAnno string) (infos map[string]ReservedVGInfo, err error) {
	// step 0: var definition
	infos = make(map[string]ReservedVGInfo, 0)
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
