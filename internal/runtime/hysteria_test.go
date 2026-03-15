package runtime_test

import (
	"strings"
	"testing"

	"vpnclient/internal/config"
	"vpnclient/internal/runtime"
)

func TestHysteriaAdapterBuildPlan(t *testing.T) {
	t.Parallel()

	adapter, err := runtime.AdapterFor(config.ProtocolHysteria, nil)
	if err != nil {
		t.Fatalf("AdapterFor() error = %v", err)
	}

	plan, err := adapter.BuildPlan(config.Profile{
		Name:     "hysteria-test",
		Protocol: config.ProtocolHysteria,
		Server: config.Server{
			Host: "hysteria.example.com",
			Port: 8443,
		},
		Credentials: config.Credentials{
			Password:            "shared-secret",
			ObfuscationPassword: "mask",
		},
		Transport: config.Transport{
			TLS: &config.TLS{
				ServerName: "cdn.example.com",
			},
			QUIC: &config.QUIC{
				UpMbps:   100,
				DownMbps: 250,
			},
		},
		Engine: config.Engine{
			Binary: "hysteria",
		},
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	configText := string(plan.ConfigData)
	if got, want := len(plan.ReadyAddresses), 2; got != want {
		t.Fatalf("len(ReadyAddresses) = %d, want %d", got, want)
	}
	if !strings.Contains(configText, "server: 'hysteria.example.com:8443'") {
		t.Fatalf("ConfigData does not contain the server:\n%s", configText)
	}
	if !strings.Contains(configText, "socks5:") {
		t.Fatalf("ConfigData does not contain a socks5 section:\n%s", configText)
	}
}
