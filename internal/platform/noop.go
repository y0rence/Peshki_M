package platform

import (
	"context"

	"vpnclient/internal/config"
)

// Prepare logs the active fallback behavior.
func (c *NoopController) Prepare(_ context.Context, profile config.Profile) error {
	c.logger.Warn(
		"native platform integration is not linked; client stays in local proxy mode",
		"protocol", profile.Protocol,
		"socks", profile.Local.SOCKSAddress,
		"http", profile.Local.HTTPAddress,
	)
	return nil
}

// Cleanup is a no-op for the local proxy fallback.
func (c *NoopController) Cleanup(_ context.Context, _ config.Profile) error {
	return nil
}
