package flags

import (
	"github.com/capcom6/mariadb-backup-s3/internal/config"
	"github.com/urfave/cli/v3"
)

const categoryDatabase = "Database"

func Database() []cli.Flag {
	fl := databaseConnectionFlags()
	fl = append(fl, databaseBackupOptionFlags()...)
	fl = append(fl, databaseBinaryFlags()...)
	fl = append(fl, mongoDBFlags()...)

	return fl
}

func databaseConnectionFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "db-engine",
			Usage:       "database engine: mariadb or mongodb",
			DefaultText: config.EngineMariaDB,
			Sources:     cli.EnvVars("DB__ENGINE"),
			Category:    categoryDatabase,
		},
		&cli.StringFlag{
			Name:        "db-host",
			Aliases:     []string{"host"},
			Usage:       "database host",
			DefaultText: "localhost",
			Sources:     cli.EnvVars("DB__HOST", "MARIADB__HOST"),
			Category:    categoryDatabase,
		},
		&cli.IntFlag{
			Name:        "db-port",
			Aliases:     []string{"port"},
			Usage:       "database port",
			DefaultText: "engine default (3306 or 27017)",
			Sources:     cli.EnvVars("DB__PORT", "MARIADB__PORT"),
			Category:    categoryDatabase,
		},
		&cli.StringFlag{
			Name:        "db-user",
			Aliases:     []string{"user"},
			Usage:       "database user",
			DefaultText: "root",
			Sources:     cli.EnvVars("DB__USER", "MARIADB__USER"),
			Category:    categoryDatabase,
		},
		&cli.StringFlag{
			Name:     "db-password",
			Aliases:  []string{"password"},
			Usage:    "database password",
			Sources:  cli.EnvVars("DB__PASSWORD", "MARIADB__PASSWORD"),
			Category: categoryDatabase,
		},
	}
}

func databaseBackupOptionFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "db-backup-options",
			Usage:    "database backup tool additional options",
			Sources:  cli.EnvVars("DB__BACKUP_OPTIONS", "MARIADB__BACKUP_OPTIONS"),
			Category: categoryDatabase,
		},
	}
}

func databaseBinaryFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "db-backup-binary",
			Usage:       "backup binary path (defaults from --db-engine and --backup-method)",
			DefaultText: "engine and method default",
			Sources:     cli.EnvVars("DB__BACKUP_BINARY", "MARIADB__BACKUP_BINARY"),
			Category:    categoryDatabase,
		},
		&cli.StringFlag{
			Name:        "db-client-binary",
			Usage:       "client binary path for database discovery",
			DefaultText: "engine default",
			Sources:     cli.EnvVars("DB__CLIENT_BINARY", "MARIADB__CLIENT_BINARY"),
			Category:    categoryDatabase,
		},
		&cli.StringFlag{
			Name:        "backup-method",
			Usage:       "backup method: mariadb-backup (physical), mariadb-dump (logical), or mongodump",
			DefaultText: "derived from --db-engine",
			Sources:     cli.EnvVars("DB__BACKUP_METHOD", "MARIADB__BACKUP_METHOD"),
			Category:    categoryDatabase,
		},
	}
}

func mongoDBFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "mongo-uri",
			Usage:    "MongoDB connection URI",
			Sources:  cli.EnvVars("MONGODB__URI"),
			Category: categoryDatabase,
		},
		&cli.StringFlag{
			Name:     "mongo-auth-source",
			Usage:    "MongoDB authentication source database",
			Sources:  cli.EnvVars("MONGODB__AUTH_SOURCE"),
			Category: categoryDatabase,
		},
		&cli.BoolFlag{
			Name:     "mongo-tls",
			Usage:    "enable MongoDB TLS",
			Sources:  cli.EnvVars("MONGODB__TLS"),
			Category: categoryDatabase,
		},
		&cli.StringFlag{
			Name:     "mongo-tls-ca-file",
			Usage:    "MongoDB TLS CA file",
			Sources:  cli.EnvVars("MONGODB__TLS_CA_FILE"),
			Category: categoryDatabase,
		},
		&cli.StringFlag{
			Name:     "mongo-tls-certificate-key-file",
			Usage:    "MongoDB TLS certificate key file",
			Sources:  cli.EnvVars("MONGODB__TLS_CERTIFICATE_KEY_FILE"),
			Category: categoryDatabase,
		},
	}
}
