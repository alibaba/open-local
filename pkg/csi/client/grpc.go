/*
Copyright 2020 The Kubernetes Authors.

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

package client

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/alibaba/open-local/pkg/csi/lib"
	"github.com/alibaba/open-local/pkg/csi/test"
	"google.golang.org/grpc/test/bufconn"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	log "k8s.io/klog/v2"
)

// Connection lvm connection interface
type Connection interface {
	GetLvm(ctx context.Context, volGroup string, volumeID string) (string, error)
	CreateLvm(ctx context.Context, opt *LVMOptions) (string, error)
	DeleteLvm(ctx context.Context, volGroup string, volumeID string) error
	CreateSnapshot(ctx context.Context, volGroup string, snapVolumeID string, volumeID string, size uint64) (string, error)
	DeleteSnapshot(ctx context.Context, volGroup string, snapVolumeID string) error
	ExpandLvm(ctx context.Context, volGroup string, volumeID string, size uint64) error
	CleanPath(ctx context.Context, path string) error
	CleanDevice(ctx context.Context, device string) error
	Close() error
}

// LVMOptions lvm options
type LVMOptions struct {
	VolumeGroup string   `json:"volumeGroup,omitempty"`
	Name        string   `json:"name,omitempty"`
	Size        uint64   `json:"size,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Striping    bool     `json:"striping,omitempty"`
}

type workerConnection struct {
	conn *grpc.ClientConn
}

var (
	_ Connection = &workerConnection{}
)

func MustRunThisWhenTest() {
	const bufSize = 1024 * 1024
	testfunc := func() {
		test.Lis = bufconn.Listen(bufSize)
	}
	test.Once.Do(testfunc)
}

// NewGrpcConnection lvm connection
func NewGrpcConnection(address string, timeout time.Duration) (Connection, error) {
	conn, err := connect(address, timeout)
	if err != nil {
		return nil, err
	}
	return &workerConnection{
		conn: conn,
	}, nil
}

func (c *workerConnection) Close() error {
	return c.conn.Close()
}

func connect(address string, timeout time.Duration) (*grpc.ClientConn, error) {
	log.V(6).Infof("New Connecting to %s", address)
	// only for unit test
	var bufDialerFunc func(context.Context, string) (net.Conn, error)
	dialOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(logGRPC),
	}
	if test.Lis != nil {
		bufDialerFunc = func(context.Context, string) (net.Conn, error) {
			return test.Lis.Dial()
		}
		dialOptions = append(dialOptions, grpc.WithContextDialer(bufDialerFunc))
	}
	// if strings.HasPrefix(address, "/") {
	// 	dialOptions = append(dialOptions, grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
	// 		return net.DialTimeout("unix", addr, timeout)
	// 	}))
	// }
	if strings.HasPrefix(address, "/") {
		dialOptions = append(
			dialOptions,
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				return net.DialTimeout("unix", addr, timeout)
			}))
	}
	conn, err := grpc.Dial(address, dialOptions...)

	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		if !conn.WaitForStateChange(ctx, conn.GetState()) {
			log.Warningf("Connection to %s timed out", address)
			return conn, nil // return nil, subsequent GetPluginInfo will show the real connection error
		}
		if conn.GetState() == connectivity.Ready {
			log.V(6).Infof("Connected to %s", address)
			return conn, nil
		}
		log.V(6).Infof("Still trying to connect %s, connection is %s", address, conn.GetState())
	}
}

func (c *workerConnection) CreateLvm(ctx context.Context, opt *LVMOptions) (string, error) {
	client := lib.NewLVMClient(c.conn)
	req := lib.CreateLVRequest{
		VolumeGroup: opt.VolumeGroup,
		Name:        opt.Name,
		Size:        opt.Size,
		Tags:        opt.Tags,
		Striping:    opt.Striping,
	}

	rsp, err := client.CreateLV(ctx, &req)
	if err != nil {
		log.Errorf("Create Lvm with error: %s", err.Error())
		return "", err
	}
	log.V(6).Infof("Create Lvm with result: %+v", rsp.CommandOutput)
	return rsp.GetCommandOutput(), nil
}

func (c *workerConnection) CreateSnapshot(ctx context.Context, volGroup string, snapVolumeID string, volumeID string, size uint64) (string, error) {
	client := lib.NewLVMClient(c.conn)

	req := lib.CreateSnapshotRequest{
		VolumeGroup: volGroup,
		SnapName:    snapVolumeID,
		LvName:      volumeID,
		Size:        size,
	}
	rsp, err := client.CreateSnapshot(ctx, &req)
	if err != nil {
		log.Errorf("Create Lvm Snapshot with error: %s", err.Error())
		return "", err
	}
	log.V(6).Infof("Create Lvm Snapshot with result: %+v", rsp.CommandOutput)
	return rsp.GetCommandOutput(), nil
}

func (c *workerConnection) GetLvm(ctx context.Context, volGroup string, volumeID string) (string, error) {
	client := lib.NewLVMClient(c.conn)
	req := lib.ListLVRequest{
		VolumeGroup: fmt.Sprintf("%s/%s", volGroup, volumeID),
	}

	rsp, err := client.ListLV(ctx, &req)
	if err != nil {
		log.Errorf("Get Lvm with error: %s", err.Error())
		return "", err
	}
	if len(rsp.GetVolumes()) <= 0 {
		log.Warningf("Volume %s/%s is not exist", volGroup, volumeID)
		return "", nil
	}
	log.V(6).Infof("Get Lvm with result: %+v", rsp.Volumes)
	return rsp.GetVolumes()[0].String(), nil
}

func (c *workerConnection) DeleteLvm(ctx context.Context, volGroup, volumeID string) error {
	client := lib.NewLVMClient(c.conn)
	req := lib.RemoveLVRequest{
		VolumeGroup: volGroup,
		Name:        volumeID,
	}
	response, err := client.RemoveLV(ctx, &req)
	if err != nil {
		log.Errorf("Remove Lvm with error: %v", err.Error())
		return err
	}
	log.V(6).Infof("Remove Lvm with result: %v", response.GetCommandOutput())
	return err
}

func (c *workerConnection) DeleteSnapshot(ctx context.Context, volGroup string, snapVolumeID string) error {
	client := lib.NewLVMClient(c.conn)
	req := lib.RemoveSnapshotRequest{
		VolumeGroup: volGroup,
		SnapName:    snapVolumeID,
	}
	response, err := client.RemoveSnapshot(ctx, &req)
	if err != nil {
		log.Errorf("Remove Lvm Snapshot with error: %v", err.Error())
		return err
	}
	log.V(6).Infof("Remove Lvm Snapshot with result: %v", response.GetCommandOutput())
	return err
}

func (c *workerConnection) CleanPath(ctx context.Context, path string) error {
	client := lib.NewLVMClient(c.conn)
	req := lib.CleanPathRequest{
		Path: path,
	}
	response, err := client.CleanPath(ctx, &req)
	if err != nil {
		log.Errorf("CleanPath with error: %v", err.Error())
		return err
	}
	log.V(6).Infof("CleanPath with result: %v", response.GetCommandOutput())
	return err
}

func (c *workerConnection) CleanDevice(ctx context.Context, device string) error {
	client := lib.NewLVMClient(c.conn)
	req := lib.CleanDeviceRequest{
		Device: device,
	}
	response, err := client.CleanDevice(ctx, &req)
	if err != nil {
		log.Errorf("fail to clean device %s: %s", device, err.Error())
		return err
	}
	log.V(6).Infof("clean device %s successfully with result: %s", device, response.GetCommandOutput())
	return err
}

func (c *workerConnection) ExpandLvm(ctx context.Context, volGroup string, volumeID string, size uint64) error {
	client := lib.NewLVMClient(c.conn)
	req := lib.ExpandLVRequest{
		VolumeGroup: volGroup,
		Name:        volumeID,
		Size:        size,
	}
	response, err := client.ExpandLV(ctx, &req)
	if err != nil {
		log.Errorf("Expand Lvm with error: %v", err.Error())
		return err
	}
	log.V(6).Infof("Expand Lvm with result: %v", response.GetCommandOutput())
	return err
}

func logGRPC(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	log.V(6).Infof("GRPC request: %s, %+v", method, req)
	err := invoker(ctx, method, req, reply, cc, opts...)
	log.V(6).Infof("GRPC response: %+v, %v", reply, err)
	return err
}
