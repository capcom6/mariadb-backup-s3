package scheduler

import (
	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/scheduler/repository"
)

func LoadState(filePath string) (*State, error) {
	return loadState(filePath)
}

func BuildMariaDB(job repository.Job) config.MariaDB {
	return (&Scheduler{}).buildMariaDB(job)
}
