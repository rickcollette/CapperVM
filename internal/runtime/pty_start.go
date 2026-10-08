package runtime

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

func startPTY(cmd *exec.Cmd) (*exec.Cmd, *os.File, error) {
	f, err := pty.Start(cmd)
	if err != nil {
		return nil, nil, fmt.Errorf("pty start: %w", err)
	}
	return cmd, f, nil
}
