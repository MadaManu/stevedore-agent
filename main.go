package main

import (
	"fmt"
	mangoEnv "github.com/bitstep-ie/mango-go/pkg/env"
	"os"
	"stevedore-agent/cmd/stevedore"
	"stevedore-agent/internal/logging"
)

func main() {
	if err := logging.ConfigureDefault(resolveStartupLogDir()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	stevedore.Execute()
}

func resolveStartupLogDir() string {
	return mangoEnv.EnvOrDefault("STEVEDORE_LOG_DIR", "/var/log/")
}
