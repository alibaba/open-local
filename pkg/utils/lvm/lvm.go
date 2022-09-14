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

package lvm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	localtype "github.com/alibaba/open-local/pkg"
	log "k8s.io/klog/v2"
)

// Control verbose output of all LVM CLI commands
var Verbose bool

// isInsufficientSpace returns true if the error is due to insufficient space
func isInsufficientSpace(err error) bool {
	return strings.Contains(err.Error(), "insufficient free space")
}

// MaxSize states that all available space should be used by the
// create operation.
const MaxSize uint64 = 0

type simpleError string

func (s simpleError) Error() string { return string(s) }

const ErrNoSpace = simpleError("lvm: not enough free space")
const ErrPhysicalVolumeNotFound = simpleError("lvm: physical volume not found")
const ErrVolumeGroupNotFound = simpleError("lvm: volume group not found")

var vgnameRegexp = regexp.MustCompile("^[A-Za-z0-9_+.][A-Za-z0-9_+.-]*$")

const ErrInvalidVGName = simpleError("lvm: Name contains invalid character, valid set includes: [A-Za-z0-9_+.-]")

var lvnameRegexp = regexp.MustCompile("^[A-Za-z0-9_+.][A-Za-z0-9_+.-]*$")

const ErrInvalidLVName = simpleError("lvm: Name contains invalid character, valid set includes: [A-Za-z0-9_+.-]")

var tagRegexp = regexp.MustCompile("^[A-Za-z0-9_+.][A-Za-z0-9_+.-]*$")

const ErrTagInvalidLength = simpleError("lvm: Tag length must be between 1 and 1024 characters")
const ErrTagHasInvalidChars = simpleError("lvm: Tag must consist of only [A-Za-z0-9_+.-] and cannot start with a '-'")

type PhysicalVolume struct {
	dev string
}

// Remove removes the physical volume.
func (pv *PhysicalVolume) Remove() error {
	if err := run("pvremove", nil, pv.dev); err != nil {
		log.Errorf("pv remove error: %s", err.Error())
		return err
	}
	return nil
}

// Check runs the pvck command on the physical volume.
func (pv *PhysicalVolume) Check() error {
	if err := run("pvck", nil, pv.dev); err != nil {
		log.Errorf("pv check error: %s", err.Error())
		return err
	}
	return nil
}

type VolumeGroup struct {
	name string
}

func (vg *VolumeGroup) Name() string {
	return vg.name
}

// Check runs the vgck command on the volume group.
func (vg *VolumeGroup) Check() error {
	if err := run("vgck", nil, vg.name); err != nil {
		log.Errorf("vg check error: %s", err.Error())
		return err
	}
	return nil
}

// BytesTotal returns the current size in bytes of the volume group.
func (vg *VolumeGroup) BytesTotal() (uint64, error) {
	result := new(vgsOutput)
	if err := run("vgs", result, "--options=vg_size", vg.name); err != nil {
		if IsVolumeGroupNotFound(err) {
			return 0, ErrVolumeGroupNotFound
		}
		log.Errorf("BytesTotal error: %s", err.Error())
		return 0, err
	}
	for _, report := range result.Report {
		for _, vg := range report.Vg {
			return vg.VgSize, nil
		}
	}
	return 0, ErrVolumeGroupNotFound
}

// BytesFree returns the unallocated space in bytes of the volume group.
func (vg *VolumeGroup) BytesFree() (uint64, error) {
	result := new(vgsOutput)
	if err := run("vgs", result, "--options=vg_free", vg.name); err != nil {
		if IsVolumeGroupNotFound(err) {
			return 0, ErrVolumeGroupNotFound
		}
		log.Errorf("BytesFree error: %s", err.Error())
		return 0, err
	}
	for _, report := range result.Report {
		for _, vg := range report.Vg {
			return vg.VgFree, nil
		}
	}
	return 0, ErrVolumeGroupNotFound
}

