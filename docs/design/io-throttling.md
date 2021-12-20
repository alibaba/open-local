# IO限流方案

期望达到的状态

设置pvc的IOPS、BPS等参数，在annotation中。即申请PV时，一个块设备最大的IOPS、BPS已经确定下了。

创建pv时如何获取这个值。。

需要考虑是否可以动态调整这个值。

创建pv时，需要设置这些内容在.spec.csi中
创建pv时，需设置maj:min字段在.spec.csi中（看是否有必要）

当pod挂载pv时，在对应的pod cgroup中设置参数（task？）

删除pv后

需要考虑的点：
- 需要把blkio所有参数调研一遍
- 块设备的设备放在最高一层是否合适
- 具体要为每个pod设置哪些参数，跟最上级的关系是什么