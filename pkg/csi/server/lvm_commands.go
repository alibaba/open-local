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
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/csi/lib"
	"github.com/alibaba/open-local/pkg/utils"
	"golang.org/x/net/context"
)

const (
	// ProjQuotaPrefix is the template of quota fullpath
	ProjQuotaPrefix = "/mnt/quotapath.%s/%s"
	// ProjQuotaNamespacePrefix ...
	ProjQuotaNamespacePrefix = "/mnt/quotapath.%s"
)

type LvmCommads struct{}

// ListLV lists lvm volumes
func (lvm *LvmCommads) ListLV(listspec string) ([]*lib.LV, error) {
	lvs := []*lib.LV{}
	cmdList := []string{localtype.NsenterCmd, "lvs", "--units=b", fmt.Sprintf("--separator=\"%s\"", localtype.Separator), "--nosuffix", "--noheadings",
		"-o", "lv_name,lv_size,lv_uuid,lv_attr,copy_percent,lv_kernel_major,lv_kernel_minor,lv_tags", "--nameprefixes", "-a", listspec}
	cmd := strings.Join(cmdList, " ")
	out, err := utils.Run(cmd)
	if err != nil {
		if strings.Contains(err.Error(), "Failed to find logical volume") {
			return lvs, nil
		}
		return nil, err
	}
	outStr := strings.TrimSpace(string(out))
	outLines := strings.Split(outStr, "\n")
	for _, line := range outLines {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "LVM2_LV_NAME") {
			continue
		}
		lv, err := lib.ParseLV(line)
		if err != nil {
			return nil, errors.New("Parse LVM: " + line + ", with error: " + err.Error())
		}
		lvs = append(lvs, lv)
	}
	return lvs, nil
}

// CreateLV creates a new volume
func (lvm *LvmCommads) CreateLV(ctx context.Context, vg string, name string, size uint64, mirrors uint32, tags []string, striping bool) (string, error) {
	if size == 0 {
		return "", errors.New("size must be greater than 0")
	}
	args := []string{localtype.NsenterCmd, "lvcreate", "-n", name, "-L", fmt.Sprintf("%db", size), "-W", "y", "-y"}
	if mirrors > 0 {
		args = append(args, "-m", fmt.Sprintf("%d", mirrors), "--nosync")
	}
	for _, tag := range tags {
		args = append(args, "--add-tag", tag)
	}
	if striping {
		pvCount, err := getRequiredPVNumber(vg, size)
		if err != nil {
			return "", err
		}
		if pvCount == 0 {
			return "", fmt.Errorf("could not create `striping` logical volume, not enough space")
		}
		args = append(args, "-i", strconv.Itoa(pvCount))
	}

	args = append(args, vg)
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)
	return string(out), err
}

func getRequiredPVNumber(vgName string, lvSize uint64) (int, error) {
	pvs, err := ListPV(vgName)
	if err != nil {
		return 0, err
	}
	// calculate pv count
	pvCount := len(pvs)
	for pvCount > 0 {
		avgPvRequest := lvSize / uint64(pvCount)
		for num, pv := range pvs {
			if pv.FreeSize < avgPvRequest {
				pvs = append(pvs[:num], pvs[num+1:]...)
			}
		}
		if pvCount == len(pvs) {
			break
		}
		pvCount = len(pvs)
	}
	return pvCount, nil
}

// ProtectedTagName is a tag that prevents RemoveLV & RemoveVG from removing a volume
const ProtectedTagName = "protected"

// RemoveLV removes a volume
func (lvm *LvmCommads) RemoveLV(ctx context.Context, vg string, name string) (string, error) {
	lvs, err := lvm.ListLV(fmt.Sprintf("%s/%s", vg, name))
	if err != nil {
		return "", fmt.Errorf("failed to list LVs: %v", err)
	}
	if len(lvs) == 0 {
		return "lvm " + vg + "/" + name + " is not exist, skip remove", nil
	}
	if len(lvs) != 1 {
		return "", fmt.Errorf("expected 1 LV, got %d", len(lvs))
	}
	for _, tag := range lvs[0].Tags {
		if tag == ProtectedTagName {
			return "", errors.New("volume is protected")
		}
	}

	args := []string{localtype.NsenterCmd, "lvremove", "-v", "-f", fmt.Sprintf("%s/%s", vg, name)}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)

	return string(out), err
}

