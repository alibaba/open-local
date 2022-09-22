package csi

import (
	"os"
	"strings"

	"github.com/alibaba/open-local/pkg/utils"
	mountutils "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

type OSTool interface {
	Remove(name string) error
	Stat(name string) (os.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
	RunCommand(cmd string) (string, error)
	IsBlockDevice(fullPath string) (bool, error)
	MountBlock(source, target string, opts ...string) error
	EnsureBlock(target string) error
	CleanupMountPoint(mountPath string, mounter mountutils.Interface, extensiveMountPointCheck bool) error
	ResizeFS(devicePath string, deviceMountPath string) (bool, error)
}

type osTool struct{}

func NewOSTool() OSTool {
	return &osTool{}
}

func (tool *osTool) Remove(name string) error {
	return os.Remove(name)
}

func (tool *osTool) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (tool *osTool) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (tool *osTool) IsBlockDevice(fullPath string) (bool, error) {
	return utils.IsBlockDevice(fullPath)
}

func (tool *osTool) RunCommand(cmd string) (string, error) {
	return utils.Run(cmd)
}

func (tool *osTool) MountBlock(source, target string, opts ...string) error {
	return utils.MountBlock(source, target, opts...)
}

func (tool *osTool) EnsureBlock(target string) error {
	return utils.EnsureBlock(target)
}

func (tool *osTool) CleanupMountPoint(mountPath string, mounter mountutils.Interface, extensiveMountPointCheck bool) error {
	return mountutils.CleanupMountPoint(mountPath, mounter, extensiveMountPointCheck)
}

func (tool *osTool) ResizeFS(devicePath string, deviceMountPath string) (bool, error) {
	resizer := mountutils.NewResizeFs(utilexec.New())
	return resizer.Resize(devicePath, deviceMountPath)
}

type fakeOSTool struct{}

func NewFakeOSTool() OSTool {
	return &fakeOSTool{}
}

func (tool *fakeOSTool) Remove(name string) error {
	return nil
}

func (tool *fakeOSTool) Stat(name string) (os.FileInfo, error) {
	if strings.Contains(name, "snapshot") {
		return nil, nil
	}
	return nil, os.ErrNotExist
}

func (tool *fakeOSTool) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func (tool *fakeOSTool) RunCommand(cmd string) (string, error) {
	return "", nil
}

func (tool *fakeOSTool) IsBlockDevice(fullPath string) (bool, error) {
	return true, nil
}

func (tool *fakeOSTool) MountBlock(source, target string, opts ...string) error {
	return nil
}

func (tool *fakeOSTool) EnsureBlock(target string) error {
	return nil
}

func (tool *fakeOSTool) CleanupMountPoint(mountPath string, mounter mountutils.Interface, extensiveMountPointCheck bool) error {
	return nil
}

func (tool *fakeOSTool) ResizeFS(devicePath string, deviceMountPath string) (bool, error) {
	return true, nil
}
