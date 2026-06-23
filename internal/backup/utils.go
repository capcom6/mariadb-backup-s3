package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

var (
	ErrNoArguments = errors.New("no arguments")
)

func run(ctx context.Context, args []string, extraEnv map[string]string) error {
	if len(args) == 0 {
		return ErrNoArguments
	}

	buf := bytes.Buffer{}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // is not user input

	cmd.Env = os.Environ()
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", buf.String(), err)
	}
	return nil
}