// ExtentSize returns the size in bytes of a single extent.
func (vg *VolumeGroup) ExtentSize() (uint64, error) {
	result := new(vgsOutput)
	if err := run("vgs", result, "--options=vg_extent_size", vg.name); err != nil {
		if IsVolumeGroupNotFound(err) {
			return 0, ErrVolumeGroupNotFound
		}
		log.Errorf("ExtentSize error: %s", err.Error())
		return 0, err
	}
	for _, report := range result.Report {
		for _, vg := range report.Vg {
			return vg.VgExtentSize, nil
		}
	}
	return 0, ErrVolumeGroupNotFound
}

// ExtentCount returns the number of extents.
func (vg *VolumeGroup) ExtentCount() (uint64, error) {
	result := new(vgsOutput)
	if err := run("vgs", result, "--options=vg_extent_count", vg.name); err != nil {
		if IsVolumeGroupNotFound(err) {
			return 0, ErrVolumeGroupNotFound
		}
		log.Errorf("ExtentCount error: %s", err.Error())
		return 0, err
	}
	for _, report := range result.Report {
		for _, vg := range report.Vg {
			return vg.VgExtentCount, nil
		}
	}
	return 0, ErrVolumeGroupNotFound
}

// ExtentFreeCount returns the number of free extents.
func (vg *VolumeGroup) ExtentFreeCount() (uint64, error) {
	result := new(vgsOutput)
	if err := run("vgs", result, "--options=vg_free_count", vg.name); err != nil {
		if IsVolumeGroupNotFound(err) {
			return 0, ErrVolumeGroupNotFound
		}
		log.Errorf("ExtentFreeCount error: %s", err.Error())
		return 0, err
	}
	for _, report := range result.Report {
		for _, vg := range report.Vg {
			return vg.VgFreeExtentCount, nil
		}
	}
	return 0, ErrVolumeGroupNotFound
}

// CreateLogicalVolume creates a logical volume of the given device
// and size.
//
// The actual size may be larger than asked for as the smallest
// increment is the size of an extent on the volume group in question.
//
// If sizeInBytes is zero the entire available space is allocated.
func (vg *VolumeGroup) CreateLogicalVolume(name string, sizeInBytes uint64, tags []string) (*LogicalVolume, error) {
	if err := ValidateLogicalVolumeName(name); err != nil {
		return nil, err
	}
	// Validate the tag.
	var args []string
	for _, tag := range tags {
		if tag != "" {
			if err := ValidateTag(tag); err != nil {
				return nil, err
			}
			args = append(args, "--add-tag="+tag)
		}
	}
	args = append(args, fmt.Sprintf("--size=%db", sizeInBytes))
	args = append(args, "--name="+name)
	args = append(args, vg.name)
	if err := run("lvcreate", nil, args...); err != nil {
		if isInsufficientSpace(err) {
			return nil, ErrNoSpace
		}
		log.Errorf("CreateLogicalVolume error: %s", err.Error())
		return nil, err
	}
	return &LogicalVolume{name, sizeInBytes, vg, "", 0}, nil
}

// ValidateLogicalVolumeName validates a volume group name. A valid volume
// group name can consist of a limited range of characters only. The allowed
// characters are [A-Za-z0-9_+.-].
func ValidateLogicalVolumeName(name string) error {
	if !lvnameRegexp.MatchString(name) {
		return ErrInvalidLVName
	}
	return nil
}

const ErrLogicalVolumeNotFound = simpleError("lvm: logical volume not found")

type lvsOutput struct {
	Report []struct {
		Lv []struct {
			Name        string  `json:"lv_name"`
			VgName      string  `json:"vg_name"`
			LvPath      string  `json:"lv_path"`
			LvSize      uint64  `json:"lv_size,string"`
			LvTags      string  `json:"lv_tags"`
			LvOrigin    string  `json:"origin"`
			LvSnapUsage float64 `json:"snap_percent,string"`
		} `json:"lv"`
	} `json:"report"`
}

