//go:build darwin

package platform

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"vpnclient/internal/config"
)

type commandRunner func(context.Context, string, ...string) ([]byte, error)

type networkService struct {
	Name   string
	Device string
}

type proxyKind string

const (
	proxyKindWeb       proxyKind = "web"
	proxyKindSecureWeb proxyKind = "secureweb"
	proxyKindSOCKS     proxyKind = "socks"
)

type proxySetting struct {
	Enabled       bool
	Server        string
	Port          int
	Authenticated bool
	Username      string
}

type serviceSnapshot struct {
	Service networkService
	Web     proxySetting
	Secure  proxySetting
	SOCKS   proxySetting
}

type conflictingTunnelError struct {
	Interface string
}

func (e *conflictingTunnelError) Error() string {
	return fmt.Sprintf(
		"another VPN or packet tunnel is already active (default route via %s); disable Shadowrocket or any other VPN/TUN app first",
		e.Interface,
	)
}

type NetworkSetupController struct {
	logger    *slog.Logger
	run       commandRunner
	mu        sync.Mutex
	snapshots []serviceSnapshot
}

func NewNetworkSetupController(logger *slog.Logger) *NetworkSetupController {
	if logger == nil {
		logger = slog.Default()
	}

	return &NetworkSetupController{
		logger: logger,
		run:    defaultCommandRunner,
	}
}

func (c *NetworkSetupController) Prepare(ctx context.Context, profile config.Profile) error {
	if err := c.prepareSystemProxy(ctx, profile); err != nil {
		var conflictErr *conflictingTunnelError
		if errors.As(err, &conflictErr) {
			return err
		}

		c.logger.Warn(
			"macOS system proxy setup failed; client stays in local proxy mode",
			"protocol", profile.Protocol,
			"socks", profile.Local.SOCKSAddress,
			"http", profile.Local.HTTPAddress,
			"error", err,
		)

		if cleanupErr := c.Cleanup(ctx, profile); cleanupErr != nil {
			c.logger.Warn("failed to restore macOS proxy state after setup error", "error", cleanupErr)
		}
	}

	return nil
}

func (c *NetworkSetupController) prepareSystemProxy(
	ctx context.Context,
	profile config.Profile,
) error {
	if err := c.ensureNoConflictingDefaultRoute(ctx, profile.Metadata["outbound_interface"]); err != nil {
		return err
	}

	socksHost, socksPort, err := splitHostPort(profile.Local.SOCKSAddress)
	if err != nil {
		return fmt.Errorf("parse local SOCKS address: %w", err)
	}
	httpHost, httpPort, err := splitHostPort(profile.Local.HTTPAddress)
	if err != nil {
		return fmt.Errorf("parse local HTTP address: %w", err)
	}

	services, err := c.listEligibleServices(ctx)
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return fmt.Errorf("no eligible macOS network services found")
	}

	snapshots := make([]serviceSnapshot, 0, len(services))
	for _, service := range services {
		snapshot, err := c.snapshotService(ctx, service)
		if err != nil {
			return err
		}
		snapshots = append(snapshots, snapshot)
	}

	c.mu.Lock()
	c.snapshots = snapshots
	c.mu.Unlock()

	for _, snapshot := range snapshots {
		service := snapshot.Service
		if err := c.setProxy(ctx, service.Name, proxyKindSOCKS, socksHost, socksPort); err != nil {
			return err
		}
		if err := c.setProxy(ctx, service.Name, proxyKindWeb, httpHost, httpPort); err != nil {
			return err
		}
		if err := c.setProxy(ctx, service.Name, proxyKindSecureWeb, httpHost, httpPort); err != nil {
			return err
		}
	}

	c.logger.Info(
		"macOS system proxy enabled",
		"services", len(snapshots),
		"socks", profile.Local.SOCKSAddress,
		"http", profile.Local.HTTPAddress,
	)

	return nil
}

func (c *NetworkSetupController) Cleanup(ctx context.Context, _ config.Profile) error {
	c.mu.Lock()
	snapshots := append([]serviceSnapshot(nil), c.snapshots...)
	c.snapshots = nil
	c.mu.Unlock()

	if len(snapshots) == 0 {
		return nil
	}

	var restoreErrors []error
	for _, snapshot := range snapshots {
		if err := c.restoreProxy(ctx, snapshot.Service.Name, proxyKindWeb, snapshot.Web); err != nil {
			restoreErrors = append(restoreErrors, err)
		}
		if err := c.restoreProxy(ctx, snapshot.Service.Name, proxyKindSecureWeb, snapshot.Secure); err != nil {
			restoreErrors = append(restoreErrors, err)
		}
		if err := c.restoreProxy(ctx, snapshot.Service.Name, proxyKindSOCKS, snapshot.SOCKS); err != nil {
			restoreErrors = append(restoreErrors, err)
		}
	}

	if len(restoreErrors) > 0 {
		return joinErrors("restore macOS proxy settings", restoreErrors)
	}

	c.logger.Info("macOS system proxy restored", "services", len(snapshots))
	return nil
}

