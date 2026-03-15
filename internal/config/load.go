package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type envelope struct {
	Type     string `json:"type"`
	Protocol string `json:"protocol"`
}

type outlineInput struct {
	Name     string            `json:"name"`
	Server   Server            `json:"server"`
	Endpoint Server            `json:"endpoint"`
	Method   string            `json:"method"`
	Cipher   string            `json:"cipher"`
	Password string            `json:"password"`
	Prefix   string            `json:"prefix"`
	Local    LocalProxy        `json:"local"`
	Engine   Engine            `json:"engine"`
	Metadata map[string]string `json:"metadata"`
}

type xrayInput struct {
	Name       string            `json:"name"`
	Server     Server            `json:"server"`
	Endpoint   Server            `json:"endpoint"`
	UUID       string            `json:"uuid"`
	UserID     string            `json:"user_id"`
	ID         string            `json:"id"`
	Flow       string            `json:"flow"`
	Network    string            `json:"network"`
	Security   string            `json:"security"`
	TLS        *TLS              `json:"tls"`
	Reality    *Reality          `json:"reality"`
	WebSocket  *WebSocket        `json:"websocket"`
	Local      LocalProxy        `json:"local"`
	Engine     Engine            `json:"engine"`
	Metadata   map[string]string `json:"metadata"`
	ServerName string            `json:"server_name"`
	SNI        string            `json:"sni"`
}

type hysteriaInput struct {
	Name                string            `json:"name"`
	Server              Server            `json:"server"`
	Endpoint            Server            `json:"endpoint"`
	Password            string            `json:"password"`
	Auth                string            `json:"auth"`
	ObfuscationPassword string            `json:"obfs_password"`
	TLS                 *TLS              `json:"tls"`
	UpMbps              int               `json:"up_mbps"`
	DownMbps            int               `json:"down_mbps"`
	Local               LocalProxy        `json:"local"`
	Engine              Engine            `json:"engine"`
	Metadata            map[string]string `json:"metadata"`
	ServerName          string            `json:"server_name"`
	SNI                 string            `json:"sni"`
}

type importedVLESSInput struct {
	Title     string `json:"title"`
	Host      string `json:"host"`
	IP        string `json:"ip"`
	Port      string `json:"port"`
	UUID      string `json:"uuid"`
	Password  string `json:"password"`
	TLS       bool   `json:"tls"`
	ALPN      string `json:"alpn"`
	Path      string `json:"path"`
	PublicKey string `json:"publicKey"`
	ShortID   string `json:"shortId"`
	Peer      string `json:"peer"`
	SNI       string `json:"sni"`
	XTLS      int    `json:"xtls"`
}

// LoadProfile reads a JSON config file and normalizes it into the internal model.
func LoadProfile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read config %q: %w", path, err)
	}

	profile, err := NormalizeProfile(data)
	if err != nil {
		return Profile{}, err
	}

	return resolveRelativePaths(profile, filepath.Dir(path)), nil
}

// NormalizeProfile converts a protocol-family JSON config into the internal model.
func NormalizeProfile(data []byte) (Profile, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Profile{}, fmt.Errorf("config is empty")
	}

	var meta envelope
	if err := json.Unmarshal(data, &meta); err != nil {
		return Profile{}, fmt.Errorf("decode config envelope: %w", err)
	}

	if normalizedProtocol := normalizeProtocol(meta.Protocol); normalizedProtocol != "" {
		var profile Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			return Profile{}, fmt.Errorf("decode normalized profile: %w", err)
		}
		profile.Protocol = normalizedProtocol
		profile = profile.WithDefaults()
		if err := profile.Validate(); err != nil {
			return Profile{}, err
		}
		return profile, nil
	}

	switch normalizeProtocol(meta.Type) {
	case ProtocolOutline:
		return normalizeOutline(data)
	case ProtocolXray:
		return normalizeXray(data, ProtocolXray)
	case ProtocolXrayReality:
		return normalizeXray(data, ProtocolXrayReality)
	case ProtocolHysteria:
		return normalizeHysteria(data)
	default:
		switch strings.ToLower(strings.TrimSpace(meta.Type)) {
		case "vless":
			return normalizeImportedVLESS(data)
		default:
			return Profile{}, fmt.Errorf("unsupported config type %q", meta.Type)
		}
	}
}

