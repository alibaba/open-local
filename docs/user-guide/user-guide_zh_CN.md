# 用户使用手册

- [用户使用手册](#用户使用手册)
  - [部署&安装](#部署安装)
    - [前期准备](#前期准备)
    - [部署](#部署)
  - [存储类型介绍](#存储类型介绍)

## 部署&安装

### 前期准备

- 基于 Red Hat 和 Debian 的 Linux 发行版
- Kubernetes v1.20+
- Helm v3.0+
- [lvm2](https://en.wikipedia.org/wiki/Logical_Volume_Manager_(Linux))
- 集群中至少提供一个空闲块设备用来测试，块设备可以是一整块磁盘，也可以是一个分区。建议一个节点至少一个空闲块设备。

### 部署

假设集群中每个节点上均有一块空闲块设备(e.g. /dev/vdb)，编辑 helm/values.yaml 中的 {{ .Values.agent.device }} 字段，这样 `Open-Local` 将在每个节点上将块设备初始化为名为 open-local-pool-0 的 VolumeGroup。若 /dev/vdb 不存在或已经有文件系统，则 `Open-Local` 不作操作。

通过下面的命令在 k8s 环境中部署`Open-Local`:

```bash
# helm install open-local ./helm
```

确认`Open-Local`是否安装成功:

```bash
# kubectl get po -nkube-system  -l app=open-local

NAME                                             READY   STATUS      RESTARTS   AGE
open-local-agent-6zmkb                           3/3     Running     0          28s
open-local-csi-provisioner-6dbb7c459c-mcp9l      1/1     Running     0          28s
open-local-csi-resizer-57cfd85df7-x44zg          1/1     Running     0          28s
open-local-csi-snapshotter-689b6bbcfc-wwc57      1/1     Running     0          28s
open-local-init-job-2wvbs                        0/1     Completed   0          28s
open-local-init-job-bw8nh                        0/1     Completed   0          28s
open-local-init-job-frdxp                        0/1     Completed   0          28s
open-local-scheduler-extender-7d5cf688b6-pr426   1/1     Running     0          28s
open-local-snapshot-controller-d6f78754-czfnw    1/1     Running     0          28s
```

## 存储类型介绍

`Open-Local`支持两种类型本地磁盘的使用:

- [LVM](./type-lvm_zh_CN.md): **共享盘类型**，即通过 LVM 方式管理存储设备。当在 K8s 中创建 PV 资源时，`Open-Local`会从对应 VolumeGroup 中创建 LogicalVolume 来代表该PV。
- [Device](./type-device_zh_CN.md): **独占盘类型**，即一个 PV 一个块设备。这里块设备可以是一整块磁盘，也可以是一个分区。

不同类型 PV 所支持的存储能力也不同。

| 类型 | 动态分配 | PV扩容 | PV快照 | 原生块设备 | 监控数据 |
|----|----|----|----|----|----|
| [LVM（共享盘类型）](./type-lvm_zh_CN.md) | 支持 | 支持 | 支持 | 支持 | 支持 |
| [Device（独占盘类型）](./type-device_zh_CN.md) | 支持 | 不支持 | 不支持 | 支持 | 支持 |