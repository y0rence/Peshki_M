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

var errSessionTransitionInProgress = errors.New("a VPN session transition is already in progress")

type sessionState int

const (
	sessionStateIdle sessionState = iota
	sessionStateConnecting
	sessionStateConnected
	sessionStateDisconnecting
)

type Client struct {
	logger     *slog.Logger
	controller platform.Controller
	starter    runtime.Starter

	mu      sync.Mutex
	state   sessionState
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
	switch c.state {
	case sessionStateConnected:
		c.mu.Unlock()
		return fmt.Errorf("a VPN session is already active")
	case sessionStateConnecting, sessionStateDisconnecting:
		c.mu.Unlock()
		return errSessionTransitionInProgress
	}
	c.state = sessionStateConnecting
	c.mu.Unlock()

	resetState := func() {
		c.mu.Lock()
		c.state = sessionStateIdle
		c.mu.Unlock()
	}

	profile = profile.WithDefaults()
	if err := profile.Validate(); err != nil {
		resetState()
		return err
	}

	adapter, err := runtime.AdapterFor(profile.Protocol, c.logger)
	if err != nil {
		resetState()
		return err
	}

	plan, err := adapter.BuildPlan(profile)
	if err != nil {
		resetState()
		return err
	}

	if err := c.controller.Prepare(ctx, profile); err != nil {
		resetState()
		return fmt.Errorf("prepare platform: %w", err)
	}

	process, err := c.starter.Start(ctx, plan)
	if err != nil {
		cleanupErr := c.controller.Cleanup(ctx, profile)
		resetState()
		return errors.Join(err, cleanupErr)
	}

	c.mu.Lock()
	c.state = sessionStateConnected
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
	switch c.state {
	case sessionStateIdle:
		c.mu.Unlock()
		return nil
	case sessionStateConnecting, sessionStateDisconnecting:
		c.mu.Unlock()
		return errSessionTransitionInProgress
	}

	process := c.process
	profile := c.profile
	c.state = sessionStateDisconnecting
	c.process = nil
	c.profile = config.Profile{}
	c.mu.Unlock()

	stopErr := process.Stop(ctx)
	cleanupErr := c.controller.Cleanup(ctx, profile)

	c.mu.Lock()
	c.state = sessionStateIdle
	c.mu.Unlock()

	return errors.Join(stopErr, cleanupErr)
}
