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

package errors

import (
	"fmt"

	"github.com/alibaba/open-local/pkg"
	"k8s.io/apimachinery/pkg/api/resource"
)

// A generic interface for getting
// simple error message of same type to avoid pouring too many error
// into etcd store
type PredicateError interface {
	// GetReason should not any node or pod specific info in order not
	// to break the scheduler side message aggregation
	GetReason() string
	// Error() should return detailed message, contains the
	// detailed message regarding node or reason
	Error() string
}

// NoSuchVGError means no named vg `name`
type NoSuchVGError struct {
	resource pkg.VolumeType
	vgName   string
	nodeName string
}

func (e *NoSuchVGError) GetReason() string {
	return fmt.Sprintf("no vg(%s) named %s in node %s", e.resource, e.vgName, e.nodeName)
}

func (e *NoSuchVGError) Error() string {
	return fmt.Sprintf("no vg(%s) named %s in node %s", e.resource, e.vgName, e.nodeName)
}

func NewNoSuchVGError(vgName string, nodeName string) *NoSuchVGError {
	return &NoSuchVGError{
		vgName:   vgName,
		nodeName: nodeName,
		resource: pkg.VolumeTypeLVM,
	}
}

// NoAvailableVGError means there is no volume group on `nodeName`
type NoAvailableVGError struct {
	resource pkg.VolumeType
	nodeName string
}

func (e *NoAvailableVGError) GetReason() string {
	return fmt.Sprintf("no %s storage configured on node %s. you can run commands \"kubectl get nls --template={{.spec}} -o template %s\" and \"vgscan\" and \"lsblk\" on this node to get more details", e.resource, e.nodeName, e.nodeName)
}

func (e *NoAvailableVGError) Error() string {
	return fmt.Sprintf("no %s storage configured on node %s", e.resource, e.nodeName)
}

func NewNoAvailableVGError(nodeName string) *NoAvailableVGError {
	return &NoAvailableVGError{
		nodeName: nodeName,
		resource: pkg.VolumeTypeLVM,
	}
}

type InsufficientLVMError struct {
	requested int64
	used      int64
	capacity  int64
	nodeName  string
	vgName    string
	resource  pkg.VolumeType
}

func (e *InsufficientLVMError) GetReason() string {
	requested := resource.NewQuantity(e.requested, resource.BinarySI)
	used := resource.NewQuantity(e.used, resource.BinarySI)
	capacity := resource.NewQuantity(e.capacity, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s storage on node %s, vg is %s, pvc requested %s, vg used %s, vg capacity %s",
		e.resource, e.nodeName, e.vgName, requested.String(), used.String(), capacity.String())
}

func (e *InsufficientLVMError) Error() string {
	requested := resource.NewQuantity(e.requested, resource.BinarySI)
	used := resource.NewQuantity(e.used, resource.BinarySI)
	capacity := resource.NewQuantity(e.capacity, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s storage on node %s, vg is %s, pvc requested %s, vg used %s, vg capacity %s",
		e.resource, e.nodeName, e.vgName, requested.String(), used.String(), capacity.String())
}

func NewInsufficientLVMError(requested, used, capacity int64, vgName string, nodeName string) *InsufficientLVMError {
	return &InsufficientLVMError{
		resource:  pkg.VolumeTypeLVM,
		requested: requested,
		used:      used,
		capacity:  capacity,
		vgName:    vgName,
		nodeName:  nodeName,
	}
}

type InsufficientDeviceCountError struct {
	requestedCount int64
	availableCount int64
	total          int64
	resource       pkg.VolumeType
	mediaType      pkg.MediaType
	nodeName       string
}

func (e InsufficientDeviceCountError) GetReason() string {
	return fmt.Sprintf("Insufficient %s(%s) storage on node %s, pod requested pvc count is %d, node available device count is %d, node device total is %d",
		e.resource, e.mediaType, e.nodeName, e.requestedCount, e.availableCount, e.total)
}

func (e *InsufficientDeviceCountError) Error() string {
	return fmt.Sprintf("Insufficient %s(%s) storage on node %s, pod requested pvc count is %d, node available device count is %d, node device total is %d",
		e.resource, e.mediaType, e.nodeName, e.requestedCount, e.availableCount, e.total)
}

func NewInsufficientDeviceCountError(requestedCount, availableCount, total int64, mediaType pkg.MediaType, nodeName string) *InsufficientDeviceCountError {
	return &InsufficientDeviceCountError{
		resource:       pkg.VolumeTypeDevice,
		requestedCount: requestedCount,
		availableCount: availableCount,
		total:          total,
		mediaType:      mediaType,
		nodeName:       nodeName,
	}
}

type InsufficientMountPointCountError struct {
	requestedCount int64
	availableCount int64
	total          int64
	resource       pkg.VolumeType
	mediaType      pkg.MediaType
	nodeName       string
}

func (e InsufficientMountPointCountError) GetReason() string {
	return fmt.Sprintf("Insufficient %s(%s) storage on node %s, pod requested pvc count is %d, node available mp count is %d, node total mp is %d",
		e.resource, e.mediaType, e.nodeName, e.requestedCount, e.availableCount, e.total)
}

func (e *InsufficientMountPointCountError) Error() string {
	return fmt.Sprintf("Insufficient %s(%s) storage on node %s, pod requested pvc count is %d, node available mp count is %d, node total mp is %d",
		e.resource, e.mediaType, e.nodeName, e.requestedCount, e.availableCount, e.total)
}
func NewInsufficientMountPointCountError(requestedCount, availableCount, total int64, mediaType pkg.MediaType, nodeName string) *InsufficientMountPointCountError {
	return &InsufficientMountPointCountError{
		resource:       pkg.VolumeTypeMountPoint,
		requestedCount: requestedCount,
		availableCount: availableCount,
		total:          total,
		mediaType:      mediaType,
		nodeName:       nodeName,
	}
}

type InsufficientExclusiveResourceError struct {
	requestedSize int64
	maxSize       int64
	resource      pkg.VolumeType
	nodeName      string
}

func (e InsufficientExclusiveResourceError) GetReason() string {
	requested := resource.NewQuantity(e.requestedSize, resource.BinarySI)
	max := resource.NewQuantity(e.maxSize, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s storage, pvc requested size is %s, max size of local devices on node %s is %s",
		e.resource, requested.String(), e.nodeName, max.String())
}

func (e *InsufficientExclusiveResourceError) Error() string {
	requested := resource.NewQuantity(e.requestedSize, resource.BinarySI)
	max := resource.NewQuantity(e.maxSize, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s storage, pvc requested size is %s, max size of local devices on node %s is %s",
		e.resource, requested.String(), e.nodeName, max.String())
}
func NewInsufficientExclusiveResourceError(resource pkg.VolumeType, requested, max int64) *InsufficientExclusiveResourceError {
	return &InsufficientExclusiveResourceError{
		resource:      resource,
		requestedSize: requested,
		maxSize:       max,
	}
}
