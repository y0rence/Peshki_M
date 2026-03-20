package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	startupTimeout      = 5 * time.Second
	startupPollInterval = 100 * time.Millisecond
	outputTailLimit     = 20
)

type Launcher struct {
	logger                        *slog.Logger
	newCommand                    func(context.Context, string, ...string) *exec.Cmd
	ensureReadyAddressesAvailable func([]string) error
	addressesAreReachable         func([]string) (bool, error)
}

func NewLauncher(logger *slog.Logger) *Launcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Launcher{
		logger:                        logger,
		newCommand:                    exec.CommandContext,
		ensureReadyAddressesAvailable: ensureReadyAddressesAvailable,
		addressesAreReachable:         addressesAreReachable,
	}
}

func (l *Launcher) Start(ctx context.Context, plan Plan) (Process, error) {
	if err := l.ensureReadyAddressesAvailable(plan.ReadyAddresses); err != nil {
		return nil, err
	}

	configDir, err := os.MkdirTemp("", "vpnclient-*")
	if err != nil {
		return nil, fmt.Errorf("create runtime config dir: %w", err)
	}

	configPath := filepath.Join(configDir, plan.ConfigFileName)
	if err := os.WriteFile(configPath, plan.ConfigData, 0o600); err != nil {
		_ = os.RemoveAll(configDir)
		return nil, fmt.Errorf("write runtime config: %w", err)
	}

	args := replaceConfigPlaceholder(plan.Args, configPath)
	command := l.newCommand(context.WithoutCancel(ctx), plan.Binary, args...)
	command.Dir = plan.WorkingDir
	command.Env = append(os.Environ(), environmentEntries(plan.Environment)...)

	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = os.RemoveAll(configDir)
		return nil, fmt.Errorf("attach stdout pipe: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = os.RemoveAll(configDir)
		return nil, fmt.Errorf("attach stderr pipe: %w", err)
	}

	if err := command.Start(); err != nil {
		_ = os.RemoveAll(configDir)
		return nil, fmt.Errorf("start runtime %q: %w", plan.Binary, err)
	}

	handle := &Handle{
		logger:                l.logger,
		command:               command,
		configDir:             configDir,
		done:                  make(chan struct{}),
		addressesAreReachable: l.addressesAreReachable,
	}

	go handle.streamOutput("stdout", stdout)
	go handle.streamOutput("stderr", stderr)
	go handle.waitForExit()

	if err := handle.waitUntilReady(ctx, plan.ReadyAddresses); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = handle.Stop(stopCtx)
		return nil, err
	}

	l.logger.Info("runtime started", "binary", plan.Binary, "pid", command.Process.Pid)

	return handle, nil
}

type Handle struct {
	logger                *slog.Logger
	command               *exec.Cmd
	configDir             string
	done                  chan struct{}
	waitErr               error
	cleanupOnce           sync.Once
	stopRequested         atomic.Bool
	outputMu              sync.Mutex
	recentOutput          []string
	addressesAreReachable func([]string) (bool, error)
}

func (h *Handle) Wait() error {
	<-h.done
	return h.waitErr
}

func (h *Handle) Stop(ctx context.Context) error {
	h.stopRequested.Store(true)

	if h.command.Process == nil {
		<-h.done
		return nil
	}

	_ = h.command.Process.Signal(os.Interrupt)

	select {
	case <-h.done:
		if h.waitErr != nil {
			return nil
		}
		return nil
	case <-ctx.Done():
		_ = h.command.Process.Kill()

		select {
		case <-h.done:
			return nil
		case <-time.After(500 * time.Millisecond):
			return ctx.Err()
		}
	}
}

func (h *Handle) waitForExit() {
	h.waitErr = h.command.Wait()
	if h.waitErr != nil && !h.stopRequested.Load() {
		h.logger.Error("runtime exited with error", "error", h.waitErr)
	}
	h.cleanup()
	close(h.done)
}

func (h *Handle) cleanup() {
	h.cleanupOnce.Do(func() {
		if h.configDir != "" {
			_ = os.RemoveAll(h.configDir)
		}
	})
}

func (h *Handle) streamOutput(stream string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		h.recordOutput(stream, line)
		h.logger.Info("runtime output", "stream", stream, "line", line)
	}
	if err := scanner.Err(); err != nil {
		h.logger.Warn("runtime output stream failed", "stream", stream, "error", err)
	}
}

func (h *Handle) waitUntilReady(ctx context.Context, readyAddresses []string) error {
	startupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	if len(readyAddresses) == 0 {
		select {
		case <-h.done:
			return h.startupError("runtime exited before it became ready")
		case <-time.After(250 * time.Millisecond):
			return nil
		case <-startupCtx.Done():
			return h.startupError("timed out waiting for runtime startup")
		}
	}

	ticker := time.NewTicker(startupPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.done:
			return h.startupError("runtime exited before it became ready")
		case <-startupCtx.Done():
			return h.startupError("timed out waiting for runtime readiness")
		case <-ticker.C:
			ready, err := h.addressesAreReachable(readyAddresses)
			if err != nil {
				return err
			}
			if ready {
				return nil
			}
		}
	}
}

func (h *Handle) recordOutput(stream string, line string) {
	h.outputMu.Lock()
	defer h.outputMu.Unlock()

	entry := stream + ": " + line
	h.recentOutput = append(h.recentOutput, entry)
	if len(h.recentOutput) > outputTailLimit {
		h.recentOutput = append([]string(nil), h.recentOutput[len(h.recentOutput)-outputTailLimit:]...)
	}
}

func (h *Handle) recentOutputText() string {
	h.outputMu.Lock()
	defer h.outputMu.Unlock()

	if len(h.recentOutput) == 0 {
		return ""
	}
	return strings.Join(h.recentOutput, " | ")
}

func (h *Handle) startupError(message string) error {
	if exitErr := h.exitError(); exitErr != nil {
		if output := h.recentOutputText(); output != "" {
			return fmt.Errorf("%s: %w; output: %s", message, exitErr, output)
		}
		return fmt.Errorf("%s: %w", message, exitErr)
	}

	if output := h.recentOutputText(); output != "" {
		return fmt.Errorf("%s: %s", message, output)
	}

	return errors.New(message)
}

func (h *Handle) exitError() error {
	select {
	case <-h.done:
		return h.waitErr
	default:
		return nil
	}
}

func replaceConfigPlaceholder(args []string, configPath string) []string {
	resolved := make([]string, len(args))
	for index, arg := range args {
		if arg == configPathPlaceholder {
			resolved[index] = configPath
			continue
		}
		resolved[index] = arg
	}
	return resolved
}

func environmentEntries(environment map[string]string) []string {
	if len(environment) == 0 {
		return nil
	}

	entries := make([]string, 0, len(environment))
	for key, value := range environment {
		entries = append(entries, key+"="+value)
	}
	return entries
}

func ensureReadyAddressesAvailable(addresses []string) error {
	for _, address := range addresses {
		if address == "" {
			continue
		}

		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("local address %q is not available: %w", address, err)
		}
		_ = listener.Close()
	}
	return nil
}

func addressesAreReachable(addresses []string) (bool, error) {
	for _, address := range addresses {
		if address == "" {
			continue
		}

		connection, err := net.DialTimeout("tcp", address, 150*time.Millisecond)
		if err != nil {
			return false, nil
		}
		_ = connection.Close()
	}

	return true, nil
}
