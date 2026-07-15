package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
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
	"github.com/zzq/agent-card-container/services/cloud/migrations"
)

type Runtime struct {
	handler  http.Handler
	worker   *worker.Worker
	database *sql.DB
}

func NewFromEnvironment(environment map[string]string) (*Runtime, error) {
	return NewFromEnvironmentWithLogger(
		environment,
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
	)
}

func NewFromEnvironmentWithLogger(environment map[string]string, logger *slog.Logger) (*Runtime, error) {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
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

	repositories, err := buildRepositories(environment)
	if err != nil {
		return nil, err
	}
	composed := false
	defer func() {
		if !composed && repositories.database != nil {
			_ = repositories.database.Close()
		}
	}()
	jobStore := repositories.jobs
	publisher := publish.NewPublisher(
		artifact.NewBuilder(keyID, privateKey),
		repositories.objects,
		repositories.versions,
	)
	generations := generation.NewService(
		repositories.generations,
		func() string { return randomID("gen_") },
		time.Now,
		generation.WithJobQueue(jobs.NewGenerationQueue(jobStore)),
		generation.WithAtomicJobID(func() string { return randomID("job_") }),
		generation.WithBaseVersionCatalog(publisher),
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
		append(agentOptions,
			agent.WithLogger(logger),
			agent.WithModelName(environment["AGENTCARD_MODEL"]),
		)...,
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
		Logger:       logger,
	})
	handler := httpapi.NewCloudHandler(httpapi.CloudHandlerConfig{
		ServiceName:   "agent-card-cloud",
		Generations:   generations,
		Publisher:     publisher,
		Authenticator: authenticator,
		NewRequestID:  func() string { return randomID("req_") },
		Logger:        logger,
		Now:           time.Now,
		Readiness:     repositories.readiness,
	})
	composed = true
	return &Runtime{
		handler:  handler,
		worker:   workerRuntime,
		database: repositories.database,
	}, nil
}

func (runtime *Runtime) Handler() http.Handler {
	return runtime.handler
}

func (runtime *Runtime) RunWorkerOnce(ctx context.Context) (worker.Outcome, error) {
	return runtime.worker.RunOnce(ctx)
}

func (runtime *Runtime) Close() error {
	if runtime.database != nil {
		return runtime.database.Close()
	}
	return nil
}

type repositorySet struct {
	generations generation.Repository
	jobs        jobs.Store
	versions    publish.VersionRepository
	objects     publish.ObjectStore
	database    *sql.DB
	readiness   httpapi.ReadinessChecker
}

func buildRepositories(environment map[string]string) (repositorySet, error) {
	persistenceRequired, err := parsePersistenceRequired(environment["AGENTCARD_PERSISTENCE_REQUIRED"])
	if err != nil {
		return repositorySet{}, err
	}
	databaseURL := strings.TrimSpace(environment["AGENTCARD_DATABASE_URL"])
	s3Endpoint := strings.TrimSpace(environment["AGENTCARD_S3_ENDPOINT"])
	if databaseURL == "" && s3Endpoint == "" {
		if persistenceRequired {
			return repositorySet{}, fmt.Errorf("AGENTCARD_DATABASE_URL and AGENTCARD_S3_ENDPOINT are required when persistence is required")
		}
		return repositorySet{
			generations: generation.NewMemoryRepository(),
			jobs:        jobs.NewMemoryStore(func() string { return randomID("job_") }),
			versions:    publish.NewMemoryVersionRepository(),
			objects:     publish.NewMemoryObjectStore(),
			readiness:   readyRuntime{},
		}, nil
	}
	if databaseURL == "" || s3Endpoint == "" {
		return repositorySet{}, fmt.Errorf("AGENTCARD_DATABASE_URL and AGENTCARD_S3_ENDPOINT must be configured together")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return repositorySet{}, err
	}
	cleanup := func(err error) (repositorySet, error) {
		_ = database.Close()
		return repositorySet{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		return cleanup(fmt.Errorf("connect PostgreSQL: %w", err))
	}
	if err := migrations.Apply(ctx, database); err != nil {
		return cleanup(err)
	}
	objects, err := publish.NewS3ObjectStore(ctx, publish.S3Config{
		Endpoint:  s3Endpoint,
		AccessKey: environment["AGENTCARD_S3_ACCESS_KEY"],
		SecretKey: environment["AGENTCARD_S3_SECRET_KEY"],
		Bucket:    environment["AGENTCARD_S3_BUCKET"],
		Region:    environment["AGENTCARD_S3_REGION"],
		Secure:    environment["AGENTCARD_S3_SECURE"] != "false",
	})
	if err != nil {
		return cleanup(fmt.Errorf("connect S3 object store: %w", err))
	}
	jobStore := jobs.NewPostgresStore(database, func() string { return randomID("job_") })
	return repositorySet{
		generations: generation.NewPostgresRepository(database),
		jobs:        jobStore,
		versions:    publish.NewPostgresVersionRepository(database),
		objects:     objects,
		database:    database,
		readiness: dependencyReadiness{
			database: database.PingContext,
			objects:  objects.Ready,
		},
	}, nil
}

func parsePersistenceRequired(value string) (bool, error) {
	switch strings.TrimSpace(value) {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, fmt.Errorf("AGENTCARD_PERSISTENCE_REQUIRED must be true or false")
	}
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
