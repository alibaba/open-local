/*
Copyright 2022 Intel Corporation

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
package spdk

import (
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/alibaba/open-local/pkg/utils"
	log "k8s.io/klog/v2"
)

const (
	iqnPrefix   = "iqn.2022-06.csi.local:"
	defaultPort = "3260"
)

// iscsi_create_initiator_group
func (client *SpdkClient) iscsiCreateInitiatorGroup(tag uint, initiators, netmasks []string) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	params := struct {
		Tag        uint     `json:"tag"`
		Initiators []string `json:"initiators"`
		Netmasks   []string `json:"netmasks"`
	}{
		Tag:        tag,
		Initiators: initiators,
		Netmasks:   netmasks,
	}

	if err := conn.Call("iscsi_create_initiator_group", &params, nil); err != nil {
		log.Error("iscsiCreateInitiatorGroup failed: ", err.Error())
		return err
	}

	return nil
}

// iscsi_delete_initiator_group
func (client *SpdkClient) iscsiDeleteInitiatorGroup(tag uint) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	params := struct {
		Tag uint `json:"tag"`
	}{
		Tag: tag,
	}

	if err := conn.Call("iscsi_delete_initiator_group", &params, nil); err != nil {
		log.Errorf("iscsiDeleteInitiatorGroup (%d) failed: %s", tag, err.Error())
		return err
	}

	return nil
}

// iscsi_create_portal_group
func (client *SpdkClient) iscsiCreatePortalGroup(tag uint, host, port string) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	type Portal struct {
		Host string `json:"host"`
		Port string `json:"port"`
	}
	params := struct {
		Tag     uint     `json:"tag"`
		Portals []Portal `json:"portals"`
	}{
		Tag:     tag,
		Portals: []Portal{{host, port}},
	}

	if err := conn.Call("iscsi_create_portal_group", &params, nil); err != nil {
		log.Error("iscsiCreatePortalGroup failed: ", err.Error())
		return err
	}

	return nil
}

// iscsi_delete_portal_group
func (client *SpdkClient) iscsiDeletePortalGroup(tag uint) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	params := struct {
		Tag uint `json:"tag"`
	}{
		Tag: tag,
	}

	if err := conn.Call("iscsi_delete_portal_group", &params, nil); err != nil {
		log.Errorf("iscsiDeletePortalGroup (%d) failed: %s", tag, err.Error())
	}

	return err
}

// iscsi_create_target_node
func (client *SpdkClient) iscsiCreateTargetNode(name, bdevName string, igTag, pgTag int) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	type Lun struct {
		BdevName string `json:"bdev_name"`
		ID       int    `json:"lun_id"`
	}
	type PgIgMap struct {
		IgTag int `json:"ig_tag"`
		PgTag int `json:"pg_tag"`
	}
	params := struct {
		Name        string    `json:"name"`
		AliasName   string    `json:"alias_name"`
		PgIgMaps    []PgIgMap `json:"pg_ig_maps"`
		Luns        []Lun     `json:"luns"`
		QueueDepth  int       `json:"queue_depth"`
		DisableChap bool      `json:"disable_chap"`
	}{
		Name:        name,
		AliasName:   name + "_alias",
		PgIgMaps:    []PgIgMap{{igTag, pgTag}},
		Luns:        []Lun{{bdevName, 0}},
		QueueDepth:  64,
		DisableChap: true,
	}

	if err := conn.Call("iscsi_create_target_node", &params, nil); err != nil {
		log.Errorf("iscsiCreateTargetNode (%s %s %d %d) failed: %s", name, bdevName, igTag, pgTag, err.Error())
		return err
	}

	return nil
}

// iscsi_delete_target_node
func (client *SpdkClient) iscsiDeleteTargetNode(name string) error {
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	params := struct {
		Name string `json:"name"`
	}{
		Name: name,
	}

	if err := conn.Call("iscsi_delete_target_node", &params, nil); err != nil {
		log.Errorf("iscsiDeleteTargetNode (%s) failed: %s", name, err.Error())
		return err
	}

	return nil
}

func (client *SpdkClient) MakeFs(bdevName, fsType string) bool {
	result := false
	stage := 0
	var err error
	var localIp, portal, target, dev string

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Error("MakeFs - fail to get local addresses")
		return false
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					localIp = ipnet.IP.String()
					break
				}
			}
		}
	}

	if err := client.iscsiCreateInitiatorGroup(1, []string{"ANY"}, []string{"127.0.0.0/24", "10.0.0.0/8"}); err != nil {
		log.Error("MakeFs - iscsiCreateInitiatorGroup failed")
		goto clean
	}
	stage++

	if err := client.iscsiCreatePortalGroup(1, localIp, defaultPort); err != nil {
		log.Error("MakeFs - iscsiCreatePortalGroup failed")
		goto clean
	}
	stage++

	target = iqnPrefix + "target0"
	if err := client.iscsiCreateTargetNode(target, bdevName, 1, 1); err != nil {
		log.Error("MakeFs - iscsiCreateTargetNode failed")
		goto clean
	}
	stage++

	portal = fmt.Sprintf("%s:%s", localIp, defaultPort)
	dev, err = connect(portal, target)
	if err != nil {
		log.Error("MakeFs - connect failed")
		goto clean
	}
	stage++

	if err := utils.FormatBlockDevice(dev, fsType); err != nil {
		log.Error("FormatBlockDevice failed: ", err.Error())
	} else {
		result = true
	}

clean:
	switch stage {
	case 4:
		if err := disconnect(portal, target); err != nil {
			log.Error("MakeFs - disconnect failed", err.Error())
		}
		fallthrough
	case 3:
		if err := client.iscsiDeleteTargetNode(target); err != nil {
			log.Error("MakeFs - iscsiDeleteTargetNode failed", err.Error())
		}
		fallthrough
	case 2:
		if err := client.iscsiDeletePortalGroup(1); err != nil {
			log.Error("MakeFs - iscsiDeletePortalGroup failed", err.Error())
		}
		fallthrough
	case 1:
		if err := client.iscsiDeleteInitiatorGroup(1); err != nil {
			log.Error("MakeFs - iscsiDeleteInitiatorGroup failed", err.Error())
		}
	}

	return result
}

func connect(portal, target string) (string, error) {
	// Discovery
	cmd := "iscsiadm -m discovery -t sendtargets -p " + portal
	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		log.Errorf("run cmd (%s) failed (%s): %s\n", cmd, string(out), err.Error())
		return "", fmt.Errorf("Failed to run cmd: %s; output: %s; error: %s", cmd, string(out), err.Error())
	}

	// Connect to target
	cmd = fmt.Sprintf("iscsiadm -m node -T %s -p %s --login", target, portal)
	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		log.Errorf("run cmd (%s) failed (%s): %s\n", cmd, string(out), err.Error())
		return "", fmt.Errorf("Failed to run cmd: %s; output: %s; error: %s", cmd, string(out), err.Error())
	}

	// Wait and find the device path
	pattern := fmt.Sprintf("/dev/disk/by-path/*%s*", target)
	for i := 0; i <= 10; i++ {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return "", err
		}

		if len(matches) >= 1 {
			return matches[0], nil
		}

		time.Sleep(time.Second)
	}

	return "", fmt.Errorf("no device appears in 10s: %s", pattern)
}

func disconnect(portal, target string) error {
	cmd := fmt.Sprintf("iscsiadm -m node -T %s -p %s --logout", target, portal)
	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		log.Errorf("run cmd (%s) failed (%s): %s\n", cmd, string(out), err.Error())
		return fmt.Errorf("Failed to run cmd: %s; output: %s; error: %s", cmd, string(out), err.Error())
	}

	return nil
}
