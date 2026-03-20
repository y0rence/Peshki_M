package runtime

import (
	"fmt"
	"log/slog"
	"strings"

	"vpnclient/internal/config"
)

type XrayAdapter struct {
	logger *slog.Logger
}

func (a *XrayAdapter) BuildPlan(profile config.Profile) (Plan, error) {
	profile = profile.WithDefaults()
	if err := profile.Validate(); err != nil {
		return Plan{}, err
	}
	if profile.Protocol != config.ProtocolXray && profile.Protocol != config.ProtocolXrayReality {
		return Plan{}, fmt.Errorf("xray adapter cannot handle protocol %q", profile.Protocol)
	}

	inbounds, err := buildXrayLocalInbounds(profile.Local)
	if err != nil {
		return Plan{}, err
	}

	streamSettings := &xrayStreamSettings{}
	if outboundInterface := strings.TrimSpace(profile.Metadata["outbound_interface"]); outboundInterface != "" {
		streamSettings.Sockopt = &xraySockoptSettings{
			Interface: outboundInterface,
		}
	}

	switch profile.Transport.Network {
	case "", "tcp":
	case "ws", "websocket":
		streamSettings.Network = "ws"
		if profile.Transport.WS != nil {
			streamSettings.WSSettings = &xrayWSSettings{
				Path:    profile.Transport.WS.Path,
				Headers: profile.Transport.WS.Headers,
			}
		}
	default:
		streamSettings.Network = profile.Transport.Network
	}

	switch profile.Protocol {
	case config.ProtocolXrayReality:
		streamSettings.Security = "reality"
		streamSettings.RealitySettings = &xrayRealitySettings{
			ServerName:  profile.Transport.Reality.ServerName,
			Password:    profile.Transport.Reality.PublicKey,
			ShortID:     profile.Transport.Reality.ShortID,
			Fingerprint: profile.Transport.Reality.Fingerprint,
			SpiderX:     profile.Transport.Reality.SpiderX,
		}
	case config.ProtocolXray:
		if profile.Transport.TLS != nil && profile.Transport.TLS.Enabled {
			streamSettings.Security = "tls"
			streamSettings.TLSSettings = &xrayTLSSettings{
				ServerName:    profile.Transport.TLS.ServerName,
				AllowInsecure: profile.Transport.TLS.InsecureSkipVerify,
				ALPN:          profile.Transport.TLS.ALPN,
			}
		}
	}

	outbound := xrayOutbound{
		Tag:      "proxy-out",
		Protocol: "vless",
		Settings: xrayVLESSSettings{
			VNext: []xrayVLESSServer{
				{
					Address: profile.Server.Host,
					Port:    profile.Server.Port,
					Users: []xrayVLESSUser{
						{
							ID:         profile.Credentials.UUID,
							Encryption: "none",
							Flow:       profile.Credentials.Flow,
						},
					},
				},
			},
		},
	}

	if streamSettings.Network != "" || streamSettings.Security != "" || streamSettings.WSSettings != nil {
		outbound.StreamSettings = streamSettings
	}

	configData, err := marshalXrayConfig(inbounds, outbound)
	if err != nil {
		return Plan{}, err
	}

	return Plan{
		Protocol:       profile.Protocol,
		Binary:         profile.Engine.Binary,
		Args:           append([]string{"run", "-c", configPathPlaceholder}, profile.Engine.ExtraArgs...),
		WorkingDir:     profile.Engine.WorkingDir,
		Environment:    cloneEnvironment(profile.Engine.Environment),
		ReadyAddresses: buildReadyAddresses(profile.Local),
		ConfigFileName: "xray-client.json",
		ConfigData:     configData,
	}, nil
}
