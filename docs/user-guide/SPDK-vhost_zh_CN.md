# SPDK vhost 存储使用手册

  SPDK (Storage Performance Development Kit) 是一组用来编写高性能、高扩展性的用户态存储应用的工具集。 使用 SPDK 可以通过 vhost-user 技术来加速虚拟化环境下存储设备的 I/O 性能。 open-local 支持以 SPDK vhost 作为存储后端为 kata 容器提供高速本地存储服务。

- [SPDK vhost 存储使用手册](#SPDK vhost 存储使用手册)
  - [运行 SPDK](#运行 SPDK)
  - [修改 kata 配置](#修改 kata 配置)
  - [设置 SPDK 作为存储后端](#设置 SPDK 作为存储后端)
  - [注意](#注意)

## 运行 SPDK

  请参考 <https://spdk.io/doc/getting_started.html> 先下载并编译 SPDK。然后执行下面命令运行 spdk_tgt 应用来提供后端存储服务:

```
  # sudo HUGEMEM=4096 PCI_WHITELIST="none" scripts/setup.sh
  # sudo mkdir -p /var/run/kata-containers/vhost-user/block/sockets/
    sudo mkdir -p /var/run/kata-containers/vhost-user/block/devices/
    sudo mkdir -p /run/kata-containers/shared/direct-volumes/
  # sudo <spdk_src_root>/build/bin/spdk_tgt -S /var/run/kata-containers/vhost-user/block/sockets/ &
```

## 修改 kata 配置

  Kata 默认没有开启 vhost-user 存储设备功能。需要修改 Kata 配置开启此功能。Kata 配置文件默认为 /etc/kata-containers/configuration.toml 或 /usr/share/defaults/kata-containers/configuration.toml。在配置文件中找到下面的配置项并修改其值.

```
	enable_hugepages = true
	enable_vhost_user_store = true
```

## 设置 SPDK 作为存储后端

  open-local 默认直接访问本地存储设备。要通过 SPDK 访问存储设备需要:

  `1.` 在 NodeLocalStorageInitConfig 中加入 SPDK 设置。

```
apiVersion: csi.aliyun.com/v1alpha1
kind: NodeLocalStorageInitConfig
metadata:
  name: open-local
spec:
# globalConfig is the default global node configuration
# when the agent creates the NodeLocalStorage resource, the value will be filled in spec of the NodeLocalStorage
  globalConfig:
    spdkConfig:                      # 目前 SPDK 设置仅有两项 (设备类型和 RPC 通信 socket)
      deviceType: block              # SPDK 设备类型
      rpcSocket: /var/tmp/spdk.sock  # SPDK RPC 通信 socket
    # listConfig is the white and black list of storage devices(vgs and mountPoints) and supports regular expressions
    listConfig:
      vgs:
        include:
        - open-local-pool-[0-9]+
    resourceToBeInited:
      vgs:
      - devices:
        - /dev/vdb
        name: open-local-pool-0
      - devices:
        - /work/test.img  # 除了块设备，SPDK 还支持文件作为后端存储
        name: open-local-pool-1
```

  `2.` 在 Pod.spec.volumes.csi.volumeAttributes 或 StorageClass.parameters 中加上参数 spdk 来设置 Volume 是否是 SPDK volume。值为 true 时表明这个 volume 是 SPDK volume, 否则就不是。

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  runtimeClassName: kata
  containers:
  - name: nginx
    image: nginx:1.14.2
    volumeMounts:
    - mountPath: /spdk_s
      name: spdk-lvm
  volumes:
  - name: spdk-lvm
    csi:
      driver: local.csi.aliyun.com
      fsType: ext4
      volumeAttributes:
        vgName: open-local-pool-0
        size: 1Gi
        spdk: "true"   # 指明此volume为SPDK volume
```

## 注意

  SPDK vhost 只支持虚拟化场景，非虚拟化场景下不能使用。非虚拟化场景下请务必将 [设置 SPDK 作为存储后端](#设置 SPDK 作为存储后端) 中所述 SPDK 相关配置去除。
