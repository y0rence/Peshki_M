package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"vpnclient/internal/app"
	"vpnclient/internal/config"
	"vpnclient/internal/logging"
	"vpnclient/internal/platform"
	"vpnclient/internal/runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "vpnclient: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var configPath string
	var command string
	var logLevel string

	flag.StringVar(&configPath, "config", "", "Path to a JSON VPN profile.")
	flag.StringVar(&command, "command", "validate", "Command to run: validate, print, or connect.")
	flag.StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, or error.")
	flag.Parse()

	if configPath == "" {
		return errors.New("missing -config")
	}

	logger, err := logging.NewLogger(logLevel)
	if err != nil {
		return err
	}

	profile, err := config.LoadProfile(configPath)
	if err != nil {
		return err
	}

	switch command {
	case "validate":
		logger.Info(
			"profile validated",
			"name", profile.Name,
			"protocol", profile.Protocol,
			"server", fmt.Sprintf("%s:%d", profile.Server.Host, profile.Server.Port),
		)
		return nil
	case "print":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(profile)
	case "connect":
		return runConnect(logger, profile)
	default:
		return fmt.Errorf("unsupported command %q", command)
	}
}

func runConnect(logger *logging.Logger, profile config.Profile) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := app.NewClient(
		logger.Slog(),
		platform.NewController(logger.Slog()),
		runtime.NewLauncher(logger.Slog()),
	)

	if err := client.Connect(ctx, profile); err != nil {
		return err
	}

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return client.Disconnect(shutdownCtx)
}