func (c *NetworkSetupController) listEligibleServices(ctx context.Context) ([]networkService, error) {
	output, err := c.run(ctx, "networksetup", "-listnetworkserviceorder")
	if err != nil {
		return nil, fmt.Errorf("list macOS network services: %w", err)
	}

	services, err := parseNetworkServiceOrder(output)
	if err != nil {
		return nil, err
	}

	result := make([]networkService, 0, len(services))
	for _, service := range services {
		if service.Device == "" {
			continue
		}
		result = append(result, service)
	}
	if len(result) > 0 {
		return result, nil
	}

	fallback, err := parseNetworkServiceNames(output)
	if err != nil {
		return nil, err
	}
	if len(fallback) > 0 {
		c.logger.Warn(
			"falling back to service-name-only macOS proxy discovery",
			"services", len(fallback),
		)
	}
	return fallback, nil
}

func (c *NetworkSetupController) snapshotService(ctx context.Context, service networkService) (serviceSnapshot, error) {
	web, err := c.getProxy(ctx, service.Name, proxyKindWeb)
	if err != nil {
		return serviceSnapshot{}, err
	}
	secure, err := c.getProxy(ctx, service.Name, proxyKindSecureWeb)
	if err != nil {
		return serviceSnapshot{}, err
	}
	socks, err := c.getProxy(ctx, service.Name, proxyKindSOCKS)
	if err != nil {
		return serviceSnapshot{}, err
	}

	for _, setting := range []proxySetting{web, secure, socks} {
		if setting.Authenticated {
			return serviceSnapshot{}, fmt.Errorf(
				"service %q uses an authenticated proxy, which is not supported for automatic restore yet",
				service.Name,
			)
		}
	}

	return serviceSnapshot{
		Service: service,
		Web:     web,
		Secure:  secure,
		SOCKS:   socks,
	}, nil
}

func (c *NetworkSetupController) getProxy(
	ctx context.Context,
	serviceName string,
	kind proxyKind,
) (proxySetting, error) {
	output, err := c.run(ctx, "networksetup", getProxyCommand(kind), serviceName)
	if err != nil {
		return proxySetting{}, fmt.Errorf("read %s proxy for service %q: %w", kind, serviceName, err)
	}
	setting, err := parseProxySetting(output)
	if err != nil {
		return proxySetting{}, fmt.Errorf("parse %s proxy for service %q: %w", kind, serviceName, err)
	}
	return setting, nil
}

func (c *NetworkSetupController) setProxy(
	ctx context.Context,
	serviceName string,
	kind proxyKind,
	host string,
	port int,
) error {
	_, err := c.run(
		ctx,
		"networksetup",
		setProxyCommand(kind),
		serviceName,
		host,
		strconv.Itoa(port),
		"off",
	)
	if err != nil {
		return fmt.Errorf("set %s proxy for service %q: %w", kind, serviceName, err)
	}
	return nil
}

func (c *NetworkSetupController) restoreProxy(
	ctx context.Context,
	serviceName string,
	kind proxyKind,
	setting proxySetting,
) error {
	if setting.Authenticated {
		return fmt.Errorf(
			"cannot restore authenticated %s proxy for service %q without stored credentials",
			kind,
			serviceName,
		)
	}

	if !setting.Enabled {
		_, err := c.run(ctx, "networksetup", setProxyStateCommand(kind), serviceName, "off")
		if err != nil {
			return fmt.Errorf("disable %s proxy for service %q: %w", kind, serviceName, err)
		}
		return nil
	}

	_, err := c.run(
		ctx,
		"networksetup",
		setProxyCommand(kind),
		serviceName,
		setting.Server,
		strconv.Itoa(setting.Port),
		"off",
	)
	if err != nil {
		return fmt.Errorf("restore %s proxy for service %q: %w", kind, serviceName, err)
	}
	return nil
}

func defaultCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, trimmedOutput)
	}
	return output, nil
}

