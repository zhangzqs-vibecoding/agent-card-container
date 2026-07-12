package artifact_test

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
)

func TestBuilderCreatesDeterministicSignedAgentCard(t *testing.T) {
	t.Parallel()

	seed := sha256.Sum256([]byte("agent-card-test-key"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	builder := artifact.NewBuilder("test-key", privateKey)
	input := artifact.BuildInput{
		Definition: nativeDefinition(),
		Files: map[string][]byte{
			"payload/native.json":     []byte(`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text","props":{"text":"完成"}}}`),
			"reports/validation.json": []byte(`{"status":"passed"}`),
		},
	}

	first, err := builder.Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	second, err := builder.Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Archive, second.Archive) {
		t.Fatal("Build() is not deterministic")
	}
	if len(first.SHA256) != 64 {
		t.Fatalf("SHA256 = %q", first.SHA256)
	}

	manifest := readManifest(t, first.Archive)
	signatureText := manifest["signature"].(string)
	delete(manifest, "signature")
	signature, err := base64.RawURLEncoding.DecodeString(signatureText)
	if err != nil {
		t.Fatal(err)
	}
	message, err := artifact.CanonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(privateKey.Public().(ed25519.PublicKey), message, signature) {
		t.Fatal("manifest signature did not verify")
	}
	files := manifest["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("files = %#v", files)
	}
}

func TestBuilderRejectsDefinitionAndFileSetMismatch(t *testing.T) {
	t.Parallel()

	seed := sha256.Sum256([]byte("key"))
	builder := artifact.NewBuilder("key", ed25519.NewKeyFromSeed(seed[:]))
	definition := nativeDefinition()
	definition.Entrypoint = "payload/native.json"

	_, err := builder.Build(artifact.BuildInput{
		Definition: definition,
		Files:      map[string][]byte{"reports/validation.json": []byte("{}")},
	})
	if err == nil {
		t.Fatal("Build() accepted missing entrypoint")
	}
}

func TestEd25519FixtureMatchesGoCanonicalSigner(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(
		filepath.Dir(file),
		"..", "..", "..", "..",
		"contracts", "card", "fixtures", "ed25519-signature-vector.json",
	)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Message   string `json:"message"`
		PublicKey string `json:"publicKey"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(vector.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(vector.Signature)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(publicKey, []byte(vector.Message), signature) {
		t.Fatal("shared Ed25519 fixture failed Go verification")
	}
}

func nativeDefinition() contracts.CardDefinition {
	return contracts.CardDefinition{
		FormatVersion:      1,
		MinHostVersion:     "1.0.0",
		CardID:             "card_test",
		VersionID:          "ver_test_1",
		DisplayVersion:     "1.0.0",
		Runtime:            contracts.CardRuntimeNative,
		StateSchemaVersion: 1,
		Title:              "测试卡片",
		Description:        "离线测试",
		Entrypoint:         "payload/native.json",
		CatalogVersion:     "1",
		MinSize:            contracts.Size{Width: 240, Height: 160},
		PreferredSize:      contracts.Size{Width: 360, Height: 240},
		MaxSize:            contracts.Size{Width: 720, Height: 480},
		Capabilities:       []string{"storage"},
		NetworkPolicy:      contracts.NetworkPolicy{Mode: "none", Domains: []string{}},
		CreatedAt:          time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
	}
}

func readManifest(t *testing.T, archive []byte) map[string]any {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name != "manifest.json" {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		var manifest map[string]any
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	t.Fatal("manifest.json missing")
	return nil
}
