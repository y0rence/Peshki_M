package platform

import (
	"context"

	"vpnclient/internal/config"
)

func (c *NoopController) Prepare(_ context.Context, profile config.Profile) error {
	c.logger.Warn(
		"native platform integration is not linked; client stays in local proxy mode",
		"protocol", profile.Protocol,
		"socks", profile.Local.SOCKSAddress,
		"http", profile.Local.HTTPAddress,
	)
	return nil
}

func (c *NoopController) Cleanup(_ context.Context, _ config.Profile) error {
	return nil
}
