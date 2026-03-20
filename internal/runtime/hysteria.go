package runtime

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"vpnclient/internal/config"
)

type HysteriaAdapter struct {
	logger *slog.Logger
}

func (a *HysteriaAdapter) BuildPlan(profile config.Profile) (Plan, error) {
	profile = profile.WithDefaults()
	if err := profile.Validate(); err != nil {
		return Plan{}, err
	}
	if profile.Protocol != config.ProtocolHysteria {
		return Plan{}, fmt.Errorf("hysteria adapter cannot handle protocol %q", profile.Protocol)
	}

	configData := buildHysteriaConfig(profile)

	return Plan{
		Protocol:       profile.Protocol,
		Binary:         profile.Engine.Binary,
		Args:           append([]string{"-c", configPathPlaceholder}, profile.Engine.ExtraArgs...),
		WorkingDir:     profile.Engine.WorkingDir,
		Environment:    cloneEnvironment(profile.Engine.Environment),
		ReadyAddresses: buildReadyAddresses(profile.Local),
		ConfigFileName: "hysteria-client.yaml",
		ConfigData:     configData,
	}, nil
}

func buildHysteriaConfig(profile config.Profile) []byte {
	var builder strings.Builder

	builder.WriteString("server: ")
	builder.WriteString(yamlQuote(profile.Server.Host + ":" + strconv.Itoa(profile.Server.Port)))
	builder.WriteString("\n")

	builder.WriteString("auth: ")
	builder.WriteString(yamlQuote(profile.Credentials.Password))
	builder.WriteString("\n")

	if profile.Transport.TLS != nil {
		builder.WriteString("tls:\n")
		if profile.Transport.TLS.ServerName != "" {
			builder.WriteString("  sni: ")
			builder.WriteString(yamlQuote(profile.Transport.TLS.ServerName))
			builder.WriteString("\n")
		}
		builder.WriteString("  insecure: ")
		if profile.Transport.TLS.InsecureSkipVerify {
			builder.WriteString("true\n")
		} else {
			builder.WriteString("false\n")
		}
	}

	if profile.Credentials.ObfuscationPassword != "" {
		builder.WriteString("obfs:\n")
		builder.WriteString("  type: salamander\n")
		builder.WriteString("  salamander:\n")
		builder.WriteString("    password: ")
		builder.WriteString(yamlQuote(profile.Credentials.ObfuscationPassword))
		builder.WriteString("\n")
	}

	if profile.Transport.QUIC != nil && (profile.Transport.QUIC.UpMbps > 0 || profile.Transport.QUIC.DownMbps > 0) {
		builder.WriteString("bandwidth:\n")
		if profile.Transport.QUIC.UpMbps > 0 {
			builder.WriteString("  up: ")
			builder.WriteString(yamlQuote(strconv.Itoa(profile.Transport.QUIC.UpMbps) + " mbps"))
			builder.WriteString("\n")
		}
		if profile.Transport.QUIC.DownMbps > 0 {
			builder.WriteString("  down: ")
			builder.WriteString(yamlQuote(strconv.Itoa(profile.Transport.QUIC.DownMbps) + " mbps"))
			builder.WriteString("\n")
		}
	}

	builder.WriteString("socks5:\n")
	builder.WriteString("  listen: ")
	builder.WriteString(yamlQuote(profile.Local.SOCKSAddress))
	builder.WriteString("\n")

	if profile.Local.HTTPAddress != "" {
		builder.WriteString("http:\n")
		builder.WriteString("  listen: ")
		builder.WriteString(yamlQuote(profile.Local.HTTPAddress))
		builder.WriteString("\n")
	}

	return []byte(builder.String())
}

func yamlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
