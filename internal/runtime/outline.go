package runtime

import (
	"fmt"
	"log/slog"

	"vpnclient/internal/config"
)

type OutlineAdapter struct {
	logger *slog.Logger
}

func (a *OutlineAdapter) BuildPlan(profile config.Profile) (Plan, error) {
	profile = profile.WithDefaults()
	if err := profile.Validate(); err != nil {
		return Plan{}, err
	}
	if profile.Protocol != config.ProtocolOutline {
		return Plan{}, fmt.Errorf("outline adapter cannot handle protocol %q", profile.Protocol)
	}

	inbounds, err := buildXrayLocalInbounds(profile.Local)
	if err != nil {
		return Plan{}, err
	}

	outbound := xrayOutbound{
		Tag:      "proxy-out",
		Protocol: "shadowsocks",
		Settings: xrayShadowsocksSettings{
			Servers: []xrayShadowsocksServer{
				{
					Address:  profile.Server.Host,
					Port:     profile.Server.Port,
					Method:   profile.Credentials.Method,
					Password: profile.Credentials.Password,
				},
			},
		},
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
		ConfigFileName: "outline-xray.json",
		ConfigData:     configData,
	}, nil
}