func (c *NetworkSetupController) ensureNoConflictingDefaultRoute(
	ctx context.Context,
	outboundInterface string,
) error {
	output, err := c.run(ctx, "route", "get", "default")
	if err != nil {
		return fmt.Errorf("inspect default route: %w", err)
	}

	defaultInterface, err := parseDefaultRouteInterface(output)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(defaultInterface, "utun") {
		return nil
	}
	if strings.TrimSpace(outboundInterface) != "" {
		c.logger.Warn(
			"another VPN or packet tunnel is active, but Xray is pinned to a specific interface",
			"default_route_interface", defaultInterface,
			"outbound_interface", outboundInterface,
		)
		return nil
	}

	return &conflictingTunnelError{
		Interface: defaultInterface,
	}
}

func parseNetworkServiceOrder(output []byte) ([]networkService, error) {
	namePattern := regexp.MustCompile(`^\(\d+\)\s+(.*)$`)
	devicePattern := regexp.MustCompile(`^\(Hardware Port:\s*.*,\s*Device:\s*(.*)\)$`)

	var services []networkService
	var currentName string
	var currentDisabled bool

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if matches := namePattern.FindStringSubmatch(line); matches != nil {
			currentName = strings.TrimSpace(matches[1])
			currentDisabled = strings.HasPrefix(currentName, "*")
			currentName = strings.TrimSpace(strings.TrimPrefix(currentName, "*"))
			continue
		}

		if matches := devicePattern.FindStringSubmatch(line); matches != nil && currentName != "" {
			device := strings.TrimSpace(matches[1])
			if !currentDisabled {
				services = append(services, networkService{
					Name:   currentName,
					Device: device,
				})
			}
			currentName = ""
			currentDisabled = false
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan network service order: %w", err)
	}

	return services, nil
}

func parseDefaultRouteInterface(output []byte) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if strings.TrimSpace(key) != "interface" {
			continue
		}

		value = strings.TrimSpace(value)
		if value == "" {
			return "", fmt.Errorf("default route interface is empty")
		}
		return value, nil
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan default route output: %w", err)
	}

	return "", fmt.Errorf("default route interface not found")
}

func parseNetworkServiceNames(output []byte) ([]networkService, error) {
	namePattern := regexp.MustCompile(`^\(\d+\)\s+(.*)$`)

	var services []networkService

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		matches := namePattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		name := strings.TrimSpace(matches[1])
		if strings.HasPrefix(name, "*") {
			continue
		}
		services = append(services, networkService{
			Name: name,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan network service names: %w", err)
	}

	return services, nil
}

func parseProxySetting(output []byte) (proxySetting, error) {
	var setting proxySetting

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "Enabled":
			enabled, err := parseYesNo(value)
			if err != nil {
				return proxySetting{}, err
			}
			setting.Enabled = enabled
		case "Server":
			setting.Server = value
		case "Port":
			if value == "" {
				setting.Port = 0
				continue
			}
			port, err := strconv.Atoi(value)
			if err != nil {
				return proxySetting{}, fmt.Errorf("parse proxy port %q: %w", value, err)
			}
			setting.Port = port
		case "Authenticated Proxy Enabled":
			authenticated, err := parseOneZero(value)
			if err != nil {
				return proxySetting{}, err
			}
			setting.Authenticated = authenticated
		case "Username":
			setting.Username = value
		}
	}

	if err := scanner.Err(); err != nil {
		return proxySetting{}, fmt.Errorf("scan proxy setting: %w", err)
	}

	return setting, nil
}

func parseYesNo(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "on", "true":
		return true, nil
	case "no", "off", "false":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected boolean value %q", value)
	}
}

func parseOneZero(value string) (bool, error) {
	switch strings.TrimSpace(value) {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected one-zero value %q", value)
	}
}

func getProxyCommand(kind proxyKind) string {
	switch kind {
	case proxyKindWeb:
		return "-getwebproxy"
	case proxyKindSecureWeb:
		return "-getsecurewebproxy"
	case proxyKindSOCKS:
		return "-getsocksfirewallproxy"
	default:
		return ""
	}
}

func setProxyCommand(kind proxyKind) string {
	switch kind {
	case proxyKindWeb:
		return "-setwebproxy"
	case proxyKindSecureWeb:
		return "-setsecurewebproxy"
	case proxyKindSOCKS:
		return "-setsocksfirewallproxy"
	default:
		return ""
	}
}

func setProxyStateCommand(kind proxyKind) string {
	switch kind {
	case proxyKindWeb:
		return "-setwebproxystate"
	case proxyKindSecureWeb:
		return "-setsecurewebproxystate"
	case proxyKindSOCKS:
		return "-setsocksfirewallproxystate"
	default:
		return ""
	}
}

func splitHostPort(address string) (string, int, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
