package publish_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

func TestPublisherStoresImmutableVersionIdempotently(t *testing.T) {
	t.Parallel()

	seed := sha256.Sum256([]byte("publisher-key"))
	publisher := publish.NewPublisher(
		artifact.NewBuilder("release-key", ed25519.NewKeyFromSeed(seed[:])),
		publish.NewMemoryObjectStore(),
		publish.NewMemoryVersionRepository(),
	)
	input := publish.Input{
		UserID:     "user_01",
		Definition: definition("ver_01"),
		Files: map[string][]byte{
			"payload/native.json":     []byte(`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`),
			"reports/validation.json": []byte(`{"status":"passed"}`),
		},
		Preview:   map[string]any{"title": "预览"},
		CreatedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC),
	}

	first, err := publisher.Publish(context.Background(), input)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	repeated, err := publisher.Publish(context.Background(), input)
	if err != nil {
		t.Fatalf("repeated Publish() error = %v", err)
	}
	if first.ArtifactSHA256 != repeated.ArtifactSHA256 || first.VersionID != "ver_01" {
		t.Fatalf("versions = %#v, %#v", first, repeated)
	}
	download, err := publisher.Download(context.Background(), "user_01", "card_01", "ver_01", time.Minute)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if download.URL == "" || download.KeyID != "release-key" || len(download.SHA256) != 64 {
		t.Fatalf("download = %#v", download)
	}
}

func TestPublisherRejectsVersionMutationAndCrossUserRead(t *testing.T) {
	t.Parallel()

	seed := sha256.Sum256([]byte("publisher-key"))
	publisher := publish.NewPublisher(
		artifact.NewBuilder("release-key", ed25519.NewKeyFromSeed(seed[:])),
		publish.NewMemoryObjectStore(),
		publish.NewMemoryVersionRepository(),
	)
	input := publish.Input{
		UserID:     "owner",
		Definition: definition("ver_01"),
		Files: map[string][]byte{
			"payload/native.json":     []byte(`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`),
			"reports/validation.json": []byte(`{"status":"passed"}`),
		},
	}
	if _, err := publisher.Publish(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	input.Files["payload/native.json"] = []byte(`{"different":true}`)
	if _, err := publisher.Publish(context.Background(), input); !errors.Is(err, publish.ErrVersionConflict) {
		t.Fatalf("mutating Publish() error = %v", err)
	}
	if _, err := publisher.Download(context.Background(), "other", "card_01", "ver_01", time.Minute); !errors.Is(err, publish.ErrNotFound) {
		t.Fatalf("cross-user Download() error = %v", err)
	}
}

func definition(versionID string) contracts.CardDefinition {
	return contracts.CardDefinition{
		FormatVersion:      1,
		MinHostVersion:     "1.0.0",
		CardID:             "card_01",
		VersionID:          versionID,
		DisplayVersion:     "1.0.0",
		Runtime:            contracts.CardRuntimeNative,
		StateSchemaVersion: 1,
		Title:              "卡片",
		Entrypoint:         "payload/native.json",
		CatalogVersion:     "1",
		MinSize:            contracts.Size{Width: 240, Height: 160},
		PreferredSize:      contracts.Size{Width: 360, Height: 240},
		MaxSize:            contracts.Size{Width: 720, Height: 480},
		NetworkPolicy:      contracts.NetworkPolicy{Mode: "none", Domains: []string{}},
	}
}
