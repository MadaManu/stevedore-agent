package stevedore

import (
	"fmt"
	"io"
	"strings"

	"stevedore-agent/internal/buildinfo"

	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the installed stevedore-agent version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVersion(cmd.OutOrStdout())
		},
	}
	return cmd
}

func runVersion(w io.Writer) error {
	summary := buildinfo.Summary()
	if strings.TrimSpace(summary) == "" {
		summary = "dev"
	}
	if _, err := fmt.Fprintf(w, "stevedore-agent %s\n", summary); err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	return nil
}
