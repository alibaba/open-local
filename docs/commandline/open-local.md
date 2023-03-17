## open-local



### Options

```
      --add-dir-header                   If true, adds the file directory to the header of the log messages
      --alsologtostderr                  log to standard error as well as files
  -h, --help                             help for open-local
      --log-backtrace-at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log-dir string                   If non-empty, write log files in this directory
      --log-file string                  If non-empty, use this log file
      --log-file-max-size uint           Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800)
      --log-flush-frequency duration     Maximum number of seconds between log flushes (default 5s)
      --logtostderr                      log to standard error instead of files (default true)
      --one-output                       If true, only write logs to their native severity level (vs also writing to each lower severity level)
      --skip-headers                     If true, avoid header prefixes in the log messages
      --skip-log-headers                 If true, avoid headers when opening log files
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          number for the log level verbosity
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### SEE ALSO

* [open-local agent](open-local_agent.md)	 - command for collecting local storage information
* [open-local completion](open-local_completion.md)	 - Generate the autocompletion script for the specified shell
* [open-local controller](open-local_controller.md)	 - command for starting a controller
* [open-local csi](open-local_csi.md)	 - command for running csi plugin
* [open-local gen-doc](open-local_gen-doc.md)	 - generate document for Open-Local CLI with MarkDown format
* [open-local scheduler](open-local_scheduler.md)	 - scheduler is a scheduler extender implementation for local storage
* [open-local version](open-local_version.md)	 - Print the version of open-local