// CloneLV clones a volume via dd
func (lvm *LvmCommads) CloneLV(ctx context.Context, src, dest string) (string, error) {
	// FIXME(farcaller): bloody insecure. And broken.

	args := []string{localtype.NsenterCmd, "dd", fmt.Sprintf("if=%s", src), fmt.Sprintf("of=%s", dest), "bs=4M"}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)

	return string(out), err
}

// ExpandLV expand a volume
func (lvm *LvmCommads) ExpandLV(ctx context.Context, vgName string, volumeId string, expectSize uint64) (string, error) {
	// resize lvm volume
	// lvextend -L3G /dev/vgtest/lvm-5db74864-ea6b-11e9-a442-00163e07fb69
	resizeCmd := fmt.Sprintf("%s lvextend -L%dB %s/%s", localtype.NsenterCmd, expectSize, vgName, volumeId)
	out, err := utils.Run(resizeCmd)
	if err != nil {
		return "", err
	}

	return string(out), err
}

// ListVG get vg info
func (lvm *LvmCommads) ListVG() ([]*lib.VG, error) {
	args := []string{localtype.NsenterCmd, "vgs", "--units=b", fmt.Sprintf("--separator=\"%s\"", localtype.Separator), "--nosuffix", "--noheadings",
		"-o", "vg_name,vg_size,vg_free,vg_uuid,vg_tags,pv_count", "--nameprefixes", "-a"}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	outStr := strings.TrimSpace(string(out))
	outLines := strings.Split(outStr, "\n")
	vgs := make([]*lib.VG, len(outLines))
	for i, line := range outLines {
		line = strings.TrimSpace(line)
		vg, err := lib.ParseVG(line)
		if err != nil {
			return nil, err
		}
		vgs[i] = vg
	}
	return vgs, nil
}

// ListPV get pv info in vg
func ListPV(vgName string) ([]*lib.PV, error) {
	args := []string{localtype.NsenterCmd, "pvs", "--units=b", fmt.Sprintf("--separator=\"%s\"", localtype.Separator), "--nosuffix", "--noheadings",
		"-o", "pv_name,pv_size,pv_free,pv_uuid,pv_tags,vg_name", "-S", fmt.Sprintf("%s=%s", "vg_name", vgName), "--nameprefixes", "-a"}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	outStr := strings.TrimSpace(string(out))
	outLines := strings.Split(outStr, "\n")
	pvs := make([]*lib.PV, len(outLines))
	for i, line := range outLines {
		line = strings.TrimSpace(line)
		pv, err := lib.ParsePV(line)
		if err != nil {
			return nil, err
		}
		pvs[i] = pv
	}
	return pvs, nil
}

// CreateSnapshot creates a new volume snapshot
func (lvm *LvmCommads) CreateSnapshot(ctx context.Context, vg string, snapshotName string, originLVName string, size uint64) (string, error) {
	if size == 0 {
		return "", errors.New("size must be greater than 0")
	}

	args := []string{localtype.NsenterCmd, "lvcreate", "-s", "-n", snapshotName, "-L", fmt.Sprintf("%db", size), fmt.Sprintf("%s/%s", vg, originLVName), "-y"}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)

	return string(out), err
}

// RemoveSnapshot removes a volume snapshot
func (lvm *LvmCommads) RemoveSnapshot(ctx context.Context, vg string, name string) (string, error) {
	lvs, err := lvm.ListLV(fmt.Sprintf("%s/%s", vg, name))
	if err != nil {
		return "", fmt.Errorf("failed to list LVs: %v", err)
	}
	if len(lvs) == 0 {
		return "lvm " + vg + "/" + name + " is not exist, skip remove", nil
	}
	if len(lvs) != 1 {
		return "", fmt.Errorf("expected 1 LV, got %d", len(lvs))
	}

	args := []string{localtype.NsenterCmd, "lvremove", "-v", "-f", fmt.Sprintf("%s/%s", vg, name)}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)

	return string(out), err
}

