/*
Copyright © 2021 Alibaba Group Holding Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package om

import (
	"os"
	"strconv"
	"strings"
	"time"

	log "k8s.io/klog/v2"
)

const (
	// SysLog log file
	SysLog = "/var/log/messages"
	// IssueMessageFile tag
	IssueMessageFile = "ISSUE_MESSAGE_FILE"
	// IssueBlockReference tag
	IssueBlockReference = "ISSUE_BLOCK_REFERENCE"
	// IssueOrphanedPod tag
	IssueOrphanedPod = "ISSUE_ORPHANED_POD"
	// MessageFileLines tag
	MessageFileLines = "MESSAGE_FILE_LINES"
)

var (
	// GlobalConfigVar var
	GlobalConfigVar GlobalConfig
)

// GlobalConfig save global values for om
type GlobalConfig struct {
	IssueMessageFile     bool
	MessageFileTailLines int
	IssueBlockReference  bool
	IssueOrphanedPod     bool
}

// StorageOM storage Operation and Maintenance
func StorageOM() {
	GlobalConfigSet()

	for {
		// fix block volume reference not removed issue;
		// error message: The device %q is still referenced from other Pods;
		if GlobalConfigVar.IssueBlockReference {
			CheckMessageFileIssue()
		}

		// loop interval time
		time.Sleep(time.Duration(time.Second * 10))
	}
}

// CheckMessageFileIssue check/fix issues from message file
func CheckMessageFileIssue() {
	// got the last few lines of message file
	lines := ReadFileLinesFromHost(SysLog)

	// loop in lines
	for _, line := range lines {
		// Fix Block Volume Reference Issue;
		if GlobalConfigVar.IssueBlockReference && strings.Contains(line, "is still referenced from other Pods") {
			if FixReferenceMountIssue(line) {
				log.Info("fix reference mount issue done")
			} else {
				log.Warning("fix reference mount issue failed")
			}
			// Fix Orphaned Pod Issue
		} else if GlobalConfigVar.IssueOrphanedPod && strings.Contains(line, "rphaned pod") && (strings.Contains(line, "found, but volume paths are still present on disk") || strings.Contains(line, "found, but volumes are not cleaned up") || strings.Contains(line, "terminated, but some volumes have not been cleaned up")) {
			if FixOrphanedPodIssue(line) {
				log.Info("fix orphaned pod done")
			} else {
				log.Warning("fix orphaned pod failed")
			}
		}
	}
}

// GlobalConfigSet set Global Config
func GlobalConfigSet() {
	GlobalConfigVar.IssueMessageFile = false
	messageFile := os.Getenv(IssueMessageFile)
	if messageFile == "true" {
		GlobalConfigVar.IssueMessageFile = true
	}

	GlobalConfigVar.MessageFileTailLines = 20
	messageFileLine := os.Getenv(MessageFileLines)
	if messageFileLine != "" {
		lineNum, err := strconv.Atoi(messageFileLine)
		if err != nil {
			log.Errorf("OM GlobalConfigSet: MessageFileLines error format: %s", messageFileLine)
		} else {
			GlobalConfigVar.MessageFileTailLines = lineNum
			if GlobalConfigVar.MessageFileTailLines > 500 {
				log.Warningf("OM GlobalConfigSet: MessageFileLines too large: %s", messageFileLine)
			}
		}
	}

	GlobalConfigVar.IssueBlockReference = false
	blockRef := os.Getenv(IssueBlockReference)
	if blockRef == "true" {
		GlobalConfigVar.IssueBlockReference = true
	}

	GlobalConfigVar.IssueOrphanedPod = false
	orphanedPod := os.Getenv(IssueOrphanedPod)
	if orphanedPod == "true" {
		GlobalConfigVar.IssueOrphanedPod = true
	}
}
