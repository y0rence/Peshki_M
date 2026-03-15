package runtime

import (
	"context"
	"fmt"
	"log/slog"

	"vpnclient/internal/config"
)

const configPathPlaceholder = "__VPNCLIENT_CONFIG_PATH__"

// Plan is the fully rendered runtime launch plan.
type Plan struct {
	Protocol       config.Protocol
	Binary         string
	Args           []string
	WorkingDir     string
	Environment    map[string]string
	ReadyAddresses []string
	ConfigFileName string
	ConfigData     []byte
}

// Adapter converts a normalized profile into a runtime launch plan.
type Adapter interface {
	BuildPlan(config.Profile) (Plan, error)
}

// Process represents a running protocol runtime.
type Process interface {
	Stop(context.Context) error
	Wait() error
}

// Starter launches runtime plans.
type Starter interface {
	Start(context.Context, Plan) (Process, error)
}

// AdapterFor selects the correct runtime adapter for the profile protocol.
func AdapterFor(protocol config.Protocol, logger *slog.Logger) (Adapter, error) {
	switch protocol {
	case config.ProtocolOutline:
		return &OutlineAdapter{logger: logger}, nil
	case config.ProtocolXray, config.ProtocolXrayReality:
		return &XrayAdapter{logger: logger}, nil
	case config.ProtocolHysteria:
		return &HysteriaAdapter{logger: logger}, nil
	default:
		return nil, fmt.Errorf("unsupported protocol %q", protocol)
	}
}

func buildReadyAddresses(local config.LocalProxy) []string {
	addresses := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)

	for _, address := range []string{local.SOCKSAddress, local.HTTPAddress} {
		if address == "" {
			continue
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		addresses = append(addresses, address)
	}

	return addresses
}
