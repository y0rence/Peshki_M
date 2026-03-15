package platform

import (
	"context"
	"log/slog"

	"vpnclient/internal/config"
)

// Controller models the native platform boundary.
type Controller interface {
	Prepare(context.Context, config.Profile) error
	Cleanup(context.Context, config.Profile) error
}

// Capabilities describes available native integration features.
type Capabilities struct {
	Platform            string `json:"platform"`
	SupportsSystemProxy bool   `json:"supports_system_proxy"`
	SupportsTun         bool   `json:"supports_tun"`
}

// NoopController is a safe default until the native bridge is linked in.
type NoopController struct {
	logger *slog.Logger
}

// NewNoopController creates a controller that keeps the client in local proxy mode.
func NewNoopController(logger *slog.Logger) *NoopController {
	if logger == nil {
		logger = slog.Default()
	}
	return &NoopController{
		logger: logger,
	}
}
