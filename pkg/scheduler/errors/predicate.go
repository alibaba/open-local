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

// NotSuchVGError means no named vg `name`
type NotSuchVGError struct {
	resource pkg.VolumeType
	name     string
}

func (e *NotSuchVGError) GetReason() string {
	return fmt.Sprintf("not %s named %s", e.resource, e.name)
}

func (e *NotSuchVGError) Error() string {
	return fmt.Sprintf("not %s named %s", e.resource, e.name)
}

func NewNotSuchVGError(name string) *NotSuchVGError {
	return &NotSuchVGError{
		name:     name,
		resource: pkg.VolumeTypeLVM,
	}
}

// NoAvailableVGError means there is no volume group on `nodeName`
type NoAvailableVGError struct {
	resource pkg.VolumeType
	nodeName string
}

func (e *NoAvailableVGError) GetReason() string {
	return fmt.Sprintf("no %s storage configured", e.resource)
}

func (e *NoAvailableVGError) Error() string {
	return fmt.Sprintf("not %s on node %s", e.resource, e.nodeName)
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
	resource  pkg.VolumeType
}

func (e *InsufficientLVMError) GetReason() string {
	requested := resource.NewQuantity(e.requested, resource.BinarySI)
	used := resource.NewQuantity(e.used, resource.BinarySI)
	capacity := resource.NewQuantity(e.capacity, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s storage, requested %s, used %s, capacity %s",
		e.resource, requested.String(), used.String(), capacity.String())
}

func (e *InsufficientLVMError) Error() string {
	requested := resource.NewQuantity(e.requested, resource.BinarySI)
	used := resource.NewQuantity(e.used, resource.BinarySI)
	capacity := resource.NewQuantity(e.capacity, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s storage, requested %s, used %s, capacity %s",
		e.resource, requested.String(), used.String(), capacity.String())
}
func NewInsufficientLVMError(requested, used, capacity int64) *InsufficientLVMError {
	return &InsufficientLVMError{
		resource:  pkg.VolumeTypeLVM,
		requested: requested,
		used:      used,
		capacity:  capacity,
	}
}

type InsufficientDeviceError struct {
	requested int64
	used      int64
	capacity  int64
	resource  pkg.VolumeType
}

func (e InsufficientDeviceError) GetReason() string {
	requested := resource.NewQuantity(e.requested, resource.BinarySI)
	used := resource.NewQuantity(e.used, resource.BinarySI)
	capacity := resource.NewQuantity(e.capacity, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s storage, requested %s, used %s, capacity %s",
		e.resource, requested.String(), used.String(), capacity.String())
}

func (e *InsufficientDeviceError) Error() string {
	requested := resource.NewQuantity(e.requested, resource.BinarySI)
	used := resource.NewQuantity(e.used, resource.BinarySI)
	capacity := resource.NewQuantity(e.capacity, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s storage, requested %s, used %s, capacity %s",
		e.resource, requested.String(), used.String(), capacity.String())
}
func NewInsufficientDeviceError(requested, used, capacity int64) *InsufficientDeviceError {
	return &InsufficientDeviceError{
		resource:  pkg.VolumeTypeDevice,
		requested: requested,
		used:      used,
		capacity:  capacity,
	}
}

type InsufficientMountPointError struct {
	requested int64
	available int64
	capacity  int64
	resource  pkg.VolumeType
	mediaType pkg.MediaType
}

func (e InsufficientMountPointError) GetReason() string {
	requested := resource.NewQuantity(e.requested, resource.BinarySI)
	available := resource.NewQuantity(e.available, resource.BinarySI)
	capacity := resource.NewQuantity(e.capacity, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s(%s) storage, requested %s, available %s, capacity %s(all media type)",
		e.resource, e.mediaType, requested.String(), available.String(), capacity.String())
}

func (e *InsufficientMountPointError) Error() string {
	requested := resource.NewQuantity(e.requested, resource.BinarySI)
	available := resource.NewQuantity(e.available, resource.BinarySI)
	capacity := resource.NewQuantity(e.capacity, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s(%s) storage, requested %s, available %s, capacity %s(all media type)",
		e.resource, e.mediaType, requested.String(), available.String(), capacity.String())
}
func NewInsufficientMountPointError(requested, available, capacity int64, mediaType pkg.MediaType) *InsufficientMountPointError {
	return &InsufficientMountPointError{
		resource:  pkg.VolumeTypeMountPoint,
		requested: requested,
		available: available,
		capacity:  capacity,
		mediaType: mediaType,
	}
}

type InsufficientExclusiveResourceError struct {
	requested int64
	available int64
	capacity  int64
	resource  pkg.VolumeType
}

func (e InsufficientExclusiveResourceError) GetReason() string {
	requested := resource.NewQuantity(e.requested, resource.BinarySI)
	available := resource.NewQuantity(e.available, resource.BinarySI)
	capacity := resource.NewQuantity(e.capacity, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s storage, requested %s, available %s, capacity %s",
		e.resource, requested.String(), available.String(), capacity.String())
}

func (e *InsufficientExclusiveResourceError) Error() string {
	requested := resource.NewQuantity(e.requested, resource.BinarySI)
	available := resource.NewQuantity(e.available, resource.BinarySI)
	capacity := resource.NewQuantity(e.capacity, resource.BinarySI)
	return fmt.Sprintf("Insufficient %s storage, requested %s, available %s, capacity %s",
		e.resource, requested.String(), available.String(), capacity.String())
}
func NewInsufficientExclusiveResourceError(resource pkg.VolumeType, requested, available, capacity int64) *InsufficientExclusiveResourceError {
	return &InsufficientExclusiveResourceError{
		resource:  resource,
		requested: requested,
		available: available,
		capacity:  capacity,
	}
}