func IsLogicalVolumeNotFound(err error) bool {
	const prefix = "Failed to find logical volume"
	lines := strings.Split(err.Error(), "\n")
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// LookupLogicalVolume looks up the logical volume in the volume group
// with the given name.
func (vg *VolumeGroup) LookupLogicalVolume(name string) (*LogicalVolume, error) {
	var err error
	result := new(lvsOutput)
	if err = run("lvs", result, "--options=lv_name,lv_size,vg_name,origin", vg.Name()); err != nil {
		if IsLogicalVolumeNotFound(err) {
			return nil, ErrLogicalVolumeNotFound
		}
		log.Errorf("LookupLogicalVolume error: %s", err.Error())
		return nil, err
	}
	for _, report := range result.Report {
		for _, lv := range report.Lv {
			if lv.VgName != vg.Name() {
				continue
			}
			if lv.Name != name {
				continue
			}
			var usage float64 = 0
			if lv.LvOrigin != "" {
				tmpResult := new(lvsOutput)
				_ = run("lvs", tmpResult, "--options=lv_name,lv_size,vg_name,origin,snap_percent", lv.VgName+"/"+lv.Name)
				usage = tmpResult.Report[0].Lv[0].LvSnapUsage
			}
			return &LogicalVolume{lv.Name, lv.LvSize, vg, lv.LvOrigin, usage / 100}, nil
		}
	}
	return nil, ErrLogicalVolumeNotFound
}

// ListLogicalVolumes returns the names of the logical volumes in this volume group.
func (vg *VolumeGroup) ListLogicalVolumeNames() ([]string, error) {
	var names []string
	result := new(lvsOutput)
	if err := run("lvs", result, "--options=lv_name,vg_name", vg.name); err != nil {
		log.Errorf("ListLogicalVolumeNames error: %s", err.Error())
		return nil, err
	}
	for _, report := range result.Report {
		for _, lv := range report.Lv {
			if lv.VgName == vg.name {
				names = append(names, lv.Name)
			}
		}
	}
	return names, nil
}

func IsPhysicalVolumeNotFound(err error) bool {
	return isPhysicalVolumeNotFound(err) ||
		isNoPhysicalVolumeLabel(err)
}

func isPhysicalVolumeNotFound(err error) bool {
	const prefix = "Failed to find device"
	lines := strings.Split(err.Error(), "\n")
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func isNoPhysicalVolumeLabel(err error) bool {
	const prefix = "No physical volume label read from"
	lines := strings.Split(err.Error(), "\n")
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func IsVolumeGroupNotFound(err error) bool {
	const prefix = "Volume group"
	const notFound = "not found"
	lines := strings.Split(err.Error(), "\n")
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) && strings.Contains(line, notFound) {
			return true
		}
	}
	return false
}

// ListPhysicalVolumeNames returns the names of the physical volumes in this volume group.
func (vg *VolumeGroup) ListPhysicalVolumeNames() ([]string, error) {
	var names []string
	result := new(pvsOutput)
	if err := run("pvs", result, "--options=pv_name,vg_name"); err != nil {
		log.Errorf("ListPhysicalVolumeNames error: %s", err.Error())
		return nil, err
	}
	for _, report := range result.Report {
		for _, pv := range report.Pv {
			if pv.VgName == vg.name {
				names = append(names, pv.Name)
			}
		}
	}
	return names, nil
}

// Remove removes the volume group from disk.
func (vg *VolumeGroup) Remove() error {
	if err := run("vgremove", nil, "-f", vg.name); err != nil {
		log.Errorf("volume group Remove error: %s", err.Error())
		return err
	}
	return nil
}

type LogicalVolume struct {
	name           string
	sizeInBytes    uint64
	vg             *VolumeGroup
	originLvName   string
	usageInPercent float64
}

func (lv *LogicalVolume) Name() string {
	return lv.name
}

func (lv *LogicalVolume) SizeInBytes() uint64 {
	return lv.sizeInBytes
}

func (lv *LogicalVolume) OriginLVName() string {
	return lv.originLvName
}

func (lv *LogicalVolume) Usage() float64 {
	return lv.usageInPercent
}

// Path returns the device path for the logical volume.
func (lv *LogicalVolume) Path() (string, error) {
	result := new(lvsOutput)
	if err := run("lvs", result, "--options=lv_path", lv.vg.name+"/"+lv.name); err != nil {
		if IsLogicalVolumeNotFound(err) {
			return "", ErrLogicalVolumeNotFound
		}
		log.Errorf("device path for the logical volume error: %s", err.Error())
		return "", err
	}
	for _, report := range result.Report {
		for _, lv := range report.Lv {
			return lv.LvPath, nil
		}
	}
	return "", ErrLogicalVolumeNotFound
}

func (lv *LogicalVolume) IsSnapshot() bool {
	return lv.originLvName != ""
}

func (lv *LogicalVolume) Remove() error {
	if err := run("lvremove", nil, "-f", lv.vg.name+"/"+lv.name); err != nil {
		log.Errorf("lvremove error: %s", err.Error())
		return err
	}
	return nil
}

func (lv *LogicalVolume) Expand(size uint64) error {
	args := []string{localtype.NsenterCmd, "lvextend", fmt.Sprintf("--size=+%db", size), lv.vg.name + "/" + lv.name}
	cmd := strings.Join(args, " ")
	log.V(6).Infof("[Expand]cmd: %s", cmd)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return err
	}
	log.Infof("[Expand]out: %s", string(out))

	return nil
}

