package generation_test

import (
	"errors"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
)

func TestSessionFollowsGenerationStateMachine(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)
	session, err := generation.NewSession(generation.CreateInput{
		ID:        "gen_01",
		UserID:    "user_01",
		Prompt:    "做一个离线番茄钟",
		Target:    generation.TargetAuto,
		Locale:    "zh-CN",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	transitions := []generation.Status{
		generation.StatusAwaitingConfirmation,
		generation.StatusQueued,
		generation.StatusGenerating,
		generation.StatusValidating,
		generation.StatusReady,
	}
	for index, status := range transitions {
		event, transitionErr := session.Transition(status, generation.Transition{
			Message:  string(status),
			Progress: float64(index+1) / float64(len(transitions)),
			At:       now.Add(time.Duration(index+1) * time.Minute),
		})
		if transitionErr != nil {
			t.Fatalf("Transition(%q) error = %v", status, transitionErr)
		}
		if event.EventID != int64(index+1) {
			t.Fatalf("EventID = %d, want %d", event.EventID, index+1)
		}
		if event.SessionID != session.ID {
			t.Fatalf("SessionID = %q, want %q", event.SessionID, session.ID)
		}
	}

	if session.Status != generation.StatusReady {
		t.Fatalf("Status = %q, want ready", session.Status)
	}
	if _, err := session.Transition(generation.StatusFailed, generation.Transition{At: now}); !errors.Is(err, generation.ErrInvalidTransition) {
		t.Fatalf("terminal transition error = %v, want ErrInvalidTransition", err)
	}
}

func TestSessionRejectsInvalidInputAndTransitions(t *testing.T) {
	t.Parallel()

	_, err := generation.NewSession(generation.CreateInput{
		ID:     "gen_01",
		UserID: "user_01",
		Prompt: "prompt",
		Target: "shell",
		Locale: "zh-CN",
	})
	if !errors.Is(err, generation.ErrInvalidTarget) {
		t.Fatalf("NewSession() error = %v, want ErrInvalidTarget", err)
	}

	session := mustSession(t)
	if _, err := session.Transition(generation.StatusGenerating, generation.Transition{}); !errors.Is(err, generation.ErrInvalidTransition) {
		t.Fatalf("Transition() error = %v, want ErrInvalidTransition", err)
	}
}

func TestSessionCanCancelOnlyBeforeTerminalState(t *testing.T) {
	t.Parallel()

	session := mustSession(t)
	if _, err := session.Transition(generation.StatusAwaitingConfirmation, generation.Transition{}); err != nil {
		t.Fatal(err)
	}
	event, err := session.Cancel(time.Now().UTC())
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if session.Status != generation.StatusCancelled {
		t.Fatalf("Status = %q, want cancelled", session.Status)
	}
	if event.Type != "status.changed" {
		t.Fatalf("event type = %q", event.Type)
	}
	if _, err := session.Cancel(time.Now().UTC()); !errors.Is(err, generation.ErrTerminalSession) {
		t.Fatalf("second Cancel() error = %v, want ErrTerminalSession", err)
	}
}

func mustSession(t *testing.T) *generation.Session {
	t.Helper()
	session, err := generation.NewSession(generation.CreateInput{
		ID:        "gen_test",
		UserID:    "user_test",
		Prompt:    "做一个待办卡片",
		Target:    generation.TargetAuto,
		Locale:    "zh-CN",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}
