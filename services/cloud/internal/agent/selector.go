package agent

import (
	"errors"
	"strings"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
)

type Runtime string

const (
	RuntimeNative Runtime = "native"
	RuntimeWeb    Runtime = "web"
)

var (
	ErrForbiddenRequirement   = errors.New("forbidden requirement")
	ErrUnsupportedRequirement = errors.New("unsupported requirement")
	ErrValidationFailed       = errors.New("validation failed")
)

type Decision struct {
	Runtime Runtime `json:"runtime"`
	Reason  string  `json:"reason"`
}

type Selector struct{}

func (Selector) Select(prompt string, target generation.Target) (Decision, error) {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	if containsAny(normalized, []string{
		"shell", "命令行", "系统进程", "进程控制", "任意文件系统",
		"全局键盘监听", "浏览器扩展", "读取任意文件",
	}) {
		return Decision{}, ErrForbiddenRequirement
	}
	requiresWeb := containsAny(normalized, []string{
		"自由绘制", "画板", "小游戏", "webgl", "canvas", "自由画布",
		"自定义渲染算法",
	})
	switch target {
	case generation.TargetNative:
		if requiresWeb {
			return Decision{}, ErrUnsupportedRequirement
		}
		return Decision{Runtime: RuntimeNative, Reason: "需求可由可信原生组件、表达式和动作完整实现"}, nil
	case generation.TargetWeb:
		return Decision{Runtime: RuntimeWeb, Reason: "用户明确指定 CodeCard 本地 Web 运行时"}, nil
	case generation.TargetAuto, "":
		if requiresWeb {
			return Decision{Runtime: RuntimeWeb, Reason: "需求包含自由绘制或原生组件目录无法表达的交互"}, nil
		}
		return Decision{Runtime: RuntimeNative, Reason: "auto 模式优先使用更轻量且可信的 NativeCard"}, nil
	default:
		return Decision{}, generation.ErrInvalidTarget
	}
}

func containsAny(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