// CreateVG create volume group
func (lvm *LvmCommads) CreateVG(ctx context.Context, name string, physicalVolume string, tags []string) (string, error) {
	args := []string{localtype.NsenterCmd, "vgcreate", name, physicalVolume, "-v"}
	for _, tag := range tags {
		args = append(args, "--add-tag", tag)
	}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)

	return string(out), err
}

// RemoveVG remove volume group
func (lvm *LvmCommads) RemoveVG(ctx context.Context, name string) (string, error) {
	vgs, err := lvm.ListVG()
	if err != nil {
		return "", fmt.Errorf("failed to list VGs: %v", err)
	}
	var vg *lib.VG
	for _, v := range vgs {
		if v.Name == name {
			vg = v
			break
		}
	}
	if vg == nil {
		return "", fmt.Errorf("could not find vg to delete")
	}
	for _, tag := range vg.Tags {
		if tag == ProtectedTagName {
			return "", errors.New("volume is protected")
		}
	}

	args := []string{localtype.NsenterCmd, "vgremove", "-v", "-f", name}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)

	return string(out), err
}

// CleanPath deletes all the contents under the given directory
func (lvm *LvmCommads) CleanPath(ctx context.Context, path string) error {
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

func (lvm *LvmCommads) CleanDevice(ctx context.Context, device string) (string, error) {
	if _, err := os.Stat(device); err != nil {
		return "", err
	}

	args := make([]string, 0)
	args = append(args, localtype.NsenterCmd)
	args = append(args, "wipefs")
	args = append(args, "-a")
	args = append(args, device)

	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)

	return string(out), err
}

// AddTagLV add tag
func (lvm *LvmCommads) AddTagLV(ctx context.Context, vg string, name string, tags []string) (string, error) {

	lvs, err := lvm.ListLV(fmt.Sprintf("%s/%s", vg, name))
	if err != nil {
		return "", fmt.Errorf("failed to list LVs: %v", err)
	}
	if len(lvs) != 1 {
		return "", fmt.Errorf("expected 1 LV, got %d", len(lvs))
	}

	args := make([]string, 0)
	args = append(args, localtype.NsenterCmd)
	args = append(args, "lvchange")
	for _, tag := range tags {
		args = append(args, "--addtag", tag)
	}

	args = append(args, fmt.Sprintf("%s/%s", vg, name))
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)

	return string(out), err
}

// RemoveTagLV remove tag
func (lvm *LvmCommads) RemoveTagLV(ctx context.Context, vg string, name string, tags []string) (string, error) {

	lvs, err := lvm.ListLV(fmt.Sprintf("%s/%s", vg, name))
	if err != nil {
		return "", fmt.Errorf("failed to list LVs: %v", err)
	}
	if len(lvs) != 1 {
		return "", fmt.Errorf("expected 1 LV, got %d", len(lvs))
	}

	args := make([]string, 0)
	args = append(args, localtype.NsenterCmd)
	args = append(args, "lvchange")
	for _, tag := range tags {
		args = append(args, "--deltag", tag)
	}

	args = append(args, fmt.Sprintf("%s/%s", vg, name))
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)
	return string(out), err
}

// CreateNameSpace creates a new namespace
// ndctl create-namespace -r region0 --size=6G -n webpmem1
func (lvm *LvmCommads) CreateNameSpace(ctx context.Context, region string, name string, size uint64) (string, error) {
	if size == 0 {
		return "", errors.New("size must be greater than 0")
	}
	args := []string{localtype.NsenterCmd, "ndctl", "create-namespace", "-r", region, "-s", fmt.Sprintf("%d", size), "-n", name}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)
	return string(out), err
}

// ConvertString2int convert pvName to int data
func ConvertString2int(origin string) string {
	h := sha256.New()
	h.Write([]byte(origin))
	hashResult := fmt.Sprintf("%x", h.Sum(nil))
	for {
		if hashResult[0] == '0' {
			hashResult = hashResult[1:]
			continue
		}
		break
	}
	return str2ASCII(hashResult[:9])[:9]
}

func str2ASCII(origin string) string {
	var result string
	for _, c := range origin {
		if !unicode.IsDigit(c) {
			result += strconv.Itoa(int(rune(c)))
		} else {
			result += string(c)
		}
	}
	return result
}

