package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

type CardHandlerConfig struct {
	Publisher     *publish.Publisher
	Authenticator Authenticator
	NewRequestID  func() string
}

type cardAPI struct {
	publisher     *publish.Publisher
	authenticator Authenticator
	newRequestID  func() string
}

func NewCardHandler(config CardHandlerConfig) http.Handler {
	api := &cardAPI{
		publisher:     config.Publisher,
		authenticator: config.Authenticator,
		newRequestID:  config.NewRequestID,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/cards", api.list)
	mux.HandleFunc("GET /v1/cards/{cardId}", api.get)
	mux.HandleFunc("GET /v1/cards/{cardId}/versions/{versionId}/artifact", api.download)
	return mux
}

func (api *cardAPI) list(writer http.ResponseWriter, request *http.Request) {
	userID, requestID, ok := api.authorize(writer, request)
	if !ok {
		return
	}
	cards, err := api.publisher.ListCards(request.Context(), userID)
	if err != nil {
		writeAPIError(writer, requestID, http.StatusInternalServerError, "INTERNAL", "服务暂时不可用")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"cards": cards})
}

func (api *cardAPI) get(writer http.ResponseWriter, request *http.Request) {
	userID, requestID, ok := api.authorize(writer, request)
	if !ok {
		return
	}
	card, err := api.publisher.Card(request.Context(), userID, request.PathValue("cardId"))
	if err != nil {
		api.writeError(writer, requestID, err)
		return
	}
	writeJSON(writer, http.StatusOK, card)
}

func (api *cardAPI) download(writer http.ResponseWriter, request *http.Request) {
	userID, requestID, ok := api.authorize(writer, request)
	if !ok {
		return
	}
	download, err := api.publisher.Download(
		request.Context(),
		userID,
		request.PathValue("cardId"),
		request.PathValue("versionId"),
		5*time.Minute,
	)
	if err != nil {
		api.writeError(writer, requestID, err)
		return
	}
	writeJSON(writer, http.StatusOK, download)
}

func (api *cardAPI) authorize(
	writer http.ResponseWriter,
	request *http.Request,
) (string, string, bool) {
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

func (api *cardAPI) writeError(writer http.ResponseWriter, requestID string, err error) {
	if errors.Is(err, publish.ErrNotFound) {
		writeAPIError(writer, requestID, http.StatusNotFound, "NOT_FOUND", "卡片版本不存在")
		return
	}
	writeAPIError(writer, requestID, http.StatusInternalServerError, "INTERNAL", "服务暂时不可用")
}
