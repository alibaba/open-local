package restic

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-local/pkg/utils"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	log "k8s.io/klog/v2"
)

type ResticClient struct {
	s3Endpoint    string
	ak            string
	sk            string
	encryptionKey string
	kubeclient    kubernetes.Interface
	clusterID     string
}

// 申请 restic client 要求必须传入 repository
func NewResticClient(s3Endpoint, ak, sk, encryptionKey string, kubeclient kubernetes.Interface) (*ResticClient, error) {
	clusterID, err := utils.GetClusterID(kubeclient)
	if err != nil {
		return nil, err
	}

	return &ResticClient{
		s3Endpoint:    s3Endpoint,
		ak:            ak,
		sk:            sk,
		encryptionKey: encryptionKey,
		kubeclient:    kubeclient,
		clusterID:     clusterID,
	}, nil
}

// GetOrCreateRepository will check if the repository already exists and initialize one if not
func (r *ResticClient) GetOrCreateRepository(repository string) error {
	_, err := r.getLatestSnapshots(repository)
	if err == nil {
		return nil
	}
	// Create a repository
	cmd := InitCommand(r.s3Endpoint, r.ak, r.sk, repository, r.encryptionKey, r.clusterID)
	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create object store backup location: %s, %s", err.Error(), string(out))
	}

	return nil
}

// CheckIfRepoIsReachable checks if repo can be reached by trying to list snapshots
func (r *ResticClient) CheckIfRepoIsReachable(repository string) error {
	out, err := r.getLatestSnapshots(repository)
	if err != nil && IsPasswordIncorrect(out) { // If password didn't work
		return fmt.Errorf(PasswordIncorrect)
	}
	if err != nil && DoesRepoExist(out) {
		return fmt.Errorf(RepoDoesNotExist)
	}
	if err != nil {
		return fmt.Errorf("failed to list restic snapshots: %s, %s", out, err.Error())
	}
	return nil
}

const (
	backupProgressCheckInterval = 4 * time.Second
)

