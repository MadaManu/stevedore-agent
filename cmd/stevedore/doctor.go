package stevedore

import (
	"fmt"
	"io"

	"stevedore-agent/internal/doctor"

	"github.com/spf13/cobra"
)

var (
	runDoctorReport    = doctor.Run
	renderDoctorReport = doctor.RenderText
)

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect effective configuration and validate local stevedore setup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.OutOrStdout())
		},
	}
	return cmd
}

func runDoctor(w io.Writer) error {
	report := runDoctorReport()
	if _, err := io.WriteString(w, renderDoctorReport(report)); err != nil {
		return fmt.Errorf("write doctor report: %w", err)
	}
	if report.FailureCount() > 0 {
		return newConfigError(fmt.Sprintf("doctor found %d failing check(s)", report.FailureCount()))
	}
	return nil
}
