package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

var (
	ErrNoArguments = errors.New("no arguments")
)

// Run executes an external command with optional extra environment variables.
// stdout is forwarded to [os.Stdout]; stderr is captured and returned on failure.
func Run(ctx context.Context, args []string, extraEnv map[string]string, stdout io.Writer) error {
	if len(args) == 0 {
		return ErrNoArguments
	}

	binaryPath, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("binary '%s' not found: %w", args[0], err)
	}

	buf := bytes.Buffer{}

	cmd := exec.CommandContext(ctx, binaryPath, args[1:]...)
	cmd.Env = os.Environ()
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdout = stdout
	cmd.Stderr = &buf

	if runErr := cmd.Run(); runErr != nil {
		return fmt.Errorf("%s: %w", buf.String(), runErr)
	}
	return nil
}
