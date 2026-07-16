package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	mangologger "github.com/bitstep-ie/mango-go/pkg/logger"
)

var (
	configMu sync.RWMutex
)

// logConfiguration holds the base mango configuration used for all logger instances.
var logConfiguration = &mangologger.LogConfig{
	MangoConfig: &mangologger.MangoConfig{
		Strict: false,
		CorrelationId: &mangologger.CorrelationIdConfig{
			Strict:       false,
			AutoGenerate: true,
		},
	},
	Out: &mangologger.OutConfig{
		Enabled: true,
		File: &mangologger.FileOutputConfig{
			Enabled:    true,
			Debug:      false,
			Path:       "",
			MaxSize:    100,
			MaxBackups: 5,
			MaxAge:     30,
			Compress:   true,
		},
		Cli: &mangologger.CliConfig{
			Enabled:  true,
			Friendly: true,
			FriendlyFormat: `"[\(.level)] \(.operation) | \(.message)"
				+ (if (.attributes.app // "") != "" then " | app=\(.attributes.app)" else "" end)
				+ (if (.attributes.status // "") != "" then " | status=\(.attributes.status)" else "" end)
				+ (if (.attributes.drift // "") != "" then " | drift=\(.attributes.drift)" else "" end)
				+ (if (.attributes.changes // "") != "" then " | changes=\(.attributes.changes)" else "" end)
				+ (if (.attributes.action // "") != "" then " | action=\(.attributes.action)" else "" end)
				+ (if (.attributes.error // "") != "" then " | error=\(.attributes.error)" else "" end)`,
			Verbose: false,
			VerboseFormat: `"[\(.level)] \(.operation) | \(.message)"
				+ (if (.attributes.app // "") != "" then " | app=\(.attributes.app)" else "" end)
				+ (if (.attributes.status // "") != "" then " | status=\(.attributes.status)" else "" end)
				+ (if (.attributes.drift // "") != "" then " | drift=\(.attributes.drift)" else "" end)
				+ (if (.attributes.changes // "") != "" then " | changes=\(.attributes.changes)" else "" end)
				+ (if (.attributes.action // "") != "" then " | action=\(.attributes.action)" else "" end)
				+ (if (.attributes.error // "") != "" then " | error=\(.attributes.error)" else "" end)`,
		},
		Syslog: &mangologger.SyslogConfig{Facility: ""},
	},
}

// SetDebug toggles debug/verbose behavior for subsequent ConfigureDefault calls.
func SetDebug(debug bool) {
	configMu.Lock()
	defer configMu.Unlock()
	logConfiguration.Out.File.Debug = debug
	logConfiguration.Out.Cli.Verbose = debug
}

// GetConfig returns a concrete mango config built from the shared template.
func GetConfig(logPath string) *mangologger.LogConfig {
	configMu.Lock()
	defer configMu.Unlock()
	logConfiguration.Out.File.Path = filepath.Join(logPath, "stevedore.log")
	return logConfiguration
}

// ConfigureDefault installs a mango-backed slog logger as the process default.
func ConfigureDefault(logDir string) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	slog.SetDefault(slog.New(mangologger.NewMangoLogger(GetConfig(logDir))))
	return nil
}

func businessContext(ctx context.Context, operation string) context.Context {
	ctx = context.WithValue(ctx, mangologger.OPERATION, "reconcile")
	if operation != "" {
		ctx = context.WithValue(ctx, mangologger.OPERATION, operation)
	}
	ctx = context.WithValue(ctx, mangologger.APPLICATION, "stevedore")
	ctx = context.WithValue(ctx, mangologger.TYPE, mangologger.BusinessType)
	return ctx
}

func securityContext(ctx context.Context, operation string) context.Context {
	ctx = businessContext(ctx, operation)
	ctx = context.WithValue(ctx, mangologger.TYPE, mangologger.SecurityType)
	return ctx
}

func BusinessContext(ctx context.Context, operation string) context.Context {
	return businessContext(ctx, operation)
}

func SecurityContext(ctx context.Context, operation string) context.Context {
	return securityContext(ctx, operation)
}
