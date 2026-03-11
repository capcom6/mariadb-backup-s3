package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/storage"
)

type Service struct {
	storage storage.Backend

	options options
}

func NewService(storage storage.Backend, opts ...Option) *Service {
	options := new(options)
	for _, opt := range opts {
		opt(options)
	}

	return &Service{
		storage: storage,

		options: *options,
	}
}

func (s *Service) Load(ctx context.Context) (*Registry, error) {
	buf := bytes.Buffer{}

	if err := s.storage.Download(ctx, fileName, &buf); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return s.rebuild(ctx)
		}
		return nil, fmt.Errorf("failed to download registry: %w", err)
	}

	reg, err := s.parse(&buf)
	if err != nil {
		return nil, err
	}

	return reg, nil
}

func (s *Service) Append(ctx context.Context, b BackupEntry) (*Registry, error) {
	reg, err := s.Load(ctx)
	if err != nil {
		return nil, err
	}

	reg.Backups = append(reg.Backups, b)

	if saveErr := s.save(ctx, reg); saveErr != nil {
		return nil, saveErr
	}

	return reg, nil
}

func (s *Service) MarkDeleted(ctx context.Context, id string) error {
	reg, err := s.Load(ctx)
	if err != nil {
		return err
	}

	for i, b := range reg.Backups {
		if b.ID == id {
			reg.Backups[i].Status = StatusDeleted
			break
		}
	}

	return s.save(ctx, reg)
}

func (s *Service) parse(r io.Reader) (*Registry, error) {
	var reg Registry
	dec := json.NewDecoder(r)
	if err := dec.Decode(&reg); err != nil {
		return nil, fmt.Errorf("failed to decode registry: %w", err)
	}

	if err := reg.Validate(); err != nil {
		return nil, err
	}

	return &reg, nil
}

func (s *Service) rebuild(ctx context.Context) (*Registry, error) {
	if !s.options.withRecovery {
		return New(), nil
	}

	files, err := s.storage.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list storage objects: %w", err)
	}

	entries := make([]BackupEntry, 0, len(files))
	for _, filename := range files {
		if filename == fileName || strings.HasSuffix(filename, "/") {
			continue
		}

		createdAt, ok := backupTimeFromFilename(filename)
		if !ok {
			continue
		}

		encrypted := strings.HasSuffix(filename, ".enc")
		entries = append(entries, BackupEntry{
			ID:        createdAt.Format("2006-01-02-15-04-05"),
			Filename:  filename,
			CreatedAt: createdAt,
			SizeBytes: 0,
			SHA256:    "unknown",
			Encrypted: encrypted,
			Status:    StatusReady,
			Tool: &ToolMetadata{
				Name:    "mariadb-backup-s3",
				Version: "unknown",
			},
			Encryption: nil,
		})
	}

	if len(entries) == 0 {
		return New(), nil
	}

	reg := New()
	reg.Backups = entries
	reg.SortByCreatedAtDesc()

	if saveErr := s.save(ctx, reg); saveErr != nil {
		return nil, saveErr
	}

	return reg, nil
}

func (s *Service) save(ctx context.Context, reg *Registry) error {
	var buf bytes.Buffer
	if err := reg.Validate(); err != nil {
		return err
	}

	reg.SortByCreatedAtDesc()
	reg.UpdatedAt = time.Now().UTC()

	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(reg); err != nil {
		return fmt.Errorf("failed to encode registry: %w", err)
	}

	if err := s.storage.Upload(ctx, fileName, &buf); err != nil {
		return fmt.Errorf("failed to upload registry: %w", err)
	}

	return nil
}
