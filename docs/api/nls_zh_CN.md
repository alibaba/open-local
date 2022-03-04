# NodeLocalStorage

`Open-Local` 通过 NodeLocalStorage 资源上报每个节点上的存储设备信息，该资源由 Controller 创建，由每个节点的 Agent 组件更新其 status。该 CRD 属于全局范围的资源。目前版本为 `csi.aliyun.com/v1alpha1`。

用户可根据需要编辑 NodeLocalStorage 资源的 Spec 字段。

下面为 NodeLocalStorage 的 Yaml 文件介绍。

```yaml
apiVersion: csi.aliyun.com/v1alpha1
kind: NodeLocalStorage
spec:
  nodeName: [node name]       # 与节点名称相同
  listConfig:                 # 可被 Open-Local 分配的存储设备列表，形式为正则表达式。Status 中 .nodeStorageInfo.deviceInfo 和 .nodeStorageInfo.volumeGroups 满足该正则表达式的设备名称将成为 Open-Local 具体可分配的存储设备列表，并在 Status 的 .filteredStorageInfo 中显示。Open-Local 先通过 include 得出全量列表，然后通过 exclude 从全量列表中剔除不需要的项
    devices:                  # Device（独占盘）名单
      include:                # include 正则
      - /dev/vd[a-d]+
      exclude:                # exclude 正则
      - /dev/vda
      - /dev/vdb
    vgs:                      # LVM（共享盘）白黑名单，这里的共享盘名称指的是 VolumeGroup 名称
      include:
      - share
      - paas[0-9]*
      - open-local-pool-[0-9]+
  resourceToBeInited:         # 设备初始化列表
    vgs:                      # LVM（共享盘）初始化
    - devices:                # 将块设备 /dev/vdb3 初始化为名为 open-local-pool-0 的 VolumeGroup。注意：当节点上包含同名 VG，则 Open-Local 不做操作
      - /dev/vdb3
      name: open-local-pool-0
status:
  nodeStorageInfo:            # 具体设备情况，由 Agent 组件更新。包含 分区 和 一整个块设备。设备名称可由 open-local agent --regexp 参数决定（默认为 ^(s|v|xv)d[a-z]+$ ）
    deviceInfo:               # 磁盘情况
    - condition: DiskReady    # 磁盘状态，有三种状态：DiskReady、DiskFull、DiskFault
      mediaType: hdd          # 媒介类型，分为 hdd 和 sdd 两种
      name: /dev/vda1         # 设备名称
      readOnly: false         # 是否只读
      total: 53685353984      # 设备总量
    - condition: DiskReady
      mediaType: hdd
      name: /dev/vda
      readOnly: false
      total: 53687091200
    - condition: DiskReady
      mediaType: hdd
      name: /dev/vdb1
      readOnly: false
      total: 107374164992
    - condition: DiskReady
      mediaType: hdd
      name: /dev/vdb2
      readOnly: false
      total: 106300440576
    - condition: DiskReady
      mediaType: hdd
      name: /dev/vdb3
      readOnly: false
      total: 860066152448
    - condition: DiskReady
      mediaType: hdd
      name: /dev/vdb
      readOnly: false
      total: 1073741824000
    - condition: DiskReady
      mediaType: hdd
      name: /dev/vdc
      readOnly: false
      total: 1073741824000
    volumeGroups:                 # VolumeGroup 情况
    - allocatable: 860063006720   # 可被 Open-Local 分配的VG可用量，会剔除非 Open-Local 的 LV 总量。Open-Local 的 LV 名称由 open-local agent --lvname 参数决定，前缀不匹配的 LV 为非 Open-Local 的 LV。
      available: 800298369024     # VG 可用量
      condition: DiskReady        # VG 状态
      logicalVolumes:                                       # LV 信息
      - condition: DiskReady                                # LV 状态
        name: local-482c664d-764b-461e-be5e-0a60a3abd5ac    # LV 名称
        total: 1073741824                                   # LV 总量
        vgname: open-local-pool-0                           # LV 所在的 VG 名称
      - condition: DiskReady
        name: local-4e0b8d6f-8ea2-431d-8b5d-aaec1b5d4242
        total: 53687091200
        vgname: open-local-pool-0
      - condition: DiskReady
        name: local-cc69d090-15b9-4abd-af1f-04380e1654d9
        total: 5003804672
        vgname: open-local-pool-0
      name: open-local-pool-0     # VG 名称
      physicalVolumes:            # VG 对应的 PVs（Physical Volumes）
      - /dev/vdb3
      total: 860063006720         # VG 总量
  filteredStorageInfo:            # 设备筛选情况，筛选后的设备会参与存储调度&分配。该字段的值由 Status 中的 .nodeStorageInfo.deviceInfo 和 .nodeStorageInfo.volumeGroups 与 Spec 中的 .listConfig 共同决定。本例中 Spec 的 VG 列表中有 open-local-pool-[0-9]+，且该节点有名为 open-local-pool-0 的 VG，故可被纳管。/dev/vdc 同理。
    volumeGroups:
    - open-local-pool-0
    devices:
    - /dev/vdc
```