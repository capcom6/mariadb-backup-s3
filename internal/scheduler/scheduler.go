package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/backup"
	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/retention"
	"github.com/capcom6/mariadb-backup-s3/internal/scheduler/repository"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
	"github.com/capcom6/mariadb-backup-s3/internal/webhooks"
	"github.com/robfig/cron/v3"
)

const (
	logFieldJobName = "job_name"
	logFieldCommand = "command"
)

type Scheduler struct {
	jobs      repository.Config
	jobByName map[string]*jobEntry

	webhooksSvc *webhooks.Service
	cron        *cron.Cron

	state *State
	wg    sync.WaitGroup

	logger logging.Logger
}

type jobEntry struct {
	scheduleExpr string
	entryID      cron.EntryID
	schedule     cron.Schedule
}

func New(cfg Config, logger logging.Logger) (*Scheduler, error) {
	jobs, err := repository.Load(cfg.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load jobs: %w", err)
	}

	webhooksSvc, err := webhooks.NewService(webhooks.Config{
		DefaultURL:     jobs.Webhook.URL,
		DefaultTimeout: jobs.Webhook.Timeout.Duration(),
	})
	if err != nil {
		return nil, fmt.Errorf("create webhooks service: %w", err)
	}

	stateFile := jobs.StateFile
	if stateFile == "" {
		stateFile = cfg.StatePath
	}
	if stateFile == "" {
		stateFile = repository.DefaultStateFile
	}
	state, err := loadState(stateFile)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	s := &Scheduler{
		jobs:        jobs,
		jobByName:   make(map[string]*jobEntry, len(jobs.Jobs)),
		webhooksSvc: webhooksSvc,
		cron:        nil,
		state:       state,
		wg:          sync.WaitGroup{},
		logger:      logger.WithContext("scheduler", ""),
	}

	location := time.UTC
	s.cron = cron.New(
		cron.WithLocation(location),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger), cron.Recover(cron.DefaultLogger)),
	)

	for _, job := range jobs.Jobs {
		if !job.IsEnabled() {
			logger.Info(context.Background(), "Job disabled, skipping", logging.Fields{
				logFieldJobName: job.Name,
			})
			continue
		}

		if regErr := s.registerJob(job); regErr != nil {
			return nil, fmt.Errorf("register job %q: %w", job.Name, regErr)
		}
	}

	return s, nil
}

func (s *Scheduler) RunOnce(ctx context.Context) error {
	for _, job := range s.jobs.Jobs {
		if !job.IsEnabled() {
			continue
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("run once cancelled: %w", ctx.Err())
		default:
		}

		if err := s.runJob(ctx, job); err != nil {
			return err
		}
	}

	return nil
}

func (s *Scheduler) Run(ctx context.Context) error {
	s.cron.Start()

	s.logger.Info(ctx, "Scheduler started")

	<-ctx.Done()
	s.logger.Info(ctx, "Context cancelled, stopping scheduler")

	return s.stop(ctx)
}

func (s *Scheduler) Status() []JobState {
	return s.state.Status()
}

func (s *Scheduler) Close() error {
	if err := s.state.Close(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	return nil
}

func (s *Scheduler) stop(ctx context.Context) error {
	s.logger.Info(ctx, "Stopping scheduler")

	stopCtx := s.cron.Stop()

	s.wg.Wait()

	select {
	case <-stopCtx.Done():
	case <-ctx.Done():
	}

	if err := s.Close(); err != nil {
		s.logger.Error(ctx, "Failed to save state", err)
	}

	s.logger.Info(ctx, "Scheduler stopped")
	return nil
}

func (s *Scheduler) registerJob(job repository.Job) error {
	entryID, err := s.cron.AddFunc(job.Schedule, func() {
		if err := s.runJob(context.Background(), job); err != nil {
			s.logger.Error(context.Background(), "Failed to run job", err)
		}
	})
	if err != nil {
		return fmt.Errorf("parse schedule %q: %w", job.Schedule, err)
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	sched, err := parser.Parse(job.Schedule)
	if err != nil {
		return fmt.Errorf("parse schedule %q: %w", job.Schedule, err)
	}

	s.jobByName[job.Name] = &jobEntry{
		scheduleExpr: job.Schedule,
		entryID:      entryID,
		schedule:     sched,
	}

	// Record the next scheduled run time
	now := time.Now().UTC()
	next := sched.Next(now)
	s.state.RecordNextRun(job.Name, next)

	s.logger.Info(context.Background(), "Job registered", logging.Fields{
		logFieldJobName: job.Name,
		logFieldCommand: job.Command,
		"schedule":      job.Schedule,
	})

	return nil
}

func (s *Scheduler) runJob(ctx context.Context, job repository.Job) error {
	s.wg.Add(1)
	defer s.wg.Done()

	start := time.Now()
	jobLogger := s.logger.WithContext("job", job.Name)

	jobLogger.Info(ctx, "Starting job", logging.Fields{
		logFieldJobName: job.Name,
		logFieldCommand: job.Command,
	})

	timeout := job.GetTimeout()
	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Recover from panics so one bad job doesn't take down the scheduler
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("job panic: %v", r) //nolint:err113 // panic recovery
			jobLogger.Error(ctx, "Job panicked", err)
			s.state.RecordRun(job.Name, StatusFailed, err, time.Since(start))
			_ = s.state.Save()
			if ntfErr := s.webhooksSvc.Notify(context.Background(), webhooks.Payload{
				Event:     "job.completed",
				JobName:   job.Name,
				Status:    string(StatusFailed),
				Duration:  webhooks.Duration(time.Since(start)),
				StartedAt: start,
				Error:     err.Error(),
			}); ntfErr != nil {
				jobLogger.Error(ctx, "Failed to notify webhooks", ntfErr)
			}
		}
	}()

	err := s.executeJob(jobCtx, job, jobLogger)

	duration := time.Since(start)
	if err != nil {
		jobLogger.Error(ctx, "Job failed", err, logging.Fields{
			logFieldJobName: job.Name,
			"duration":      duration.Round(time.Second).String(),
		})
		s.state.RecordRun(job.Name, StatusFailed, err, duration)
	} else {
		jobLogger.Info(ctx, "Job completed", logging.Fields{
			logFieldJobName: job.Name,
			"duration":      duration.Round(time.Second).String(),
		})
		s.state.RecordRun(job.Name, StatusSuccess, nil, duration)
	}

	// Record next run time
	if je, ok := s.jobByName[job.Name]; ok {
		if entry := s.cron.Entry(je.entryID); entry.Next.After(start) {
			s.state.RecordNextRun(job.Name, entry.Next)
		}
	}

	if saveErr := s.state.Save(); saveErr != nil {
		jobLogger.Error(ctx, "Failed to save state", saveErr)
	}

	status := StatusSuccess
	errMsg := ""
	if err != nil {
		status = StatusFailed
		errMsg = err.Error()
	}
	if ntfErr := s.webhooksSvc.Notify(context.Background(), webhooks.Payload{
		Event:     "job.completed",
		JobName:   job.Name,
		Status:    string(status),
		Duration:  webhooks.Duration(time.Since(start)),
		StartedAt: start,
		Error:     errMsg,
	}); ntfErr != nil {
		jobLogger.Error(ctx, "Failed to notify webhooks", ntfErr)
	}

	return err
}