func (r *ResticClient) BackupData(repository, pathToBackup, backupTag string, updateEventFunc func(float64)) (int64, error) {
	defer utils.TimeTrack(time.Now(), fmt.Sprintf("backup %s to s3 %s", pathToBackup, repository))
	if err := r.GetOrCreateRepository(repository); err != nil {
		return 0, fmt.Errorf("fail to get or create restic repository: %s", err.Error())
	}

	if err := r.CheckIfRepoIsReachable(repository); err != nil {
		return 0, fmt.Errorf("fail to check if repository is reachable: %s", err.Error())
	}

	// Create backup and dump it on the object store
	quit := make(chan struct{})
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)

	cmdStr := BackupCommandByTag(r.s3Endpoint, r.ak, r.sk, repository, r.encryptionKey, r.clusterID, backupTag, pathToBackup)
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err := cmd.Start()
	if err != nil {
		return 0, err
	}

	go func() {
		ticker := time.NewTicker(backupProgressCheckInterval)
		for {
			select {
			case <-ticker.C:
				lastLine := getLastLine(stdoutBuf.Bytes())
				if len(lastLine) > 0 {
					stat, err := decodeBackupStatusLine(lastLine)
					if err != nil {
						log.Errorf("fail to get restic backup progress: %s", err.Error())
					}

					// if the line contains a non-empty bytes_done field, we can update the
					// caller with the progress
					if stat.BytesDone != 0 {
						updateEventFunc(stat.PercentDone)
					}
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	err = cmd.Wait()
	if err != nil {
		return 0, err
	}
	quit <- struct{}{}

	summary, err := getSummaryLine(stdoutBuf.Bytes())
	if err != nil {
		return 0, err
	}
	stat, err := decodeBackupStatusLine(summary)
	if err != nil {
		return 0, err
	}
	if stat.MessageType != "summary" {
		return 0, fmt.Errorf("fail to get restic backup summary: %s", string(summary))
	}

	// update progress to 100%
	updateEventFunc(1)

	return stat.TotalBytesProcessed, nil
}

func (r *ResticClient) RestoreData(repository, pathToRestore, backupTag string) (map[string]interface{}, error) {
	defer utils.TimeTrack(time.Now(), fmt.Sprintf("restore %s from s3 %s", pathToRestore, repository))
	var cmd string
	var err error
	if err := r.CheckIfRepoIsReachable(repository); err != nil {
		return nil, err
	}

	// Generate restore command based on the identifier passed
	cmd = RestoreCommandByTag(r.s3Endpoint, r.ak, r.sk, repository, r.encryptionKey, r.clusterID, backupTag, pathToRestore)
	fmt.Printf("restore cmd: %s\n", cmd)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s, %s", err, out)
	}

	outMap, err := parseLogAndCreateOutput(string(out))
	if err != nil {
		return nil, err
	}
	return outMap, nil
}

func (r *ResticClient) PruneData(repository string) (string, error) {
	defer utils.TimeTrack(time.Now(), fmt.Sprintf("prune data from s3 %s", repository))
	if err := r.CheckIfRepoIsReachable(repository); err != nil {
		return "", err
	}
	cmd := PruneCommand(r.s3Endpoint, r.ak, r.sk, repository, r.encryptionKey, r.clusterID)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s, %s", err, out)
	}
	spaceFreed := SpaceFreedFromPruneLog(string(out))
	return spaceFreed, errors.Wrapf(err, "failed to prune data after forget")
}

func (r *ResticClient) DeleteDataByTag(repository string, deleteTag string, reclaimSpace bool) (map[string]interface{}, error) {
	defer utils.TimeTrack(time.Now(), fmt.Sprintf("delete data from s3 %s", repository))
	if err := r.CheckIfRepoIsReachable(repository); err != nil {
		if strings.Contains(err.Error(), RepoDoesNotExist) {
			return nil, nil
		}
		return nil, err
	}

	log.Infof("delete tag is %s", deleteTag)
	cmd := ForgetCommandByTag(r.s3Endpoint, r.ak, r.sk, repository, r.encryptionKey, r.clusterID, deleteTag)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to forget data: %s, %s", err.Error(), string(out))
	}
	log.Infof("delete data (tag: %s) success", deleteTag)

	var spaceFreedTotal int64
	if reclaimSpace {
		spaceFreedStr, err := r.PruneData(repository)
		if err != nil {
			return nil, errors.Wrapf(err, "Error executing prune command")
		}
		spaceFreedTotal = ParseResticSizeStringBytes(spaceFreedStr)
		log.Infof("prune data %d byte for %s", spaceFreedTotal, repository)
	}

	return map[string]interface{}{
		DeleteDataOutputSpaceFreed: fmt.Sprintf("%d B", spaceFreedTotal),
	}, nil
}

func (r *ResticClient) CheckSnapshotExist(repository string) (bool, error) {
	cmd := SnapshotsCommand(r.s3Endpoint, r.ak, r.sk, repository, r.encryptionKey, r.clusterID)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return false, errors.Wrapf(err, "failed to check snapshot data, could not get snapshotID: %s, %s", err.Error(), string(out))
	}
	ids, err := SnapshotIDFromSnapshotLog(string(out))
	if err != nil {
		return false, errors.Wrapf(err, "failed to check snapshot log, could not get snapshotID")
	}
	return len(ids) != 0, nil
}

func parseLogAndCreateOutput(out string) (map[string]interface{}, error) {
	if out == "" {
		return nil, nil
	}
	var op map[string]interface{}
	logs := regexp.MustCompile("[\n]").Split(out, -1)
	for _, l := range logs {
		opObj, err := Parse(l)
		if err != nil {
			return nil, err
		}
		if opObj == nil {
			continue
		}
		if op == nil {
			op = make(map[string]interface{})
		}
		op[opObj.Key] = opObj.Value
	}
	return op, nil
}

func (r *ResticClient) getLatestSnapshots(repository string) (string, error) {
	// Use the latest snapshots command to check if the repository exists
	cmd := LatestSnapshotsCommand(r.s3Endpoint, r.ak, r.sk, repository, r.encryptionKey, r.clusterID)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	return string(out), err
}

