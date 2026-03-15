package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vpnclient/internal/config"
	"vpnclient/internal/ui"
)

type fakeService struct {
	connectCalls    int
	disconnectCalls int
}

func (s *fakeService) Connect(context.Context, config.Profile) error {
	s.connectCalls++
	return nil
}

func (s *fakeService) Disconnect(context.Context) error {
	s.disconnectCalls++
	return nil
}

func TestServerValidateAndConnect(t *testing.T) {
	t.Parallel()

	service := &fakeService{}
	server := ui.NewServer(nil, service)

	validateRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/validate",
		bytes.NewBufferString(`{"config":"{\"type\":\"outline\",\"server\":{\"host\":\"198.51.100.10\",\"port\":443},\"cipher\":\"chacha20-ietf-poly1305\",\"password\":\"secret\",\"engine\":{\"binary\":\"xray\"}}"}`),
	)
	validateRequest.Header.Set("Content-Type", "application/json")

	validateRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(validateRecorder, validateRequest)

	if got, want := validateRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("validate status = %d, want %d", got, want)
	}

	var validateResponse ui.ValidateResponse
	if err := json.Unmarshal(validateRecorder.Body.Bytes(), &validateResponse); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if got, want := validateResponse.Profile.Protocol, "outline"; got != want {
		t.Fatalf("validate protocol = %q, want %q", got, want)
	}

	connectRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/connect",
		bytes.NewBufferString(`{"config":"{\"type\":\"outline\",\"server\":{\"host\":\"198.51.100.10\",\"port\":443},\"cipher\":\"chacha20-ietf-poly1305\",\"password\":\"secret\",\"engine\":{\"binary\":\"xray\"}}"}`),
	)
	connectRequest.Header.Set("Content-Type", "application/json")

	connectRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(connectRecorder, connectRequest)

	if got, want := connectRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("connect status = %d, want %d", got, want)
	}
	if got, want := service.connectCalls, 1; got != want {
		t.Fatalf("connect calls = %d, want %d", got, want)
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, statusRequest)

	var status ui.SessionStatus
	if err := json.Unmarshal(statusRecorder.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if !status.Connected {
		t.Fatalf("status connected = false, want true")
	}
	if got, want := status.Profile.Name, "outline-198.51.100.10-443"; got != want {
		t.Fatalf("status profile name = %q, want %q", got, want)
	}
}

func TestServerDisconnect(t *testing.T) {
	t.Parallel()

	service := &fakeService{}
	server := ui.NewServer(nil, service)

	connectRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/connect",
		bytes.NewBufferString(`{"config":"{\"type\":\"outline\",\"server\":{\"host\":\"198.51.100.10\",\"port\":443},\"cipher\":\"chacha20-ietf-poly1305\",\"password\":\"secret\",\"engine\":{\"binary\":\"xray\"}}"}`),
	)
	connectRequest.Header.Set("Content-Type", "application/json")
	connectRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(connectRecorder, connectRequest)

	disconnectRequest := httptest.NewRequest(http.MethodPost, "/api/disconnect", bytes.NewBufferString(`{}`))
	disconnectRequest.Header.Set("Content-Type", "application/json")
	disconnectRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(disconnectRecorder, disconnectRequest)

	if got, want := disconnectRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("disconnect status = %d, want %d", got, want)
	}
	if got, want := service.disconnectCalls, 1; got != want {
		t.Fatalf("disconnect calls = %d, want %d", got, want)
	}
}

func TestServerConnectDefaultProfile(t *testing.T) {
	t.Parallel()

	service := &fakeService{}
	profile := config.Profile{
		Name:     "default-profile",
		Protocol: config.ProtocolXrayReality,
		Server: config.Server{
			Host: "144.31.13.152",
			Port: 2443,
		},
		Credentials: config.Credentials{
			UUID: "0834236D-4C9F-4180-AECC-8786DCB5A374",
		},
		Transport: config.Transport{
			Reality: &config.Reality{
				PublicKey:  "test-public-key",
				ServerName: "www.cloudflare.com",
			},
		},
		Local: config.LocalProxy{
			SOCKSAddress: "127.0.0.1:1080",
			HTTPAddress:  "127.0.0.1:1081",
		},
		Engine: config.Engine{
			Binary: "xray",
		},
	}

	server := ui.NewServerWithOptions(nil, service, ui.ServerOptions{
		DefaultProfile: &profile,
	})

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, statusRequest)

	var initialStatus ui.SessionStatus
	if err := json.Unmarshal(statusRecorder.Body.Bytes(), &initialStatus); err != nil {
		t.Fatalf("decode initial status response: %v", err)
	}
	if !initialStatus.HasDefaultProfile {
		t.Fatalf("HasDefaultProfile = false, want true")
	}
	if initialStatus.Profile == nil || initialStatus.Profile.Name != "default-profile" {
		t.Fatalf("default profile is missing from initial status")
	}
	if initialStatus.SupportTarget == "" {
		t.Fatalf("SupportTarget is empty")
	}

	connectRequest := httptest.NewRequest(http.MethodPost, "/api/connect-default", bytes.NewBufferString(`{}`))
	connectRequest.Header.Set("Content-Type", "application/json")
	connectRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(connectRecorder, connectRequest)

	if got, want := connectRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("connect-default status = %d, want %d", got, want)
	}
	if got, want := service.connectCalls, 1; got != want {
		t.Fatalf("connect calls = %d, want %d", got, want)
	}
}
