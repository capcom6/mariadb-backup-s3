package registry

import (
	"context"
	"io"
	"time"
)

func (s *Service) Parse(r io.Reader) (*Registry, error) {
	return s.parse(r)
}

func (s *Service) Rebuild(ctx context.Context) (*Registry, error) {
	return s.rebuild(ctx)
}

func (s *Service) Save(ctx context.Context, reg *Registry) error {
	return s.save(ctx, reg)
}

func BackupTimeFromFilename(filename string) (time.Time, bool) {
	return backupTimeFromFilename(filename)
}
