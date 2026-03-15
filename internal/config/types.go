package config

import "fmt"

// Protocol identifies the external config family and runtime behavior.
type Protocol string

const (
	ProtocolOutline     Protocol = "outline"
	ProtocolXray        Protocol = "xray"
	ProtocolXrayReality Protocol = "xray-reality"
	ProtocolHysteria    Protocol = "hysteria"
	defaultSOCKSAddress          = "127.0.0.1:1080"
	defaultHTTPAddress           = "127.0.0.1:1081"
)

// Profile is the normalized internal configuration model.
type Profile struct {
	Name        string            `json:"name"`
	Protocol    Protocol          `json:"protocol"`
	Server      Server            `json:"server"`
	Credentials Credentials       `json:"credentials,omitempty"`
	Transport   Transport         `json:"transport,omitempty"`
	Local       LocalProxy        `json:"local,omitempty"`
	Engine      Engine            `json:"engine"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Server identifies the remote endpoint.
type Server struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Credentials contains protocol-specific authentication material.
type Credentials struct {
	Method              string `json:"method,omitempty"`
	Password            string `json:"password,omitempty"`
	UUID                string `json:"uuid,omitempty"`
	Flow                string `json:"flow,omitempty"`
	Prefix              string `json:"prefix,omitempty"`
	ObfuscationPassword string `json:"obfuscation_password,omitempty"`
}

// Transport holds transport-layer options shared across protocol families.
type Transport struct {
	Network string     `json:"network,omitempty"`
	TLS     *TLS       `json:"tls,omitempty"`
	Reality *Reality   `json:"reality,omitempty"`
	WS      *WebSocket `json:"websocket,omitempty"`
	QUIC    *QUIC      `json:"quic,omitempty"`
}

// TLS contains TLS-related settings.
type TLS struct {
	Enabled            bool     `json:"enabled"`
	ServerName         string   `json:"server_name,omitempty"`
	ALPN               []string `json:"alpn,omitempty"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify,omitempty"`
}

// Reality contains Xray REALITY client options.
type Reality struct {
	Enabled     bool   `json:"enabled"`
	ServerName  string `json:"server_name,omitempty"`
	PublicKey   string `json:"public_key"`
	ShortID     string `json:"short_id,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	SpiderX     string `json:"spider_x,omitempty"`
}

// WebSocket contains WebSocket transport options.
type WebSocket struct {
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// QUIC contains Hysteria-specific QUIC options.
type QUIC struct {
	UpMbps   int `json:"up_mbps,omitempty"`
	DownMbps int `json:"down_mbps,omitempty"`
}

// LocalProxy describes local proxy listeners exposed by the runtime.
type LocalProxy struct {
	SOCKSAddress string `json:"socks_address,omitempty"`
	HTTPAddress  string `json:"http_address,omitempty"`
}

// Engine defines the runtime executable and process settings.
type Engine struct {
	Binary      string            `json:"binary"`
	WorkingDir  string            `json:"working_dir,omitempty"`
	ExtraArgs   []string          `json:"extra_args,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}

// WithDefaults fills pragmatic defaults while keeping the public model stable.
func (p Profile) WithDefaults() Profile {
	if p.Name == "" && p.Server.Host != "" && p.Server.Port > 0 && p.Protocol != "" {
		p.Name = fmt.Sprintf("%s-%s-%d", p.Protocol, p.Server.Host, p.Server.Port)
	}

	if p.Transport.Network == "" {
		switch {
		case p.Protocol == ProtocolHysteria:
			p.Transport.Network = "udp"
		case p.Transport.WS != nil:
			p.Transport.Network = "ws"
		default:
			p.Transport.Network = "tcp"
		}
	}

	if p.Local.SOCKSAddress == "" {
		p.Local.SOCKSAddress = defaultSOCKSAddress
	}
	if p.Local.HTTPAddress == "" {
		p.Local.HTTPAddress = defaultHTTPAddress
	}

	if p.Transport.TLS != nil {
		p.Transport.TLS.Enabled = true
	}
	if p.Transport.Reality != nil {
		p.Transport.Reality.Enabled = true
	}

	if p.Protocol == ProtocolHysteria {
		if p.Transport.TLS == nil {
			p.Transport.TLS = &TLS{
				Enabled: true,
			}
		}
		if p.Transport.TLS.ServerName == "" {
			p.Transport.TLS.ServerName = p.Server.Host
		}
	}

	if p.Protocol == ProtocolXrayReality && p.Transport.Reality != nil && p.Transport.Reality.ServerName == "" {
		if p.Transport.TLS != nil && p.Transport.TLS.ServerName != "" {
			p.Transport.Reality.ServerName = p.Transport.TLS.ServerName
		} else {
			p.Transport.Reality.ServerName = p.Server.Host
		}
	}
	if p.Protocol == ProtocolXrayReality && p.Transport.Reality != nil &&
		p.Transport.Reality.Fingerprint == "" {
		p.Transport.Reality.Fingerprint = "chrome"
	}

	if p.Engine.Binary == "" {
		p.Engine.Binary = defaultBinaryForProtocol(p.Protocol)
	}

	return p
}

func defaultBinaryForProtocol(protocol Protocol) string {
	switch protocol {
	case ProtocolOutline, ProtocolXray, ProtocolXrayReality:
		return "xray"
	case ProtocolHysteria:
		return "hysteria"
	default:
		return ""
	}
}
