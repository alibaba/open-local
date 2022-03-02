# Open-Local

[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/open-local)](https://goreportcard.com/report/github.com/alibaba/open-local)
![workflow build](https://github.com/alibaba/open-local/actions/workflows/build.yml/badge.svg)

English | [简体中文](./README_zh_CN.md)

`Open-Local` is a **local disk management system** composed of multiple components. With `Open-Local`, **using local storage in Kubernetes will be as simple as centralized storage**.

## Features

- Local storage pool management
- Dynamic volume provisioning
- Extended scheduler
- Volume expansion
- Volume snapshot
- Volume metrics
- Raw block volume
- IO Throttling
- Ephemeral inline volume

## Overall Architecture

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

`Open-Local`contains four types of components:

- Scheduler extender: as an extended component of Kubernetes Scheduler, adding local storage scheduling algorithm
- CSI plugins: providing the ability to create/delete volume, expand volume and take snapshots of the volume
- Agent: running on each node in the K8s cluster, initializing the storage device according to the configuration list, and reporting local storage device information for Scheduler extender
- Controller: getting the cluster initial configuration of the storage and deliver a detailed configuration list to Agents running on each node

## Who uses Open-Local

`Open-Local` has been widely used in production environments, and currently used products include:

- [ACK Distro](https://github.com/AliyunContainerService/ackdistro)
- Alibaba Cloud ECP (Enterprise Container Platform)
- Alibaba Cloud ADP (Cloud-Native Application Delivery Platform)
- [CNStack Products](https://github.com/alibaba/CNStackCommunityEdition)
- AntStack Plus Products

## User guide

More details [here](docs/user-guide/user-guide.md)

## Contact

Join us from DingTalk: Group No.34118035

## License

[Apache 2.0 License](LICENSE)