package httpapi_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
	"github.com/zzq/agent-card-container/services/cloud/internal/httpapi"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

func TestCardsAPIListsVersionsAndReturnsArtifactDownload(t *testing.T) {
	t.Parallel()

	seed := sha256.Sum256([]byte("api-key"))
	publisher := publish.NewPublisher(
		artifact.NewBuilder("release-key", ed25519.NewKeyFromSeed(seed[:])),
		publish.NewMemoryObjectStore(),
		publish.NewMemoryVersionRepository(),
	)
	definition := contracts.CardDefinition{
		FormatVersion:      1,
		MinHostVersion:     "1.0.0",
		CardID:             "card_01",
		VersionID:          "ver_01",
		DisplayVersion:     "1.0.0",
		Runtime:            contracts.CardRuntimeNative,
		StateSchemaVersion: 1,
		Title:              "番茄钟",
		Entrypoint:         "payload/native.json",
		CatalogVersion:     "1",
		MinSize:            contracts.Size{Width: 240, Height: 160},
		PreferredSize:      contracts.Size{Width: 360, Height: 240},
		MaxSize:            contracts.Size{Width: 720, Height: 480},
		NetworkPolicy:      contracts.NetworkPolicy{Mode: "none", Domains: []string{}},
	}
	if _, err := publisher.Publish(context.Background(), publish.Input{
		UserID:     "user-owner",
		Definition: definition,
		Files: map[string][]byte{
			"payload/native.json":     []byte(`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`),
			"reports/validation.json": []byte(`{"status":"passed"}`),
		},
	}); err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewCardHandler(httpapi.CardHandlerConfig{
		Publisher: publisher,
		Authenticator: httpapi.StaticBearerAuthenticator{
			"token-owner": "user-owner",
		},
		NewRequestID: func() string { return "req_01" },
	})

	cards := cardRequest(t, handler, "/v1/cards")
	if len(cards["cards"].([]any)) != 1 {
		t.Fatalf("cards = %#v", cards)
	}
	detail := cardRequest(t, handler, "/v1/cards/card_01")
	if len(detail["versions"].([]any)) != 1 {
		t.Fatalf("detail = %#v", detail)
	}
	download := cardRequest(t, handler, "/v1/cards/card_01/versions/ver_01/artifact")
	if download["url"] == "" || download["keyId"] != "release-key" {
		t.Fatalf("download = %#v", download)
	}
}

func cardRequest(t *testing.T, handler http.Handler, path string) map[string]any {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer token-owner")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d; body=%s", path, response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}
