package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"vpnclient/internal/config"
)

func TestNormalizeOutlineProfile(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
	  "type": "outline",
	  "name": "edge-outline",
	  "server": {
	    "host": "198.51.100.10",
	    "port": 443
	  },
	  "cipher": "chacha20-ietf-poly1305",
	  "password": "secret",
	  "engine": {
	    "binary": "xray"
	  }
	}`)

	profile, err := config.NormalizeProfile(raw)
	if err != nil {
		t.Fatalf("NormalizeProfile() error = %v", err)
	}

	if got, want := profile.Protocol, config.ProtocolOutline; got != want {
		t.Fatalf("Protocol = %q, want %q", got, want)
	}
	if got, want := profile.Credentials.Method, "chacha20-ietf-poly1305"; got != want {
		t.Fatalf("Method = %q, want %q", got, want)
	}
	if got, want := profile.Local.SOCKSAddress, "127.0.0.1:1080"; got != want {
		t.Fatalf("SOCKSAddress = %q, want %q", got, want)
	}
}

func TestNormalizeXrayRealityProfile(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
	  "type": "xray-reality",
	  "name": "edge-reality",
	  "server": {
	    "host": "203.0.113.5",
	    "port": 443
	  },
	  "uuid": "11111111-1111-1111-1111-111111111111",
	  "flow": "xtls-rprx-vision",
	  "reality": {
	    "public_key": "test-public-key",
	    "short_id": "abcd1234",
	    "server_name": "www.example.com",
	    "fingerprint": "chrome"
	  },
	  "engine": {
	    "binary": "xray"
	  }
	}`)

	profile, err := config.NormalizeProfile(raw)
	if err != nil {
		t.Fatalf("NormalizeProfile() error = %v", err)
	}

	if got, want := profile.Protocol, config.ProtocolXrayReality; got != want {
		t.Fatalf("Protocol = %q, want %q", got, want)
	}
	if got, want := profile.Transport.Reality.PublicKey, "test-public-key"; got != want {
		t.Fatalf("PublicKey = %q, want %q", got, want)
	}
	if got, want := profile.Engine.Binary, "xray"; got != want {
		t.Fatalf("Binary = %q, want %q", got, want)
	}
}

func TestNormalizeHysteriaProfile(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
	  "type": "hysteria",
	  "name": "edge-hysteria",
	  "server": {
	    "host": "hysteria.example.com",
	    "port": 8443
	  },
	  "password": "shared-secret",
	  "sni": "cdn.example.com",
	  "up_mbps": 100,
	  "down_mbps": 250,
	  "engine": {
	    "binary": "hysteria"
	  }
	}`)

	profile, err := config.NormalizeProfile(raw)
	if err != nil {
		t.Fatalf("NormalizeProfile() error = %v", err)
	}

	if got, want := profile.Protocol, config.ProtocolHysteria; got != want {
		t.Fatalf("Protocol = %q, want %q", got, want)
	}
	if got, want := profile.Transport.TLS.ServerName, "cdn.example.com"; got != want {
		t.Fatalf("ServerName = %q, want %q", got, want)
	}
	if got, want := profile.Transport.QUIC.DownMbps, 250; got != want {
		t.Fatalf("DownMbps = %d, want %d", got, want)
	}
}

func TestNormalizeImportedVLESSRealityProfile(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
	  "host": "144.31.13.152",
	  "port": "2443",
	  "tls": true,
	  "uuid": "0834236D-4C9F-4180-AECC-8786DCB5A374",
	  "password": "193d46ca-b199-4c29-9f1e-5efa1a8dbf2c",
	  "type": "VLESS",
	  "xtls": 2,
	  "publicKey": "test-public-key",
	  "peer": "www.cloudflare.com",
	  "title": "Imported VLESS Profile",
	  "shortId": "0509bdb253e2471f"
	}`)

	profile, err := config.NormalizeProfile(raw)
	if err != nil {
		t.Fatalf("NormalizeProfile() error = %v", err)
	}

	if got, want := profile.Protocol, config.ProtocolXrayReality; got != want {
		t.Fatalf("Protocol = %q, want %q", got, want)
	}
	if got, want := profile.Server.Port, 2443; got != want {
		t.Fatalf("Port = %d, want %d", got, want)
	}
	if got, want := profile.Credentials.UUID, "193d46ca-b199-4c29-9f1e-5efa1a8dbf2c"; got != want {
		t.Fatalf("UUID = %q, want %q", got, want)
	}
	if got, want := profile.Credentials.Flow, "xtls-rprx-vision"; got != want {
		t.Fatalf("Flow = %q, want %q", got, want)
	}
	if got, want := profile.Transport.Reality.ServerName, "www.cloudflare.com"; got != want {
		t.Fatalf("Reality.ServerName = %q, want %q", got, want)
	}
	if got, want := profile.Transport.Reality.Fingerprint, "chrome"; got != want {
		t.Fatalf("Reality.Fingerprint = %q, want %q", got, want)
	}
}

func TestLoadProfileResolvesRelativeEnginePaths(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "profile.json")

	raw := []byte(`{
	  "type": "xray-reality",
	  "name": "relative-runtime",
	  "server": {
	    "host": "203.0.113.5",
	    "port": 443
	  },
	  "uuid": "11111111-1111-1111-1111-111111111111",
	  "reality": {
	    "public_key": "test-public-key",
	    "server_name": "www.example.com"
	  },
	  "engine": {
	    "binary": "bin/xray.exe",
	    "working_dir": "runtime"
	  }
	}`)

	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	profile, err := config.LoadProfile(configPath)
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}

	if got, want := profile.Engine.Binary, filepath.Join(tempDir, "bin", "xray.exe"); got != want {
		t.Fatalf("Engine.Binary = %q, want %q", got, want)
	}
	if got, want := profile.Engine.WorkingDir, filepath.Join(tempDir, "runtime"); got != want {
		t.Fatalf("Engine.WorkingDir = %q, want %q", got, want)
	}
}
