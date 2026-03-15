package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"vpnclient/internal/app"
	"vpnclient/internal/config"
	"vpnclient/internal/logging"
	"vpnclient/internal/platform"
	"vpnclient/internal/runtime"
	"vpnclient/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "vpnclient-ui: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var listenAddress string
	var logLevel string
	var openBrowser bool
	var configPath string
	var supportURL string

	flag.StringVar(&listenAddress, "listen", "127.0.0.1:18080", "HTTP listen address for the control panel.")
	flag.StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, or error.")
	flag.BoolVar(&openBrowser, "open-browser", true, "Open the control panel in the default browser.")
	flag.StringVar(&configPath, "config", "", "Optional path to a default JSON VPN profile.")
	flag.StringVar(&supportURL, "support-url", "", "Optional support URL opened from the control panel.")
	flag.Parse()

	logger, err := logging.NewLogger(logLevel)
	if err != nil {
		return err
	}

	client := app.NewClient(
		logger.Slog(),
		platform.NewController(logger.Slog()),
		runtime.NewLauncher(logger.Slog()),
	)

	var defaultProfile *config.Profile
	if configPath != "" {
		profile, err := config.LoadProfile(configPath)
		if err != nil {
			return fmt.Errorf("load default profile: %w", err)
		}
		defaultProfile = &profile
	}

	controlPanel := ui.NewServerWithOptions(logger.Slog(), client, ui.ServerOptions{
		DefaultProfile: defaultProfile,
		SupportURL:     supportURL,
	})

	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", listenAddress, err)
	}
	defer listener.Close()

	server := &http.Server{
		Handler:           controlPanel.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	url := "http://" + listener.Addr().String()
	logger.Info("vpn control panel listening", "url", url)

	if openBrowser {
		go func() {
			time.Sleep(250 * time.Millisecond)
			if err := ui.OpenBrowser(url); err != nil {
				logger.Warn("failed to open browser", "url", url, "error", err)
			}
		}()
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Serve(listener)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	select {
	case err := <-serverErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serverErr := server.Shutdown(shutdownCtx)
	controlPanelErr := controlPanel.Shutdown(shutdownCtx)

	if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
		return serverErr
	}
	return controlPanelErr
}
