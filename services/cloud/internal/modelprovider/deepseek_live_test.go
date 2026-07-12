package modelprovider_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

func TestDeepSeekLiveGeneratesValidNativeCard(t *testing.T) {
	if os.Getenv("AGENTCARD_DEEPSEEK_LIVE") != "1" {
		t.Skip("set AGENTCARD_DEEPSEEK_LIVE=1 to run the paid live model test")
	}
	environment := map[string]string{
		"AGENTCARD_MODEL_BASE_URL": os.Getenv("AGENTCARD_MODEL_BASE_URL"),
		"AGENTCARD_MODEL_API_KEY":  os.Getenv("AGENTCARD_MODEL_API_KEY"),
		"AGENTCARD_MODEL":          os.Getenv("AGENTCARD_MODEL"),
	}
	provider, err := modelprovider.NewHTTPProviderFromEnvironment(environment)
	if err != nil {
		t.Fatalf("model environment configuration is invalid: %v", err)
	}
	codingAgent := agent.NewCodingAgent(provider, agent.NewNativeValidator())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := codingAgent.Generate(ctx, agent.Request{
		SessionID: "gen_deepseek_live",
		Prompt:    "生成一个完全离线的番茄钟卡片，有开始、暂停和重置按钮",
		Target:    generation.TargetNative,
		Locale:    "zh-CN",
	})
	if err != nil {
		t.Fatalf("DeepSeek live generation failed: %v", err)
	}
	if result.Runtime != agent.RuntimeNative || result.Content == "" {
		t.Fatalf("unexpected generation result: runtime=%s attempts=%d", result.Runtime, result.Attempts)
	}
}
