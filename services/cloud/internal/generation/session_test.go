package generation_test

import (
	"errors"
	"reflect"
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

func TestSessionRequiresPairedBaseVersionIdentity(t *testing.T) {
	t.Parallel()

	for _, input := range []generation.CreateInput{
		{ID: "gen_card_only", UserID: "user", Prompt: "修改卡片", BaseCardID: "card_01"},
		{ID: "gen_version_only", UserID: "user", Prompt: "修改卡片", BaseVersionID: "ver_01"},
	} {
		if _, err := generation.NewSession(input); !errors.Is(err, generation.ErrInvalidBaseVersion) {
			t.Fatalf("NewSession(%#v) error = %v, want ErrInvalidBaseVersion", input, err)
		}
	}
}

func TestSessionConfirmFreezesBaseVersionIdentity(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC)
	session, err := generation.NewSession(generation.CreateInput{
		ID:            "gen_iteration",
		UserID:        "user",
		Prompt:        "增加一个暂停按钮",
		Target:        generation.TargetNative,
		Locale:        "zh-CN",
		BaseCardID:    "card_01",
		BaseVersionID: "ver_01",
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Transition(generation.StatusAwaitingConfirmation, generation.Transition{At: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Confirm([]string{"storage"}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if session.BaseCardID != "card_01" || session.BaseVersionID != "ver_01" {
		t.Fatalf("session base = %q/%q", session.BaseCardID, session.BaseVersionID)
	}
	if session.ConfirmedRequirement.BaseCardID != "card_01" ||
		session.ConfirmedRequirement.BaseVersionID != "ver_01" {
		t.Fatalf("snapshot base = %#v", session.ConfirmedRequirement)
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

func TestSessionConfirmFreezesCompleteRequirement(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 7, 13, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	confirmedAt := createdAt.Add(2 * time.Minute)
	session, err := generation.NewSession(generation.CreateInput{
		ID:        "gen_confirm",
		UserID:    "user_confirm",
		Prompt:    "生成离线番茄钟",
		Target:    generation.TargetNative,
		Locale:    "zh-CN",
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if len(session.Messages) != 1 || session.Messages[0].Content != session.Prompt {
		t.Fatalf("initial messages = %#v, want initial prompt at index 0", session.Messages)
	}
	if _, err := session.Transition(generation.StatusAwaitingConfirmation, generation.Transition{At: createdAt}); err != nil {
		t.Fatalf("Transition(awaiting_confirmation) error = %v", err)
	}
	firstAt := createdAt.Add(time.Minute)
	if _, err := session.AddMessage("增加暂停按钮", firstAt); err != nil {
		t.Fatalf("AddMessage(first) error = %v", err)
	}
	if _, err := session.AddMessage("使用中文显示", confirmedAt); err != nil {
		t.Fatalf("AddMessage(second) error = %v", err)
	}

	capabilities := []string{" window.manageSelf ", "storage", "storage", " "}
	event, err := session.Confirm(capabilities, confirmedAt)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if session.Status != generation.StatusQueued || event.Stage != "queued" {
		t.Fatalf("status/event = %q/%q, want queued/queued", session.Status, event.Stage)
	}
	want := &generation.RequirementSnapshot{
		InitialPrompt: "生成离线番茄钟",
		AdditionalMessages: []generation.Message{
			{Role: "user", Content: "增加暂停按钮", CreatedAt: firstAt.UTC()},
			{Role: "user", Content: "使用中文显示", CreatedAt: confirmedAt.UTC()},
		},
		Target:              generation.TargetNative,
		Locale:              "zh-CN",
		AllowedCapabilities: []string{"storage", "window.manageSelf"},
		ConfirmedAt:         confirmedAt.UTC(),
	}
	if !reflect.DeepEqual(session.ConfirmedRequirement, want) {
		t.Fatalf("ConfirmedRequirement = %#v, want %#v", session.ConfirmedRequirement, want)
	}

	capabilities[0] = "network"
	if got := session.ConfirmedRequirement.AllowedCapabilities[1]; got != "window.manageSelf" {
		t.Fatalf("AllowedCapabilities mutated through input = %#v", session.ConfirmedRequirement.AllowedCapabilities)
	}
	frozen := *session.ConfirmedRequirement
	frozen.AdditionalMessages = append([]generation.Message(nil), session.ConfirmedRequirement.AdditionalMessages...)
	frozen.AllowedCapabilities = append([]string(nil), session.ConfirmedRequirement.AllowedCapabilities...)
	if _, err := session.Confirm([]string{"network"}, confirmedAt.Add(time.Hour)); err == nil {
		t.Fatal("second Confirm() error = nil, want rejection")
	}
	if !reflect.DeepEqual(session.ConfirmedRequirement, &frozen) {
		t.Fatalf("second Confirm() changed snapshot to %#v, want %#v", session.ConfirmedRequirement, frozen)
	}
	if _, err := session.AddMessage("确认后追加", confirmedAt.Add(2*time.Hour)); err == nil {
		t.Fatal("AddMessage() after confirmation error = nil, want rejection")
	}
}

func TestSessionConfirmRejectsMissingInitialMessageWithoutMutation(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		messages []generation.Message
	}{
		{name: "nil", messages: nil},
		{name: "empty", messages: []generation.Message{}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			session := mustAwaitingSession(t)
			session.Messages = testCase.messages
			assertConfirmRejectedWithoutMutation(t, session)
		})
	}
}

func TestSessionConfirmRejectsInvalidInitialMessageWithoutMutation(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		mutate func(*generation.Session)
	}{
		{
			name: "role is not user",
			mutate: func(session *generation.Session) {
				session.Messages[0].Role = "assistant"
			},
		},
		{
			name: "content does not exactly match prompt",
			mutate: func(session *generation.Session) {
				session.Messages[0].Content += " "
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			session := mustAwaitingSession(t)
			testCase.mutate(session)
			assertConfirmRejectedWithoutMutation(t, session)
		})
	}
}

func assertConfirmRejectedWithoutMutation(t *testing.T, session *generation.Session) {
	t.Helper()
	wantStatus := session.Status
	wantRequirement := session.ConfirmedRequirement
	wantEvents := append([]generation.Event(nil), session.Events...)

	_, err := session.Confirm([]string{"storage"}, time.Now())
	if !errors.Is(err, generation.ErrInvalidTransition) {
		t.Fatalf("Confirm() error = %v, want ErrInvalidTransition", err)
	}
	if session.Status != wantStatus {
		t.Fatalf("Status = %q, want unchanged %q", session.Status, wantStatus)
	}
	if !reflect.DeepEqual(session.ConfirmedRequirement, wantRequirement) {
		t.Fatalf("ConfirmedRequirement = %#v, want unchanged %#v", session.ConfirmedRequirement, wantRequirement)
	}
	if !reflect.DeepEqual(session.Events, wantEvents) {
		t.Fatalf("Events = %#v, want unchanged %#v", session.Events, wantEvents)
	}
}

func mustAwaitingSession(t *testing.T) *generation.Session {
	t.Helper()
	session := mustSession(t)
	if _, err := session.Transition(generation.StatusAwaitingConfirmation, generation.Transition{At: time.Now()}); err != nil {
		t.Fatalf("Transition(awaiting_confirmation) error = %v", err)
	}
	return session
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
