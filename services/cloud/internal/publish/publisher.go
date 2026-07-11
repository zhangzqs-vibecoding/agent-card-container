package publish

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
)

var (
	ErrNotFound        = errors.New("card version not found")
	ErrVersionConflict = errors.New("immutable card version conflict")
	ErrObjectConflict  = errors.New("object content conflict")
)

type CardVersion struct {
	VersionID      string         `json:"versionId"`
	CardID         string         `json:"cardId"`
	UserID         string         `json:"-"`
	Runtime        string         `json:"runtime"`
	DisplayVersion string         `json:"displayVersion"`
	ArtifactKey    string         `json:"-"`
	ArtifactSHA256 string         `json:"artifactSha256"`
	KeyID          string         `json:"keyId"`
	Preview        map[string]any `json:"preview"`
	CreatedAt      time.Time      `json:"createdAt"`
}

type Input struct {
	UserID     string
	Definition contracts.CardDefinition
	Files      map[string][]byte
	Preview    map[string]any
	CreatedAt  time.Time
}

type Download struct {
	URL       string    `json:"url"`
	SHA256    string    `json:"sha256"`
	KeyID     string    `json:"keyId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type ObjectStore interface {
	PutIfAbsent(context.Context, string, []byte) error
	SignedURL(context.Context, string, time.Duration) (string, time.Time, error)
}

type VersionRepository interface {
	Create(context.Context, CardVersion) (CardVersion, error)
	Find(context.Context, string, string, string) (CardVersion, error)
}

type Publisher struct {
	builder  *artifact.Builder
	objects  ObjectStore
	versions VersionRepository
}

func NewPublisher(
	builder *artifact.Builder,
	objects ObjectStore,
	versions VersionRepository,
) *Publisher {
	return &Publisher{builder: builder, objects: objects, versions: versions}
}

func (publisher *Publisher) Publish(ctx context.Context, input Input) (CardVersion, error) {
	built, err := publisher.builder.Build(artifact.BuildInput{
		Definition: input.Definition,
		Files:      input.Files,
	})
	if err != nil {
		return CardVersion{}, err
	}
	objectKey := "artifacts/sha256/" + built.SHA256 + ".agentcard"
	if err := publisher.objects.PutIfAbsent(ctx, objectKey, built.Archive); err != nil {
		return CardVersion{}, err
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	return publisher.versions.Create(ctx, CardVersion{
		VersionID:      input.Definition.VersionID,
		CardID:         input.Definition.CardID,
		UserID:         input.UserID,
		Runtime:        string(input.Definition.Runtime),
		DisplayVersion: input.Definition.DisplayVersion,
		ArtifactKey:    objectKey,
		ArtifactSHA256: built.SHA256,
		KeyID:          built.KeyID,
		Preview:        cloneMap(input.Preview),
		CreatedAt:      input.CreatedAt.UTC(),
	})
}

func (publisher *Publisher) Download(
	ctx context.Context,
	userID string,
	cardID string,
	versionID string,
	ttl time.Duration,
) (Download, error) {
	version, err := publisher.versions.Find(ctx, userID, cardID, versionID)
	if err != nil {
		return Download{}, err
	}
	signedURL, expiresAt, err := publisher.objects.SignedURL(ctx, version.ArtifactKey, ttl)
	if err != nil {
		return Download{}, err
	}
	return Download{
		URL:       signedURL,
		SHA256:    version.ArtifactSHA256,
		KeyID:     version.KeyID,
		ExpiresAt: expiresAt,
	}, nil
}

type MemoryObjectStore struct {
	mu      sync.RWMutex
	objects map[string][]byte
	now     func() time.Time
}

func NewMemoryObjectStore() *MemoryObjectStore {
	return &MemoryObjectStore{
		objects: make(map[string][]byte),
		now:     time.Now,
	}
}

func (store *MemoryObjectStore) PutIfAbsent(ctx context.Context, key string, content []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, exists := store.objects[key]; exists {
		if !bytes.Equal(existing, content) {
			return ErrObjectConflict
		}
		return nil
	}
	store.objects[key] = append([]byte(nil), content...)
	return nil
}

func (store *MemoryObjectStore) SignedURL(
	ctx context.Context,
	key string,
	ttl time.Duration,
) (string, time.Time, error) {
	if err := ctx.Err(); err != nil {
		return "", time.Time{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if _, exists := store.objects[key]; !exists {
		return "", time.Time{}, ErrNotFound
	}
	expiresAt := store.now().UTC().Add(ttl)
	return "memory://" + url.PathEscape(key) + "?expires=" + url.QueryEscape(expiresAt.Format(time.RFC3339)), expiresAt, nil
}

type MemoryVersionRepository struct {
	mu       sync.RWMutex
	versions map[string]CardVersion
}

func NewMemoryVersionRepository() *MemoryVersionRepository {
	return &MemoryVersionRepository{versions: make(map[string]CardVersion)}
}

func (repository *MemoryVersionRepository) Create(
	ctx context.Context,
	version CardVersion,
) (CardVersion, error) {
	if err := ctx.Err(); err != nil {
		return CardVersion{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if existing, exists := repository.versions[version.VersionID]; exists {
		if existing.ArtifactSHA256 != version.ArtifactSHA256 ||
			existing.CardID != version.CardID ||
			existing.UserID != version.UserID {
			return CardVersion{}, ErrVersionConflict
		}
		return cloneVersion(existing), nil
	}
	repository.versions[version.VersionID] = cloneVersion(version)
	return cloneVersion(version), nil
}

func (repository *MemoryVersionRepository) Find(
	ctx context.Context,
	userID string,
	cardID string,
	versionID string,
) (CardVersion, error) {
	if err := ctx.Err(); err != nil {
		return CardVersion{}, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	version, exists := repository.versions[versionID]
	if !exists || version.UserID != userID || version.CardID != cardID {
		return CardVersion{}, ErrNotFound
	}
	return cloneVersion(version), nil
}

func cloneVersion(version CardVersion) CardVersion {
	version.Preview = cloneMap(version.Preview)
	return version
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
