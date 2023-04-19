# 环境变量

| 变量名                      | 变量作用                      | 变量值举例       | 作用组件      |
|--------------------------|---------------------------|-------------|-----------|
| EXTENDER_SVC_IP          | Extender 的 服务地址           | 10.96.0.4   | csi       |
| EXTENDER_SVC_PORT        | Extender 的 服务端口号          | 23000       | csi       |
| Force_Create_VG          | 是否强制创建 VG（即使在有文件系统的情况下） | "false"（默认） | agent     |
| SNAPSHOT_PREFIX          | 快照前缀                    | snap（默认）    | agent、csi |
| Expand_Snapshot_Interval | 自动扩容lv快照的间隔             | 60（默认，单位秒）  | agent     |
| MNT_Double_Check         | 是否在Mount后双重检查挂载点        | "false"（默认） | csi       |                          |