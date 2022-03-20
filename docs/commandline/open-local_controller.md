## open-local controller

command for starting a controller

```
open-local controller [flags]
```

### Options

```
      --feature-gates mapStringBool   A set of key=value pairs that describe feature gates for alpha/experimental features. Options are:
                                      AllAlpha=true|false (ALPHA - default=false)
                                      AllBeta=true|false (BETA - default=false)
                                      OrphanedSnapshotContent=true|false (ALPHA - default=true)
                                      UpdateNLS=true|false (ALPHA - default=true)
  -h, --help                          help for controller
      --initconfig string             initconfig is NodeLocalStorageInitConfig(CRD) for controller to create NodeLocalStorage (default "open-local")
      --kubeconfig string             Path to the kubeconfig file to use.
      --master string                 URL/IP for master.
```

### SEE ALSO

* [open-local](open-local.md)	 - 