// SpaceFreedFromPruneLog gets the space freed from the prune log output
// For reference, here is the logging command from restic codebase:
//
//		Verbosef("will delete %d packs and rewrite %d packs, this frees %s\n",
//	              len(removePacks), len(rewritePacks), formatBytes(uint64(removeBytes)))
func SpaceFreedFromPruneLog(output string) string {
	var spaceFreed string
	logs := regexp.MustCompile("[\n]").Split(output, -1)
	// Log should contain "will delete x packs and rewrite y packs, this frees zz.zzz [[TGMK]i]B"
	pattern := regexp.MustCompile(`^will delete \d+ packs and rewrite \d+ packs, this frees ([\d]+(\.[\d]+)?\s([TGMK]i)?B)$`)
	for _, l := range logs {
		match := pattern.FindAllStringSubmatch(l, 1)
		if len(match) > 0 && len(match[0]) > 1 {
			spaceFreed = match[0][1]
		}
	}
	return spaceFreed
}

// GeneratePassword generates a password
func GeneratePassword() string {
	h := sha256.New()
	_, _ = h.Write([]byte(tempPW))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ParseResticSizeStringBytes parses size strings as formatted by restic to
// a int64 number of bytes
func ParseResticSizeStringBytes(sizeStr string) int64 {
	components := regexp.MustCompile(`[\s]`).Split(sizeStr, -1)
	if len(components) != 2 {
		return 0
	}
	sizeNumStr := components[0]
	sizeNum, err := strconv.ParseFloat(sizeNumStr, 64)
	if err != nil {
		return 0
	}
	if sizeNum < 0 {
		return 0
	}
	magnitudeStr := components[1]
	pattern := regexp.MustCompile(`^(([TGMK]i)?B)$`)
	match := pattern.FindAllStringSubmatch(magnitudeStr, 1)
	if match != nil {
		if len(match) != 1 || len(match[0]) != 3 {
			return 0
		}
		magnitude := match[0][1]
		switch magnitude {
		case "TiB":
			return int64(sizeNum * (1 << 40))
		case "GiB":
			return int64(sizeNum * (1 << 30))
		case "MiB":
			return int64(sizeNum * (1 << 20))
		case "KiB":
			return int64(sizeNum * (1 << 10))
		case "B":
			return int64(sizeNum)
		default:
			return 0
		}
	}
	return 0
}

func decodeBackupStatusLine(lastLine []byte) (backupStatusLine, error) {
	var stat backupStatusLine
	if err := json.Unmarshal(lastLine, &stat); err != nil {
		return stat, errors.Wrapf(err, "unable to decode backup JSON line: %s", string(lastLine))
	}
	return stat, nil
}

// getLastLine returns the last line of a byte array. The string is assumed to
// have a newline at the end of it, so this returns the substring between the
// last two newlines.
func getLastLine(b []byte) []byte {
	if b == nil || len(b) == 0 {
		return []byte("")
	}
	// subslice the byte array to ignore the newline at the end of the string
	lastNewLineIdx := bytes.LastIndex(b[:len(b)-1], []byte("\n"))
	return b[lastNewLineIdx+1 : len(b)-1]
}

// getSummaryLine looks for the summary JSON line
// (`{"message_type:"summary",...`) in the restic backup command output. Due to
// an issue in Restic, this might not always be the last line
// (https://github.com/restic/restic/issues/2389). It returns an error if it
// can't be found.
func getSummaryLine(b []byte) ([]byte, error) {
	summaryLineIdx := bytes.LastIndex(b, []byte(`{"message_type":"summary"`))
	if summaryLineIdx < 0 {
		return nil, errors.New("unable to find summary in restic backup command output")
	}
	// find the end of the summary line
	newLineIdx := bytes.Index(b[summaryLineIdx:], []byte("\n"))
	if newLineIdx < 0 {
		return nil, errors.New("unable to get summary line from restic backup command output")
	}
	return b[summaryLineIdx : summaryLineIdx+newLineIdx], nil
}
