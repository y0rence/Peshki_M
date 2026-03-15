package config

import (
	"fmt"
	"net"
	"strings"
)

// Validate checks whether the normalized profile is actionable.
func (p Profile) Validate() error {
	if p.Protocol == "" {
		return fmt.Errorf("protocol is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("profile name is required")
	}
	if strings.TrimSpace(p.Server.Host) == "" {
		return fmt.Errorf("server.host is required")
	}
	if p.Server.Port < 1 || p.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if strings.TrimSpace(p.Engine.Binary) == "" {
		return fmt.Errorf("engine.binary is required")
	}
	if err := validateLocalAddress("local.socks_address", p.Local.SOCKSAddress); err != nil {
		return err
	}
	if p.Local.HTTPAddress != "" {
		if err := validateLocalAddress("local.http_address", p.Local.HTTPAddress); err != nil {
			return err
		}
	}

	switch p.Protocol {
	case ProtocolOutline:
		if strings.TrimSpace(p.Credentials.Method) == "" {
			return fmt.Errorf("credentials.method is required for outline")
		}
		if strings.TrimSpace(p.Credentials.Password) == "" {
			return fmt.Errorf("credentials.password is required for outline")
		}
	case ProtocolXray:
		if strings.TrimSpace(p.Credentials.UUID) == "" {
			return fmt.Errorf("credentials.uuid is required for xray")
		}
		if p.Transport.Reality != nil {
			return fmt.Errorf("transport.reality is only valid for xray-reality")
		}
	case ProtocolXrayReality:
		if strings.TrimSpace(p.Credentials.UUID) == "" {
			return fmt.Errorf("credentials.uuid is required for xray-reality")
		}
		if p.Transport.Reality == nil {
			return fmt.Errorf("transport.reality is required for xray-reality")
		}
		if strings.TrimSpace(p.Transport.Reality.PublicKey) == "" {
			return fmt.Errorf("transport.reality.public_key is required for xray-reality")
		}
		if strings.TrimSpace(p.Transport.Reality.ServerName) == "" {
			return fmt.Errorf("transport.reality.server_name is required for xray-reality")
		}
	case ProtocolHysteria:
		if strings.TrimSpace(p.Credentials.Password) == "" {
			return fmt.Errorf("credentials.password is required for hysteria")
		}
		if p.Transport.TLS == nil || !p.Transport.TLS.Enabled {
			return fmt.Errorf("transport.tls is required for hysteria")
		}
	default:
		return fmt.Errorf("unsupported protocol %q", p.Protocol)
	}

	return nil
}

func validateLocalAddress(field string, address string) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("%s is required", field)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%s must use host:port format: %w", field, err)
	}
	if host == "" {
		return fmt.Errorf("%s host must not be empty", field)
	}
	if port == "" {
		return fmt.Errorf("%s port must not be empty", field)
	}
	return nil
}
