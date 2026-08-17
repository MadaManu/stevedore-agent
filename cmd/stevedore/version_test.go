package stevedore

import (
	"bytes"
	"strings"
	"testing"

	"stevedore-agent/internal/buildinfo"
)

func TestRunVersion(t *testing.T) {
	oldVersion := buildinfo.Version
	oldCommit := buildinfo.Commit
	oldBuildDate := buildinfo.BuildDate
	defer func() {
		buildinfo.Version = oldVersion
		buildinfo.Commit = oldCommit
		buildinfo.BuildDate = oldBuildDate
	}()

	buildinfo.Version = "v0.1.0"
	buildinfo.Commit = "abc123"
	buildinfo.BuildDate = "2026-08-17T12:00:00Z"

	var out bytes.Buffer
	if err := runVersion(&out); err != nil {
		t.Fatal(err)
	}

	text := strings.TrimSpace(out.String())
	if text != "stevedore-agent v0.1.0 abc123 2026-08-17T12:00:00Z" {
		t.Fatalf("unexpected version output: %q", text)
	}
}
