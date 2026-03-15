package runtime_test

import (
	"strings"
	"testing"

	"vpnclient/internal/config"
	"vpnclient/internal/runtime"
)

func TestOutlineAdapterBuildPlan(t *testing.T) {
	t.Parallel()

	adapter, err := runtime.AdapterFor(config.ProtocolOutline, nil)
	if err != nil {
		t.Fatalf("AdapterFor() error = %v", err)
	}

	plan, err := adapter.BuildPlan(config.Profile{
		Name:     "outline-test",
		Protocol: config.ProtocolOutline,
		Server: config.Server{
			Host: "198.51.100.10",
			Port: 443,
		},
		Credentials: config.Credentials{
			Method:   "chacha20-ietf-poly1305",
			Password: "secret",
		},
		Engine: config.Engine{
			Binary: "xray",
		},
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	if got, want := plan.Binary, "xray"; got != want {
		t.Fatalf("Binary = %q, want %q", got, want)
	}
	if got, want := len(plan.ReadyAddresses), 2; got != want {
		t.Fatalf("len(ReadyAddresses) = %d, want %d", got, want)
	}
	if !strings.Contains(string(plan.ConfigData), "\"protocol\": \"shadowsocks\"") {
		t.Fatalf("ConfigData does not contain a shadowsocks outbound:\n%s", string(plan.ConfigData))
	}
}
