//go:build linux
// +build linux

/*
Copyright 2022/9/2 Alibaba Cloud.

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
package utils

import (
	"errors"
	"fmt"
	log "k8s.io/klog/v2"
	utilexec "k8s.io/utils/exec"
	k8smount "k8s.io/utils/mount"
	"os/exec"
)

func FormatBlockDevice(dev, fsType string) error {
	mounter := &k8smount.SafeFormatAndMount{Interface: k8smount.New(""), Exec: utilexec.New()}
	existingFormat, err := mounter.GetDiskFormat(dev)
	if err != nil {
		log.Errorf("FormatBlockDevice - failed to get disk format of disk %s: %s", dev, err.Error())
		return err
	}

	if existingFormat == "" {
		log.Info("going to mkfs: ", fsType)
		cmd := fmt.Sprintf("mkfs.%s %s", fsType, dev)
		if fsType == "xfs" {
			cmd = cmd + " -f"
		} else {
			cmd = cmd + " -F"
		}
		if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
			log.Errorf("run cmd (%s) failed (%s): %s\n", cmd, string(out), err.Error())
			return err
		} else {
			log.Info("FS create: ", fsType)
		}
	} else {
		if fsType != existingFormat {
			log.Warningf("disk %s current is %s but try to format as %s", dev, existingFormat, fsType)
			log.Warning("To avoid data damage, Fs creating is skipped")
			return errors.New("The block device is already formatted, to avoid data damage FS creating is skipped")
		} else {
			log.Info("FS exsits: ", fsType)
		}
	}

	return nil
}