func normalizeOutline(data []byte) (Profile, error) {
	var input outlineInput
	if err := json.Unmarshal(data, &input); err != nil {
		return Profile{}, fmt.Errorf("decode outline config: %w", err)
	}

	profile := Profile{
		Name:     input.Name,
		Protocol: ProtocolOutline,
		Server:   selectServer(input.Server, input.Endpoint),
		Credentials: Credentials{
			Method:   firstNonEmpty(input.Method, input.Cipher),
			Password: input.Password,
			Prefix:   input.Prefix,
		},
		Local:    input.Local,
		Engine:   input.Engine,
		Metadata: input.Metadata,
	}

	profile = profile.WithDefaults()
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func normalizeXray(data []byte, declared Protocol) (Profile, error) {
	var input xrayInput
	if err := json.Unmarshal(data, &input); err != nil {
		return Profile{}, fmt.Errorf("decode xray config: %w", err)
	}

	protocol := declared
	if normalizeProtocol(input.Security) == ProtocolXrayReality || input.Reality != nil {
		protocol = ProtocolXrayReality
	}

	tlsConfig := input.TLS
	if tlsConfig == nil && firstNonEmpty(input.ServerName, input.SNI) != "" {
		tlsConfig = &TLS{}
	}
	if tlsConfig != nil && tlsConfig.ServerName == "" {
		tlsConfig.ServerName = firstNonEmpty(input.ServerName, input.SNI)
	}

	profile := Profile{
		Name:     input.Name,
		Protocol: protocol,
		Server:   selectServer(input.Server, input.Endpoint),
		Credentials: Credentials{
			UUID: firstNonEmpty(input.UUID, input.UserID, input.ID),
			Flow: input.Flow,
		},
		Transport: Transport{
			Network: input.Network,
			TLS:     tlsConfig,
			Reality: input.Reality,
			WS:      input.WebSocket,
		},
		Local:    input.Local,
		Engine:   input.Engine,
		Metadata: input.Metadata,
	}

	profile = profile.WithDefaults()
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func normalizeHysteria(data []byte) (Profile, error) {
	var input hysteriaInput
	if err := json.Unmarshal(data, &input); err != nil {
		return Profile{}, fmt.Errorf("decode hysteria config: %w", err)
	}

	tlsConfig := input.TLS
	if tlsConfig == nil && firstNonEmpty(input.ServerName, input.SNI) != "" {
		tlsConfig = &TLS{}
	}
	if tlsConfig != nil && tlsConfig.ServerName == "" {
		tlsConfig.ServerName = firstNonEmpty(input.ServerName, input.SNI)
	}

	profile := Profile{
		Name:     input.Name,
		Protocol: ProtocolHysteria,
		Server:   selectServer(input.Server, input.Endpoint),
		Credentials: Credentials{
			Password:            firstNonEmpty(input.Password, input.Auth),
			ObfuscationPassword: input.ObfuscationPassword,
		},
		Transport: Transport{
			TLS: tlsConfig,
			QUIC: &QUIC{
				UpMbps:   input.UpMbps,
				DownMbps: input.DownMbps,
			},
		},
		Local:    input.Local,
		Engine:   input.Engine,
		Metadata: input.Metadata,
	}

	profile = profile.WithDefaults()
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func normalizeImportedVLESS(data []byte) (Profile, error) {
	var input importedVLESSInput
	if err := json.Unmarshal(data, &input); err != nil {
		return Profile{}, fmt.Errorf("decode imported vless config: %w", err)
	}

	port, err := parsePort(input.Port)
	if err != nil {
		return Profile{}, fmt.Errorf("parse vless port: %w", err)
	}

	protocol := ProtocolXray
	var reality *Reality
	var tlsConfig *TLS

	serverName := firstNonEmpty(input.Peer, input.SNI, input.Host, input.IP)
	if input.TLS || serverName != "" || strings.TrimSpace(input.ALPN) != "" {
		tlsConfig = &TLS{
			Enabled:    input.TLS,
			ServerName: serverName,
			ALPN:       splitCSV(input.ALPN),
		}
	}

	if strings.TrimSpace(input.PublicKey) != "" {
		protocol = ProtocolXrayReality
		reality = &Reality{
			Enabled:     true,
			ServerName:  serverName,
			PublicKey:   input.PublicKey,
			ShortID:     input.ShortID,
			Fingerprint: "chrome",
		}
	}

	profile := Profile{
		Name:     firstNonEmpty(input.Title, input.UUID),
		Protocol: protocol,
		Server: Server{
			Host: firstNonEmpty(input.Host, input.IP),
			Port: port,
		},
		Credentials: Credentials{
			UUID: firstNonEmpty(input.Password, input.UUID),
			Flow: inferImportedVLESSFlow(input.XTLS),
		},
		Transport: Transport{
			Network: inferImportedVLESSNetwork(input.Path),
			TLS:     tlsConfig,
			Reality: reality,
			WS:      buildImportedVLESSWebSocket(input.Path),
		},
	}

	profile = profile.WithDefaults()
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func normalizeProtocol(value string) Protocol {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "_", "-")

	switch normalized {
	case string(ProtocolOutline):
		return ProtocolOutline
	case string(ProtocolXray):
		return ProtocolXray
	case "reality", string(ProtocolXrayReality):
		return ProtocolXrayReality
	case string(ProtocolHysteria), "hysteria2":
		return ProtocolHysteria
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	return port, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func inferImportedVLESSFlow(xtls int) string {
	// Many exported VLESS profiles encode Vision as a numeric xtls flag.
	if xtls > 0 {
		return "xtls-rprx-vision"
	}
	return ""
}

func inferImportedVLESSNetwork(path string) string {
	if strings.TrimSpace(path) != "" {
		return "ws"
	}
	return "tcp"
}

func buildImportedVLESSWebSocket(path string) *WebSocket {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return &WebSocket{
		Path: path,
	}
}

func selectServer(primary Server, secondary Server) Server {
	if primary.Host != "" || primary.Port != 0 {
		return primary
	}
	return secondary
}

func resolveRelativePaths(profile Profile, baseDir string) Profile {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return profile
	}

	if shouldResolveRelativePath(profile.Engine.Binary) {
		profile.Engine.Binary = filepath.Clean(filepath.Join(baseDir, profile.Engine.Binary))
	}

	if strings.TrimSpace(profile.Engine.WorkingDir) != "" &&
		!filepath.IsAbs(profile.Engine.WorkingDir) {
		profile.Engine.WorkingDir = filepath.Clean(filepath.Join(baseDir, profile.Engine.WorkingDir))
	}

	return profile
}

func shouldResolveRelativePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return false
	}

	return strings.Contains(path, "/") ||
		strings.Contains(path, `\`) ||
		strings.HasPrefix(path, ".")
}
