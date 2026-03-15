package app_test

import (
	"context"
	"log/slog"
	"testing"

	"vpnclient/internal/app"
	"vpnclient/internal/config"
	"vpnclient/internal/runtime"
)

type fakeController struct {
	prepared bool
	cleaned  bool
}

func (c *fakeController) Prepare(context.Context, config.Profile) error {
	c.prepared = true
	return nil
}

func (c *fakeController) Cleanup(context.Context, config.Profile) error {
	c.cleaned = true
	return nil
}

type fakeStarter struct {
	started bool
}

func (s *fakeStarter) Start(context.Context, runtime.Plan) (runtime.Process, error) {
	s.started = true
	return &fakeProcess{}, nil
}

type fakeProcess struct {
	stopped bool
}

func (p *fakeProcess) Stop(context.Context) error {
	p.stopped = true
	return nil
}

func (p *fakeProcess) Wait() error {
	return nil
}

func TestClientConnectDisconnect(t *testing.T) {
	t.Parallel()

	controller := &fakeController{}
	starter := &fakeStarter{}
	client := app.NewClient(slog.Default(), controller, starter)

	profile := config.Profile{
		Name:     "outline-test",
		Protocol: config.ProtocolOutline,
		Server: config.Server{
			Host: "198.51.100.10",
			Port: 443,
		},
		Credentials: config.Credentials{
			Method:   "chacha20-ietf-poly1305",
			Password: "secret",
		},
		Engine: config.Engine{
			Binary: "xray",
		},
	}

	if err := client.Connect(context.Background(), profile); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if !controller.prepared {
		t.Fatalf("Prepare() was not called")
	}
	if !starter.started {
		t.Fatalf("Start() was not called")
	}

	if err := client.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if !controller.cleaned {
		t.Fatalf("Cleanup() was not called")
	}
}