// PVScan runs the `pvscan --cache <dev>` command. It scans for the
// device at `dev` and adds it to the LVM metadata cache if `lvmetad`
// is running. If `dev` is an empty string, it scans all devices.
func PVScan(dev string) error {
	args := []string{"--cache"}
	if dev != "" {
		args = append(args, dev)
	}

	err := run("pvscan", nil, args...)
	if err != nil {
		log.Errorf("PVScan error: %s", err.Error())
	}

	return err
}

// VGScan runs the `vgscan --cache <name>` command. It scans for the
// volume group and adds it to the LVM metadata cache if `lvmetad`
// is running. If `name` is an empty string, it scans all volume groups.
func VGScan(name string) error {
	args := []string{"--cache"}
	if name != "" {
		args = append(args, name)
	}

	err := run("vgscan", nil, args...)
	if err != nil {
		log.Errorf("VGScan error: %s", err.Error())
	}

	return err
}

// CreateVolumeGroup creates a new volume group.
func CreateVolumeGroup(
	name string,
	pvs []*PhysicalVolume,
	tags []string,
	force bool) (*VolumeGroup, error) {
	var args []string
	if err := ValidateVolumeGroupName(name); err != nil {
		return nil, err
	}
	for _, tag := range tags {
		if tag != "" {
			if err := ValidateTag(tag); err != nil {
				return nil, err
			}
			args = append(args, "--add-tag="+tag)
		}
	}
	args = append(args, name)
	for _, pv := range pvs {
		args = append(args, pv.dev)
	}

	if force {
		args = append(args, "--force")
	}
	if err := run("vgcreate", nil, args...); err != nil {
		log.Errorf("CreateVolumeGroup error: %s", err.Error())
		return nil, err
	}
	// Perform a best-effort scan to trigger a lvmetad cache refresh.
	// We ignore errors as for better or worse, the volume group now exists.
	// Without this lvmetad can fail to pickup newly created volume groups.
	// See https://bugzilla.redhat.com/show_bug.cgi?id=837599
	if err := PVScan(""); err != nil {
		log.Errorf("error during pvscan: %s", err.Error())
	}
	if err := VGScan(""); err != nil {
		log.Errorf("error during vgscan: %s", err.Error())
	}
	return &VolumeGroup{name}, nil
}

