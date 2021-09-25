## open-local agent

command for collecting local storage information

```
open-local agent [flags]
```

### Options

```
      --config string       Path to the open-local config file to use. (default "/etc/controller/config/")
  -h, --help                help for agent
      --initconfig string   initconfig is NodeLocalStorageInitConfig(CRD) for agent to create NodeLocalStorage (default "open-local")
      --interval int        The interval that the agent checks the local storage at one time (default 60)
      --kubeconfig string   Path to the kubeconfig file to use.
      --lvname string       The prefix of Logical Volume Name created by open-local (default "local")
      --master string       URL/IP for master.
      --nodename string     Kubernetes node name.
      --path.mount string   Path that specifies mount path of local volumes (default "/mnt/open-local")
      --path.sysfs string   Path of sysfs mountpoint (default "/sys")
      --regexp string       regexp is used to filter device names (default "^(s|v|xv)d[a-z]+$")
```

### SEE ALSO

* [open-local](open-local.md)	 - 

