# NodeLocalStorageInitConfig

`Open-Local` 可通过 NodeLocalStorageInitConfig 资源初始化每个 NodeLocalStorage 资源。目前版本为 `csi.aliyun.com/v1alpha1`。

Agent 在创建 NodeLocalStorage 资源时，会观察环境中是否有 NodeLocalStorageInitConfig。NodeLocalStorageInitConfig 名称由 open-local agent --initconfig 参数指定。若有，则 Agent 会将 NodeLocalStorageInitConfig 中的相关配置填充到新创建的 NodeLocalStorage 资源的 Spec 中。

编辑 NodeLocalStorageInitConfig 对环境中已存在的 NodeLocalStorage 资源无效，即 NodeLocalStorageInitConfig 资源只会在创建 NodeLocalStorage 资源时有效。

下面为 NodeLocalStorageInitConfig 的 Yaml 文件介绍。

```yaml
apiVersion: csi.aliyun.com/v1alpha1
kind: NodeLocalStorageInitConfig
metadata:
  name: [ nlsc 名称 ]
spec:
  globalConfig:     # 全局默认节点配置，在初始化创建 NodeLocalStorage 资源时会填充到其Spec中
    listConfig:
      vgs:
        include:
        - open-local-pool-[0-9]+
      devices:
        include:
        - /dev/vdc
    resourceToBeInited:
      vgs:
      - devices:
        - /dev/vdb3
        name: open-local-pool-0
  nodesConfig:      # 为 node label 满足表达式的特定节点进行初始化配置。该配置会覆盖默认配置globalConfig
  - selector:       # 筛选规则
      matchExpressions:
      - key: node-role.kubernetes.io/master
        operator: In
        values:
        - ""
    listConfig:
      vgs:
        include:
        - share-pool
      devices:
        include:
        - /dev/vdd
  - selector:
      matchLabels:
        kubernetes.io/hostname: cn-hangzhou.10.0.95.236
    resourceInit:
      vg:
      - vgName: share
        devices:
        - /dev/vdc
```