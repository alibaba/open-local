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

	"github.com/alibaba/open-local/pkg/csi/lib"
	"golang.org/x/net/context"
)

type FakeCommands struct{}

// ListLV lists lvm volumes
func (fake *FakeCommands) ListLV(listspec string) ([]*lib.LV, error) {
	return []*lib.LV{
		{
			Name: "newLV",
		},
	}, nil
}
func (fake *FakeCommands) CreateLV(ctx context.Context, vg string, name string, size uint64, mirrors uint32, tags []string, striping bool) (string, error) {
	if size == 0 {
		return "", errors.New("size must be greater than 0")
	}
	return "CreateLV", nil
}
func (fake *FakeCommands) RemoveLV(ctx context.Context, vg string, name string) (string, error) {
	return "RemoveLV", nil
}
func (fake *FakeCommands) CloneLV(ctx context.Context, src, dest string) (string, error) {
	return "CloneLV", nil
}
func (fake *FakeCommands) ExpandLV(ctx context.Context, vgName string, volumeId string, expectSize uint64) (string, error) {
	return "ExpandLV", nil
}
func (fake *FakeCommands) CreateSnapshot(ctx context.Context, vg string, snapshotName string, originLVName string, size uint64) (string, error) {
	return "CreateSnapshot", nil
}
func (fake *FakeCommands) RemoveSnapshot(ctx context.Context, vg string, name string) (string, error) {
	return "RemoveSnapshot", nil
}
func (fake *FakeCommands) AddTagLV(ctx context.Context, vg string, name string, tags []string) (string, error) {
	return "AddTagLV", nil
}
func (fake *FakeCommands) RemoveTagLV(ctx context.Context, vg string, name string, tags []string) (string, error) {
	return "AddRemoveTagLVagLV", nil
}
func (fake *FakeCommands) CreateVG(ctx context.Context, name string, physicalVolume string, tags []string) (string, error) {
	return "CreateVG", nil
}
func (fake *FakeCommands) RemoveVG(ctx context.Context, name string) (string, error) {
	return "RemoveVG", nil
}
func (fake *FakeCommands) ListVG() ([]*lib.VG, error) {
	return []*lib.VG{
		{
			Name: "newVG",
		},
	}, nil
}
func (fake *FakeCommands) CleanPath(ctx context.Context, path string) error {
	return nil
}
func (fake *FakeCommands) CleanDevice(ctx context.Context, device string) (string, error) {
	return "CleanDevice", nil
}
