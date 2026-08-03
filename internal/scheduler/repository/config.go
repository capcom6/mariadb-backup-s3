package repository

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

var (
	ErrNoJobs       = errors.New("at least one job must be defined")
	ErrDuplicateJob = errors.New("duplicate job name")
	ErrNameRequired = errors.New("name is required")
	ErrSchedule     = errors.New("schedule is required")
	ErrCommand      = errors.New("unsupported command")
	ErrStorageURL   = errors.New("storage.url is required")
)

const (
	CommandBackup    = "backup"
	CommandRetention = "retention"

	defaultTimeoutBackup    = 4 * time.Hour
	defaultTimeoutRetention = 30 * time.Minute
	defaultTimeoutDefault   = 1 * time.Hour
)

type Config struct {
	StateFile string  `yaml:"state_file"`
	Jobs      []Job   `yaml:"jobs"`
	Webhook   Webhook `yaml:"webhook,omitempty"`
}

type Job struct {
	Name      string     `yaml:"name"`
	Schedule  string     `yaml:"schedule"`
	Command   string     `yaml:"command"`
	Timeout   Duration   `yaml:"timeout,omitempty"`
	Enabled   *bool      `yaml:"enabled,omitempty"`
	MariaDB   *MariaDB   `yaml:"mariadb,omitempty"`
	Storage   Storage    `yaml:"storage"`
	Encrypt   *Encrypt   `yaml:"encrypt,omitempty"`
	Retention *Retention `yaml:"retention,omitempty"`
}

type MariaDB struct {
	Host          string `yaml:"host,omitempty"`
	Port          int    `yaml:"port,omitempty"`
	User          string `yaml:"user,omitempty"`
	Password      string `yaml:"password,omitempty"`
	BackupMethod  string `yaml:"backup_method,omitempty"`
	BackupBinary  string `yaml:"backup_binary,omitempty"`
	ClientBinary  string `yaml:"client_binary,omitempty"`
	BackupOptions string `yaml:"backup_options,omitempty"`
}

type Storage struct {
	URL string `yaml:"url"`
}

type Encrypt struct {
	Key string `yaml:"key,omitempty"`
}

type Retention struct {
	MaxCount    int           `yaml:"max_count,omitempty"`
	MaxAge      time.Duration `yaml:"max_age,omitempty"`
	KeepDaily   int           `yaml:"keep_daily,omitempty"`
	KeepWeekly  int           `yaml:"keep_weekly,omitempty"`
	KeepMonthly int           `yaml:"keep_monthly,omitempty"`
}

type Webhook struct {
	URL     string   `yaml:"url"`
	Timeout Duration `yaml:"timeout,omitempty"`
}

const (
	DefaultStateFile = "/var/lib/mariadb-backup-s3/state.json"
)

func (c Config) Validate() error {
	if len(c.Jobs) == 0 {
		return ErrNoJobs
	}

	names := make(map[string]struct{}, len(c.Jobs))
	for i, job := range c.Jobs {
		if err := job.Validate(); err != nil {
			return fmt.Errorf("job %d (%q): %w", i, job.Name, err)
		}
		if _, ok := names[job.Name]; ok {
			return fmt.Errorf("%w: %q", ErrDuplicateJob, job.Name)
		}
		names[job.Name] = struct{}{}
	}

	return nil
}

func (j Job) Validate() error {
	if j.Name == "" {
		return ErrNameRequired
	}
	if j.Schedule == "" {
		return ErrSchedule
	}
	if _, err := cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	).Parse(j.Schedule); err != nil {
		return fmt.Errorf("invalid schedule %q: %w", j.Schedule, err)
	}
	switch j.Command {
	case CommandBackup, CommandRetention:
	default:
		return fmt.Errorf("%w %q: must be 'backup' or 'retention'", ErrCommand, j.Command)
	}
	if j.Storage.URL == "" {
		return ErrStorageURL
	}
	return nil
}

func (j Job) IsEnabled() bool {
	if j.Enabled == nil {
		return true
	}
	return *j.Enabled
}

type Duration time.Duration

func (d *Duration) Duration() time.Duration {
	if d == nil {
		return 0
	}
	return time.Duration(*d)
}

func (j Job) EncryptionKey() string {
	if j.Encrypt == nil {
		return ""
	}
	return j.Encrypt.Key
}

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return fmt.Errorf("unmarshal duration: %w", err)
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", s, err)
	}
	*d = Duration(dur)
	return nil
}

func (j Job) GetTimeout() time.Duration {
	if j.Timeout > 0 {
		return time.Duration(j.Timeout)
	}
	switch j.Command {
	case CommandBackup:
		return defaultTimeoutBackup
	case CommandRetention:
		return defaultTimeoutRetention
	default:
		return defaultTimeoutDefault
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	data = []byte(os.ExpandEnv(string(data)))

	var cfg Config
	if marshalErr := yaml.Unmarshal(data, &cfg); marshalErr != nil {
		return Config{}, fmt.Errorf("parse YAML: %w", marshalErr)
	}

	if validErr := cfg.Validate(); validErr != nil {
		return Config{}, fmt.Errorf("validate: %w", validErr)
	}

	return cfg, nil
}
