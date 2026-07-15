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
	owned, err := publisher.OwnsVersion(context.Background(), "owner", "card_01", "ver_01")
	if err != nil || !owned {
		t.Fatalf("OwnsVersion(owner) = %v, %v", owned, err)
	}
	for _, identity := range [][3]string{
		{"other", "card_01", "ver_01"},
		{"owner", "card_other", "ver_01"},
		{"owner", "card_01", "ver_other"},
	} {
		owned, err := publisher.OwnsVersion(context.Background(), identity[0], identity[1], identity[2])
		if err != nil || owned {
			t.Fatalf("OwnsVersion(%v) = %v, %v", identity, owned, err)
		}
	}
}

func TestPublisherListsUserCardsAndVersionHistory(t *testing.T) {
	t.Parallel()

	seed := sha256.Sum256([]byte("publisher-key"))
	publisher := publish.NewPublisher(
		artifact.NewBuilder("release-key", ed25519.NewKeyFromSeed(seed[:])),
		publish.NewMemoryObjectStore(),
		publish.NewMemoryVersionRepository(),
	)
	for index, versionID := range []string{"ver_01", "ver_02"} {
		card := definition(versionID)
		card.DisplayVersion = []string{"1.0.0", "1.1.0"}[index]
		if _, err := publisher.Publish(context.Background(), publish.Input{
			UserID:     "owner",
			Definition: card,
			Files: map[string][]byte{
				"payload/native.json":     []byte(`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`),
				"reports/validation.json": []byte(`{"status":"passed"}`),
			},
			CreatedAt: time.Date(2026, 7, 12, 12+index, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatal(err)
		}
	}

	cards, err := publisher.ListCards(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].LatestVersion.VersionID != "ver_02" {
		t.Fatalf("cards = %#v", cards)
	}
	detail, err := publisher.Card(context.Background(), "owner", "card_01")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Versions) != 2 || detail.Versions[0].VersionID != "ver_02" {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestPublisherComputesStrictNextPatchDisplayVersion(t *testing.T) {
	t.Parallel()

	repository := publish.NewMemoryVersionRepository()
	publisher := publish.NewPublisher(nil, publish.NewMemoryObjectStore(), repository)
	for index, displayVersion := range []string{"1.0.0", "1.0.1", "0.99.99"} {
		if _, err := repository.Create(context.Background(), publish.CardVersion{
			VersionID: "ver_" + displayVersion,
			CardID:    "card_01", UserID: "owner", DisplayVersion: displayVersion,
			ArtifactSHA256: string(rune('a' + index)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	next, err := publisher.NextDisplayVersion(context.Background(), "owner", "card_01")
	if err != nil || next != "1.0.2" {
		t.Fatalf("NextDisplayVersion() = %q, %v", next, err)
	}
}

func TestPublisherRejectsInvalidDisplayVersionHistory(t *testing.T) {
	t.Parallel()

	repository := publish.NewMemoryVersionRepository()
	if _, err := repository.Create(context.Background(), publish.CardVersion{
		VersionID: "ver_invalid", CardID: "card_01", UserID: "owner",
		DisplayVersion: "v1", ArtifactSHA256: "hash",
	}); err != nil {
		t.Fatal(err)
	}
	publisher := publish.NewPublisher(nil, publish.NewMemoryObjectStore(), repository)
	if _, err := publisher.NextDisplayVersion(context.Background(), "owner", "card_01"); !errors.Is(err, publish.ErrInvalidDisplayVersion) {
		t.Fatalf("NextDisplayVersion() error = %v", err)
	}
}

func TestMemoryVersionRepositoryRejectsDuplicateDisplayVersion(t *testing.T) {
	t.Parallel()

	repository := publish.NewMemoryVersionRepository()
	first := publish.CardVersion{
		VersionID: "ver_01", CardID: "card_01", UserID: "owner",
		DisplayVersion: "1.0.0", ArtifactSHA256: "hash-1",
	}
	if _, err := repository.Create(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.VersionID = "ver_02"
	second.ArtifactSHA256 = "hash-2"
	if _, err := repository.Create(context.Background(), second); !errors.Is(err, publish.ErrDisplayVersionConflict) {
		t.Fatalf("duplicate display version error = %v", err)
	}
}

func TestPublisherLoadsOwnedArtifactWithinLimit(t *testing.T) {
	t.Parallel()

	seed := sha256.Sum256([]byte("publisher-load-key"))
	objects := publish.NewMemoryObjectStore()
	publisher := publish.NewPublisher(
		artifact.NewBuilder("release-key", ed25519.NewKeyFromSeed(seed[:])),
		objects,
		publish.NewMemoryVersionRepository(),
	)
	if _, err := publisher.Publish(context.Background(), publish.Input{
		UserID:     "owner",
		Definition: definition("ver_01"),
		Files: map[string][]byte{
			"payload/native.json": []byte(`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`),
		},
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := publisher.LoadVersionArtifact(
		context.Background(), "owner", "card_01", "ver_01", 8*1024*1024,
	)
	if err != nil {
		t.Fatalf("LoadVersionArtifact() error = %v", err)
	}
	if loaded.Version.VersionID != "ver_01" || len(loaded.Archive) == 0 {
		t.Fatalf("loaded artifact = %#v", loaded)
	}
	loaded.Archive[0] ^= 0xff
	reloaded, err := publisher.LoadVersionArtifact(
		context.Background(), "owner", "card_01", "ver_01", 8*1024*1024,
	)
	if err != nil || len(reloaded.Archive) == 0 || loaded.Archive[0] == reloaded.Archive[0] {
		t.Fatalf("stored artifact was not isolated: %#v, %v", reloaded, err)
	}

	if _, err := publisher.LoadVersionArtifact(
		context.Background(), "other", "card_01", "ver_01", 8*1024*1024,
	); !errors.Is(err, publish.ErrNotFound) {
		t.Fatalf("cross-user LoadVersionArtifact() error = %v", err)
	}
	if _, err := publisher.LoadVersionArtifact(
		context.Background(), "owner", "card_01", "ver_01", 1,
	); !errors.Is(err, publish.ErrObjectTooLarge) {
		t.Fatalf("oversized LoadVersionArtifact() error = %v", err)
	}
}

func TestMemoryObjectStoreGetRejectsMissingObject(t *testing.T) {
	t.Parallel()

	store := publish.NewMemoryObjectStore()
	if _, err := store.Get(context.Background(), "missing", 1024); !errors.Is(err, publish.ErrNotFound) {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestPublisherRejectsArtifactHashMismatch(t *testing.T) {
	t.Parallel()

	version := publish.CardVersion{
		UserID:         "owner",
		CardID:         "card_01",
		VersionID:      "ver_01",
		ArtifactKey:    "artifact",
		ArtifactSHA256: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}
	publisher := publish.NewPublisher(nil, staticObjectStore{content: []byte("tampered")}, staticVersionRepository{version: version})
	if _, err := publisher.LoadVersionArtifact(
		context.Background(), "owner", "card_01", "ver_01", 1024,
	); !errors.Is(err, publish.ErrArtifactIntegrity) {
		t.Fatalf("LoadVersionArtifact() error = %v", err)
	}
}

type staticObjectStore struct {
	content []byte
}

func (store staticObjectStore) PutIfAbsent(context.Context, string, []byte) error { return nil }
func (store staticObjectStore) SignedURL(context.Context, string, time.Duration) (string, time.Time, error) {
	return "", time.Time{}, nil
}
func (store staticObjectStore) Get(context.Context, string, int64) ([]byte, error) {
	return append([]byte(nil), store.content...), nil
}

type staticVersionRepository struct {
	version publish.CardVersion
}

func (repository staticVersionRepository) Create(context.Context, publish.CardVersion) (publish.CardVersion, error) {
	return publish.CardVersion{}, nil
}
func (repository staticVersionRepository) Find(context.Context, string, string, string) (publish.CardVersion, error) {
	return repository.version, nil
}
func (staticVersionRepository) ListByUser(context.Context, string) ([]publish.CardVersion, error) {
	return nil, nil
}
func (staticVersionRepository) ListByCard(context.Context, string, string) ([]publish.CardVersion, error) {
	return nil, nil
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
