# Open-Local - 云原生本地磁盘管理系统

[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/open-local)](https://goreportcard.com/report/github.com/alibaba/open-local)
![workflow build](https://github.com/alibaba/open-local/actions/workflows/build.yml/badge.svg)
[![codecov](https://codecov.io/gh/alibaba/open-local/branch/main/graphs/badge.svg)](https://codecov.io/gh/alibaba/open-local)

[English](./README.md) | 简体中文

`Open-Local`是由多个组件构成的**本地磁盘管理系统**，目标是解决当前 Kubernetes 本地存储能力缺失问题。通过`Open-Local`，**使用本地存储会像集中式存储一样简单**。

`Open-Local`已广泛用于生产环境，目前使用的产品包括：

- [ACK 发行版](https://github.com/AliyunContainerService/ackdistro)
- 阿里云 ADP (云原生应用交付平台)
- [云原生 CNStack 产品](https://github.com/alibaba/CNStackCommunityEdition)
- 蚂蚁金融分布式架构 SOFAStack

## 特性

- 本地存储池管理
- 存储卷动态分配
- 存储调度算法扩展
- 存储卷扩容
- 存储卷快照
- 存储卷监控
- 原生块设备
- IO 限流（仅支持direct-io）
- 临时卷

## Open-Local版本能力矩阵

| 特性                             | Open-Local版本 | K8S版本 |
| ---------------------------------- | -------------- | --------- |
| 节点磁盘池化(Node Disk pooling) | v0.1.0+        | 1.18-1.20 |
| 卷动态供应(Dynamic Provisioning) | v0.1.0+        | 1.20-1.22 |
| 卷原地扩容(Volume Expansion)  | v0.1.0+        | 1.20-1.22 |
| 卷快照(Snapshot)                | v0.1.0+        | 1.20-1.22 |
| LVM/块设备/本地路径         | v0.1.0+        | 1.18-1.20 |
| 块设备挂载(volumeMode: Block) | v0.3.0+        | 1.20-1.22 |
| IO限流(IO-Throttling)            | v0.4.0+        | 1.20-1.22 |
| 本地临时盘(CSI ephemeral volumes) | v0.5.0+        | 1.20-1.22 |
| IPv6支持                         | v0.5.3+        | 1.20-1.22 |
| SPDK设备支持                   | v0.6.0+        | 1.20-1.22 |
| 读写快照（read-write snapshot) | v0.7.0+        | 1.20-1.22 |

## 架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Master                                                                      │
│                   ┌───┬───┐           ┌────────────────┐                    │
│                   │Pod│PVC│           │   API-Server   │                    │
│                   └───┴┬──┘           └────────────────┘                    │
│                        │ bound                ▲                             │
│                        ▼                      │ watch                       │
│                      ┌────┐           ┌───────┴────────┐                    │
│                      │ PV │           │ Kube-Scheduler │                    │
│                      └────┘         ┌─┴────────────────┴─┐                  │
│                        ▲            │     open-local     │                  │
│                        │            │ scheduler-extender │                  │
│                        │      ┌────►└────────────────────┘◄───┐             │
│ ┌──────────────────┐   │      │               ▲               │             │
│ │ NodeLocalStorage │   │create│               │               │  callback   │
│ │    InitConfig    │  ┌┴──────┴─────┐  ┌──────┴───────┐  ┌────┴────────┐    │
│ └──────────────────┘  │  External   │  │   External   │  │  External   │    │
│          ▲            │ Provisioner │  │   Resizer    │  │ Snapshotter │    │
│          │ watch      ├─────────────┤  ├──────────────┤  ├─────────────┤    │
│    ┌─────┴──────┐     ├─────────────┴──┴──────────────┴──┴─────────────┤GRPC│
│    │ open-local │     │                 open-local                     │    │
│    │ controller │     │             CSI ControllerServer               │    │
│    └─────┬──────┘     └────────────────────────────────────────────────┘    │
│          │ create                                                           │
└──────────┼──────────────────────────────────────────────────────────────────┘
           │
┌──────────┼──────────────────────────────────────────────────────────────────┐
│ Worker   │                                                                  │
│          │                                                                  │
│          ▼                ┌───────────┐                                     │
│ ┌──────────────────┐      │  Kubelet  │                                     │
│ │ NodeLocalStorage │      └─────┬─────┘                                     │
│ └──────────────────┘            │ GRPC                     Shared Disks     │
│          ▲                      ▼                          ┌───┐  ┌───┐     │
│          │              ┌────────────────┐                 │sdb│  │sdc│     │
│          │              │   open-local   │ create volume   └───┘  └───┘     │
│          │              │ CSI NodeServer ├───────────────► VolumeGroup      │
│          │              └────────────────┘                                  │
│          │                                                                  │
│          │                                                 Exclusive Disks  │
│          │                ┌─────────────┐                  ┌───┐            │
│          │ update         │ open-local  │  init device     │sdd│            │
│          └────────────────┤    agent    ├────────────────► └───┘            │
│                           └─────────────┘                  Block Device     │
│                                                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

`Open-Local`包含四大类组件：

- Scheduler-Extender: 作为 Kubernetes Scheduler 的扩展组件，通过 Extender 方式实现，新增本地存储调度算法
- CSI: 按照 [CSI(Container Storage Interface)](https://kubernetes.io/blog/2019/01/15/container-storage-interface-ga/) 标准实现本地磁盘管理能力
- Agent: 运行在集群中的每个节点，根据配置清单初始化存储设备，并通过上报集群中本地存储设备信息以供 Scheduler-Extender 决策调度
- Controller: 获取集群存储初始化配置，并向运行在各个节点的 Agent 下发详细的配置清单

若集群中部署有 Prometheus 和 Grafana，部署 `Open-Local` 时也可选择性安装监控大盘:

![](docs/imgs/open-local-dashboard.png)

## 用户手册

详见[文档](docs/user-guide/user-guide_zh_CN.md)

## 许可证

[Apache 2.0 License](LICENSE)