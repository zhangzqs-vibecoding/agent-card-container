package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
)

var ErrUnauthenticated = errors.New("unauthenticated")

type Authenticator interface {
	Authenticate(*http.Request) (string, error)
}

type StaticBearerAuthenticator map[string]string

func (authenticator StaticBearerAuthenticator) Authenticate(request *http.Request) (string, error) {
	const prefix = "Bearer "
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return "", ErrUnauthenticated
	}
	userID, ok := authenticator[strings.TrimPrefix(header, prefix)]
	if !ok || userID == "" {
		return "", ErrUnauthenticated
	}
	return userID, nil
}

type GenerationHandlerConfig struct {
	ServiceName   string
	Service       *generation.Service
	Authenticator Authenticator
	NewRequestID  func() string
}

type generationAPI struct {
	service       *generation.Service
	authenticator Authenticator
	newRequestID  func() string
}

func NewGenerationHandler(config GenerationHandlerConfig) http.Handler {
	api := &generationAPI{
		service:       config.Service,
		authenticator: config.Authenticator,
		newRequestID:  config.NewRequestID,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/generations", api.create)
	mux.HandleFunc("POST /v1/generations/{id}/messages", api.addMessage)
	mux.HandleFunc("POST /v1/generations/{id}/confirm", api.confirm)
	mux.HandleFunc("POST /v1/generations/{id}/cancel", api.cancel)
	mux.HandleFunc("GET /v1/generations/{id}", api.get)
	mux.HandleFunc("GET /v1/generations/{id}/events", api.events)
	return mux
}

func (api *generationAPI) create(writer http.ResponseWriter, request *http.Request) {
	userID, requestID, ok := api.authorize(writer, request)
	if !ok {
		return
	}
	var input struct {
		Prompt        string            `json:"prompt"`
		Target        generation.Target `json:"target"`
		Locale        string            `json:"locale"`
		BaseCardID    string            `json:"baseCardId"`
		BaseVersionID string            `json:"baseVersionId"`
	}
	if err := decodeStrictJSON(request, &input); err != nil {
		writeAPIError(writer, requestID, http.StatusBadRequest, "INVALID_REQUEST", "请求格式不正确")
		return
	}
	session, err := api.service.Create(request.Context(), userID, generation.CreateRequest{
		Prompt:        input.Prompt,
		Target:        input.Target,
		Locale:        input.Locale,
		BaseCardID:    input.BaseCardID,
		BaseVersionID: input.BaseVersionID,
	})
	if err != nil {
		api.writeServiceError(writer, requestID, err)
		return
	}
	writeJSON(writer, http.StatusCreated, session)
}

func (api *generationAPI) addMessage(writer http.ResponseWriter, request *http.Request) {
	userID, requestID, ok := api.authorize(writer, request)
	if !ok {
		return
	}
	var input struct {
		Content string `json:"content"`
	}
	if err := decodeStrictJSON(request, &input); err != nil {
		writeAPIError(writer, requestID, http.StatusBadRequest, "INVALID_REQUEST", "请求格式不正确")
		return
	}
	session, err := api.service.AddMessage(request.Context(), userID, request.PathValue("id"), input.Content)
	if err != nil {
		api.writeServiceError(writer, requestID, err)
		return
	}
	writeJSON(writer, http.StatusOK, session)
}

func (api *generationAPI) confirm(writer http.ResponseWriter, request *http.Request) {
	userID, requestID, ok := api.authorize(writer, request)
	if !ok {
		return
	}
	session, err := api.service.Confirm(request.Context(), userID, request.PathValue("id"))
	if err != nil {
		api.writeServiceError(writer, requestID, err)
		return
	}
	writeJSON(writer, http.StatusOK, session)
}

func (api *generationAPI) cancel(writer http.ResponseWriter, request *http.Request) {
	userID, requestID, ok := api.authorize(writer, request)
	if !ok {
		return
	}
	session, err := api.service.Cancel(request.Context(), userID, request.PathValue("id"))
	if err != nil {
		api.writeServiceError(writer, requestID, err)
		return
	}
	writeJSON(writer, http.StatusOK, session)
}

func (api *generationAPI) get(writer http.ResponseWriter, request *http.Request) {
	userID, requestID, ok := api.authorize(writer, request)
	if !ok {
		return
	}
	session, err := api.service.Get(request.Context(), userID, request.PathValue("id"))
	if err != nil {
		api.writeServiceError(writer, requestID, err)
		return
	}
	writeJSON(writer, http.StatusOK, session)
}

func (api *generationAPI) events(writer http.ResponseWriter, request *http.Request) {
	userID, requestID, ok := api.authorize(writer, request)
	if !ok {
		return
	}
	var after int64
	if raw := request.Header.Get("Last-Event-ID"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeAPIError(writer, requestID, http.StatusBadRequest, "INVALID_REQUEST", "Last-Event-ID 不正确")
			return
		}
		after = parsed
	}
	events, cancel, err := api.service.SubscribeEvents(request.Context(), userID, request.PathValue("id"), after)
	if err != nil {
		api.writeServiceError(writer, requestID, err)
		return
	}
	defer cancel()
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
	for event := range events {
		data, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return
		}
		_, _ = fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", event.EventID, event.Type, data)
		flusher.Flush()
	}
}

func (api *generationAPI) authorize(writer http.ResponseWriter, request *http.Request) (string, string, bool) {
	requestID := observability.RequestSnapshot(request.Context()).RequestID
	if requestID == "" {
		requestID = api.newRequestID()
	}
	writer.Header().Set("X-Request-ID", requestID)
	userID, err := api.authenticator.Authenticate(request)
	if err != nil {
		writeAPIError(writer, requestID, http.StatusUnauthorized, "UNAUTHENTICATED", "请先登录")
		return "", requestID, false
	}
	observability.SetAuthenticatedUser(request.Context(), userID)
	return userID, requestID, true
}

func (api *generationAPI) writeServiceError(writer http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, generation.ErrNotFound):
		writeAPIError(writer, requestID, http.StatusNotFound, "NOT_FOUND", "生成会话不存在")
	case errors.Is(err, generation.ErrConflict):
		writeAPIError(writer, requestID, http.StatusConflict, "CONFLICT", "当前状态不允许此操作")
	case generation.IsClientError(err):
		writeAPIError(writer, requestID, http.StatusBadRequest, "INVALID_REQUEST", "请求无法处理")
	default:
		writeAPIError(writer, requestID, http.StatusInternalServerError, "INTERNAL", "服务暂时不可用")
	}
}

func decodeStrictJSON(request *http.Request, destination any) error {
	if request.Header.Get("Content-Type") != "application/json" {
		return fmt.Errorf("content type must be application/json")
	}
	reader := http.MaxBytesReader(nil, request.Body, 256*1024)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request must contain one JSON value")
	}
	return nil
}

type apiErrorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

func writeAPIError(writer http.ResponseWriter, requestID string, status int, code, message string) {
	writeJSON(writer, status, apiErrorEnvelope{Error: apiError{
		Code:      code,
		Message:   message,
		RequestID: requestID,
	}})
}

func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}

func randomRequestID() string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic("crypto/rand unavailable")
	}
	return "req_" + hex.EncodeToString(bytes[:])
}
