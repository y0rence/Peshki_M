//go:build darwin

package platform

import "log/slog"

// NewController creates a macOS controller backed by the networksetup tool.
func NewController(logger *slog.Logger) Controller {
	return NewNetworkSetupController(logger)
}
