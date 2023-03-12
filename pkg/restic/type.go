package restic

import "strings"

const (
	AWSAccessKeyID     = "AWS_ACCESS_KEY_ID"
	AWSSecretAccessKey = "AWS_SECRET_ACCESS_KEY"
	ResticPassword     = "RESTIC_PASSWORD"
	ResticRepository   = "RESTIC_REPOSITORY"
	ResticCommand      = "restic"
	// DeleteDataOutputSpaceFreed is the key for the output reporting the space freed
	DeleteDataOutputSpaceFreed = "spaceFreed"
	tempPW                     = "tempPW"

	PasswordIncorrect = "password is incorrect"
	RepoDoesNotExist  = "repo does not exist"
	RepoBucket        = "open-local"
)

var ClusterID = "unknown"

type backupStatusLine struct {
	MessageType string `json:"message_type"`
	// seen in status lines
	TotalBytes int64 `json:"total_bytes_processed"`
	DataAdded  int64 `json:"data_added"`
	// seen in summary line at the end
	SnapshotID string `json:"snapshot_id"`
}

// IsPasswordIncorrect checks if password was wrong from Snapshot Command log
func IsPasswordIncorrect(output string) bool {
	return strings.Contains(output, "wrong password")
}

// DoesRepoExists checks if repo exists from Snapshot Command log
func DoesRepoExist(output string) bool {
	return strings.Contains(output, "Is there a repository at the following location?")
}
