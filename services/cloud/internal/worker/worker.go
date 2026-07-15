package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/internal/observability"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

type Config struct {
	WorkerID          string
	Jobs              jobs.Store
	Generations       *generation.Service
	Agent             *agent.CodingAgent
	Publisher         *publish.Publisher
	NewCardID         func() string
	NewVersionID      func() string
	Now               func() time.Time
	Logger            *slog.Logger
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
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
	if config.Logger == nil {
		config.Logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	if config.LeaseDuration == 0 {
		config.LeaseDuration = 90 * time.Second
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	if config.LeaseDuration <= 0 ||
		config.HeartbeatInterval <= 0 ||
		config.HeartbeatInterval*2 >= config.LeaseDuration {
		panic("worker heartbeat interval must be less than half the lease duration")
	}
	return &Worker{config: config}
}

func (worker *Worker) RunOnce(ctx context.Context) (outcome Outcome, runErr error) {
	now := worker.config.Now().UTC()
	job, err := worker.config.Jobs.Claim(
		ctx,
		worker.config.WorkerID,
		now,
		worker.config.LeaseDuration,
	)
	if err != nil {
		return Outcome{}, err
	}
	heartbeat := startLeaseHeartbeat(
		ctx,
		worker.config.Jobs,
		job.ID,
		worker.config.WorkerID,
		worker.config.LeaseDuration,
		worker.config.HeartbeatInterval,
		worker.config.Now,
	)
	ctx = heartbeat.workContext
	defer func() {
		if heartbeatErr := heartbeat.Stop(); heartbeatErr != nil {
			outcome = Outcome{}
			runErr = fmt.Errorf("JOB_LEASE_LOST: %w", heartbeatErr)
		}
	}()
	jobStartedAt := time.Now()
	worker.config.Logger.InfoContext(ctx, "generation_job_started",
		"jobId", job.ID,
		"sessionId", job.SessionID,
		"attempt", job.Attempts,
	)
	session, err := worker.config.Generations.GetSystem(ctx, job.SessionID)
	if err != nil {
		return Outcome{}, worker.fail(ctx, job, "GENERATION_STATE_INVALID", err)
	}
	if session.Status == generation.StatusQueued {
		session, err = worker.config.Generations.StartGenerating(ctx, job.SessionID)
		if err != nil {
			return Outcome{}, worker.fail(ctx, job, "GENERATION_STATE_INVALID", err)
		}
	} else if session.Status != generation.StatusGenerating &&
		session.Status != generation.StatusValidating &&
		session.Status != generation.StatusReady {
		return Outcome{}, worker.fail(
			ctx, job, "GENERATION_STATE_INVALID",
			fmt.Errorf("generation session cannot be resumed from %s", session.Status),
		)
	}
	if session.ConfirmedRequirement == nil {
		return Outcome{}, worker.fail(
			ctx,
			job,
			"GENERATION_REQUIREMENTS_MISSING",
			fmt.Errorf("confirmed requirement snapshot is missing"),
		)
	}
	requirement := cloneRequirementSnapshot(*session.ConfirmedRequirement)
	publication, err := worker.config.Jobs.ReservePublication(
		ctx,
		job.ID,
		worker.config.WorkerID,
		worker.config.NewCardID(),
		worker.config.NewVersionID(),
		now,
	)
	if err != nil {
		return Outcome{}, err
	}
	existing, findErr := worker.config.Publisher.FindVersion(
		ctx, session.UserID, publication.CardID, publication.VersionID,
	)
	if findErr == nil {
		if session.Status != generation.StatusReady {
			if _, err := worker.config.Generations.MarkReady(ctx, session.ID, existing.VersionID); err != nil {
				return Outcome{}, worker.fail(ctx, job, "GENERATION_STATE_INVALID", err)
			}
		} else if session.VersionID != existing.VersionID {
			return Outcome{}, worker.fail(
				ctx, job, "GENERATION_STATE_INVALID",
				fmt.Errorf("ready generation version does not match reserved publication"),
			)
		}
		if err := worker.config.Jobs.Complete(
			ctx, job.ID, worker.config.WorkerID, worker.config.Now(),
		); err != nil {
			return Outcome{}, err
		}
		return Outcome{JobID: job.ID, SessionID: session.ID, VersionID: existing.VersionID}, nil
	}
	if !errors.Is(findErr, publish.ErrNotFound) {
		return Outcome{}, worker.fail(ctx, job, "ARTIFACT_LOOKUP_FAILED", findErr)
	}
	if session.Status == generation.StatusReady {
		return Outcome{}, worker.fail(
			ctx, job, "GENERATION_STATE_INVALID",
			fmt.Errorf("ready generation publication is missing"),
		)
	}
	result, err := worker.config.Agent.Generate(ctx, agent.Request{
		SessionID:   session.ID,
		Requirement: requirement,
	})
	if err != nil {
		return Outcome{}, worker.fail(ctx, job, "VALIDATION_FAILED", err)
	}
	if session.Status != generation.StatusValidating {
		if _, err := worker.config.Generations.StartValidating(ctx, session.ID); err != nil {
			return Outcome{}, worker.fail(ctx, job, "GENERATION_STATE_INVALID", err)
		}
	}
	cardID := publication.CardID
	versionID := publication.VersionID
	report, err := json.Marshal(map[string]any{
		"status":   "passed",
		"runtime":  result.Runtime,
		"attempts": result.Attempts,
	})
	if err != nil {
		return Outcome{}, worker.fail(ctx, job, "INTERNAL", err)
	}
	runtime := contracts.CardRuntimeNative
	entrypoint := "payload/native.json"
	catalogVersion := "1"
	artifactFiles := map[string][]byte{
		"reports/validation.json": report,
	}
	if result.Runtime == agent.RuntimeWeb {
		runtime = contracts.CardRuntimeWeb
		entrypoint = "payload/web/index.html"
		catalogVersion = ""
		for name, content := range result.Files {
			artifactFiles["payload/web/"+name] = content
		}
	} else {
		artifactFiles["payload/native.json"] = []byte(result.Content)
	}
	definition := contracts.CardDefinition{
		FormatVersion:      1,
		MinHostVersion:     "1.0.0",
		CardID:             cardID,
		VersionID:          versionID,
		DisplayVersion:     "1.0.0",
		Runtime:            runtime,
		StateSchemaVersion: 1,
		Title:              titleFromPrompt(requirement.InitialPrompt),
		Description:        descriptionFromRequirement(requirement),
		Entrypoint:         entrypoint,
		CatalogVersion:     catalogVersion,
		MinSize:            contracts.Size{Width: 240, Height: 160},
		PreferredSize:      contracts.Size{Width: 360, Height: 240},
		MaxSize:            contracts.Size{Width: 1200, Height: 900},
		Capabilities:       append([]string(nil), requirement.AllowedCapabilities...),
		NetworkPolicy:      contracts.NetworkPolicy{Mode: "none", Domains: []string{}},
		CreatedAt:          job.CreatedAt,
	}
	publishStartedAt := time.Now()
	worker.config.Logger.InfoContext(ctx, "artifact_publish_started",
		"jobId", job.ID,
		"sessionId", session.ID,
		"cardId", cardID,
		"versionId", versionID,
		"runtime", result.Runtime,
	)
	version, err := worker.config.Publisher.Publish(ctx, publish.Input{
		UserID:     session.UserID,
		Definition: definition,
		Files:      artifactFiles,
		Preview: map[string]any{
			"title":   definition.Title,
			"runtime": string(result.Runtime),
			"reason":  result.Reason,
		},
		CreatedAt: job.CreatedAt,
	})
	if err != nil {
		worker.config.Logger.WarnContext(ctx, "artifact_publish_failed",
			"jobId", job.ID,
			"sessionId", session.ID,
			"cardId", cardID,
			"versionId", versionID,
			"runtime", result.Runtime,
			"durationMs", time.Since(publishStartedAt).Milliseconds(),
			"errorKind", observability.ErrorKind(err),
		)
		return Outcome{}, worker.fail(ctx, job, "ARTIFACT_PUBLISH_FAILED", err)
	}
	worker.config.Logger.InfoContext(ctx, "artifact_publish_completed",
		"jobId", job.ID,
		"sessionId", session.ID,
		"cardId", cardID,
		"versionId", version.VersionID,
		"runtime", result.Runtime,
		"durationMs", time.Since(publishStartedAt).Milliseconds(),
	)
	if _, err := worker.config.Generations.MarkReady(ctx, session.ID, version.VersionID); err != nil {
		return Outcome{}, worker.fail(ctx, job, "GENERATION_STATE_INVALID", err)
	}
	if err := worker.config.Jobs.Complete(ctx, job.ID, worker.config.WorkerID, worker.config.Now()); err != nil {
		worker.config.Logger.WarnContext(ctx, "generation_job_failed",
			"jobId", job.ID,
			"sessionId", session.ID,
			"durationMs", time.Since(jobStartedAt).Milliseconds(),
			"errorKind", "job_store_failed",
		)
		return Outcome{}, err
	}
	worker.config.Logger.InfoContext(ctx, "generation_job_completed",
		"jobId", job.ID,
		"sessionId", session.ID,
		"versionId", version.VersionID,
		"runtime", result.Runtime,
		"durationMs", time.Since(jobStartedAt).Milliseconds(),
	)
	return Outcome{
		JobID:     job.ID,
		SessionID: session.ID,
		VersionID: version.VersionID,
	}, nil
}

func cloneRequirementSnapshot(requirement generation.RequirementSnapshot) generation.RequirementSnapshot {
	requirement.AdditionalMessages = append([]generation.Message(nil), requirement.AdditionalMessages...)
	requirement.AllowedCapabilities = append([]string(nil), requirement.AllowedCapabilities...)
	return requirement
}

func descriptionFromRequirement(requirement generation.RequirementSnapshot) string {
	parts := make([]string, 0, len(requirement.AdditionalMessages)+1)
	if initialPrompt := strings.TrimSpace(requirement.InitialPrompt); initialPrompt != "" {
		parts = append(parts, initialPrompt)
	}
	for _, message := range requirement.AdditionalMessages {
		if content := strings.TrimSpace(message.Content); content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n")
}

func (worker *Worker) fail(ctx context.Context, job *jobs.Job, code string, cause error) error {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	if isRetryableJobError(cause) && job.Attempts < job.MaxAttempts {
		now := worker.config.Now().UTC()
		nextAttemptAt := now.Add(jobRetryDelay(job.Attempts))
		if err := worker.config.Jobs.Retry(
			ctx, job.ID, worker.config.WorkerID, now, nextAttemptAt,
		); err != nil {
			return fmt.Errorf("schedule generation retry: %w", err)
		}
		worker.config.Logger.WarnContext(ctx, "generation_job_retry_scheduled",
			"jobId", job.ID,
			"sessionId", job.SessionID,
			"attempt", job.Attempts,
			"nextAttemptAt", nextAttemptAt,
			"errorKind", strings.ToLower(code),
		)
		return fmt.Errorf("JOB_RETRY_SCHEDULED: %w", cause)
	}
	_, _ = worker.config.Generations.MarkFailed(ctx, job.SessionID, code)
	_ = worker.config.Jobs.Fail(ctx, job.ID, worker.config.WorkerID, worker.config.Now())
	worker.config.Logger.WarnContext(ctx, "generation_job_failed",
		"jobId", job.ID,
		"sessionId", job.SessionID,
		"errorKind", strings.ToLower(code),
	)
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
