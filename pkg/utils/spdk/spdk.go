/*
Copyright 2022 Intel Corporation

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
package spdk

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	log "k8s.io/klog/v2"
)

const (
	DefaultSpdkSocket         = "/var/tmp/spdk.sock"
	DefaultVhostUserStorePath = "/var/run/kata-containers/vhost-user/"
)

type SpdkClient struct {
	rpcSocket          string
	vhostUserStorePath string
}

type Bdev struct {
	Name             string                 `json:"name"`
	Aliases          []string               `json:"aliases"`
	UUID             string                 `json:"uuid"`
	NumBlocks        uint64                 `json:"num_blocks"`
	BlockSize        uint64                 `json:"block_size"`
	SupportedIoTypes map[string]bool        `json:"supported_io_types,omitempty"`
	DriverSpecific   map[string]interface{} `json:"driver_specific,omitempty"`
}

type Lvstore struct {
	Uuid          string `json:"uuid"`
	Name          string `json:"name"`
	BaseBdev      string `json:"base_bdev"`
	TotalClusters uint64 `json:"total_data_clusters"`
	FreeClusters  uint64 `json:"free_clusters"`
	ClusterSize   uint64 `json:"cluster_size"`
}

type Lvol struct {
	Name     string
	UUID     string
	Total    uint64
	LvsUuid  string //lvstore uuid
	ReadOnly bool
	Snapshot bool
	Clone    bool
}

func NewSpdkClient(rpcSocket string) *SpdkClient {
	if rpcSocket == "" {
		rpcSocket = DefaultSpdkSocket
	}

	cli := &SpdkClient{
		rpcSocket:          rpcSocket,
		vhostUserStorePath: DefaultVhostUserStorePath,
	}

	// ensure vhost paths exist
	dir := cli.vhostUserStorePath + "/block/sockets/"
	if _, err := os.Stat(dir); err != nil {
		if !os.IsExist(err) {
			if err := os.MkdirAll(dir, 0700); err != nil {
				log.Errorf("create dir (%s) failed: %s", dir, err.Error())
			}
		}
	}

	dir = cli.vhostUserStorePath + "/block/devices/"
	if _, err := os.Stat(dir); err != nil {
		if !os.IsExist(err) {
			if err := os.MkdirAll(dir, 0700); err != nil {
				log.Errorf("create dir (%s) failed: %s", dir, err.Error())
			}
		}
	}

	return cli
}

func (client *SpdkClient) Connect() (*rpc.Client, error) {
	pc1, _, _, _ := runtime.Caller(1)
	pc2, _, _, _ := runtime.Caller(2)
	log.Infof("... ... ... %s => %s", runtime.FuncForPC(pc2).Name(), runtime.FuncForPC(pc1).Name())

	conn, err := Dial("unix", client.rpcSocket)
	if err != nil {
		log.Error("can't connect to SPDK server:", err)
		return nil, err
	}

	return conn, nil
}

func (d *Bdev) IsReadonly() bool {
	if v, ok := d.SupportedIoTypes["write"]; ok && v {
		return false
	}

	return true
}

func (d *Bdev) IsLogicalVolume() bool {
	_, ok := d.DriverSpecific["lvol"]
	return ok
}

func (d *Bdev) getDriverSpecific(driver, key string) (interface{}, bool) {
	dsv, ok := d.DriverSpecific[driver]
	if !ok {
		return nil, false
	}

	info := dsv.(map[string]interface{})
	if v, ok := info[key]; ok {
		return v, true
	}

	return nil, false
}

func (d *Bdev) IsClone() bool {
	if v, ok := d.getDriverSpecific("lvol", "clone"); ok {
		return v.(bool)
	}

	return false
}

func (d *Bdev) IsSnapshot() bool {
	if v, ok := d.getDriverSpecific("lvol", "snapshot"); ok {
		return v.(bool)
	}

	return false
}

func (d *Bdev) GetFilename() string {
	if v, ok := d.getDriverSpecific("aio", "filename"); ok {
		return v.(string)
	}

	if v, ok := d.getDriverSpecific("uring", "filename"); ok {
		return v.(string)
	}

	return ""
}

func (d *Bdev) GetLvstoreUuid() string {
	if v, ok := d.getDriverSpecific("lvol", "lvol_store_uuid"); ok {
		return v.(string)
	}

	return ""
}

// CreateBdev creates a bdev
func (client *SpdkClient) CreateBdev(name, filename string) (string, error) {
	// check if bdev alreay exists
	bdevs, err := client.GetBdevs()
	if err != nil {
		return "", err
	}

	for _, bdev := range *bdevs {
		if bdev.Name == name {
			log.Infof("CreateBdev (%s %s): bdev exists", name, filename)
			return bdev.Name, nil
		}
	}

	params := struct {
		Name      string `json:"name"`
		Filename  string `json:"filename"`
		BlockSize uint64 `json:"block_size"`
	}{
		Name:      name,
		Filename:  filename,
		BlockSize: 512,
	}

	var result string

	conn, err := client.Connect()
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if err := conn.Call("bdev_aio_create", &params, &result); err != nil {
		log.Errorf("CreateBdev (%s %s) failed: %s", name, filename, err.Error())
		return "", err
	}

	return name, nil
}

// DeleteBdev deletes a bdev
func (client *SpdkClient) DeleteBdev(name string) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	params := struct {
		Name string `json:"name"`
	}{
		Name: name,
	}

	if err := conn.Call("bdev_aio_delete", &params, nil); err != nil {
		log.Errorf("DeleteBdev (%s) failed: %s", name, err.Error())
		return err
	}

	return nil
}

// GetBdevs gets SPDK block devices (include logical volumes)
func (client *SpdkClient) GetBdevs() (*[]Bdev, error) {
	var result []Bdev

	conn, err := client.Connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.Call("bdev_get_bdevs", nil, &result); err != nil {
		log.Error("GetBdevs failed:", err.Error())
		return nil, err
	}

	return &result, nil
}

// CleanBdev deletes a bdev or vbdev (LV) and the relative vhost device
func (client *SpdkClient) CleanBdev(name string) error {
	bdevs, err := client.GetBdevs()
	if err != nil {
		log.Error("CleanBdev - GetBdevs failed:", err.Error())
		return err
	}

	for _, bdev := range *bdevs {
		if bdev.Name != name {
			continue
		}

		// find and delete the vhost device which uses the bdev as backend
		ctrls, err := client.GetVhostControllers()
		if err != nil {
			log.Error("CleanBdev - GetVhostControllers failed:", err.Error())
			return err
		}

		for _, ctrl := range *ctrls {
			if ctrl.GetBdev() == bdev.Name {
				if err := client.DeleteVhostDevice(ctrl.Name); err != nil {
					log.Error("CleanBdev - DeleteVhostDevice failed:", err.Error())
					return err
				}
				break
			}
		}

		// delete the bdev / vbdev
		if bdev.IsLogicalVolume() {
			if err := client.DeleteLV(name); err != nil {
				log.Error("CleanBdev - DeleteLV failed:", err.Error())
				return err
			}
		} else {
			if err := client.DeleteBdev(name); err != nil {
				log.Error("CleanBdev - DeleteBdev failed:", err.Error())
				return err
			}
		}
		break
	}

	return nil
}

// GetLV gets logical volumes.
// Note: Just to keep compatible to lvmd. In fact, it only return one LV at most.
func (client *SpdkClient) GetLV(aliase string) (*[]Lvol, error) {
	var result []Lvol
	var lvol Lvol

	bdevs, err := client.GetBdevs()
	if err != nil {
		return nil, err
	}

	for _, bdev := range *bdevs {
		if !bdev.IsLogicalVolume() {
			continue
		}

		for _, a := range bdev.Aliases {
			if a == aliase {
				lvol.Name = bdev.Name
				lvol.UUID = bdev.UUID
				lvol.Total = bdev.NumBlocks * bdev.BlockSize
				lvol.LvsUuid = bdev.GetLvstoreUuid()
				lvol.ReadOnly = bdev.IsReadonly()
				lvol.Snapshot = bdev.IsSnapshot()
				lvol.Clone = bdev.IsClone()
				result = append(result, lvol)

				break
			}
		}
	}

	if len(result) == 0 {
		log.Infof("GetLV (%s): Not found", aliase)
	}

	return &result, nil
}

// CreateLV creates a new logical volume
func (client *SpdkClient) CreateLV(lvsName, lvName string, size uint64) (string, error) {
	conn, err := client.Connect()
	if err != nil {
		return "", err
	}
	defer conn.Close()

	// check if LV alreay exists
	bdevs, err := client.GetBdevs()
	if err != nil {
		return "", err
	}

	aliase := lvsName + "/" + lvName
	for _, bdev := range *bdevs {
		for _, a := range bdev.Aliases {
			if a == aliase {
				log.Infof("CreateLV (%s %s %d): LV exists", lvsName, lvName, size)
				return bdev.Name, nil
			}
		}
	}

	params := struct {
		LvName  string `json:"lvol_name"`
		Size    uint64 `json:"size"`
		LvsName string `json:"lvs_name"`
	}{
		LvName:  lvName,
		Size:    size,
		LvsName: lvsName,
	}

	var uuid string
	if err := conn.Call("bdev_lvol_create", &params, &uuid); err != nil {
		log.Errorf("CreateLV (%s %s %d) failed: %s", lvsName, lvName, size, err.Error())
		return "", err
	}

	return uuid, nil
}

// DeleteLV deletes a logical volume
func (client *SpdkClient) DeleteLV(lvName string) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	params := struct {
		Name string `json:"name"`
	}{
		Name: lvName,
	}

	if err := conn.Call("bdev_lvol_delete", &params, nil); err != nil {
		log.Errorf("DeleteLV (%s) failed: %s", lvName, err.Error())
		return err
	}

	return nil
}

// CloneLV creates a logical volume based on a snapshot
func (client *SpdkClient) CloneLV(snapshotName, cloneName string) (string, error) {
	conn, err := client.Connect()
	if err != nil {
		return "", err
	}
	defer conn.Close()

	params := struct {
		SnapshotName string `json:"snapshot_name"`
		CloneName    string `json:"clone_name"`
	}{
		SnapshotName: snapshotName,
		CloneName:    cloneName,
	}

	var uuid string
	if err := conn.Call("bdev_lvol_clone", &params, &uuid); err != nil {
		log.Errorf("CloneLv (%s %s) failed: %s", snapshotName, cloneName, err.Error())
		return "", err
	}

	return uuid, nil
}

// ResizeLV resizes a logical volume
func (client *SpdkClient) ResizeLV(lvName string, size uint64) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	params := struct {
		Name string `json:"name"`
		Size uint64 `json:"size"`
	}{
		Name: lvName,
		Size: size,
	}

	if err := conn.Call("bdev_lvol_resize", &params, nil); err != nil {
		log.Errorf("ResizeLV (%s %d) failed: %s", lvName, size, err.Error())
		return err
	}

	return nil
}

// Snapshot captures a snapshot of the current state of a logical volume
func (client *SpdkClient) Snapshot(lvolName, snapShotName string) (string, error) {
	conn, err := client.Connect()
	if err != nil {
		return "", err
	}
	defer conn.Close()

	params := struct {
		LvolName     string `json:"lvol_name"`
		SnapShotName string `json:"snapshot_name"`
	}{
		LvolName:     lvolName,
		SnapShotName: snapShotName,
	}

	var uuid string
	if err := conn.Call("bdev_lvol_snapshot", &params, &uuid); err != nil {
		log.Errorf("Snapshot (%s %s) failed: %s", lvolName, snapShotName, err.Error())
		return "", err
	}

	return uuid, nil
}

// GetLvStores gets a list of logical volume stores
func (client *SpdkClient) GetLvStores() (*[]Lvstore, error) {
	conn, err := client.Connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var result []Lvstore
	if err := conn.Call("bdev_lvol_get_lvstores", nil, &result); err != nil {
		log.Error("GetLvStores failed:", err.Error())
		return nil, err
	}

	return &result, nil
}

// CreateLvstore creates a logical volume store
func (client *SpdkClient) CreateLvstore(bdevName, lvsName string) (string, error) {
	conn, err := client.Connect()
	if err != nil {
		return "", err
	}
	defer conn.Close()

	params := struct {
		BdevName string `json:"bdev_name"`
		LvsName  string `json:"lvs_name"`
	}{
		BdevName: bdevName,
		LvsName:  lvsName,
	}

	var uuid string
	if err := conn.Call("bdev_lvol_create_lvstore", &params, &uuid); err != nil {
		log.Errorf("CreateLvstore (%s %s) failed: %s", bdevName, lvsName, err.Error())
		return "", err
	}

	return uuid, nil
}

// RemoveLvstore destroys a logical volume store
func (client *SpdkClient) RemoveLvstore(lvsName string) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	params := struct {
		Name string `json:"lvs_name"`
	}{
		Name: lvsName,
	}

	if err := conn.Call("bdev_lvol_delete_lvstore", &params, nil); err != nil {
		log.Errorf("RemoveLvstore (%s) failed: %s", lvsName, err.Error())
		return err
	}

	return nil
}

const VhostUserBlkMajor = 241

func getMinor(dir string) (uint32, error) {
	var minors []uint32

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	for _, file := range files {
		var stat unix.Stat_t

		if file.IsDir() {
			continue
		}

		devFilePath := filepath.Join(dir, file.Name())
		err = unix.Stat(devFilePath, &stat)
		if err != nil {
			return 0, err
		}

		major := unix.Major(uint64(stat.Rdev))
		if major != VhostUserBlkMajor {
			continue
		}

		minors = append(minors, unix.Minor(uint64(stat.Rdev)))
	}

	m := uint32(0)
	for {
		exist := false
		for _, minor := range minors {
			// minor exists
			if minor == m {
				exist = true
				break
			}
		}

		// not found, the minor is available
		if !exist {
			return m, nil
		}

		m++
	}
}

func (client *SpdkClient) FindVhostDevice(bdevName string) (string, error) {
	ctrls, err := client.GetVhostControllers()
	if err != nil {
		log.Error("FindVhostDevice - GetVhostControllers failed:", err.Error())
		return "", err
	}

	for _, ctrl := range *ctrls {
		if ctrl.GetBdev() == bdevName {
			log.Infof("FindVhostDevice (%s): found", bdevName)
			return client.vhostUserStorePath + "/block/devices/" + ctrl.Name, nil
		}
	}

	log.Infof("FindVhostDevice (%s): Not found", bdevName)
	return "", nil
}

// CreateVhostDevice creates a vhost device
func (client *SpdkClient) CreateVhostDevice(ctrlrName, bdevName string) (string, error) {
	// check if device already exists
	dev := client.vhostUserStorePath + "/block/devices/" + ctrlrName
	if _, err := os.Stat(dev); err != nil {
		if os.IsExist(err) {
			log.Warningf("CreateVhostDevice (%s, %s): vhost device already exists", ctrlrName, bdevName)
			return dev, nil
		}
	} else {
		log.Warningf("CreateVhostDevice (%s, %s): vhost device already exists", ctrlrName, bdevName)
		return dev, nil
	}

	conn, err := client.Connect()
	if err != nil {
		return "", err
	}

	defer conn.Close()

	params := struct {
		CtrlrName string `json:"ctrlr"`
		BdevName  string `json:"dev_name"`
	}{
		CtrlrName: ctrlrName,
		BdevName:  bdevName,
	}

	if err := conn.Call("vhost_create_blk_controller", &params, nil); err != nil {
		log.Errorf("CreateVhostDevice (%s, %s) failed: %s", ctrlrName, bdevName, err.Error())
		return "", err
	}

	cmd := "mknod " + dev + " b 241 "
	minor, err := getMinor(filepath.Join(client.vhostUserStorePath, "/block/devices/"))
	if err != nil {
		log.Error("getMinor failed:", err.Error())
		_ = client.DeleteVhostDevice(ctrlrName)
		return "", fmt.Errorf("Failed to get minor")
	}

	cmd += strconv.FormatUint(uint64(minor), 10)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Errorf("run cmd (%s) failed (%s): %s", cmd, string(out), err.Error())
		_ = client.DeleteVhostDevice(ctrlrName)
		return "", fmt.Errorf("Failed to run cmd: " + cmd + ", with output: " + string(out) + ", with error: " + err.Error())
	}

	return dev, nil
}

// DeleteVhostDevice deletes a vhost device
func (client *SpdkClient) DeleteVhostDevice(name string) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}

	defer conn.Close()

	params := struct {
		CtrlrName string `json:"ctrlr"`
	}{
		CtrlrName: name,
	}

	if err := conn.Call("vhost_delete_controller", &params, nil); err != nil {
		log.Errorf("vhost_delete_controller (%s) failed: %s", name, err.Error())
		return err
	}

	dev := client.vhostUserStorePath + "/block/devices/" + name
	if err := os.Remove(dev); err != nil {
		log.Warningf("fail to remove (%s): %s", dev, err.Error())
		return fmt.Errorf("fail to remove (%s): %s", dev, err.Error())
	}

	return nil
}

type Ctrlr struct {
	Name            string                 `json:"ctrlr"`
	BackendSpecific map[string]interface{} `json:"backend_specific"`
}

func (c *Ctrlr) getBackendSpecific(backend, key string) (interface{}, bool) {
	spec, ok := c.BackendSpecific[backend]
	if !ok {
		return nil, false
	}

	info := spec.(map[string]interface{})
	if v, ok := info[key]; ok {
		return v, true
	}

	return nil, false
}

func (c *Ctrlr) GetBdev() string {
	if v, ok := c.getBackendSpecific("block", "bdev"); ok {
		// if backend bdev has been deleted, the value is nil
		if v == nil {
			return ""
		} else {
			return v.(string)
		}
	}

	return ""
}

func (client *SpdkClient) GetVhostControllers() (*[]Ctrlr, error) {
	conn, err := client.Connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var results []Ctrlr

	if err := conn.Call("vhost_get_controllers", nil, &results); err != nil {
		log.Error("GetVhostControllers failed: ", err.Error())
		return nil, err
	}

	return &results, nil
}

// ensure logical volume name length not exceed 63
func EnsureLVNameValid(name string) string {
	lvname := name
	if len(lvname) >= 64 {
		lvname = strings.TrimPrefix(lvname, "csi-")
		if len(lvname) >= 64 {
			lvname = string([]byte(lvname)[:62])
		}
	}

	if lvname != name {
		log.Infof("lvname (%s) is shortened to (%s)", name, lvname)
	}

	return lvname
}

func getLoopDeviceFromSysfs(path string) (string, error) {
	// If the file is a symlink.
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to evaluate path %s: %s", path, err)
	}

	devices, err := filepath.Glob("/sys/block/loop*")
	if err != nil {
		return "", fmt.Errorf("failed to list loop devices in sysfs: %s", err)
	}

	for _, device := range devices {
		backingFile := fmt.Sprintf("%s/loop/backing_file", device)

		// The contents of this file is the absolute path of "path".
		data, err := ioutil.ReadFile(backingFile)
		if err != nil {
			continue
		}

		// Return the first match.
		backingFilePath := strings.TrimSpace(string(data))
		if backingFilePath == path || backingFilePath == realPath {
			return fmt.Sprintf("/dev/%s", filepath.Base(device)), nil
		}
	}

	return "", errors.New("device not found")
}

// GenerateDeviceTempBackingFile creates and attaches a small file to
// a loop device and return the attached loop device.
func GenerateDeviceTempBackingFile(dev string) (string, error) {
	// create a temp file as backing file
	tmpfile := dev + ".img"
	cmd := fmt.Sprintf("dd if=/dev/zero of=%s bs=1024 count=1", tmpfile)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Errorf("run cmd (%s) failed (%s): %s", cmd, string(out), err.Error())
		return "", fmt.Errorf("Failed to run cmd: " + cmd + ", with output: " + string(out) + ", with error: " + err.Error())
	}

	// attach to a loop device
	cmd = "losetup -f " + tmpfile
	out, err = exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Errorf("run cmd (%s) failed (%s): %s", cmd, string(out), err.Error())
		if err := os.Remove(tmpfile); err != nil {
			log.Warningf("fail to remove (%s): %s", tmpfile, err.Error())
		}
		return "", fmt.Errorf("Failed to run cmd: " + cmd + ", with output: " + string(out) + ", with error: " + err.Error())
	}

	return getLoopDeviceFromSysfs(tmpfile)
}
