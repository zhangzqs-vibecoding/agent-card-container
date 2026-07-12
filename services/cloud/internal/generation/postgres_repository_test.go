package generation

import (
	"testing"
	"time"
)

func TestValidateConfirmedRequirement(t *testing.T) {
	t.Parallel()
	valid := func() *RequirementSnapshot {
		return &RequirementSnapshot{
			InitialPrompt:       "测试需求",
			AdditionalMessages:  make([]Message, 0),
			Target:              TargetNative,
			Locale:              "zh-CN",
			AllowedCapabilities: make([]string, 0),
			ConfirmedAt:         time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		}
	}
	testCases := []struct {
		name        string
		requirement func() *RequirementSnapshot
		wantError   bool
	}{
		{name: "valid empty lists", requirement: valid},
		{name: "empty prompt", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.InitialPrompt = " "
			return requirement
		}, wantError: true},
		{name: "invalid target", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.Target = Target("invalid")
			return requirement
		}, wantError: true},
		{name: "empty locale", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.Locale = " "
			return requirement
		}, wantError: true},
		{name: "nil additional messages", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.AdditionalMessages = nil
			return requirement
		}, wantError: true},
		{name: "nil allowed capabilities", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.AllowedCapabilities = nil
			return requirement
		}, wantError: true},
		{name: "non-user message", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.AdditionalMessages = []Message{{Role: "assistant", Content: "reply", CreatedAt: requirement.ConfirmedAt}}
			return requirement
		}, wantError: true},
		{name: "blank message content", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.AdditionalMessages = []Message{{Role: "user", Content: " ", CreatedAt: requirement.ConfirmedAt}}
			return requirement
		}, wantError: true},
		{name: "zero message time", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.AdditionalMessages = []Message{{Role: "user", Content: "addition"}}
			return requirement
		}, wantError: true},
		{name: "blank capability", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.AllowedCapabilities = []string{" "}
			return requirement
		}, wantError: true},
		{name: "duplicate capability", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.AllowedCapabilities = []string{"storage", " storage "}
			return requirement
		}, wantError: true},
		{name: "zero confirmed time", requirement: func() *RequirementSnapshot {
			requirement := valid()
			requirement.ConfirmedAt = time.Time{}
			return requirement
		}, wantError: true},
		{name: "null requirement", requirement: func() *RequirementSnapshot { return nil }, wantError: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateConfirmedRequirement(testCase.requirement())
			if (err != nil) != testCase.wantError {
				t.Fatalf("validateConfirmedRequirement() error = %v, wantError = %t", err, testCase.wantError)
			}
		})
	}
}
