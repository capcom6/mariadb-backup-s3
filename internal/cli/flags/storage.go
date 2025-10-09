package flags

import "github.com/urfave/cli/v3"

func Storage() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "storage-url",
			Aliases:  []string{"storage"},
			Usage:    "storage URL (e.g., s3://bucket/path, file:///path, ftp://host/path)",
			Required: true,
			Sources:  cli.EnvVars("STORAGE__URL"),
		},
	}
}
