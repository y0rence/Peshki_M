package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"vpnclient/internal/config"
	"vpnclient/internal/platform"
	"vpnclient/internal/runtime"
)

type Client struct {
	logger     *slog.Logger
	controller platform.Controller
	starter    runtime.Starter

	mu      sync.Mutex
	process runtime.Process
	profile config.Profile
}

func NewClient(
	logger *slog.Logger,
	controller platform.Controller,
	starter runtime.Starter,
) *Client {
	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		logger:     logger,
		controller: controller,
		starter:    starter,
	}
}

func (c *Client) Connect(ctx context.Context, profile config.Profile) error {
	c.mu.Lock()
	if c.process != nil {
		c.mu.Unlock()
		return fmt.Errorf("a VPN session is already active")
	}
	c.mu.Unlock()

	profile = profile.WithDefaults()
	if err := profile.Validate(); err != nil {
		return err
	}

	adapter, err := runtime.AdapterFor(profile.Protocol, c.logger)
	if err != nil {
		return err
	}

	plan, err := adapter.BuildPlan(profile)
	if err != nil {
		return err
	}

	if err := c.controller.Prepare(ctx, profile); err != nil {
		return fmt.Errorf("prepare platform: %w", err)
	}

	process, err := c.starter.Start(ctx, plan)
	if err != nil {
		cleanupErr := c.controller.Cleanup(ctx, profile)
		return errors.Join(err, cleanupErr)
	}

	c.mu.Lock()
	c.process = process
	c.profile = profile
	c.mu.Unlock()

	c.logger.Info(
		"vpn session connected",
		"name", profile.Name,
		"protocol", profile.Protocol,
		"socks", profile.Local.SOCKSAddress,
		"http", profile.Local.HTTPAddress,
	)

	return nil
}

func (c *Client) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	process := c.process
	profile := c.profile
	c.process = nil
	c.profile = config.Profile{}
	c.mu.Unlock()

	if process == nil {
		return nil
	}

	stopErr := process.Stop(ctx)
	cleanupErr := c.controller.Cleanup(ctx, profile)

	return errors.Join(stopErr, cleanupErr)
}
