//go:build !darwin

package platform

import "log/slog"

func NewController(logger *slog.Logger) Controller {
	return NewNoopController(logger)
}
