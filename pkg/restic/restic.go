package restic

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
	log "k8s.io/klog/v2"
)

// GetOrCreateRepository will check if the repository already exists and initialize one if not
func GetOrCreateRepository(s3Endpoint, ak, sk, repository, encryptionKey string) error {
	_, err := getLatestSnapshots(s3Endpoint, ak, sk, repository, encryptionKey)
	if err == nil {
		return nil
	}
	// Create a repository
	cmd := InitCommand(s3Endpoint, ak, sk, repository, encryptionKey)
	if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create object store backup location: %s, %s", err.Error(), string(out))
	}

	return nil
}

// CheckIfRepoIsReachable checks if repo can be reached by trying to list snapshots
func CheckIfRepoIsReachable(s3Endpoint, ak, sk, repository, encryptionKey string) error {
	out, err := getLatestSnapshots(s3Endpoint, ak, sk, repository, encryptionKey)
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

func BackupData(s3Endpoint, ak, sk, repository, encryptionKey, pathToBackup, backupTag string) (int64, error) {
	if err := GetOrCreateRepository(s3Endpoint, ak, sk, repository, encryptionKey); err != nil {
		return 0, fmt.Errorf("fail to get or create restic repository: %s", err.Error())
	}

	if err := CheckIfRepoIsReachable(s3Endpoint, ak, sk, repository, encryptionKey); err != nil {
		return 0, fmt.Errorf("fail to check if repository is reachable: %s", err.Error())
	}

	// Create backup and dump it on the object store
	cmd := BackupCommandByTag(s3Endpoint, ak, sk, repository, encryptionKey, backupTag, pathToBackup)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("fail to run restic backup command: %s,%s", err.Error(), out)
	}

	info, err := decodeBackupStatusLine(out)
	if err != nil {
		return 0, fmt.Errorf("fail to decode backup status line: %s", err.Error())
	}

	return info.TotalBytes, nil
}

func RestoreData(s3Endpoint, ak, sk, repository, encryptionKey, restorePath, backupTag string) (map[string]interface{}, error) {
	var cmd string
	var err error

	if err := CheckIfRepoIsReachable(s3Endpoint, ak, sk, repository, encryptionKey); err != nil {
		return nil, err
	}

	// Generate restore command based on the identifier passed
	cmd = RestoreCommandByTag(s3Endpoint, ak, sk, repository, encryptionKey, backupTag, restorePath)
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

func PruneData(s3Endpoint, ak, sk, repository, encryptionKey string) (string, error) {
	if err := CheckIfRepoIsReachable(s3Endpoint, ak, sk, repository, encryptionKey); err != nil {
		return "", err
	}
	cmd := PruneCommand(s3Endpoint, ak, sk, repository, encryptionKey)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s, %s", err, out)
	}
	spaceFreed := SpaceFreedFromPruneLog(string(out))
	return spaceFreed, errors.Wrapf(err, "failed to prune data after forget")
}

func DeleteDataByID(s3Endpoint, ak, sk, repository, encryptionKey, deleteTag string, reclaimSpace bool) (map[string]interface{}, error) {
	if err := CheckIfRepoIsReachable(s3Endpoint, ak, sk, repository, encryptionKey); err != nil {
		return nil, err
	}

	cmd := SnapshotsCommandByTag(s3Endpoint, ak, sk, repository, encryptionKey, deleteTag)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to forget data, could not get snapshotID from tag(%s): %s, %s", deleteTag, err.Error(), string(out))
	}
	deleteIdentifier, err := SnapshotIDFromSnapshotLog(string(out))
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to forget data, could not get snapshotID from tag, Tag: %s", deleteTag)
	}
	deleteID := deleteIdentifier

	log.Infof("delete tag is %s, deleteID is %s", deleteTag, deleteID)

	var spaceFreedTotal int64
	cmd = ForgetCommandByID(s3Endpoint, ak, sk, repository, encryptionKey, deleteID)
	out, err = exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to forget data: %s, %s", err.Error(), string(out))
	}
	log.Infof("delete data %s,%s success", deleteTag, deleteID)
	if reclaimSpace {
		spaceFreedStr, err := PruneData(s3Endpoint, ak, sk, repository, encryptionKey)
		if err != nil {
			return nil, errors.Wrapf(err, "Error executing prune command")
		}
		spaceFreedTotal = ParseResticSizeStringBytes(spaceFreedStr)
	}

	return map[string]interface{}{
		DeleteDataOutputSpaceFreed: fmt.Sprintf("%d B", spaceFreedTotal),
	}, nil
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

func getLatestSnapshots(s3Endpoint, ak, sk, repository, encryptionKey string) (string, error) {
	// Use the latest snapshots command to check if the repository exists
	cmd := LatestSnapshotsCommand(s3Endpoint, ak, sk, repository, encryptionKey)
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
