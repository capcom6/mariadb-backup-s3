package scheduler

import (
	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/scheduler/repository"
)

func LoadState(filePath string) (*State, error) {
	return loadState(filePath)
}

func BuildDatabase(job repository.Job) config.Database {
	return (&Scheduler{}).buildDatabase(job)
}

// BuildMariaDB keeps the legacy test helper name while returning the generalized database config.
func BuildMariaDB(job repository.Job) config.Database {
	return BuildDatabase(job)
}
