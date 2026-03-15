//go:build darwin

package platform

import (
	"context"
	"strings"
	"testing"

	"vpnclient/internal/config"
)

func TestParseNetworkServiceOrder(t *testing.T) {
	t.Parallel()

	raw := []byte(`
An asterisk (*) denotes that a network service is disabled.
(1) Wi-Fi
(Hardware Port: Wi-Fi, Device: en0)

(2) *USB 10/100/1000 LAN
(Hardware Port: USB 10/100/1000 LAN, Device: en5)

(3) Shadowrocket
(Hardware Port: com.liguangming.Shadowrocket, Device: )
`)

	services, err := parseNetworkServiceOrder(raw)
	if err != nil {
		t.Fatalf("parseNetworkServiceOrder() error = %v", err)
	}

	if got, want := len(services), 2; got != want {
		t.Fatalf("len(services) = %d, want %d", got, want)
	}
	if got, want := services[0].Name, "Wi-Fi"; got != want {
		t.Fatalf("services[0].Name = %q, want %q", got, want)
	}
	if got, want := services[1].Name, "Shadowrocket"; got != want {
		t.Fatalf("services[1].Name = %q, want %q", got, want)
	}
}

func TestParseProxySetting(t *testing.T) {
	t.Parallel()

	raw := []byte(`
Enabled: Yes
Server: 127.0.0.1
Port: 1080
Authenticated Proxy Enabled: 0
`)

	setting, err := parseProxySetting(raw)
	if err != nil {
		t.Fatalf("parseProxySetting() error = %v", err)
	}

	if !setting.Enabled {
		t.Fatalf("Enabled = false, want true")
	}
	if got, want := setting.Server, "127.0.0.1"; got != want {
		t.Fatalf("Server = %q, want %q", got, want)
	}
	if got, want := setting.Port, 1080; got != want {
		t.Fatalf("Port = %d, want %d", got, want)
	}
	if setting.Authenticated {
		t.Fatalf("Authenticated = true, want false")
	}
}

func TestParseNetworkServiceNames(t *testing.T) {
	t.Parallel()

	raw := []byte(`
An asterisk (*) denotes that a network service is disabled.
(1) Wi-Fi
(2) *USB 10/100/1000 LAN
(3) Shadowrocket
`)

	services, err := parseNetworkServiceNames(raw)
	if err != nil {
		t.Fatalf("parseNetworkServiceNames() error = %v", err)
	}

	if got, want := len(services), 2; got != want {
		t.Fatalf("len(services) = %d, want %d", got, want)
	}
	if got, want := services[0].Name, "Wi-Fi"; got != want {
		t.Fatalf("services[0].Name = %q, want %q", got, want)
	}
	if got, want := services[1].Name, "Shadowrocket"; got != want {
		t.Fatalf("services[1].Name = %q, want %q", got, want)
	}
}

func TestParseDefaultRouteInterface(t *testing.T) {
	t.Parallel()

	raw := []byte(`
   route to: default
destination: default
       mask: default
  interface: utun6
`)

	defaultInterface, err := parseDefaultRouteInterface(raw)
	if err != nil {
		t.Fatalf("parseDefaultRouteInterface() error = %v", err)
	}
	if got, want := defaultInterface, "utun6"; got != want {
		t.Fatalf("default interface = %q, want %q", got, want)
	}
}

func TestPrepareFallsBackToLocalModeWhenNoServicesExist(t *testing.T) {
	t.Parallel()

	controller := NewNetworkSetupController(nil)
	controller.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "route":
			if len(args) != 2 || args[0] != "get" || args[1] != "default" {
				t.Fatalf("unexpected route args: %v", args)
			}
			return []byte("interface: en0\n"), nil
		case "networksetup":
			if len(args) != 1 || args[0] != "-listnetworkserviceorder" {
				t.Fatalf("unexpected networksetup args: %v", args)
			}
			return []byte("An asterisk (*) denotes that a network service is disabled.\n"), nil
		default:
			t.Fatalf("unexpected command name = %q", name)
		}
		return nil, nil
	}

	profile := config.Profile{
		Protocol: config.ProtocolXrayReality,
		Local: config.LocalProxy{
			SOCKSAddress: "127.0.0.1:1080",
			HTTPAddress:  "127.0.0.1:1081",
		},
	}
	if err := controller.Prepare(context.Background(), profile); err != nil {
		t.Fatalf("Prepare() error = %v, want nil", err)
	}
	if len(controller.snapshots) != 0 {
		t.Fatalf("len(snapshots) = %d, want 0", len(controller.snapshots))
	}
}

func TestPrepareRejectsConflictingTunnelRoute(t *testing.T) {
	t.Parallel()

	controller := NewNetworkSetupController(nil)
	controller.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if got, want := name, "route"; got != want {
			t.Fatalf("command name = %q, want %q", got, want)
		}
		if len(args) != 2 || args[0] != "get" || args[1] != "default" {
			t.Fatalf("unexpected route args: %v", args)
		}
		return []byte("interface: utun6\n"), nil
	}

	profile := config.Profile{
		Protocol: config.ProtocolXrayReality,
		Local: config.LocalProxy{
			SOCKSAddress: "127.0.0.1:1080",
			HTTPAddress:  "127.0.0.1:1081",
		},
	}
	err := controller.Prepare(context.Background(), profile)
	if err == nil {
		t.Fatalf("Prepare() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "default route via utun6") {
		t.Fatalf("Prepare() error = %q, want utun conflict message", err)
	}
}

func TestPrepareAllowsConflictingTunnelRouteWhenOutboundInterfaceIsPinned(t *testing.T) {
	t.Parallel()

	controller := NewNetworkSetupController(nil)
	controller.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "route":
			return []byte("interface: utun6\n"), nil
		case "networksetup":
			if len(args) == 1 && args[0] == "-listnetworkserviceorder" {
				return []byte("An asterisk (*) denotes that a network service is disabled.\n"), nil
			}
			t.Fatalf("unexpected networksetup args: %v", args)
		default:
			t.Fatalf("unexpected command name = %q", name)
		}
		return nil, nil
	}

	profile := config.Profile{
		Protocol: config.ProtocolXrayReality,
		Local: config.LocalProxy{
			SOCKSAddress: "127.0.0.1:1080",
			HTTPAddress:  "127.0.0.1:1081",
		},
		Metadata: map[string]string{
			"outbound_interface": "en0",
		},
	}
	if err := controller.Prepare(context.Background(), profile); err != nil {
		t.Fatalf("Prepare() error = %v, want nil", err)
	}
}
