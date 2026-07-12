package publish_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/publish"
)

func TestS3ObjectStoreUploadsAndSignsContentAddressedArtifact(t *testing.T) {
	endpoint := os.Getenv("AGENTCARD_S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("AGENTCARD_S3_TEST_ENDPOINT is not configured")
	}
	store, err := publish.NewS3ObjectStore(context.Background(), publish.S3Config{
		Endpoint:  endpoint,
		AccessKey: os.Getenv("AGENTCARD_S3_TEST_ACCESS_KEY"),
		SecretKey: os.Getenv("AGENTCARD_S3_TEST_SECRET_KEY"),
		Bucket:    os.Getenv("AGENTCARD_S3_TEST_BUCKET"),
		Secure:    false,
		Region:    "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	key := "artifacts/sha256/integration.agentcard"
	content := []byte("signed artifact")
	if err := store.PutIfAbsent(context.Background(), key, content); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), key, content); err != nil {
		t.Fatal(err)
	}
	url, expiresAt, err := store.SignedURL(context.Background(), key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expiresAt = %s", expiresAt)
	}
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != string(content) {
		t.Fatalf("GET status = %d, body = %q", response.StatusCode, body)
	}
}
