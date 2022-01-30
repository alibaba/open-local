# Storage Class

These parameters can be configured in StorageClass:

| Parameters                  | Values                                 | Default  | Description         |
|-----------------------------|----------------------------------------|----------|---------------------|
| "csi.storage.k8s.io/fstype" | xfs, ext2, ext3, ext4 | ext4 | File system type that will be formatted during volume creation. This parameter is case sensitive! |
| "volumeType" | LVM, MountPoint, Device                | | PV type that will be created by Open-Local. This parameter is case sensitive! |
| "mediaType" | hdd,ssd |      | Media type that will be used when allocate Device for PV. The param only works when volumeType is MountPoint or Device. |
| "vgName" | | | The volume group name that the open-local will use to create the logical volume. This name must be contained in vg list, which can be found in .status.filteredStorageInfo in every [nls](../api/nls_zh_CN.md). If no value is set, open-local will choose a vg from vg list by itself. |
| "iops" | | | I/O operations per second. |
| "bps" | | | Throughput in KiB/s. |