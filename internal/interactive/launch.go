package interactive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Launch forwards each path as an argument, never as a shell command string.
// Waiting keeps Git's temporary inputs alive for the whole UI operation.
func Launch(ctx context.Context, o Options, binary string, stdout, stderr io.Writer) int {
	if binary == "" {
		self, err := os.Executable()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return Failure
		}
		name := "xmlmerge-ui"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binary = filepath.Join(filepath.Dir(self), name)
	}
	cmd := exec.CommandContext(ctx, binary, o.Args()...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if err == nil {
		return Success
	}
	if ctx.Err() != nil {
		return Cancelled
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if exit.ExitCode() == Cancelled {
			return Cancelled
		}
		fmt.Fprintf(stderr, "интерактивное приложение завершилось с кодом %d\n", exit.ExitCode())
		return Failure
	}
	fmt.Fprintf(stderr, "не удалось запустить %s: %v\n", binary, err)
	return Failure
}
