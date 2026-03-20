package app_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"vpnclient/internal/app"
	"vpnclient/internal/config"
	"vpnclient/internal/runtime"
)

type fakeController struct {
	prepared  atomic.Int32
	cleaned   atomic.Int32
	prepareFn func(context.Context, config.Profile) error
	cleanupFn func(context.Context, config.Profile) error
}

func (c *fakeController) Prepare(ctx context.Context, profile config.Profile) error {
	c.prepared.Add(1)
	if c.prepareFn != nil {
		return c.prepareFn(ctx, profile)
	}
	return nil
}

func (c *fakeController) Cleanup(ctx context.Context, profile config.Profile) error {
	c.cleaned.Add(1)
	if c.cleanupFn != nil {
		return c.cleanupFn(ctx, profile)
	}
	return nil
}

type fakeStarter struct {
	started atomic.Int32
	startFn func(context.Context, runtime.Plan) (runtime.Process, error)
}

func (s *fakeStarter) Start(ctx context.Context, plan runtime.Plan) (runtime.Process, error) {
	s.started.Add(1)
	if s.startFn != nil {
		return s.startFn(ctx, plan)
	}
	return &fakeProcess{}, nil
}

type fakeProcess struct {
	stopped atomic.Int32
	stopFn  func(context.Context) error
	waitFn  func() error
}

func (p *fakeProcess) Stop(ctx context.Context) error {
	p.stopped.Add(1)
	if p.stopFn != nil {
		return p.stopFn(ctx)
	}
	return nil
}

func (p *fakeProcess) Wait() error {
	if p.waitFn != nil {
		return p.waitFn()
	}
	return nil
}

func testProfile() config.Profile {
	return config.Profile{
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
}

func TestClientConnectDisconnect(t *testing.T) {
	t.Parallel()

	controller := &fakeController{}
	starter := &fakeStarter{}
	client := app.NewClient(slog.Default(), controller, starter)

	if err := client.Connect(context.Background(), testProfile()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if got, want := controller.prepared.Load(), int32(1); got != want {
		t.Fatalf("Prepare() was not called")
	}
	if got, want := starter.started.Load(), int32(1); got != want {
		t.Fatalf("Start() was not called")
	}

	if err := client.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if got, want := controller.cleaned.Load(), int32(1); got != want {
		t.Fatalf("Cleanup() was not called")
	}
}

func TestClientRejectsConcurrentConnect(t *testing.T) {
	t.Parallel()

	controller := &fakeController{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	starter := &fakeStarter{
		startFn: func(context.Context, runtime.Plan) (runtime.Process, error) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
			return &fakeProcess{}, nil
		},
	}
	client := app.NewClient(slog.Default(), controller, starter)

	profile := testProfile()
	errCh := make(chan error, 2)
	go func() {
		errCh <- client.Connect(context.Background(), profile)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("first Connect() did not reach starter")
	}

	go func() {
		errCh <- client.Connect(context.Background(), profile)
	}()

	time.Sleep(100 * time.Millisecond)
	close(release)

	err1 := <-errCh
	err2 := <-errCh

	nilCount := 0
	for _, err := range []error{err1, err2} {
		if err == nil {
			nilCount++
		}
	}
	if got, want := nilCount, 1; got != want {
		t.Fatalf("successful Connect() calls = %d, want %d (err1=%v, err2=%v)", got, want, err1, err2)
	}
	if got, want := starter.started.Load(), int32(1); got != want {
		t.Fatalf("Start() calls = %d, want %d", got, want)
	}
}

func TestClientRejectsDisconnectWhileConnecting(t *testing.T) {
	t.Parallel()

	controller := &fakeController{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	starter := &fakeStarter{
		startFn: func(context.Context, runtime.Plan) (runtime.Process, error) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
			return &fakeProcess{}, nil
		},
	}
	client := app.NewClient(slog.Default(), controller, starter)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Connect(context.Background(), testProfile())
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("first Connect() did not reach starter")
	}

	if err := client.Disconnect(context.Background()); err == nil {
		t.Fatalf("Disconnect() error = nil, want transition error")
	}
	if got, want := controller.cleaned.Load(), int32(0); got != want {
		t.Fatalf("Cleanup() calls = %d, want %d while connect is still in progress", got, want)
	}

	close(release)
	if err := <-errCh; err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if err := client.Disconnect(context.Background()); err != nil {
		t.Fatalf("final Disconnect() error = %v", err)
	}
}
