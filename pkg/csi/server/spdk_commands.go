/*

Copyright 2017 Google Inc.

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

package server

import (
	"errors"
	"os"
	"path/filepath"

	"strings"

	"github.com/alibaba/open-local/pkg/csi/lib"
	spdk "github.com/alibaba/open-local/pkg/utils/spdk"
	"github.com/google/uuid"
	"golang.org/x/net/context"
	log "k8s.io/klog/v2"
)

type SpdkCommands struct {
	client *spdk.SpdkClient
}

func NewSpdkCommands(rpcSocket string) *SpdkCommands {
	return &SpdkCommands{client: spdk.NewSpdkClient(rpcSocket)}
}

// ListLV lists logical volumes
// listspec: vg/volumeID
func (cmd *SpdkCommands) ListLV(listspec string) ([]*lib.LV, error) {
	lvols, err := cmd.client.GetLV(listspec)
	if err != nil {
		return nil, err
	}

	lvs := make([]*lib.LV, len(*lvols))

	for i, lvol := range *lvols {
		lvs[i] = &lib.LV{}
		lvs[i].Name = lvol.Name
		lvs[i].Size = lvol.Total
		lvs[i].UUID = lvol.UUID
	}

	return lvs, nil
}

// CreateLV creates a new logical volume and relevant vhost device
func (cmd *SpdkCommands) CreateLV(ctx context.Context, vg string, name string, size uint64, mirrors uint32, tags []string, striping bool) (string, error) {
	if size == 0 {
		log.Error("CreateLV size is 0: size must be greater than 0")
		return "", errors.New("size must be greater than 0")
	}

	lvName := spdk.EnsureLVNameValid(name)

	lvn, err := cmd.client.CreateLV(vg, lvName, size)
	if err != nil {
		return "", err
	}

	device, _ := cmd.client.FindVhostDevice(lvn)
	if device == "" {
		if _, err := cmd.client.CreateVhostDevice("ctrlr-"+uuid.New().String(), lvn); err != nil {
			return "", err
		}
	}

	return "", nil
}

// RemoveLV removes a logical volume
func (cmd *SpdkCommands) RemoveLV(ctx context.Context, vg string, name string) (string, error) {
	lvName := spdk.EnsureLVNameValid(name)

	// Get LV by alias (to get the name of the LV)
	alias := vg + "/" + lvName
	lv, err := cmd.client.GetLV(alias)
	if err != nil {
		return "", err
	}

	if len(*lv) == 0 {
		log.Warningf("Logical volume (%s) isn't found", alias)
		return "", errors.New("Logical volume isn't found")
	}

	return "", cmd.client.CleanBdev((*lv)[0].Name)
}

// CloneLV clones a logical volume
func (cmd *SpdkCommands) CloneLV(ctx context.Context, src, dest string) (string, error) {
	return cmd.client.CloneLV(src, dest)
}

// ExpandLV expand a logical volume
func (cmd *SpdkCommands) ExpandLV(ctx context.Context, vgName string, volumeId string, expectSize uint64) (string, error) {
	volumeId = spdk.EnsureLVNameValid(volumeId)

	alias := vgName + "/" + volumeId
	return "", cmd.client.ResizeLV(alias, expectSize)
}

// ListVG get volume group (lvstore) info
func (cmd *SpdkCommands) ListVG() ([]*lib.VG, error) {
	lvss, err := cmd.client.GetLvStores()
	if err != nil {
		return nil, err
	}

	vgs := make([]*lib.VG, len(*lvss))
	for i, lvs := range *lvss {
		vgs[i] = &lib.VG{}
		vgs[i].Name = lvs.Name
		vgs[i].Size = lvs.TotalClusters * lvs.ClusterSize
		vgs[i].FreeSize = lvs.FreeClusters * lvs.ClusterSize
		vgs[i].UUID = lvs.Name
		vgs[i].PvCount = 1
	}
	return vgs, nil
}

// CreateSnapshot creates a new volume snapshot
func (cmd *SpdkCommands) CreateSnapshot(ctx context.Context, vg string, snapshotName string, originLVName string, size uint64) (string, error) {
	alias := vg + "/" + originLVName
	return cmd.client.Snapshot(alias, snapshotName)
}

// RemoveSnapshot removes a volume snapshot
func (cmd *SpdkCommands) RemoveSnapshot(ctx context.Context, vg string, name string) (string, error) {
	alias := vg + "/" + name
	return "", cmd.client.DeleteLV(alias)
}

// CreateVG create volume group (lvstore)
func (cmd *SpdkCommands) CreateVG(ctx context.Context, name string, physicalVolume string, tags []string) (string, error) {
	bdevName := "bdev-aio" + strings.Replace(physicalVolume, "/", "_", -1)
	bdev, err := cmd.client.CreateBdev(bdevName, physicalVolume)
	if err != nil {
		return "", err
	}

	return cmd.client.CreateLvstore(bdev, name)
}

// RemoveVG remove volume group (lvstore)
func (cmd *SpdkCommands) RemoveVG(ctx context.Context, name string) (string, error) {
	return "", cmd.client.RemoveLvstore(name)
}

// CleanPath deletes all the contents under the given directory
func (cmd *SpdkCommands) CleanPath(ctx context.Context, path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()

	files, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}
	errList := []error{}
	for _, file := range files {
		err = os.RemoveAll(filepath.Join(path, file))
		if err != nil {
			errList = append(errList, err)
		}
	}
	if len(errList) == 0 {
		return nil
	}
	return errList[0]
}

func (cmd *SpdkCommands) CleanDevice(ctx context.Context, device string) (string, error) {
	bdevs, err := cmd.client.GetBdevs()
	if err != nil {
		log.Error("CleanDevice - GetBdevs failed:", err.Error())
		return "", err
	}

	for _, bdev := range *bdevs {
		if bdev.GetFilename() == device {
			// find and delete the vhost device which uses the bdev as backend
			ctrls, err := cmd.client.GetVhostControllers()
			if err != nil {
				log.Error("CleanDevice - GetVhostControllers failed:", err.Error())
				return "", err
			}

			for _, ctrl := range *ctrls {
				if ctrl.GetBdev() == bdev.Name {
					if err := cmd.client.DeleteVhostDevice(ctrl.Name); err != nil {
						log.Error("CleanDevice - DeleteVhostDevice failed:", err.Error())
						return "", err
					}
					break
				}
			}

			if err := cmd.client.DeleteBdev(bdev.Name); err != nil {
				log.Error("CleanDevice - DeleteBdev failed:", err.Error())
				return "", err
			}
			break
		}
	}

	return "", nil
}

// AddTagLV add tag
func (cmd *SpdkCommands) AddTagLV(ctx context.Context, vg string, name string, tags []string) (string, error) {
	return string("SPDK doesn't support add LV tag"), errors.New("SPDK doesn't support add LV tag")
}

// RemoveTagLV remove tag
func (cmd *SpdkCommands) RemoveTagLV(ctx context.Context, vg string, name string, tags []string) (string, error) {
	return string("SPDK doesn't support remove LV tag"), errors.New("SPDK doesn't support remove LV tag")
}
