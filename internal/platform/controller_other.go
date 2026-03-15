//go:build !darwin

package platform

import "log/slog"

// NewController keeps non-macOS builds in local proxy mode.
func NewController(logger *slog.Logger) Controller {
	return NewNoopController(logger)
}
