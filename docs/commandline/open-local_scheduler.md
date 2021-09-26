## open-local scheduler

scheduler is a scheduler extender implementation for local storage

### Synopsis

scheduler provides the capabilities for scheduling cluster local storage as a whole

```
open-local scheduler [flags]
```

### Options

```
      --enabled-node-anti-affinity string   whether enable node anti-affinity for open-local storage backend, example format: 'MountPoint=5,LVM=3'
  -h, --help                                help for scheduler
      --kubeconfig string                   Path to the kubeconfig file to use.
      --master string                       URL/IP for master.
      --port int32                          Port for receiving scheduler callback, set to '0' to disable http server
      --scheduler-strategy string           Scheduler Strategy: binpack or spread (default "binpack")
```

### SEE ALSO

* [open-local](open-local.md)	 - 

