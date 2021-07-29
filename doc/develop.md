# 开发指南

## 本地测试
> 开发机为Mac，使用minikube

- 本地需装有[minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/)，驱动选择virtualbox。

### 添加磁盘
- 默认启动的minikube只有一块磁盘（/dev/sda），添加磁盘步骤如下
  - 切换minikube目录：cd ~/.minikube/machines/minikube/minikube
  - 创建磁盘文件：VBoxManage createhd --filename mydisk00.vdi --size 40960
  - 查看device、port信息：cat minikube.vbox|grep HardDisk
  - 磁盘attach，此处device为上述信息中的device序号，port则在原基础上加1：VBoxManage storageattach minikube --storagectl "SATA" --port 2 --device 0 --type hdd --medium mydisk00.vdi
  - 登陆进minikube虚拟机：minikube ssh
  - 查看磁盘是否挂载成功：lsblk