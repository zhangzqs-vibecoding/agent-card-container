package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/auth"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/httpapi"
	"github.com/zzq/agent-card-container/services/cloud/internal/jobs"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
	"github.com/zzq/agent-card-container/services/cloud/internal/sandbox"
	"github.com/zzq/agent-card-container/services/cloud/internal/worker"
)

type Runtime struct {
	handler http.Handler
	worker  *worker.Worker
}

func NewFromEnvironment(environment map[string]string) (*Runtime, error) {
	authenticator, err := buildAuthenticator(environment)
	if err != nil {
		return nil, err
	}
	provider, err := modelprovider.NewHTTPProviderFromEnvironment(environment)
	if err != nil {
		return nil, err
	}
	privateKey, err := signingKey(environment["AGENTCARD_SIGNING_PRIVATE_KEY"])
	if err != nil {
		return nil, err
	}
	keyID := environment["AGENTCARD_SIGNING_KEY_ID"]
	if keyID == "" {
		return nil, fmt.Errorf("AGENTCARD_SIGNING_KEY_ID is required")
	}

	jobStore := jobs.NewMemoryStore(func() string { return randomID("job_") })
	generations := generation.NewService(
		generation.NewMemoryRepository(),
		func() string { return randomID("gen_") },
		time.Now,
		generation.WithJobQueue(jobs.NewGenerationQueue(jobStore)),
	)
	versions := publish.NewMemoryVersionRepository()
	publisher := publish.NewPublisher(
		artifact.NewBuilder(keyID, privateKey),
		publish.NewMemoryObjectStore(),
		versions,
	)
	agentOptions := make([]agent.Option, 0, 1)
	if image := environment["AGENTCARD_SANDBOX_IMAGE"]; image != "" {
		template := environment["AGENTCARD_CODECARD_TEMPLATE"]
		if template == "" {
			return nil, fmt.Errorf("AGENTCARD_CODECARD_TEMPLATE is required when sandbox is enabled")
		}
		docker := sandbox.NewDockerBuilder(sandbox.DockerConfig{
			Binary: environment["AGENTCARD_DOCKER_BINARY"],
			Image:  image,
		}, sandbox.ExecRunner{})
		agentOptions = append(
			agentOptions,
			agent.WithWebBuilder(agent.NewTemplateWebBuilder(template, docker)),
		)
	}
	codingAgent := agent.NewCodingAgent(
		provider,
		agent.NewNativeValidator(),
		agentOptions...,
	)
	workerRuntime := worker.New(worker.Config{
		WorkerID:     randomID("worker_"),
		Jobs:         jobStore,
		Generations:  generations,
		Agent:        codingAgent,
		Publisher:    publisher,
		NewCardID:    func() string { return randomID("card_") },
		NewVersionID: func() string { return randomID("ver_") },
		Now:          time.Now,
	})
	handler := httpapi.NewCloudHandler(httpapi.CloudHandlerConfig{
		ServiceName:   "agent-card-cloud",
		Generations:   generations,
		Publisher:     publisher,
		Authenticator: authenticator,
		NewRequestID:  func() string { return randomID("req_") },
	})
	return &Runtime{handler: handler, worker: workerRuntime}, nil
}

func (runtime *Runtime) Handler() http.Handler {
	return runtime.handler
}

func (runtime *Runtime) RunWorkerOnce(ctx context.Context) (worker.Outcome, error) {
	return runtime.worker.RunOnce(ctx)
}

func buildAuthenticator(environment map[string]string) (httpapi.Authenticator, error) {
	if token := environment["AGENTCARD_DEV_TOKEN"]; token != "" {
		userID := environment["AGENTCARD_DEV_USER"]
		if userID == "" {
			return nil, fmt.Errorf("AGENTCARD_DEV_USER is required with development token")
		}
		return httpapi.StaticBearerAuthenticator{token: userID}, nil
	}
	return auth.NewOIDCAuthenticator(auth.OIDCConfig{
		Issuer:   environment["AGENTCARD_OIDC_ISSUER"],
		Audience: environment["AGENTCARD_OIDC_AUDIENCE"],
		JWKSURL:  environment["AGENTCARD_OIDC_JWKS_URL"],
	})
}

func signingKey(encoded string) (ed25519.PrivateKey, error) {
	if encoded == "" {
		return nil, fmt.Errorf("AGENTCARD_SIGNING_PRIVATE_KEY is required")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, fmt.Errorf("signing private key must be base64")
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(append([]byte(nil), decoded...)), nil
	default:
		return nil, fmt.Errorf("signing private key has invalid length")
	}
}

func randomID(prefix string) string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("crypto/rand unavailable")
	}
	return prefix + hex.EncodeToString(value[:])
}
