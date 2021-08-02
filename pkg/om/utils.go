package om

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ReadFileLinesFromHost read file from /var/log/messages
func ReadFileLinesFromHost(fname string) []string {
	lineList := []string{}
	out := ""
	var err error
	cmd := fmt.Sprintf("tail -n %d %s", GlobalConfigVar.MessageFileTailLines, fname)
	if out, err = Run(cmd); err != nil {
		return lineList
	}
	return strings.Split(out, "\n")
}

// Run run shell command
func Run(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Failed to run cmd: " + cmd + ", with out: " + string(out) + ", with error: " + err.Error())
	}
	return string(out), nil
}

// IsFileExisting check file exist in volume driver;
func IsFileExisting(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}

// IsDirEmpty return status of dir empty or not
func IsDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// read in ONLY one file
	_, err = f.Readdir(1)
	// and if the file is EOF... well, the dir is empty.
	if err == io.EOF {
		return true, nil
	}
	return false, err
}
