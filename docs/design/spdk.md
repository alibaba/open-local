# SPDK

## Motivation

Virtio-fs 和 virtio-blk 是虚拟化环境下常见两种 IO 方式. 在高速 IO 场景下，这两种方式性能常常不能令人满意。 SPDK(Storage Performance Development Kit) 是一组用来编写高性能、高扩展性的用户态存储应用的工具集。 使用 SPDK 可以通过 vhost-user 技术来加速虚拟化环境下存储设备的 I/O 性能。通过在 open-local 中引入 SPDK vhost 支持为 kata 容器提供高速本地存储服务。

## Proposal

### 流程简述

SPDK 提供了一个存储设备抽象层bdev, 本地存储设备可以很容易映射到 bdev。同时 SPDK 还提供了和 LVM 类似的逻辑分区管理功能。通过 SPDK 创建一个可供 VM 使用的 vhost-user 存储设备的流程如下:
  (可以通过 RPC 调用 SPDK 应用的接口。RPC 可以通过 socket 发送消息的方式, 也可以直接通过 SPDK 提供的脚本 scripts/rpc.py。下面流程用的都是脚本命令)

  1. 创建bdev设备

```
     # scripts/rpc.py bdev_aio_create <path_to_host_block_dev> <bdev_name>
```

  2. 创建逻辑分区

```
     # scripts/rpc.py bdev_lvol_create_lvstore <bdev_name> <lvs_name>
     # scripts/rpc.py bdev_lvol_create  <lvol_name> <size> -l <lvs_name>
```

   如果不需要创建逻辑分区这一步可以跳过。这里 lvstore 相当于 LVM 中的 volume group. lvol 相当于 LVM 中的 logical volume。需要注意的是 SPDK 里没有类似 LVM 里 physical volume 的概念，所以 lvstore 和 bdev 是一对一的，不像 LVM 的 volume group 可以包含多个 physical volume。

  3. 创建vhost设备

```
    # scripts/rpc.py vhost_create_blk_controller --cpumask <cpumask> <controller_name> <bdev_name>
    # mknod /var/run/kata-containers/vhost-user/block/devices/<devname> b <major> <minor>
```

  4. 挂载设备

```
    # mkfs.<fstype> xxx
    # mount --bind [...] /var/run/kata-containers/vhost-user/block/devices/<devname> <target_path>
```

  这一步可以在guest里完成, 也可以通过 kata direct volume 机制来完成。后面 node CSI driver 部分会详细介绍。

### SPDK configuration

在 NodeLocalStorage / NodeLocalStorageInitConfig 中增加 SPDK 相关的配置。

```yaml
  spdkConfig:
    description: SpdkConfig defines SPDK configuration.
    properties:
      deviceType:
        description: DeviceType is SPDK block device type.
        maxLength: 8
        minLength: 1
        type: string
      rpcSocket:
        description: RpcSocket is the unix domain socket for RPC
        maxLength: 128
        minLength: 1
        type: string
```

我们是通过 spdkConfig 来判断一个 node 是否支持 SPDK。如果 node 上没有运行 SPDK vhost 应用不能设置 spdkConfig。 需要注意的是 SPDK vhost 只能为 VM 提供存储服务，SPDK 会独占 host 上的存储设备。一个 node 上可能有多个存储设备，当前方案暂不支持将部分设备作为 SPDK 设备另一部分作为正常 open-local 本地设备混合使用。也就是说不支持将同一个 node 上部分存储设备以 vhost-user-blk 提供给 VM 使用，另一部分设备以其他方式提供给 容器。

注意: SPDK 会覆盖存储设备上原有数据，切勿将系统磁盘和其他存有数据的存储设备加入到 NodeLocalStorage / NodeLocalStorageInitConfig 的 devices 列表中，这样可能导致数据被误损坏。

### scheduler
通过在 CSI driver parameters (也就是 Pod.spec.volumes.csi.volumeAttributes 或 StorageClass.parameters) 新加一个参数 spdk 来设置一个 Volume 是否是 SPDK volume。值为 true 时表明这个 volume 是 SPDK volume, 否则就不是。

为确保使用 SPDK volume 的 pod 被调度到支持 SPDK 的节点，使用非 SPDK volume 的 pod 不被调度到 SPDK 节点，需要对 open-local scheduler extender 作如下修改：

```
  . 在 NodeCache 中增加一项记录 node 是否支持 SPDK (可以根据 NodeLocalStorage 中是否有 spdkConfig 来判断)。
  . 在 scheduler extender 中增加一个新的 PredicateFunc. 如果 pod 需要 SPDK volume，这个 PredicateFunc 会根据 NodeCache 过滤掉不支持 SPDK 的 node; 反之，pod 中有非 SPDK volume，则过滤掉 SPDK 节点。如果 pod 没有 volume， 则不作任何操作。
```

### lvmd server 部分

```
  . pkg/csi/server/commands.go 重命名为 pkg/csi/server/lvm_commands.go; 新加 spdk_commands.go 实现SPDK版本的 LVMServer interface。
  . 在 pkg/csi/server/routers.go Server struct 中增加一个新成员变量 "impl Command"。NewServer 函数增加一个参数用来传入使用的 Command impl。
  . pkg/csi/server/server.go 在 Start 函数中增加 node 是否支持 SPDK 的判断，在调用 NewServer 时指定使用 lvm_command 还是 spdk_command。
```

### Agent 部分
  主要需要增加 SPDK 存储设备的初始化和发现功能。SPDK bdev，lvstore 及 lvol 分别和 open-local 的 device，LVM 的 volume group, logical volume 相似。故实现发现功能时可以直接将 SPDK 中相关信息转换为 nls status 相关数据，这样可以复用 open-local 现有的数据结构。初始化时会先获取 SPDK 中已有的 bdev 和 lvstore 然后和 nls 比较。然后根据比较的结果创建尚未创建的设备和 lvstore。这里会同时考虑 listConfig 和 resourceToBeInited 中的 devices 和 VGs。

### SPDK CSI node driver

```
  .  NodeStageVolume / NodeUnStageVolume
    n/a
  .  NodePublishVolume
    - Create bdev
      # scripts/rpc.py bdev_aio_create <path_to_host_block_dev> <bdev_name>
      # scripts/rpc.py bdev_lvol_create_lvstore <bdev_name> <lvs_name >
      # scripts/rpc.py bdev_lvol_create  <lvol_name> <size> -l <lvs_name>
    -  Create vhost device
      # scripts/rpc.py vhost_create_blk_controller --cpumask <cpumask> <controller_name> <bdev_name>
      # mknod /var/run/kata-containers/vhost-user/block/devices/<dev_name> b <major> <minor>
  .  NodeUnPublishVolume
      # umount <target_path>
      # scripts/rpc.py bdev_lvol_delete <lvol_name>
      # rm /var/run/kata-containers/vhost-user/block/devices/<dev_name>
```

#### 设备挂载/卸载 (direct volume)
  Direct volume 是 Kata Containers 2.4 一个新功能。可以将挂载操作移动到 Guest 中由 kata-agent 来完成存储卷的挂载。要支持 Direct volume，node csi driver 在不同阶段需要完成如下操作:

```
  - NodeStageVolume
    # n/a
  - NodePublishVolume
    # kata-runtime direct-volume add --volume-path [volumePath] --mount-info [mountInfo]
  - NodeExpandVolume
    # kata-runtime direct-volume resize--volume-path [volumePath] --size [size]
  - NodeUnStageVolume
    # kata-runtime direct-volume remove --volume-path [volumePath]
```



