# 控制器方案

> 暂不升级API，与当前API兼容。之后专门出版本，将API从v1alpha1升级到v1alpha2

## 目标

通过 nlsc 更新 nls 资源，即 nls 资源仅被 controller 控制。这样 agent 组件功能就很简单，仅做上报。

nlsc 控制 nls 实时更新的能力可 disable。

controller 还可以作为 server 对外暴露 restful api

## 代码实现

全局可能有多个 nlsc，controller 需指定 nlsc 名称，之后 controller 根据该 nlsc 对集群进行操作：

- controller 启动后，首先创建 nls 资源。集群有多少个节点，controller 就创建多少个 nls 资源，资源名称同nodename
- controller 监听 nls 资源，若用户没有 disable nls实时更新能力，则 controller 会使 nls 资源始终保持与 nlsc 一致（只编辑nls的spec字段）
- nlsc资源中优先级体现
  - 先按照前后顺序覆盖
- agent 根据节点上的资源信息，更新 status 字段
  - extender不再更新nls，由agent来更新status字段中的filteredStorageInfo部分
  - extender仅watch nls资源，并更新cache
- 事件监听逻辑
  - 监听node
    - add：createNLS
    - update：不做操作
    - delete：deleteNLS
  - 监听nlsc
    - add：createNLS
    - update：updateNLS
    - delete：不做操作
  - 监听nls
    - add：是否执行 updateNLS
    - update：是否执行 updateNLS
    - delete：createNLS