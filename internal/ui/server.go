package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"vpnclient/internal/config"
)

type vpnService interface {
	Connect(context.Context, config.Profile) error
	Disconnect(context.Context) error
}

type configRequest struct {
	Config string `json:"config"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type ServerOptions struct {
	DefaultProfile *config.Profile
	SupportURL     string
}

type ProfileSummary struct {
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	Server       string `json:"server"`
	SOCKSAddress string `json:"socks_address"`
	HTTPAddress  string `json:"http_address"`
}

type SessionStatus struct {
	State             string          `json:"state"`
	Message           string          `json:"message"`
	Connected         bool            `json:"connected"`
	Profile           *ProfileSummary `json:"profile,omitempty"`
	HasDefaultProfile bool            `json:"has_default_profile"`
	SupportTarget     string          `json:"support_target,omitempty"`
}

type ValidateResponse struct {
	Message string         `json:"message"`
	Profile ProfileSummary `json:"profile"`
}

type Server struct {
	logger         *slog.Logger
	service        vpnService
	mux            *http.ServeMux
	defaultProfile *config.Profile
	supportURL     string

	mu     sync.RWMutex
	status SessionStatus
}

func NewServer(logger *slog.Logger, service vpnService) *Server {
	return NewServerWithOptions(logger, service, ServerOptions{})
}

func NewServerWithOptions(
	logger *slog.Logger,
	service vpnService,
	options ServerOptions,
) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	server := &Server{
		logger:         logger,
		service:        service,
		mux:            http.NewServeMux(),
		defaultProfile: cloneProfilePointer(options.DefaultProfile),
		supportURL:     strings.TrimSpace(options.SupportURL),
	}
	server.status = server.buildStatus(nil, "idle", server.initialMessage(), false)
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.RLock()
	connected := s.status.Connected
	s.mu.RUnlock()

	if !connected {
		return nil
	}

	disconnectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.service.Disconnect(disconnectCtx); err != nil {
		return err
	}

	s.mu.Lock()
	s.status = s.buildStatus(
		nil,
		"idle",
		"Подключение остановлено при завершении панели управления.",
		false,
	)
	s.mu.Unlock()

	return nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("POST /api/validate", s.handleValidate)
	s.mux.HandleFunc("POST /api/connect", s.handleConnect)
	s.mux.HandleFunc("POST /api/connect-default", s.handleConnectDefault)
	s.mux.HandleFunc("POST /api/disconnect", s.handleDisconnect)
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	status := cloneStatus(s.status)
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	profile, err := s.decodeProfileRequest(w, r)
	if err != nil {
		return
	}

	response := ValidateResponse{
		Message: "Профиль валиден и готов к запуску.",
		Profile: summarizeProfile(profile),
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	profile, err := s.decodeProfileRequest(w, r)
	if err != nil {
		return
	}
	s.handleConnectProfile(w, r, profile)
}

func (s *Server) handleConnectDefault(w http.ResponseWriter, r *http.Request) {
	if s.defaultProfile == nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("preset profile is not configured"))
		return
	}
	s.handleConnectProfile(w, r, *s.defaultProfile)
}

func (s *Server) handleConnectProfile(
	w http.ResponseWriter,
	r *http.Request,
	profile config.Profile,
) {
	s.mu.Lock()
	if s.status.Connected {
		current := cloneStatus(s.status)
		s.mu.Unlock()
		currentName := "active session"
		if current.Profile != nil && current.Profile.Name != "" {
			currentName = current.Profile.Name
		}
		writeError(w, http.StatusConflict, fmt.Errorf("a VPN session is already active: %s", currentName))
		return
	}
	s.status = s.buildStatus(
		&profile,
		"connecting",
		"Запускаю VPN runtime и применяю локальные системные настройки.",
		false,
	)
	s.mu.Unlock()

	connectCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := s.service.Connect(connectCtx, profile); err != nil {
		s.mu.Lock()
		s.status = s.buildStatus(nil, "error", err.Error(), false)
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, err)
		return
	}

	status := s.buildStatus(
		&profile,
		"connected",
		"Подключено. Локальный runtime активен, а временные системные proxy-настройки применены там, где платформа это поддерживает.",
		true,
	)

	s.mu.Lock()
	s.status = status
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	disconnectCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.service.Disconnect(disconnectCtx); err != nil {
		s.mu.Lock()
		s.status = s.buildStatus(nil, "error", err.Error(), false)
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	status := s.buildStatus(
		nil,
		"idle",
		"Отключено. Временные системные proxy-настройки восстановлены.",
		false,
	)

	s.mu.Lock()
	s.status = status
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, status)
}

func (s *Server) decodeProfileRequest(w http.ResponseWriter, r *http.Request) (config.Profile, error) {
	body := http.MaxBytesReader(w, r.Body, 1<<20)
	defer body.Close()

	requestData, err := io.ReadAll(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("read request body: %w", err))
		return config.Profile{}, err
	}

	var request configRequest
	if err := json.Unmarshal(requestData, &request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode request body: %w", err))
		return config.Profile{}, err
	}

	profile, err := config.NormalizeProfile([]byte(request.Config))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return config.Profile{}, err
	}

	return profile, nil
}

func (s *Server) initialMessage() string {
	if s.defaultProfile != nil {
		return "Главное готово. Можно подключаться по заранее выбранному конфигу."
	}
	return "Вставьте JSON-конфиг, проверьте его и подключитесь."
}

func (s *Server) buildStatus(
	profile *config.Profile,
	state string,
	message string,
	connected bool,
) SessionStatus {
	activeProfile := profile
	if activeProfile == nil {
		activeProfile = s.defaultProfile
	}

	var summary *ProfileSummary
	if activeProfile != nil {
		summary = profileSummaryPointer(summarizeProfile(*activeProfile))
	}

	return SessionStatus{
		State:             state,
		Message:           message,
		Connected:         connected,
		Profile:           summary,
		HasDefaultProfile: s.defaultProfile != nil,
		SupportTarget:     s.buildSupportTarget(activeProfile, state),
	}
}

func (s *Server) buildSupportTarget(profile *config.Profile, state string) string {
	if s.supportURL != "" {
		return s.supportURL
	}

	subject := "VPN Support Request"
	bodyLines := []string{
		"Hello, I need help with my VPN connection.",
	}

	if profile != nil {
		bodyLines = append(bodyLines,
			"",
			"Profile: "+profile.Name,
			"Protocol: "+string(profile.Protocol),
			fmt.Sprintf("Server: %s:%d", profile.Server.Host, profile.Server.Port),
		)
	}

	if state != "" {
		bodyLines = append(bodyLines, "Status: "+state)
	}

	return "mailto:?subject=" + url.QueryEscape(subject) +
		"&body=" + url.QueryEscape(strings.Join(bodyLines, "\n"))
}

func summarizeProfile(profile config.Profile) ProfileSummary {
	return ProfileSummary{
		Name:         profile.Name,
		Protocol:     string(profile.Protocol),
		Server:       fmt.Sprintf("%s:%d", profile.Server.Host, profile.Server.Port),
		SOCKSAddress: profile.Local.SOCKSAddress,
		HTTPAddress:  profile.Local.HTTPAddress,
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorResponse{
		Error: err.Error(),
	})
}

func cloneStatus(status SessionStatus) SessionStatus {
	cloned := status
	if status.Profile != nil {
		profile := *status.Profile
		cloned.Profile = &profile
	}
	return cloned
}

func cloneProfilePointer(profile *config.Profile) *config.Profile {
	if profile == nil {
		return nil
	}

	cloned := *profile
	return &cloned
}

func profileSummaryPointer(summary ProfileSummary) *ProfileSummary {
	value := summary
	return &value
}