// SetProjectID2PVSubpath ...
func (lvm *LvmCommads) SetProjectID2PVSubpath(subPath, fullPath string, run utils.CommandRunFunc) (string, error) {
	projectID := ConvertString2int(subPath)
	args := []string{localtype.NsenterCmd, "chattr", "+P -p", fmt.Sprintf("%s %s", projectID, fullPath)}
	cmd := strings.Join(args, " ")
	_, err := run(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to set projectID to subpath with error: %v", err)
	}
	return projectID, nil
}

func getTotalLimitKBFromCSV(in string) (totalLimit int64, err error) {
	r := csv.NewReader(strings.NewReader(in))
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		trimedStr := strings.TrimSpace(record[0])
		if strings.HasPrefix(trimedStr, "#") && trimedStr != "#0" {
			if err != nil {
				return 0, err
			}
			limitKByte, err := strconv.ParseInt(record[5], 10, 64)
			if err != nil {
				return 0, err
			}
			totalLimit += limitKByte
		}
	}
	return
}

// GetNamespaceAssignedQuota ...
func (lvm *LvmCommads) GetNamespaceAssignedQuota(namespace string) (int, error) {
	args := []string{localtype.NsenterCmd, "repquota", "-P -O csv", fmt.Sprintf(ProjQuotaNamespacePrefix, namespace)}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)
	if err != nil {
		return 0, fmt.Errorf("failed to request namespace quota with error: %v", err)
	}
	totalLimit, err := getTotalLimitKBFromCSV(out)
	if err != nil {
		return 0, err
	}

	return int(totalLimit), nil
}

func findBlockLimitByProjectID(out, projectID string) (string, string, error) {
	r := csv.NewReader(strings.NewReader(out))
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", err
		}
		trimedStr := strings.TrimSpace(record[0])
		if strings.HasPrefix(trimedStr, "#") && trimedStr != "#0" {
			if strings.Contains(trimedStr, projectID) {
				return record[4], record[5], nil
			}
		}
	}
	return "", "", fmt.Errorf("findBlockLimitByProjectID: cannot find projectID: %s in output", projectID)
}

func checkSubpathProjQuotaEqual(projQuotaNamespacePath, projectID, blockHardLimitExpected, blockSoftLimitExpected string) (bool, error) {

	args := []string{localtype.NsenterCmd, "repquota", "-P -O csv", projQuotaNamespacePath}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)
	if err != nil {
		return false, fmt.Errorf("failed to request namespace quota with error: %v", err)
	}
	blockSoftLimitActual, blockHardLimitActual, err := findBlockLimitByProjectID(out, projectID)
	if err != nil {
		return false, err
	}
	if blockHardLimitExpected == blockHardLimitActual && blockSoftLimitActual == blockHardLimitExpected {
		return true, nil
	}
	return false, fmt.Errorf("checkSubpathProjQuotaEqual:actualData:%s is not equals with setedData:%s, blockSoftLimit:%s, blockSoftlimit:%s", blockHardLimitExpected, blockHardLimitActual, blockSoftLimitExpected, blockSoftLimitActual)
}

// SetSubpathProjQuota ...
func (lvm *LvmCommads) SetSubpathProjQuota(ctx context.Context, projQuotaSubpath, blockHardlimit, blockSoftlimit string) (string, error) {
	projectID := ConvertString2int(filepath.Base(projQuotaSubpath))
	args := []string{localtype.NsenterCmd, "setquota", "-P", fmt.Sprintf("%s %s %s 0 0 %s", projectID, blockHardlimit, blockHardlimit, filepath.Dir(projQuotaSubpath))}
	cmd := strings.Join(args, " ")
	_, err := utils.Run(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to set quota to subpath with error: %v", err)
	}
	ok, err := checkSubpathProjQuotaEqual(filepath.Dir(projQuotaSubpath), projectID, blockHardlimit, blockHardlimit)
	if ok {
		return "", nil
	}
	return "", err
}

// RemoveProjQuotaSubpath ...
func (lvm *LvmCommads) RemoveProjQuotaSubpath(ctx context.Context, quotaSubpath string) (string, error) {
	args := []string{localtype.NsenterCmd, "rm", "-rf", quotaSubpath}
	cmd := strings.Join(args, " ")
	out, err := utils.Run(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to remove proj quota subpath with error: %v", err)
	}
	return out, nil
}
