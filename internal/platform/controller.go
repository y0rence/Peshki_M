package platform

import (
	"context"
	"log/slog"

	"vpnclient/internal/config"
)

type Controller interface {
	Prepare(context.Context, config.Profile) error
	Cleanup(context.Context, config.Profile) error
}

type Capabilities struct {
	Platform            string `json:"platform"`
	SupportsSystemProxy bool   `json:"supports_system_proxy"`
	SupportsTun         bool   `json:"supports_tun"`
}

type NoopController struct {
	logger *slog.Logger
}

func NewNoopController(logger *slog.Logger) *NoopController {
	if logger == nil {
		logger = slog.Default()
	}
	return &NoopController{
		logger: logger,
	}
}
