package flags

import "github.com/urfave/cli/v3"

func Encryption() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "encryption-key",
			Usage:    "base64 encoded encryption key",
			Sources:  cli.EnvVars("ENCRYPTION__KEY"),
			Category: "Encryption",
		},
	}
}
