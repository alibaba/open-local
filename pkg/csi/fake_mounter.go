package csi

import (
	"fmt"
	"strings"

	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
	testingexec "k8s.io/utils/exec/testing"
)

// FakeSafeMounter implements a mount.Interface interface suitable for use in unit tests.
type FakeSafeMounter struct {
	mount.FakeMounter
	testingexec.FakeExec
}

// NewFakeSafeMounter creates a mount.SafeFormatAndMount instance suitable for use in unit tests.
func NewFakeSafeMounter(scripts ...testingexec.FakeAction) *mount.SafeFormatAndMount {
	fakeSafeMounter := FakeSafeMounter{}
	fakeSafeMounter.ExactOrder = true

	for _, script := range scripts {
		outputScripts := []testingexec.FakeAction{script}
		fakeCmdAction := func(cmd string, args ...string) exec.Cmd {
			fakeCmd := &testingexec.FakeCmd{}
			fakeCmd.OutputScript = outputScripts
			fakeCmd.CombinedOutputScript = outputScripts
			fakeCmd.OutputCalls = 0

			return testingexec.InitFakeCmd(fakeCmd, cmd, args...)
		}

		fakeSafeMounter.CommandScript = append(fakeSafeMounter.CommandScript, fakeCmdAction)
	}

	return &mount.SafeFormatAndMount{
		Interface: &fakeSafeMounter,
		Exec:      &fakeSafeMounter,
	}
}

// Mount overrides mount.FakeMounter.Mount.
func (f *FakeSafeMounter) Mount(source, target, fstype string, options []string) error {
	if strings.Contains(source, "error_mount") {
		return fmt.Errorf("fake Mount: source error")
	} else if strings.Contains(target, "error_mount") {
		return fmt.Errorf("fake Mount: target error")
	}

	return nil
}

// IsLikelyNotMountPoint overrides mount.FakeMounter.IsLikelyNotMountPoint.
func (f *FakeSafeMounter) IsLikelyNotMountPoint(file string) (bool, error) {
	if strings.Contains(file, "error_is_likely") {
		return false, fmt.Errorf("fake IsLikelyNotMountPoint: fake error")
	}
	if strings.Contains(file, "false_is_likely") {
		return false, nil
	}
	return true, nil
}
