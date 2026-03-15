package runtime

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"vpnclient/internal/config"
)

type xrayConfig struct {
	Log       xrayLog        `json:"log"`
	Inbounds  []xrayInbound  `json:"inbounds"`
	Outbounds []xrayOutbound `json:"outbounds"`
}

type xrayLog struct {
	LogLevel string `json:"loglevel"`
}

type xrayInbound struct {
	Listen   string        `json:"listen"`
	Port     int           `json:"port"`
	Protocol string        `json:"protocol"`
	Tag      string        `json:"tag,omitempty"`
	Settings any           `json:"settings"`
	Sniffing *xraySniffing `json:"sniffing,omitempty"`
}

type xraySniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride,omitempty"`
}

type xrayOutbound struct {
	Tag            string              `json:"tag,omitempty"`
	Protocol       string              `json:"protocol"`
	Settings       any                 `json:"settings,omitempty"`
	StreamSettings *xrayStreamSettings `json:"streamSettings,omitempty"`
}

type xraySocksInboundSettings struct {
	Auth string `json:"auth"`
	UDP  bool   `json:"udp"`
	IP   string `json:"ip,omitempty"`
}

type xrayHTTPInboundSettings struct {
	AllowTransparent bool `json:"allowTransparent"`
}

type xrayShadowsocksSettings struct {
	Servers []xrayShadowsocksServer `json:"servers"`
}

type xrayShadowsocksServer struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Method   string `json:"method"`
	Password string `json:"password"`
}

type xrayVLESSSettings struct {
	VNext []xrayVLESSServer `json:"vnext"`
}

type xrayVLESSServer struct {
	Address string          `json:"address"`
	Port    int             `json:"port"`
	Users   []xrayVLESSUser `json:"users"`
}

type xrayVLESSUser struct {
	ID         string `json:"id"`
	Encryption string `json:"encryption"`
	Flow       string `json:"flow,omitempty"`
}

type xrayStreamSettings struct {
	Network         string               `json:"network,omitempty"`
	Security        string               `json:"security,omitempty"`
	TLSSettings     *xrayTLSSettings     `json:"tlsSettings,omitempty"`
	RealitySettings *xrayRealitySettings `json:"realitySettings,omitempty"`
	Sockopt         *xraySockoptSettings `json:"sockopt,omitempty"`
	WSSettings      *xrayWSSettings      `json:"wsSettings,omitempty"`
}

type xrayTLSSettings struct {
	ServerName    string   `json:"serverName,omitempty"`
	AllowInsecure bool     `json:"allowInsecure,omitempty"`
	ALPN          []string `json:"alpn,omitempty"`
}

type xrayRealitySettings struct {
	ServerName  string `json:"serverName"`
	Password    string `json:"password"`
	ShortID     string `json:"shortId,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	SpiderX     string `json:"spiderX,omitempty"`
}

type xraySockoptSettings struct {
	Interface string `json:"interface,omitempty"`
}

type xrayWSSettings struct {
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func buildXrayLocalInbounds(local config.LocalProxy) ([]xrayInbound, error) {
	var inbounds []xrayInbound

	socksHost, socksPort, err := splitListenAddress(local.SOCKSAddress)
	if err != nil {
		return nil, fmt.Errorf("parse local SOCKS address: %w", err)
	}
	inbounds = append(inbounds, xrayInbound{
		Listen:   socksHost,
		Port:     socksPort,
		Protocol: "socks",
		Tag:      "socks-in",
		Settings: xraySocksInboundSettings{
			Auth: "noauth",
			UDP:  true,
			IP:   socksHost,
		},
		Sniffing: &xraySniffing{
			Enabled:      true,
			DestOverride: []string{"http", "tls", "quic"},
		},
	})

	if strings.TrimSpace(local.HTTPAddress) != "" {
		httpHost, httpPort, err := splitListenAddress(local.HTTPAddress)
		if err != nil {
			return nil, fmt.Errorf("parse local HTTP address: %w", err)
		}
		inbounds = append(inbounds, xrayInbound{
			Listen:   httpHost,
			Port:     httpPort,
			Protocol: "http",
			Tag:      "http-in",
			Settings: xrayHTTPInboundSettings{
				AllowTransparent: false,
			},
			Sniffing: &xraySniffing{
				Enabled:      true,
				DestOverride: []string{"http", "tls"},
			},
		})
	}

	return inbounds, nil
}

func marshalXrayConfig(inbounds []xrayInbound, outbound xrayOutbound) ([]byte, error) {
	document := xrayConfig{
		Log: xrayLog{
			LogLevel: "warning",
		},
		Inbounds:  inbounds,
		Outbounds: []xrayOutbound{outbound},
	}

	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal xray config: %w", err)
	}
	return data, nil
}

func splitListenAddress(address string) (string, int, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func cloneEnvironment(environment map[string]string) map[string]string {
	if len(environment) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(environment))
	for key, value := range environment {
		cloned[key] = value
	}
	return cloned
}