// ValidateVolumeGroupName validates a volume group name. A valid volume group
// name can consist of a limited range of characters only. The allowed
// characters are [A-Za-z0-9_+.-].
func ValidateVolumeGroupName(name string) error {
	if !vgnameRegexp.MatchString(name) {
		return ErrInvalidVGName
	}
	return nil
}

// ValidateTag validates a tag. LVM tags are strings of up to 1024
// characters. LVM tags cannot start with a hyphen. A valid tag can consist of
// a limited range of characters only. The allowed characters are
// [A-Za-z0-9_+.-]. As of the Red Hat Enterprise Linux 6.1 release, the list of
// allowed characters was extended, and tags can contain the /, =, !, :, #, and
// & characters.
// See https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/logical_volume_manager_administration/lvm_tags
func ValidateTag(tag string) error {
	if len(tag) > 1024 {
		return ErrTagInvalidLength
	}
	if !tagRegexp.MatchString(tag) {
		return ErrTagHasInvalidChars
	}
	return nil
}

type vgsOutput struct {
	Report []struct {
		Vg []struct {
			Name              string `json:"vg_name"`
			UUID              string `json:"vg_uuid"`
			VgSize            uint64 `json:"vg_size,string"`
			VgFree            uint64 `json:"vg_free,string"`
			VgExtentSize      uint64 `json:"vg_extent_size,string"`
			VgExtentCount     uint64 `json:"vg_extent_count,string"`
			VgFreeExtentCount uint64 `json:"vg_free_count,string"`
			VgTags            string `json:"vg_tags"`
		} `json:"vg"`
	} `json:"report"`
}

// LookupVolumeGroup returns the volume group with the given name.
func LookupVolumeGroup(name string) (*VolumeGroup, error) {
	result := new(vgsOutput)
	if err := run("vgs", result, "--options=vg_name", name); err != nil {
		if IsVolumeGroupNotFound(err) {
			return nil, ErrVolumeGroupNotFound
		}
		log.Errorf("LookupVolumeGroup error: %s", err.Error())
		return nil, err
	}
	for _, report := range result.Report {
		for _, vg := range report.Vg {
			return &VolumeGroup{vg.Name}, nil
		}
	}
	return nil, ErrVolumeGroupNotFound
}

// ListVolumeGroupNames returns the names of the list of volume groups. This
// does not normally scan for devices. To scan for devices, use the `Scan()`
// function.
func ListVolumeGroupNames() ([]string, error) {
	result := new(vgsOutput)
	if err := run("vgs", result); err != nil {
		log.Errorf("ListVolumeGroupNames error: %s", err.Error())
		return nil, err
	}
	var names []string
	for _, report := range result.Report {
		for _, vg := range report.Vg {
			names = append(names, vg.Name)
		}
	}
	return names, nil
}

// ListVolumeGroupUUIDs returns the UUIDs of the list of volume groups. This
// does not normally scan for devices. To scan for devices, use the `Scan()`
// function.
func ListVolumeGroupUUIDs() ([]string, error) {
	result := new(vgsOutput)
	if err := run("vgs", result, "--options=vg_uuid"); err != nil {
		log.Errorf("ListVolumeGroupUUIDs error: %s", err.Error())
		return nil, err
	}
	var uuids []string
	for _, report := range result.Report {
		for _, vg := range report.Vg {
			uuids = append(uuids, vg.UUID)
		}
	}
	return uuids, nil
}

// CreatePhysicalVolume creates a physical volume of the given device.
func CreatePhysicalVolume(dev string, force bool) (*PhysicalVolume, error) {
	var err error
	if force {
		err = run("pvcreate", nil, dev, "--force")
	} else {
		err = run("pvcreate", nil, dev)
	}
	if err != nil {
		log.Errorf("CreatePhysicalVolume error: %s", err.Error())
		return nil, fmt.Errorf("lvm: CreatePhysicalVolume: %s", err.Error())
	}
	return &PhysicalVolume{dev}, nil
}

