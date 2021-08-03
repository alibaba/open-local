package lib

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	localtype "github.com/oecp/open-local/pkg"
	"github.com/oecp/open-local/pkg/utils"
	log "github.com/sirupsen/logrus"
	utilexec "k8s.io/utils/exec"
	k8smount "k8s.io/utils/mount"
)

const (
	// fsckErrorsCorrected tag
	fsckErrorsCorrected = 1
	// fsckErrorsUncorrected tag
	fsckErrorsUncorrected = 4
)

// ErrParse is an error that is returned when parse operation fails
var ErrParse = errors.New("Cannot parse output of blkid")

func formatAndMount(diskMounter *k8smount.SafeFormatAndMount, source string, target string, fstype string, mkfsOptions []string, mountOptions []string) error {
	readOnly := false
	for _, option := range mountOptions {
		if option == "ro" {
			readOnly = true
			break
		}
	}

	// check device fs
	mountOptions = append(mountOptions, "defaults")
	if !readOnly {
		// Run fsck on the disk to fix repairable issues, only do this for volumes requested as rw.
		args := []string{"-a", source}

		out, err := diskMounter.Exec.Command("fsck", args...).CombinedOutput()
		if err != nil {
			ee, isExitError := err.(utilexec.ExitError)
			switch {
			case err == utilexec.ErrExecutableNotFound:
				log.Warningf("'fsck' not found on system; continuing mount without running 'fsck'.")
			case isExitError && ee.ExitStatus() == fsckErrorsCorrected:
				log.Infof("Device %s has errors which were corrected by fsck.", source)
			case isExitError && ee.ExitStatus() == fsckErrorsUncorrected:
				return fmt.Errorf("'fsck' found errors on device %s but could not correct them: %s", source, string(out))
			case isExitError && ee.ExitStatus() > fsckErrorsUncorrected:
			}
		}
	}

	// Try to mount the disk
	mountErr := diskMounter.Interface.Mount(source, target, fstype, mountOptions)
	if mountErr != nil {
		// Mount failed. This indicates either that the disk is unformatted or
		// it contains an unexpected filesystem.
		// existingFormat, err := diskMounter.GetDiskFormat(source)
		existingFormat := ""
		// if err != nil {
		// 	return err
		// }
		if existingFormat == "" {
			if readOnly {
				// Don't attempt to format if mounting as readonly, return an error to reflect this.
				return errors.New("failed to mount unformatted volume as read only")
			}

			// Disk is unformatted so format it.
			args := []string{source}
			// Use 'ext4' as the default
			if len(fstype) == 0 {
				fstype = "ext4"
			}

			if fstype == "ext4" || fstype == "ext3" {
				args = []string{
					"-F",  // Force flag
					"-m0", // Zero blocks reserved for super-user
					source,
				}
				// add mkfs options
				if len(mkfsOptions) != 0 {
					args = []string{}
					for _, opts := range mkfsOptions {
						args = append(args, opts)
					}
					args = append(args, source)
				}
			}
			log.Infof("Disk %q appears to be unformatted, attempting to format as type: %q with options: %v", source, fstype, args)

			_, err := diskMounter.Exec.Command("mkfs."+fstype, args...).CombinedOutput()
			if err == nil {
				// the disk has been formatted successfully try to mount it again.
				return diskMounter.Interface.Mount(source, target, fstype, mountOptions)
			}
			log.Errorf("format of disk %q failed: type:(%q) target:(%q) options:(%q)error:(%v)", source, fstype, target, mkfsOptions, err)
			return err
		}
		// Disk is already formatted and failed to mount
		if len(fstype) == 0 || fstype == existingFormat {
			// This is mount error
			return mountErr
		}
		// Block device is formatted with unexpected filesystem, let the user know
		return fmt.Errorf("failed to mount the volume as %q, it already contains %s. Mount error: %v", fstype, existingFormat, mountErr)
	}

	return mountErr
}

func checkFSType(devicePath string) (string, error) {
	// We use `file -bsL` to determine whether any filesystem type is detected.
	// If a filesystem is detected (ie., the output is not "data", we use
	// `blkid` to determine what the filesystem is. We use `blkid` as `file`
	// has inconvenient output.
	// We do *not* use `lsblk` as that requires udev to be up-to-date which
	// is often not the case when a device is erased using `dd`.
	output, err := exec.Command("file", "-bsL", devicePath).CombinedOutput()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(output)) == "data" {
		return "", nil
	}
	output, err = exec.Command("blkid", "-c", "/dev/null", "-o", "export", devicePath).CombinedOutput()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Split(strings.TrimSpace(line), "=")
		if len(fields) != 2 {
			return "", ErrParse
		}
		if fields[0] == "TYPE" {
			return fields[1], nil
		}
	}
	return "", ErrParse
}

// EnsureFolder ...
func EnsureFolder(target string) error {
	mdkirCmd := "mkdir"
	_, err := exec.LookPath(mdkirCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return fmt.Errorf("%q executable not found in $PATH", mdkirCmd)
		}
		return err
	}

	mkdirFullPath := fmt.Sprintf("%s mkdir -p %s", localtype.NsenterCmd, target)
	_, err = utils.Run(mkdirFullPath)
	if err != nil {
		log.Errorf("Create path error: %v", err)
		return err
	}
	return nil
}