func (s *Scheduler) executeJob(ctx context.Context, job repository.Job, jobLogger logging.Logger) error {
	u, err := config.Storage{URL: job.Storage.URL}.GetURL()
	if err != nil {
		return fmt.Errorf("parse storage URL: %w", err)
	}

	storageSvc, err := storage.New(u)
	if err != nil {
		return fmt.Errorf("create storage backend: %w", err)
	}
	defer func() {
		if closeErr := storageSvc.Close(); closeErr != nil {
			jobLogger.Error(ctx, "Failed to close storage backend", closeErr)
		}
	}()

	registrySvc := registry.NewService(storageSvc)

	switch job.Command {
	case repository.CommandBackup:
		return s.executeBackup(ctx, job, storageSvc, registrySvc, jobLogger)
	case repository.CommandRetention:
		return s.executeRetention(ctx, job, storageSvc, registrySvc, jobLogger)
	default:
		return fmt.Errorf("%w: %s", repository.ErrCommand, job.Command)
	}
}

func (s *Scheduler) executeBackup(
	ctx context.Context,
	job repository.Job,
	storageSvc storage.Backend,
	registrySvc *registry.Service,
	jobLogger logging.Logger,
) error {
	cfg := backup.Config{
		Version: "",
		MariaDB: s.buildMariaDB(job),
		Storage: config.Storage{URL: job.Storage.URL},
		Encryption: config.Encryption{
			EncryptionKey: job.EncryptionKey(),
		},
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate backup config: %w", err)
	}

	op := backup.NewOperation(cfg, registrySvc, storageSvc, jobLogger)
	if runErr := op.Run(ctx); runErr != nil {
		return fmt.Errorf("backup failed: %w", runErr)
	}

	// Optionally run retention after backup
	if job.Retention != nil {
		retCfg := retention.Config{
			MaxCount:    job.Retention.MaxCount,
			MaxAge:      job.Retention.MaxAge,
			KeepDaily:   job.Retention.KeepDaily,
			KeepWeekly:  job.Retention.KeepWeekly,
			KeepMonthly: job.Retention.KeepMonthly,
			DryRun:      false,
			Force:       false,
		}

		if retVErr := retCfg.Validate(); retVErr == nil {
			retOp := retention.NewOperation(retCfg, registrySvc, storageSvc, jobLogger)
			if retErr := retOp.Run(ctx); retErr != nil {
				jobLogger.Warn(ctx, "Retention after backup failed", logging.Fields{"error": retErr})
			}
		}
	}

	return nil
}

func (s *Scheduler) executeRetention(
	ctx context.Context,
	job repository.Job,
	storageSvc storage.Backend,
	registrySvc *registry.Service,
	jobLogger logging.Logger,
) error {
	cfg := retention.Config{
		MaxCount:    job.Retention.MaxCount,
		MaxAge:      job.Retention.MaxAge,
		KeepDaily:   job.Retention.KeepDaily,
		KeepWeekly:  job.Retention.KeepWeekly,
		KeepMonthly: job.Retention.KeepMonthly,
		DryRun:      false,
		Force:       false,
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate retention config: %w", err)
	}

	op := retention.NewOperation(cfg, registrySvc, storageSvc, jobLogger)
	if err := op.Run(ctx); err != nil {
		return fmt.Errorf("retention failed: %w", err)
	}

	return nil
}

func (s *Scheduler) buildMariaDB(job repository.Job) config.MariaDB {
	cfg := config.DefaultMariaDB()

	if job.MariaDB == nil {
		return cfg
	}

	m := job.MariaDB

	if m.Host != "" {
		cfg.Host = m.Host
	}
	if m.Port > 0 {
		cfg.Port = m.Port
	}
	if m.User != "" {
		cfg.User = m.User
	}
	if m.Password != "" {
		cfg.Password = m.Password
	}
	if m.BackupBinary != "" {
		cfg.BackupBinary = m.BackupBinary
	}
	if m.BackupOptions != "" {
		cfg.BackupOptions = m.BackupOptions
	}

	return cfg
}