type pvsOutput struct {
	Report []struct {
		Pv []struct {
			Name   string `json:"pv_name"`
			VgName string `json:"vg_name"`
		} `json:"pv"`
	} `json:"report"`
}

// ListPhysicalVolumes lists all physical volumes.
func ListPhysicalVolumes() ([]*PhysicalVolume, error) {
	result := new(pvsOutput)
	if err := run("pvs", result); err != nil {
		log.Errorf("ListPhysicalVolumes error: %s", err.Error())
		return nil, err
	}
	var pvs []*PhysicalVolume
	for _, report := range result.Report {
		for _, pv := range report.Pv {
			pvs = append(pvs, &PhysicalVolume{pv.Name})
		}
	}
	return pvs, nil
}

// LookupPhysicalVolume returns a physical volume with the given name.
func LookupPhysicalVolume(name string) (*PhysicalVolume, error) {
	result := new(pvsOutput)
	if err := run("pvs", result, "--options=pv_name", name); err != nil {
		if IsPhysicalVolumeNotFound(err) {
			return nil, ErrPhysicalVolumeNotFound
		}
		log.Errorf("LookupPhysicalVolume error: %s", err.Error())
		return nil, err
	}
	for _, report := range result.Report {
		for _, pv := range report.Pv {
			return &PhysicalVolume{pv.Name}, nil
		}
	}
	return nil, ErrPhysicalVolumeNotFound
}

// Extent sizing for linear logical volumes:
// https://github.com/Jajcus/lvm2/blob/266d6564d7a72fcff5b25367b7a95424ccf8089e/lib/metadata/metadata.c#L983

func run(cmd string, v interface{}, extraArgs ...string) error {
	var args []string
	cmdUseNsenter := fmt.Sprintf("%s %s", localtype.NsenterCmd, cmd)
	args = append(args, cmdUseNsenter)
	if v != nil {
		args = append(args, "--reportformat=json")
		args = append(args, "--units=b")
		args = append(args, "--nosuffix")
	}
	args = append(args, extraArgs...)
	c := exec.Command("sh", "-c", strings.Join(args[:], " "))
	// log.Infof("Executing: %s", c.String())
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	c.Stdout = stdout
	c.Stderr = stderr
	if err := c.Run(); err != nil {
		errstr := ignoreWarnings(stderr.String())
		// log.Infof("stdout: " + stdout.String())
		// log.Infof("stderr: " + errstr)
		log.V(6).Infof("[debug run]: command %s", c.String())
		log.V(6).Infof("[debug run]: error %s", err.Error())
		return errors.New(errstr)
	}
	stdoutbuf := stdout.Bytes()
	// stderrbuf := stderr.Bytes()
	// errstr := ignoreWarnings(string(stderrbuf))
	// log.Infof("stdout: " + string(stdoutbuf))
	// log.Infof("stderr: " + errstr)
	if v != nil {
		if err := json.Unmarshal(stdoutbuf, v); err != nil {
			return fmt.Errorf("unmarshal error: %s", err.Error())
		}
	}
	return nil
}

func ignoreWarnings(str string) string {
	lines := strings.Split(str, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "WARNING") {
			log.Warning(line)
			continue
		}
		// Ignore warnings of the kind:
		// "File descriptor 13 (pipe:[120900]) leaked on vgs invocation. Parent PID 2: ./csilvm"
		// For some reason lvm2 decided to complain if there are open file descriptors
		// that it didn't create when it exits. This doesn't play nice with the fact
		// that csilvm gets launched by e.g., mesos-agent.
		if strings.HasPrefix(line, "File descriptor") {
			log.Info(line)
			continue
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}
