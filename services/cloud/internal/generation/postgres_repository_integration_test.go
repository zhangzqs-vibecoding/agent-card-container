package generation_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
)

func TestPostgresRepositoryPersistsAndSerializesUpdates(t *testing.T) {
	dsn := os.Getenv("AGENTCARD_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("AGENTCARD_POSTGRES_TEST_DSN is not configured")
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	resetPostgres(t, database)
	repository := generation.NewPostgresRepository(database)
	service := generation.NewService(
		repository,
		func() string { return "gen_postgres" },
		func() time.Time { return time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC) },
	)

	session, err := service.Create(context.Background(), "owner", generation.CreateRequest{
		Prompt: "离线番茄钟",
		Target: generation.TargetNative,
		Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddMessage(context.Background(), "owner", session.ID, "增加暂停按钮"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(context.Background(), "owner", session.ID); err != nil {
		t.Fatal(err)
	}

	restored, err := repository.Get(context.Background(), "owner", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != generation.StatusQueued || len(restored.Messages) != 2 || len(restored.Events) != 3 {
		t.Fatalf("restored session = %#v", restored)
	}
	if _, err := repository.Get(context.Background(), "other", session.ID); err != generation.ErrNotFound {
		t.Fatalf("other owner error = %v, want ErrNotFound", err)
	}
	streamContext, cancelStream := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStream()
	events, cancel, err := service.SubscribeEvents(streamContext, "owner", session.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	workerService := generation.NewService(
		generation.NewPostgresRepository(database),
		func() string { return "unused" },
		time.Now,
	)
	if _, err := workerService.StartGenerating(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		if event.EventID != 4 || event.Stage != "generating" {
			t.Fatalf("cross-process event = %#v", event)
		}
	case <-streamContext.Done():
		t.Fatal("subscriber did not observe the external repository update")
	}
}

func resetPostgres(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..")
	for _, name := range []string{"001_initial.sql", "002_production_fields.sql"} {
		source, err := os.ReadFile(filepath.Join(root, "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(string(source)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
}
