package stevedore

import (
	"bytes"
	"strings"
	"testing"

	"stevedore-agent/internal/config"
	"stevedore-agent/internal/doctor"
)

func TestRunDoctor_ReturnsErrorWhenReportHasFailures(t *testing.T) {
	oldRun := runDoctorReport
	oldRender := renderDoctorReport
	defer func() {
		runDoctorReport = oldRun
		renderDoctorReport = oldRender
	}()

	runDoctorReport = func() doctor.Report {
		return doctor.Report{
			ConfigPath:       "/tmp/config.yml",
			ConfigPathOrigin: config.SettingOrigin{Source: config.SettingSourceDefault},
			Checks:           []doctor.CheckResult{{Name: "config file", Status: doctor.StatusFail, Summary: "missing"}},
		}
	}
	renderDoctorReport = func(report doctor.Report) string {
		return "doctor output\n"
	}

	var buf bytes.Buffer
	err := runDoctor(&buf)
	if err == nil {
		t.Fatal("expected failing doctor report to return an error")
	}
	if !strings.Contains(err.Error(), "doctor found 1 failing check") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := buf.String(); got != "doctor output\n" {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunDoctor_SucceedsWhenReportHasNoFailures(t *testing.T) {
	oldRun := runDoctorReport
	oldRender := renderDoctorReport
	defer func() {
		runDoctorReport = oldRun
		renderDoctorReport = oldRender
	}()

	runDoctorReport = func() doctor.Report {
		return doctor.Report{
			ConfigPath:       "/tmp/config.yml",
			ConfigPathOrigin: config.SettingOrigin{Source: config.SettingSourceDefault},
			Checks:           []doctor.CheckResult{{Name: "config file", Status: doctor.StatusPass, Summary: "ok"}},
		}
	}
	renderDoctorReport = func(report doctor.Report) string {
		return "all good\n"
	}

	var buf bytes.Buffer
	if err := runDoctor(&buf); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := buf.String(); got != "all good\n" {
		t.Fatalf("unexpected output %q", got)
	}
}
