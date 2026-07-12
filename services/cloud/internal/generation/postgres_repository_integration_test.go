package generation_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/migrations"
)

func TestPostgresRepositoryPersistsAndSerializesUpdates(t *testing.T) {
	database := openPostgres(t)
	resetPostgres(t, database)
	repository := generation.NewPostgresRepository(database)
	current := time.Date(2026, 7, 13, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	service := generation.NewService(
		repository,
		func() string { return "gen_postgres" },
		func() time.Time { return current },
	)

	session, err := service.Create(context.Background(), "owner", generation.CreateRequest{
		Prompt: "离线番茄钟",
		Target: generation.TargetNative,
		Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeConfirmation, err := generation.NewPostgresRepository(database).Get(
		context.Background(),
		"owner",
		session.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if beforeConfirmation.ConfirmedRequirement != nil {
		t.Fatalf("draft ConfirmedRequirement = %#v, want nil", beforeConfirmation.ConfirmedRequirement)
	}

	current = current.Add(time.Minute)
	firstAdditionalAt := current.UTC()
	if _, err := service.AddMessage(context.Background(), "owner", session.ID, "增加暂停按钮"); err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Minute)
	secondAdditionalAt := current.UTC()
	if _, err := service.AddMessage(context.Background(), "owner", session.ID, "使用中文显示"); err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Minute)
	confirmedAt := current.UTC()
	if _, err := service.Confirm(context.Background(), "owner", session.ID); err != nil {
		t.Fatal(err)
	}

	wantRequirement := &generation.RequirementSnapshot{
		InitialPrompt: "离线番茄钟",
		AdditionalMessages: []generation.Message{
			{Role: "user", Content: "增加暂停按钮", CreatedAt: firstAdditionalAt},
			{Role: "user", Content: "使用中文显示", CreatedAt: secondAdditionalAt},
		},
		Target:              generation.TargetNative,
		Locale:              "zh-CN",
		AllowedCapabilities: []string{"storage", "window.manageSelf"},
		ConfirmedAt:         confirmedAt,
	}
	restored, err := generation.NewPostgresRepository(database).Get(
		context.Background(),
		"owner",
		session.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != generation.StatusQueued || len(restored.Messages) != 3 || len(restored.Events) != 4 {
		t.Fatalf("restored session = %#v", restored)
	}
	assertRequirementSnapshot(t, restored.ConfirmedRequirement, wantRequirement)

	restored.ConfirmedRequirement.InitialPrompt = "已篡改"
	restored.ConfirmedRequirement.AdditionalMessages[0].Content = "已篡改"
	restored.ConfirmedRequirement.Target = generation.TargetWeb
	restored.ConfirmedRequirement.Locale = "en-US"
	restored.ConfirmedRequirement.AllowedCapabilities[0] = "network"
	restored.ConfirmedRequirement.ConfirmedAt = confirmedAt.Add(time.Hour)
	reloaded, err := generation.NewPostgresRepository(database).Get(
		context.Background(),
		"owner",
		session.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertRequirementSnapshot(t, reloaded.ConfirmedRequirement, wantRequirement)

	if _, err := repository.Get(context.Background(), "other", session.ID); err != generation.ErrNotFound {
		t.Fatalf("other owner error = %v, want ErrNotFound", err)
	}
	streamContext, cancelStream := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStream()
	events, cancel, err := service.SubscribeEvents(streamContext, "owner", session.ID, 4)
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
		if event.EventID != 5 || event.Stage != "generating" {
			t.Fatalf("cross-process event = %#v", event)
		}
	case <-streamContext.Done():
		t.Fatal("subscriber did not observe the external repository update")
	}
	afterUpdate, err := generation.NewPostgresRepository(database).Get(
		context.Background(),
		"owner",
		session.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertRequirementSnapshot(t, afterUpdate.ConfirmedRequirement, wantRequirement)
}

func TestPostgresRepositoryCreatePersistsConfirmedRequirement(t *testing.T) {
	database := openPostgres(t)
	resetPostgres(t, database)
	createdAt := time.Date(2026, 7, 13, 5, 0, 0, 0, time.UTC)
	confirmedAt := createdAt.Add(3 * time.Minute)
	wantRequirement := &generation.RequirementSnapshot{
		InitialPrompt: "创建离线待办",
		AdditionalMessages: []generation.Message{{
			Role:      "user",
			Content:   "支持排序",
			CreatedAt: createdAt.Add(time.Minute),
		}},
		Target:              generation.TargetAuto,
		Locale:              "zh-CN",
		AllowedCapabilities: []string{"storage", "window.manageSelf"},
		ConfirmedAt:         confirmedAt,
	}
	session := &generation.Session{
		ID:                   "gen_insert_confirmed",
		UserID:               "owner",
		Prompt:               wantRequirement.InitialPrompt,
		Target:               wantRequirement.Target,
		Locale:               wantRequirement.Locale,
		Status:               generation.StatusQueued,
		Summary:              generation.RequirementSummary{Goal: "创建离线待办\n支持排序", Locale: "zh-CN"},
		Messages:             append([]generation.Message{{Role: "user", Content: "创建离线待办", CreatedAt: createdAt}}, wantRequirement.AdditionalMessages...),
		ConfirmedRequirement: wantRequirement,
		CreatedAt:            createdAt,
		UpdatedAt:            confirmedAt,
		Events: []generation.Event{{
			EventID:   1,
			SessionID: "gen_insert_confirmed",
			Type:      "generation.progress",
			Stage:     "queued",
			Message:   "生成任务已入队",
			Progress:  0.2,
			Timestamp: confirmedAt,
		}},
	}
	if err := generation.NewPostgresRepository(database).Create(context.Background(), session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	restored, err := generation.NewPostgresRepository(database).Get(
		context.Background(),
		"owner",
		session.ID,
	)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	assertRequirementSnapshot(t, restored.ConfirmedRequirement, wantRequirement)
}

func TestPostgresRepositoryRejectsInvalidConfirmedRequirementBeforeCreate(t *testing.T) {
	database := openPostgres(t)
	resetPostgres(t, database)
	createdAt := time.Date(2026, 7, 13, 6, 0, 0, 0, time.UTC)
	requirement := validPostgresRequirement(createdAt)
	requirement.InitialPrompt = ""
	session := newPostgresTestSession(
		"gen_invalid_confirmed_create",
		generation.StatusQueued,
		requirement,
		createdAt,
	)

	err := generation.NewPostgresRepository(database).Create(context.Background(), session)
	const wantPrefix = "encode generation confirmed requirement:"
	if err == nil || !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("Create() error = %v, want prefix %q", err, wantPrefix)
	}
	var count int
	if err := database.QueryRow(
		`SELECT count(*) FROM generation_sessions WHERE id = $1`,
		session.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("persisted invalid session count = %d, want 0", count)
	}
}

func TestPostgresRepositoryEnforcesConfirmedRequirementWriteOnce(t *testing.T) {
	database := openPostgres(t)
	testCases := []struct {
		name   string
		update func(*generation.PostgresRepository, string) error
	}{
		{
			name: "Update mutates field",
			update: func(repository *generation.PostgresRepository, sessionID string) error {
				_, err := repository.Update(context.Background(), "owner", sessionID, func(session *generation.Session) error {
					session.ConfirmedRequirement.AdditionalMessages[0].Content = "已篡改"
					return nil
				})
				return err
			},
		},
		{
			name: "UpdateSystem clears snapshot",
			update: func(repository *generation.PostgresRepository, sessionID string) error {
				_, err := repository.UpdateSystem(context.Background(), sessionID, func(session *generation.Session) error {
					session.ConfirmedRequirement = nil
					return nil
				})
				return err
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetPostgres(t, database)
			repository, sessionID, wantRequirement := createConfirmedPostgresSession(t, database)
			if err := testCase.update(repository, sessionID); !errors.Is(err, generation.ErrConflict) {
				t.Fatalf("update error = %v, want ErrConflict", err)
			}
			restored, err := repository.Get(context.Background(), "owner", sessionID)
			if err != nil {
				t.Fatal(err)
			}
			assertRequirementSnapshot(t, restored.ConfirmedRequirement, wantRequirement)

			workerService := generation.NewService(repository, func() string { return "unused" }, time.Now)
			updated, err := workerService.StartGenerating(context.Background(), sessionID)
			if err != nil {
				t.Fatalf("ordinary status update error = %v", err)
			}
			if updated.Status != generation.StatusGenerating {
				t.Fatalf("ordinary status update = %q, want generating", updated.Status)
			}
			assertRequirementSnapshot(t, updated.ConfirmedRequirement, wantRequirement)
		})
	}
}

func TestPostgresRepositoryOnlySetsSnapshotWhenConfirming(t *testing.T) {
	database := openPostgres(t)
	testCases := []struct {
		name          string
		initialStatus generation.Status
		nextStatus    generation.Status
	}{
		{name: "awaiting without queued", initialStatus: generation.StatusAwaitingConfirmation, nextStatus: generation.StatusAwaitingConfirmation},
		{name: "draft to queued", initialStatus: generation.StatusDraft, nextStatus: generation.StatusQueued},
		{name: "awaiting to generating", initialStatus: generation.StatusAwaitingConfirmation, nextStatus: generation.StatusGenerating},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetPostgres(t, database)
			createdAt := time.Date(2026, 7, 13, 7, 0, 0, 0, time.UTC)
			session := newPostgresTestSession(
				"gen_illegal_snapshot_transition",
				testCase.initialStatus,
				nil,
				createdAt,
			)
			repository := generation.NewPostgresRepository(database)
			if err := repository.Create(context.Background(), session); err != nil {
				t.Fatal(err)
			}
			_, err := repository.Update(context.Background(), "owner", session.ID, func(candidate *generation.Session) error {
				candidate.Status = testCase.nextStatus
				candidate.ConfirmedRequirement = validPostgresRequirement(createdAt.Add(time.Minute))
				return nil
			})
			if !errors.Is(err, generation.ErrConflict) {
				t.Fatalf("Update() error = %v, want ErrConflict", err)
			}
			restored, err := repository.Get(context.Background(), "owner", session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if restored.Status != testCase.initialStatus || restored.ConfirmedRequirement != nil {
				t.Fatalf("stored session = %#v", restored)
			}
		})
	}

	t.Run("Service Confirm persists initial snapshot", func(t *testing.T) {
		resetPostgres(t, database)
		current := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
		repository := generation.NewPostgresRepository(database)
		service := generation.NewService(
			repository,
			func() string { return "gen_allowed_snapshot_transition" },
			func() time.Time { return current },
		)
		created, err := service.Create(context.Background(), "owner", generation.CreateRequest{
			Prompt: "允许确认的需求",
			Target: generation.TargetNative,
			Locale: "zh-CN",
		})
		if err != nil {
			t.Fatal(err)
		}
		current = current.Add(time.Minute)
		confirmed, err := service.Confirm(context.Background(), "owner", created.ID)
		if err != nil {
			t.Fatalf("Confirm() error = %v", err)
		}
		if confirmed.Status != generation.StatusQueued || confirmed.ConfirmedRequirement == nil {
			t.Fatalf("confirmed session = %#v", confirmed)
		}
	})
}

func TestConfirmedRequirementMigrationBackfillsLegacySessions(t *testing.T) {
	database := openPostgres(t)
	resetPostgresBeforeConfirmedRequirementMigration(t, database)
	base := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	confirmedStatuses := []generation.Status{
		generation.StatusQueued,
		generation.StatusGenerating,
		generation.StatusValidating,
		generation.StatusReady,
		generation.StatusFailed,
	}
	for _, status := range confirmedStatuses {
		sessionID := "gen_legacy_" + string(status)
		insertLegacySession(t, database, sessionID, status, base.Add(10*time.Minute))
		insertLegacyMessages(t, database, sessionID, base)
		if status != generation.StatusFailed {
			insertLegacyEvent(t, database, sessionID, 1, "queued", base.Add(3*time.Minute))
		}
	}
	insertLegacySession(t, database, "gen_cancelled_before_confirmation", generation.StatusCancelled, base.Add(11*time.Minute))
	insertLegacyMessages(t, database, "gen_cancelled_before_confirmation", base)
	insertLegacyEvent(t, database, "gen_cancelled_before_confirmation", 1, "cancelled", base.Add(2*time.Minute))
	insertLegacySession(t, database, "gen_cancelled_after_confirmation", generation.StatusCancelled, base.Add(12*time.Minute))
	insertLegacyMessages(t, database, "gen_cancelled_after_confirmation", base)
	insertLegacyEvent(t, database, "gen_cancelled_after_confirmation", 1, "queued", base.Add(4*time.Minute))
	insertLegacyEvent(t, database, "gen_cancelled_after_confirmation", 2, "queued", base.Add(3*time.Minute))
	insertLegacyEvent(t, database, "gen_cancelled_after_confirmation", 3, "cancelled", base.Add(5*time.Minute))
	insertLegacySession(t, database, "gen_legacy_draft", generation.StatusDraft, base.Add(13*time.Minute))
	insertLegacyMessages(t, database, "gen_legacy_draft", base)
	insertLegacySession(t, database, "gen_legacy_awaiting", generation.StatusAwaitingConfirmation, base.Add(14*time.Minute))
	insertLegacyMessages(t, database, "gen_legacy_awaiting", base)

	applyMigrationFile(t, database, "003_confirmed_requirement_column.sql")
	applyMigrationFile(t, database, "004_confirmed_requirement_backfill.sql")
	applyMigrationFile(t, database, "005_confirmed_requirement_constraint.sql")
	assertConfirmedRequirementConstraintValidation(t, database, false)
	applyMigrationFile(t, database, "006_validate_confirmed_requirement_constraint.sql")
	assertConfirmedRequirementConstraintValidation(t, database, true)
	repository := generation.NewPostgresRepository(database)
	for _, status := range confirmedStatuses {
		sessionID := "gen_legacy_" + string(status)
		restored, err := repository.Get(context.Background(), "owner", sessionID)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", sessionID, err)
		}
		wantConfirmedAt := base.Add(3 * time.Minute)
		if status == generation.StatusFailed {
			wantConfirmedAt = base.Add(10 * time.Minute)
		}
		assertRequirementSnapshot(t, restored.ConfirmedRequirement, legacyRequirement(base, wantConfirmedAt))
	}

	for _, sessionID := range []string{
		"gen_cancelled_before_confirmation",
		"gen_legacy_draft",
		"gen_legacy_awaiting",
	} {
		restored, err := repository.Get(context.Background(), "owner", sessionID)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", sessionID, err)
		}
		if restored.ConfirmedRequirement != nil {
			t.Fatalf("%s ConfirmedRequirement = %#v, want nil", sessionID, restored.ConfirmedRequirement)
		}
	}
	postConfirmationCancelled, err := repository.Get(
		context.Background(),
		"owner",
		"gen_cancelled_after_confirmation",
	)
	if err != nil {
		t.Fatal(err)
	}
	assertRequirementSnapshot(
		t,
		postConfirmationCancelled.ConfirmedRequirement,
		legacyRequirement(base, base.Add(3*time.Minute)),
	)

	var rawRequirement []byte
	if err := database.QueryRow(
		`SELECT confirmed_requirement_json
		 FROM generation_sessions WHERE id = 'gen_legacy_queued'`,
	).Scan(&rawRequirement); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawRequirement, &fields); err != nil {
		t.Fatal(err)
	}
	wantFields := []string{
		"initialPrompt",
		"additionalMessages",
		"target",
		"locale",
		"allowedCapabilities",
		"confirmedAt",
	}
	if len(fields) != len(wantFields) {
		t.Fatalf("confirmed requirement keys = %#v", fields)
	}
	for _, field := range wantFields {
		if _, ok := fields[field]; !ok {
			t.Fatalf("confirmed requirement missing JSON key %q: %#v", field, fields)
		}
	}

	if _, err := database.Exec(
		`UPDATE generation_sessions SET confirmed_requirement_json = NULL
		 WHERE id = 'gen_legacy_queued'`,
	); err == nil {
		t.Fatal("queued session accepted NULL confirmed_requirement_json")
	}
	if _, err := database.Exec(
		`UPDATE generation_sessions SET confirmed_requirement_json = NULL
		 WHERE id = 'gen_cancelled_before_confirmation'`,
	); err != nil {
		t.Fatalf("pre-confirmation cancelled session rejected NULL snapshot: %v", err)
	}
}

