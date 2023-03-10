package restic

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

func InitCommand(s3Endpoint, ak, sk, repository, encryptionKey string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "init")
	command := strings.Join(cmd, " ")
	return command
}

// BackupCommandByID returns restic backup command
func BackupCommandByID(s3Endpoint, ak, sk, repository, encryptionKey, pathToBackup string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "backup", pathToBackup)
	command := strings.Join(cmd, " ")
	return command
}

// BackupCommandByTag returns restic backup command with tag
func BackupCommandByTag(s3Endpoint, ak, sk, repository, encryptionKey, backupTag, includePath string) string {
	cmd := []string{
		fmt.Sprintf("cd %s\n", includePath),
	}
	cmd = append(cmd, resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)...)
	cmd = append(cmd, "backup", "--tag", backupTag, ".", "--json", "|", "jq -r 'select(.message_type==\"summary\")'")
	command := strings.Join(cmd, " ")
	return command
}

func LatestSnapshotsCommand(s3Endpoint, ak, sk, repository, encryptionKey string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "snapshots", "--last", "--json")
	command := strings.Join(cmd, " ")
	return command
}

// RestoreCommandByID returns restic restore command with snapshotID as the identifier
func RestoreCommandByID(s3Endpoint, ak, sk, repository, encryptionKey, id, restorePath string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "restore", id, "--target", restorePath)
	command := strings.Join(cmd, " ")
	return command
}

// RestoreCommandByTag returns restic restore command with tag as the identifier
func RestoreCommandByTag(s3Endpoint, ak, sk, repository, encryptionKey, tag, restorePath string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "restore", "--tag", tag, "latest", "--target", restorePath)
	command := strings.Join(cmd, " ")
	return command
}

// SnapshotsCommand returns restic snapshots command
func SnapshotsCommand(s3Endpoint, ak, sk, repository, encryptionKey string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "snapshots", "--json")
	command := strings.Join(cmd, " ")
	return command
}

// SnapshotsCommandByTag returns restic snapshots command
func SnapshotsCommandByTag(s3Endpoint, ak, sk, repository, encryptionKey, tag string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "snapshots", "--tag", tag, "--json")
	command := strings.Join(cmd, " ")
	return command
}

// ForgetCommandByTag returns restic forget command
func ForgetCommandByTag(s3Endpoint, ak, sk, repository, encryptionKey, tag string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "forget", "--tag", tag)
	command := strings.Join(cmd, " ")
	return command
}

// ForgetCommandByID returns restic forget command
func ForgetCommandByID(s3Endpoint, ak, sk, repository, encryptionKey, id string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "forget", id)
	command := strings.Join(cmd, " ")
	return command
}

// PruneCommand returns restic prune command
func PruneCommand(s3Endpoint, ak, sk, repository, encryptionKey string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "prune")
	command := strings.Join(cmd, " ")
	return command
}

// StatsCommandByID returns restic stats command
func StatsCommandByID(s3Endpoint, ak, sk, repository, encryptionKey, id, mode string) string {
	cmd := resticArgs(s3Endpoint, ak, sk, repository, encryptionKey)
	cmd = append(cmd, "stats", id, "--mode", mode)
	command := strings.Join(cmd, " ")
	return command
}

// SnapshotIDFromSnapshotLog gets the SnapshotID from Snapshot Command log
func SnapshotIDFromSnapshotLog(output string) ([]string, error) {
	var result []map[string]interface{}
	err := json.Unmarshal([]byte(output), &result)
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to unmarshall output from snapshotCommand")
	}
	if len(result) == 0 {
		return nil, nil
	}
	snapIds := make([]string, len(result))
	for i := 0; i < len(result); i++ {
		snapIds[i] = result[i]["short_id"].(string)
	}
	return snapIds, nil
}

// SnapshotIDFromBackupLog gets the SnapshotID from Backup Command log
func SnapshotIDFromBackupLog(output string) string {
	if output == "" {
		return ""
	}
	logs := regexp.MustCompile("[\n]").Split(output, -1)
	// Log should contain "snapshot ABC123 saved"
	pattern := regexp.MustCompile(`snapshot\s(.*?)\ssaved$`)
	for _, l := range logs {
		match := pattern.FindAllStringSubmatch(l, 1)
		if match != nil {
			if len(match) >= 1 && len(match[0]) >= 2 {
				return match[0][1]
			}
		}
	}
	return ""
}

// SnapshotStatsFromBackupLog gets the Snapshot file count and size from Backup Command log
func SnapshotStatsFromBackupLog(output string) (fileCount string, backupSize string, phySize string) {
	if output == "" {
		return "", "", ""
	}
	logs := regexp.MustCompile("[\n]").Split(output, -1)
	// Log should contain "processed %d files, %.3f [Xi]B in mm:ss"
	logicalPattern := regexp.MustCompile(`processed\s([\d]+)\sfiles,\s([\d]+(\.[\d]+)?\s([TGMK]i)?B)\sin\s`)
	// Log should contain "Added to the repo: %.3f [Xi]B"
	physicalPattern := regexp.MustCompile(`^Added to the repo: ([\d]+(\.[\d]+)?\s([TGMK]i)?B)$`)

	for _, l := range logs {
		logMatch := logicalPattern.FindAllStringSubmatch(l, 1)
		phyMatch := physicalPattern.FindAllStringSubmatch(l, 1)
		switch {
		case logMatch != nil:
			if len(logMatch) >= 1 && len(logMatch[0]) >= 3 {
				// Expect in order:
				// 0: entire match,
				// 1: first submatch == file count,
				// 2: second submatch == size string
				fileCount = logMatch[0][1]
				backupSize = logMatch[0][2]
			}
		case phyMatch != nil:
			if len(phyMatch) >= 1 && len(phyMatch[0]) >= 2 {
				// Expect in order:
				// 0: entire match,
				// 1: first submatch == size string,
				phySize = phyMatch[0][1]
			}
		}
	}
	return fileCount, backupSize, phySize
}

func resticArgs(s3Endpoint, ak, sk, repository, encryptionKey string) []string {
	args := []string{
		fmt.Sprintf("export %s=%s\n", AWSAccessKeyID, ak),
		fmt.Sprintf("export %s=%s\n", AWSSecretAccessKey, sk),
		fmt.Sprintf("export %s=s3:%s/%s/%s\n", ResticRepository, s3Endpoint, ResticPrefix, repository),
		fmt.Sprintf("export %s=%s\n", ResticPassword, encryptionKey),
		// 限制使用的CPU核数（默认情况下，restic 使用所有可用的 CPU 内核）
		"export GOMAXPROCS=1\n",
		ResticCommand,
	}
	return args
}
