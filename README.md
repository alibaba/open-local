# OpenLSS - 云原生本地磁盘管理系统

OpenLSS(Open Local Storage Service)是由多个组件构成的**本地磁盘管理系统**，目标是解决当前Kubernetes本地存储能力缺失问题。通过OpenLSS，**使用本地存储会像集中式存储一样简单**。

## 支持特性
- 本地存储池管理
- 存储卷动态分配
- 存储卷容量隔离
- 存储卷扩容
- 存储卷快照
- 存储卷监控

## 开发指南
```bash
mkdir -p $GOPATH/src/github.com/oecp/
cd $GOPATH/src/github.com/oecp/
git clone https://github.com/oecp/open-local-storage-service.git
# build binary
make build
# build image
make image
```