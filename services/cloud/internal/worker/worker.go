package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

type Config struct {
	WorkerID     string
	Jobs         jobs.Store
	Generations  *generation.Service
	Agent        *agent.CodingAgent
	Publisher    *publish.Publisher
	NewCardID    func() string
	NewVersionID func() string
	Now          func() time.Time
}

type Worker struct {
	config Config
}

type Outcome struct {
	JobID     string
	SessionID string
	VersionID string
}

func New(config Config) *Worker {
	return &Worker{config: config}
}

func (worker *Worker) RunOnce(ctx context.Context) (Outcome, error) {
	now := worker.config.Now().UTC()
	job, err := worker.config.Jobs.Claim(ctx, worker.config.WorkerID, now, time.Minute)
	if err != nil {
		return Outcome{}, err
	}
	session, err := worker.config.Generations.StartGenerating(ctx, job.SessionID)
	if err != nil {
		return Outcome{}, worker.fail(ctx, job, "GENERATION_STATE_INVALID", err)
	}
	result, err := worker.config.Agent.Generate(ctx, agent.Request{
		SessionID: session.ID,
		Prompt:    session.Prompt,
		Target:    session.Target,
		Locale:    session.Locale,
	})
	if err != nil {
		return Outcome{}, worker.fail(ctx, job, "VALIDATION_FAILED", err)
	}
	if _, err := worker.config.Generations.StartValidating(ctx, session.ID); err != nil {
		return Outcome{}, worker.fail(ctx, job, "GENERATION_STATE_INVALID", err)
	}
	cardID := worker.config.NewCardID()
	versionID := worker.config.NewVersionID()
	report, err := json.Marshal(map[string]any{
		"status":   "passed",
		"runtime":  result.Runtime,
		"attempts": result.Attempts,
	})
	if err != nil {
		return Outcome{}, worker.fail(ctx, job, "INTERNAL", err)
	}
	definition := contracts.CardDefinition{
		FormatVersion:      1,
		MinHostVersion:     "1.0.0",
		CardID:             cardID,
		VersionID:          versionID,
		DisplayVersion:     "1.0.0",
		Runtime:            contracts.CardRuntimeNative,
		StateSchemaVersion: 1,
		Title:              titleFromPrompt(session.Prompt),
		Description:        session.Summary.Goal,
		Entrypoint:         "payload/native.json",
		CatalogVersion:     "1",
		MinSize:            contracts.Size{Width: 240, Height: 160},
		PreferredSize:      contracts.Size{Width: 360, Height: 240},
		MaxSize:            contracts.Size{Width: 1200, Height: 900},
		Capabilities:       []string{"storage", "window.manageSelf"},
		NetworkPolicy:      contracts.NetworkPolicy{Mode: "none", Domains: []string{}},
		CreatedAt:          now,
	}
	version, err := worker.config.Publisher.Publish(ctx, publish.Input{
		UserID:     session.UserID,
		Definition: definition,
		Files: map[string][]byte{
			"payload/native.json":     []byte(result.Content),
			"reports/validation.json": report,
		},
		Preview: map[string]any{
			"title":   definition.Title,
			"runtime": string(result.Runtime),
			"reason":  result.Reason,
		},
		CreatedAt: now,
	})
	if err != nil {
		return Outcome{}, worker.fail(ctx, job, "ARTIFACT_PUBLISH_FAILED", err)
	}
	if _, err := worker.config.Generations.MarkReady(ctx, session.ID, version.VersionID); err != nil {
		return Outcome{}, worker.fail(ctx, job, "GENERATION_STATE_INVALID", err)
	}
	if err := worker.config.Jobs.Complete(ctx, job.ID, worker.config.WorkerID, worker.config.Now()); err != nil {
		return Outcome{}, err
	}
	return Outcome{
		JobID:     job.ID,
		SessionID: session.ID,
		VersionID: version.VersionID,
	}, nil
}

func (worker *Worker) fail(ctx context.Context, job *jobs.Job, code string, cause error) error {
	_, _ = worker.config.Generations.MarkFailed(ctx, job.SessionID, code)
	_ = worker.config.Jobs.Fail(ctx, job.ID, worker.config.WorkerID, worker.config.Now())
	return fmt.Errorf("%s: %w", code, cause)
}

func titleFromPrompt(prompt string) string {
	const maxRunes = 120
	if utf8.RuneCountInString(prompt) <= maxRunes {
		return prompt
	}
	runes := []rune(prompt)
	return string(runes[:maxRunes])
}
