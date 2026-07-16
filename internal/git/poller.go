package git

import (
	"fmt"
	"os/exec"
	"strings"
)

func Pull(repoRoot string) error {
	cmd := exec.Command("git", "-C", repoRoot, "pull", "--ff-only")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull --ff-only: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
