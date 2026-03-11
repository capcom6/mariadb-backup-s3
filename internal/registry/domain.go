package registry

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	currentVersion = 1
	fileName       = ".backup-registry.json"

	StatusReady   = "ready"
	StatusFailed  = "failed"
	StatusDeleted = "deleted"
)

type Registry struct {
	Version   int           `json:"version"`
	UpdatedAt time.Time     `json:"updated_at"`
	Backups   []BackupEntry `json:"backups"`
}

type BackupEntry struct {
	ID         string              `json:"id"`
	Filename   string              `json:"filename"`
	CreatedAt  time.Time           `json:"created_at"`
	SizeBytes  int64               `json:"size_bytes"`
	SHA256     string              `json:"sha256"`
	Status     string              `json:"status"`
	Encrypted  bool                `json:"encrypted"`
	Encryption *EncryptionMetadata `json:"encryption,omitempty"`
	Tool       *ToolMetadata       `json:"tool,omitempty"`
}

func NewBackupEntry(
	filename string,
	createdAt time.Time,
	sizeBytes int64,
	sha256 string,
	status string,
	encrypted bool,
	encryption *EncryptionMetadata,
	tool *ToolMetadata,
) BackupEntry {
	return BackupEntry{
		ID:         createdAt.Format("2006-01-02-15-04-05") + "-" + sha256[:8],
		Filename:   filename,
		CreatedAt:  createdAt,
		SizeBytes:  sizeBytes,
		SHA256:     sha256,
		Encrypted:  encrypted,
		Encryption: encryption,
		Tool:       tool,
		Status:     status,
	}
}

type EncryptionMetadata struct {
	Algorithm string `json:"algorithm,omitempty"`
}

type ToolMetadata struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

func New() *Registry {
	return &Registry{
		Version:   currentVersion,
		UpdatedAt: time.Now().UTC(),
		Backups:   make([]BackupEntry, 0),
	}
}

func (r *Registry) Validate() error {
	if r.Version != currentVersion {
		return fmt.Errorf("%w: unsupported registry version %d", ErrValidationFailed, r.Version)
	}
	if r.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: updated_at is required", ErrValidationFailed)
	}

	seenIDs := make(map[string]struct{}, len(r.Backups))
	seenFiles := make(map[string]struct{}, len(r.Backups))

	for i := range r.Backups {
		if err := r.Backups[i].Validate(); err != nil {
			return fmt.Errorf("invalid backup entry %d: %w", i, err)
		}
		if _, exists := seenIDs[r.Backups[i].ID]; exists {
			return fmt.Errorf("%w: duplicate backup id: %s", ErrValidationFailed, r.Backups[i].ID)
		}
		if _, exists := seenFiles[r.Backups[i].Filename]; exists {
			return fmt.Errorf("%w: duplicate backup filename: %s", ErrValidationFailed, r.Backups[i].Filename)
		}
		seenIDs[r.Backups[i].ID] = struct{}{}
		seenFiles[r.Backups[i].Filename] = struct{}{}
	}

	return nil
}

func (r *Registry) SortByCreatedAtDesc() {
	sort.Slice(r.Backups, func(i, j int) bool {
		return r.Backups[i].CreatedAt.After(r.Backups[j].CreatedAt)
	})
}

func (b *BackupEntry) Validate() error {
	if strings.TrimSpace(b.ID) == "" {
		return fmt.Errorf("%w: id is required", ErrValidationFailed)
	}
	if strings.TrimSpace(b.Filename) == "" {
		return fmt.Errorf("%w: filename is required", ErrValidationFailed)
	}
	if b.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created_at is required", ErrValidationFailed)
	}
	if b.SizeBytes < 0 {
		return fmt.Errorf("%w: size_bytes must be >= 0", ErrValidationFailed)
	}
	if strings.TrimSpace(b.SHA256) == "" {
		return fmt.Errorf("%w: sha256 is required", ErrValidationFailed)
	}
	if !isValidStatus(b.Status) {
		return fmt.Errorf("%w: unsupported status: %s", ErrValidationFailed, b.Status)
	}

	return nil
}

func isValidStatus(s string) bool {
	switch s {
	case StatusReady, StatusFailed, StatusDeleted:
		return true
	default:
		return false
	}
}