func TestPostgresRepositoryRejectsMalformedConfirmedRequirement(t *testing.T) {
	database := openPostgres(t)
	resetPostgres(t, database)
	createdAt := time.Date(2026, 7, 13, 4, 0, 0, 0, time.UTC)
	testCases := []struct {
		name        string
		sessionID   string
		requirement string
	}{
		{name: "non-object", sessionID: "gen_malformed_requirement", requirement: `"malformed"`},
		{name: "missing fields", sessionID: "gen_empty_requirement", requirement: `{}`},
		{
			name:      "unknown field",
			sessionID: "gen_unknown_requirement",
			requirement: `{
			  "initialPrompt":"legacy",
			  "additionalMessages":[],
			  "target":"native",
			  "locale":"zh-CN",
			  "allowedCapabilities":[],
			  "confirmedAt":"2026-07-13T04:00:00Z",
			  "unknown":true
			}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := database.Exec(
				`INSERT INTO generation_sessions (
				   id, user_id, prompt, target, locale, status, summary_json,
				   confirmed_requirement_json, version_id, created_at, updated_at
				 ) VALUES (
				   $1, 'owner', 'legacy', 'native', 'zh-CN', 'queued',
				   '{"goal":"legacy","constraints":[],"locale":"zh-CN"}'::jsonb,
				   $2::jsonb, NULL, $3, $3
				 )`,
				testCase.sessionID,
				testCase.requirement,
				createdAt,
			); err != nil {
				t.Fatal(err)
			}
			_, err := generation.NewPostgresRepository(database).Get(
				context.Background(),
				"owner",
				testCase.sessionID,
			)
			const wantPrefix = "decode generation confirmed requirement:"
			if err == nil || !strings.HasPrefix(err.Error(), wantPrefix) {
				t.Fatalf("Get() error = %v, want prefix %q", err, wantPrefix)
			}
		})
	}
}

func openPostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("AGENTCARD_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("AGENTCARD_POSTGRES_TEST_DSN is not configured")
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func resetPostgres(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Apply(context.Background(), database); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
}

func resetPostgresBeforeConfirmedRequirementMigration(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	applyMigrationFile(t, database, "001_initial.sql")
	applyMigrationFile(t, database, "002_production_fields.sql")
}

func applyMigrationFile(t *testing.T, database *sql.DB, name string) {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(source)); err != nil {
		t.Fatalf("apply %s: %v", name, err)
	}
}

func assertConfirmedRequirementConstraintValidation(t *testing.T, database *sql.DB, want bool) {
	t.Helper()
	var got bool
	if err := database.QueryRow(
		`SELECT convalidated
		 FROM pg_constraint
		 WHERE conname = 'generation_sessions_confirmed_requirement_required'`,
	).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("constraint convalidated = %t, want %t", got, want)
	}
}

func insertLegacySession(
	t *testing.T,
	database *sql.DB,
	sessionID string,
	status generation.Status,
	updatedAt time.Time,
) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO generation_sessions (
		   id, user_id, prompt, target, locale, status, summary_json,
		   version_id, created_at, updated_at
		 ) VALUES (
		   $1, 'owner', 'legacy initial prompt', 'native', 'zh-CN', $2,
		   '{"goal":"legacy","constraints":[],"locale":"zh-CN"}'::jsonb,
		   NULL, $3, $4
		 )`,
		sessionID,
		status,
		updatedAt.Add(-time.Hour),
		updatedAt,
	); err != nil {
		t.Fatalf("insert legacy session %s: %v", sessionID, err)
	}
}

func insertLegacyMessages(t *testing.T, database *sql.DB, sessionID string, base time.Time) {
	t.Helper()
	messages := []generation.Message{
		{Role: "user", Content: "legacy initial prompt", CreatedAt: base},
		{Role: "assistant", Content: "legacy assistant reply", CreatedAt: base.Add(30 * time.Second)},
		{Role: "user", Content: "first addition", CreatedAt: base.Add(time.Minute)},
		{Role: "user", Content: "second addition", CreatedAt: base.Add(2 * time.Minute)},
	}
	for index, message := range messages {
		if _, err := database.Exec(
			`INSERT INTO generation_messages (session_id, sequence, role, content, created_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			sessionID,
			index+1,
			message.Role,
			message.Content,
			message.CreatedAt,
		); err != nil {
			t.Fatalf("insert legacy message %s/%d: %v", sessionID, index+1, err)
		}
	}
}

func insertLegacyEvent(
	t *testing.T,
	database *sql.DB,
	sessionID string,
	eventID int64,
	stage string,
	createdAt time.Time,
) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO generation_events (
		   session_id, event_id, type, stage, message, progress,
		   version_id, error_code, created_at
		 ) VALUES ($1, $2, 'generation.progress', $3, 'legacy event', 0, NULL, NULL, $4)`,
		sessionID,
		eventID,
		stage,
		createdAt,
	); err != nil {
		t.Fatalf("insert legacy event %s/%d: %v", sessionID, eventID, err)
	}
}

func legacyRequirement(base, confirmedAt time.Time) *generation.RequirementSnapshot {
	return &generation.RequirementSnapshot{
		InitialPrompt: "legacy initial prompt",
		AdditionalMessages: []generation.Message{
			{Role: "user", Content: "first addition", CreatedAt: base.Add(time.Minute)},
			{Role: "user", Content: "second addition", CreatedAt: base.Add(2 * time.Minute)},
		},
		Target:              generation.TargetNative,
		Locale:              "zh-CN",
		AllowedCapabilities: []string{"storage", "window.manageSelf"},
		ConfirmedAt:         confirmedAt,
	}
}

func validPostgresRequirement(confirmedAt time.Time) *generation.RequirementSnapshot {
	return &generation.RequirementSnapshot{
		InitialPrompt:       "测试需求",
		AdditionalMessages:  make([]generation.Message, 0),
		Target:              generation.TargetNative,
		Locale:              "zh-CN",
		AllowedCapabilities: make([]string, 0),
		ConfirmedAt:         confirmedAt,
	}
}

func newPostgresTestSession(
	sessionID string,
	status generation.Status,
	requirement *generation.RequirementSnapshot,
	createdAt time.Time,
) *generation.Session {
	return &generation.Session{
		ID:                   sessionID,
		UserID:               "owner",
		Prompt:               "测试需求",
		Target:               generation.TargetNative,
		Locale:               "zh-CN",
		Status:               status,
		Summary:              generation.RequirementSummary{Goal: "测试需求", Constraints: []string{}, Locale: "zh-CN"},
		Messages:             []generation.Message{{Role: "user", Content: "测试需求", CreatedAt: createdAt}},
		ConfirmedRequirement: requirement,
		CreatedAt:            createdAt,
		UpdatedAt:            createdAt,
		Events:               make([]generation.Event, 0),
	}
}

func createConfirmedPostgresSession(
	t *testing.T,
	database *sql.DB,
) (*generation.PostgresRepository, string, *generation.RequirementSnapshot) {
	t.Helper()
	current := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	repository := generation.NewPostgresRepository(database)
	service := generation.NewService(
		repository,
		func() string { return "gen_write_once" },
		func() time.Time { return current },
	)
	created, err := service.Create(context.Background(), "owner", generation.CreateRequest{
		Prompt: "冻结需求",
		Target: generation.TargetNative,
		Locale: "zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Minute)
	if _, err := service.AddMessage(context.Background(), "owner", created.ID, "增加本地存储"); err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Minute)
	confirmed, err := service.Confirm(context.Background(), "owner", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	return repository, created.ID, confirmed.ConfirmedRequirement
}

func assertRequirementSnapshot(
	t *testing.T,
	got *generation.RequirementSnapshot,
	want *generation.RequirementSnapshot,
) {
	t.Helper()
	if got == nil {
		t.Fatal("ConfirmedRequirement = nil")
	}
	if got.InitialPrompt != want.InitialPrompt {
		t.Fatalf("InitialPrompt = %q, want %q", got.InitialPrompt, want.InitialPrompt)
	}
	if got.Target != want.Target {
		t.Fatalf("Target = %q, want %q", got.Target, want.Target)
	}
	if got.Locale != want.Locale {
		t.Fatalf("Locale = %q, want %q", got.Locale, want.Locale)
	}
	if !reflect.DeepEqual(got.AllowedCapabilities, want.AllowedCapabilities) {
		t.Fatalf("AllowedCapabilities = %#v, want %#v", got.AllowedCapabilities, want.AllowedCapabilities)
	}
	if !got.ConfirmedAt.Equal(want.ConfirmedAt) {
		t.Fatalf("ConfirmedAt = %s, want %s", got.ConfirmedAt, want.ConfirmedAt)
	}
	if len(got.AdditionalMessages) != len(want.AdditionalMessages) {
		t.Fatalf("AdditionalMessages = %#v, want %#v", got.AdditionalMessages, want.AdditionalMessages)
	}
	for index := range want.AdditionalMessages {
		gotMessage := got.AdditionalMessages[index]
		wantMessage := want.AdditionalMessages[index]
		if gotMessage.Role != wantMessage.Role || gotMessage.Content != wantMessage.Content ||
			!gotMessage.CreatedAt.Equal(wantMessage.CreatedAt) {
			t.Fatalf("AdditionalMessages[%d] = %#v, want %#v", index, gotMessage, wantMessage)
		}
	}
}
