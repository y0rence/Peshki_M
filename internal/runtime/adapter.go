package runtime

import (
	"context"
	"fmt"
	"log/slog"

	"vpnclient/internal/config"
)

const configPathPlaceholder = "__VPNCLIENT_CONFIG_PATH__"

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

type Adapter interface {
	BuildPlan(config.Profile) (Plan, error)
}

type Process interface {
	Stop(context.Context) error
	Wait() error
}

type Starter interface {
	Start(context.Context, Plan) (Process, error)
}

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
