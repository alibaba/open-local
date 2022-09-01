package lib

import (
	"fmt"
	"os/exec"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/utils"
	log "k8s.io/klog/v2"
)

// EnsureFolder ...
func EnsureFolder(target string) error {
	mdkirCmd := "mkdir"
	_, err := exec.LookPath(mdkirCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return fmt.Errorf("%q executable not found in $PATH", mdkirCmd)
		}
		return err
	}

	mkdirFullPath := fmt.Sprintf("%s mkdir -p %s", localtype.NsenterCmd, target)
	_, err = utils.Run(mkdirFullPath)
	if err != nil {
		log.Errorf("Create path error: %v", err)
		return err
	}
	return nil
}
