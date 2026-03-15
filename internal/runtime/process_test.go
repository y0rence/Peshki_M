package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLauncherStartFailsWhenReadyAddressIsBusy(t *testing.T) {
	t.Parallel()

	launcher := NewLauncher(nil)
	started := false
	launcher.ensureReadyAddressesAvailable = func(addresses []string) error {
		if got, want := len(addresses), 1; got != want {
			t.Fatalf("len(addresses) = %d, want %d", got, want)
		}
		return fmt.Errorf("address already in use")
	}
	launcher.newCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		started = true
		return exec.CommandContext(ctx, "sh", "-c", "sleep 1")
	}

	_, err := launcher.Start(context.Background(), Plan{
		Binary:         "ignored",
		ConfigFileName: "test.json",
		ConfigData:     []byte("{}"),
		ReadyAddresses: []string{"127.0.0.1:1080"},
	})
	if err == nil {
		t.Fatalf("Start() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("Start() error = %q, want local address availability failure", err)
	}
	if started {
		t.Fatalf("command started = true, want false")
	}
}

func TestLauncherStartFailsWhenRuntimeExitsBeforeReady(t *testing.T) {
	t.Parallel()

	launcher := NewLauncher(nil)
	launcher.ensureReadyAddressesAvailable = func([]string) error {
		return nil
	}
	launcher.addressesAreReachable = func([]string) (bool, error) {
		return false, nil
	}
	launcher.newCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "exit 7")
	}

	_, err := launcher.Start(context.Background(), Plan{
		Binary:         "ignored",
		ConfigFileName: "test.json",
		ConfigData:     []byte("{}"),
		ReadyAddresses: []string{"127.0.0.1:1080"},
	})
	if err == nil {
		t.Fatalf("Start() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "runtime exited before it became ready") {
		t.Fatalf("Start() error = %q, want startup failure", err)
	}
}

func TestLauncherStartDoesNotTieRuntimeLifetimeToCallerContext(t *testing.T) {
	t.Parallel()

	launcher := NewLauncher(nil)
	launcher.ensureReadyAddressesAvailable = func([]string) error {
		return nil
	}
	launcher.newCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "sleep 5")
	}

	ctx, cancel := context.WithCancel(context.Background())
	process, err := launcher.Start(ctx, Plan{
		Binary:         "ignored",
		ConfigFileName: "test.json",
		ConfigData:     []byte("{}"),
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	handle, ok := process.(*Handle)
	if !ok {
		t.Fatalf("process type = %T, want *Handle", process)
	}

	cancel()

	select {
	case <-handle.done:
		t.Fatalf("runtime exited after caller context cancellation")
	case <-time.After(200 * time.Millisecond):
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := process.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}
