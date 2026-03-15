package runtime_test

import (
	"strings"
	"testing"

	"vpnclient/internal/config"
	"vpnclient/internal/runtime"
)

func TestXrayAdapterBuildPlanReality(t *testing.T) {
	t.Parallel()

	adapter, err := runtime.AdapterFor(config.ProtocolXrayReality, nil)
	if err != nil {
		t.Fatalf("AdapterFor() error = %v", err)
	}

	plan, err := adapter.BuildPlan(config.Profile{
		Name:     "reality-test",
		Protocol: config.ProtocolXrayReality,
		Server: config.Server{
			Host: "203.0.113.5",
			Port: 443,
		},
		Credentials: config.Credentials{
			UUID: "11111111-1111-1111-1111-111111111111",
			Flow: "xtls-rprx-vision",
		},
		Transport: config.Transport{
			Reality: &config.Reality{
				PublicKey:   "test-public-key",
				ServerName:  "www.example.com",
				Fingerprint: "chrome",
			},
		},
		Engine: config.Engine{
			Binary: "xray",
		},
		Metadata: map[string]string{
			"outbound_interface": "en0",
		},
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	if !strings.Contains(string(plan.ConfigData), "\"security\": \"reality\"") {
		t.Fatalf("ConfigData does not contain REALITY security:\n%s", string(plan.ConfigData))
	}
	if !strings.Contains(string(plan.ConfigData), "\"password\": \"test-public-key\"") {
		t.Fatalf("ConfigData does not contain REALITY password field:\n%s", string(plan.ConfigData))
	}
	if !strings.Contains(string(plan.ConfigData), "\"interface\": \"en0\"") {
		t.Fatalf("ConfigData does not contain outbound interface binding:\n%s", string(plan.ConfigData))
	}
	if got, want := len(plan.ReadyAddresses), 2; got != want {
		t.Fatalf("len(ReadyAddresses) = %d, want %d", got, want)
	}
	if !strings.Contains(string(plan.ConfigData), "\"protocol\": \"vless\"") {
		t.Fatalf("ConfigData does not contain a VLESS outbound:\n%s", string(plan.ConfigData))
	}
}
