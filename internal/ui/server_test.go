package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vpnclient/internal/config"
	"vpnclient/internal/ui"
)

const testConfigPayload = `{"config":"{\"type\":\"outline\",\"server\":{\"host\":\"198.51.100.10\",\"port\":443},\"cipher\":\"chacha20-ietf-poly1305\",\"password\":\"secret\",\"engine\":{\"binary\":\"xray\"}}"}`

var csrfTokenPattern = regexp.MustCompile(`const csrfToken = "([^"]+)";`)

type fakeService struct {
	connectCalls    atomic.Int32
	disconnectCalls atomic.Int32
	connectFn       func(context.Context, config.Profile) error
	disconnectFn    func(context.Context) error
}

func (s *fakeService) Connect(ctx context.Context, profile config.Profile) error {
	s.connectCalls.Add(1)
	if s.connectFn != nil {
		return s.connectFn(ctx, profile)
	}
	return nil
}

func (s *fakeService) Disconnect(ctx context.Context) error {
	s.disconnectCalls.Add(1)
	if s.disconnectFn != nil {
		return s.disconnectFn(ctx)
	}
	return nil
}

func csrfTokenForTest(t *testing.T, server *ui.Server) string {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	match := csrfTokenPattern.FindStringSubmatch(recorder.Body.String())
	if len(match) != 2 {
		t.Fatalf("failed to extract CSRF token from index page")
	}
	return match[1]
}

func newAPIRequest(method string, path string, body string, csrfToken string) *http.Request {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if csrfToken != "" {
		request.Header.Set("X-VPNClient-CSRF", csrfToken)
	}
	return request
}

func TestServerValidateAndConnect(t *testing.T) {
	t.Parallel()

	service := &fakeService{}
	server := ui.NewServer(nil, service)
	csrfToken := csrfTokenForTest(t, server)

	validateRequest := newAPIRequest(http.MethodPost, "/api/validate", testConfigPayload, csrfToken)

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

	connectRequest := newAPIRequest(http.MethodPost, "/api/connect", testConfigPayload, csrfToken)

	connectRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(connectRecorder, connectRequest)

	if got, want := connectRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("connect status = %d, want %d", got, want)
	}
	if got, want := service.connectCalls.Load(), int32(1); got != want {
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
	csrfToken := csrfTokenForTest(t, server)

	connectRequest := newAPIRequest(http.MethodPost, "/api/connect", testConfigPayload, csrfToken)
	connectRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(connectRecorder, connectRequest)

	disconnectRequest := newAPIRequest(http.MethodPost, "/api/disconnect", `{}`, csrfToken)
	disconnectRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(disconnectRecorder, disconnectRequest)

	if got, want := disconnectRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("disconnect status = %d, want %d", got, want)
	}
	if got, want := service.disconnectCalls.Load(), int32(1); got != want {
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
	csrfToken := csrfTokenForTest(t, server)

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

	connectRequest := newAPIRequest(http.MethodPost, "/api/connect-default", `{}`, csrfToken)
	connectRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(connectRecorder, connectRequest)

	if got, want := connectRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("connect-default status = %d, want %d", got, want)
	}
	if got, want := service.connectCalls.Load(), int32(1); got != want {
		t.Fatalf("connect calls = %d, want %d", got, want)
	}
}

func TestServerRejectsMissingCSRFToken(t *testing.T) {
	t.Parallel()

	server := ui.NewServer(nil, &fakeService{})
	request := newAPIRequest(http.MethodPost, "/api/connect", testConfigPayload, "")
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusForbidden; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestServerRejectsUnexpectedOrigin(t *testing.T) {
	t.Parallel()

	server := ui.NewServer(nil, &fakeService{})
	csrfToken := csrfTokenForTest(t, server)
	request := newAPIRequest(http.MethodPost, "/api/connect", testConfigPayload, csrfToken)
	request.Host = "127.0.0.1:18080"
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusForbidden; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestServerRejectsConcurrentConnectWhileConnecting(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	service := &fakeService{
		connectFn: func(context.Context, config.Profile) error {
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
			return nil
		},
	}
	server := ui.NewServer(nil, service)
	csrfToken := csrfTokenForTest(t, server)

	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	sendConnect := func() {
		defer wg.Done()
		request := newAPIRequest(http.MethodPost, "/api/connect", testConfigPayload, csrfToken)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		statuses <- recorder.Code
	}

	wg.Add(1)
	go sendConnect()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("first connect did not reach service")
	}

	wg.Add(1)
	go sendConnect()

	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	close(statuses)

	var gotStatuses []int
	for status := range statuses {
		gotStatuses = append(gotStatuses, status)
	}

	okCount := 0
	conflictCount := 0
	for _, status := range gotStatuses {
		switch status {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
			conflictCount++
		}
	}
	if got, want := okCount, 1; got != want {
		t.Fatalf("successful connect responses = %d, want %d; statuses=%v", got, want, gotStatuses)
	}
	if got, want := conflictCount, 1; got != want {
		t.Fatalf("conflict connect responses = %d, want %d; statuses=%v", got, want, gotStatuses)
	}
	if got, want := service.connectCalls.Load(), int32(1); got != want {
		t.Fatalf("Connect() calls = %d, want %d", got, want)
	}
}

func TestServerStatusUnderLoad(t *testing.T) {
	service := &fakeService{}
	server := ui.NewServer(nil, service)
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	client := testServer.Client()
	const workers = 16
	const requestsPerWorker = 50

	var wg sync.WaitGroup
	errCh := make(chan error, workers*requestsPerWorker)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				response, err := client.Get(testServer.URL + "/api/status")
				if err != nil {
					errCh <- err
					return
				}
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				if response.StatusCode != http.StatusOK {
					errCh <- fmt.Errorf("unexpected status code %d", response.StatusCode)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("status load test failed: %v", err)
		}
	}
}
