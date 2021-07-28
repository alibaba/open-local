package agent

type agentOption struct {
	Master       string
	Kubeconfig   string
	NodeName     string
	SysPath      string
	MountPath    string
	Interval     int
	LVNamePrefix string
	Config       string
	RegExp       string
	InitConfig   string
}
