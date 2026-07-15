package publish_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

func TestPostgresVersionRepositoryPreservesImmutableVersions(t *testing.T) {
	dsn := os.Getenv("AGENTCARD_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("AGENTCARD_POSTGRES_TEST_DSN is not configured")
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	resetPublishPostgres(t, database)
	repository := publish.NewPostgresVersionRepository(database)
	version := publish.CardVersion{
		VersionID: "ver_postgres_1", CardID: "card_postgres", UserID: "owner",
		Runtime: "native", DisplayVersion: "1.0.0", Title: "番茄钟",
		Description: "离线", ArtifactKey: "artifacts/hash.agentcard",
		ArtifactSHA256: "abc", KeyID: "release-key", Preview: map[string]any{"kind": "native"},
		CreatedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC),
	}

	created, err := repository.Create(context.Background(), version)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := repository.Create(context.Background(), version)
	if err != nil {
		t.Fatal(err)
	}
	if created.VersionID != repeated.VersionID {
		t.Fatalf("repeated version = %#v", repeated)
	}
	conflicting := version
	conflicting.ArtifactSHA256 = "different"
	if _, err := repository.Create(context.Background(), conflicting); err != publish.ErrVersionConflict {
		t.Fatalf("conflicting create error = %v", err)
	}
	found, err := repository.Find(context.Background(), "owner", version.CardID, version.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Title != version.Title || found.Preview["kind"] != "native" {
		t.Fatalf("found version = %#v", found)
	}
	versions, err := repository.ListByCard(context.Background(), "owner", version.CardID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions = %#v, error = %v", versions, err)
	}
	duplicateDisplay := version
	duplicateDisplay.VersionID = "ver_postgres_2"
	duplicateDisplay.ArtifactSHA256 = "def"
	if _, err := repository.Create(context.Background(), duplicateDisplay); err != publish.ErrDisplayVersionConflict {
		t.Fatalf("duplicate display version error = %v", err)
	}
}

func resetPublishPostgres(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_initial.sql", "002_production_fields.sql"} {
		source, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(string(source)); err != nil {
			t.Fatal(err)
		}
	}
}
