package agent_test

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
)

func TestParseBaseArtifactVerifiesIdentitySignatureAndNativeSource(t *testing.T) {
	t.Parallel()

	privateKey := baseArtifactKey()
	built := buildBaseArtifact(t, privateKey, baseNativeDefinition(), map[string][]byte{
		"payload/native.json":     []byte(`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`),
		"reports/validation.json": []byte(`{"status":"passed"}`),
	})
	parsed, err := agent.ParseBaseArtifact(built.Archive, agent.BaseArtifactExpectation{
		CardID:    "card_base",
		VersionID: "ver_base",
		KeyID:     "base-key",
	}, map[string]ed25519.PublicKey{
		"base-key": privateKey.Public().(ed25519.PublicKey),
	})
	if err != nil {
		t.Fatalf("ParseBaseArtifact() error = %v", err)
	}
	if parsed.Definition.CardID != "card_base" || len(parsed.Sources) != 1 || parsed.Sources["payload/native.json"] == "" {
		t.Fatalf("parsed = %#v", parsed)
	}
	if _, exists := parsed.Sources["reports/validation.json"]; exists {
		t.Fatalf("validation report leaked into sources: %#v", parsed.Sources)
	}
}

func TestParseBaseArtifactAcceptsOnlyControlledWebSources(t *testing.T) {
	t.Parallel()

	privateKey := baseArtifactKey()
	definition := baseNativeDefinition()
	definition.Runtime = contracts.CardRuntimeWeb
	definition.Entrypoint = "payload/web/index.html"
	definition.CatalogVersion = ""
	built := buildBaseArtifact(t, privateKey, definition, map[string][]byte{
		"payload/web/index.html":       []byte("<main></main>"),
		"source/web/src/card.tsx":      []byte("export function Card() { return <main /> }"),
		"source/web/src/card.css":      []byte("main { color: red; }"),
		"source/web/src/card.test.tsx": []byte("test('card', () => {})"),
		"reports/validation.json":      []byte(`{"status":"passed"}`),
	})
	parsed, err := agent.ParseBaseArtifact(built.Archive, agent.BaseArtifactExpectation{
		CardID: "card_base", VersionID: "ver_base", KeyID: "base-key",
	}, map[string]ed25519.PublicKey{"base-key": privateKey.Public().(ed25519.PublicKey)})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Sources) != 3 || parsed.Sources["src/card.tsx"] == "" {
		t.Fatalf("sources = %#v", parsed.Sources)
	}
}

func TestParseBaseArtifactRejectsUntrustedOrMismatchedArtifact(t *testing.T) {
	t.Parallel()

	privateKey := baseArtifactKey()
	built := buildBaseArtifact(t, privateKey, baseNativeDefinition(), map[string][]byte{
		"payload/native.json": []byte(`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`),
	})
	tests := []struct {
		name        string
		expectation agent.BaseArtifactExpectation
		keys        map[string]ed25519.PublicKey
	}{
		{name: "card", expectation: agent.BaseArtifactExpectation{CardID: "card_other", VersionID: "ver_base", KeyID: "base-key"}, keys: map[string]ed25519.PublicKey{"base-key": privateKey.Public().(ed25519.PublicKey)}},
		{name: "version", expectation: agent.BaseArtifactExpectation{CardID: "card_base", VersionID: "ver_other", KeyID: "base-key"}, keys: map[string]ed25519.PublicKey{"base-key": privateKey.Public().(ed25519.PublicKey)}},
		{name: "key id", expectation: agent.BaseArtifactExpectation{CardID: "card_base", VersionID: "ver_base", KeyID: "other-key"}, keys: map[string]ed25519.PublicKey{"base-key": privateKey.Public().(ed25519.PublicKey)}},
		{name: "untrusted", expectation: agent.BaseArtifactExpectation{CardID: "card_base", VersionID: "ver_base", KeyID: "base-key"}, keys: map[string]ed25519.PublicKey{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := agent.ParseBaseArtifact(built.Archive, test.expectation, test.keys); err == nil {
				t.Fatal("ParseBaseArtifact() error = nil")
			}
		})
	}
	wrongSeed := sha256.Sum256([]byte("wrong-base-artifact-key"))
	if _, err := agent.ParseBaseArtifact(built.Archive, agent.BaseArtifactExpectation{
		CardID: "card_base", VersionID: "ver_base", KeyID: "base-key",
	}, map[string]ed25519.PublicKey{
		"base-key": ed25519.NewKeyFromSeed(wrongSeed[:]).Public().(ed25519.PublicKey),
	}); err == nil {
		t.Fatal("invalid signature error = nil")
	}

	tampered := append([]byte(nil), built.Archive...)
	tampered[len(tampered)/2] ^= 0xff
	if _, err := agent.ParseBaseArtifact(tampered, agent.BaseArtifactExpectation{
		CardID: "card_base", VersionID: "ver_base", KeyID: "base-key",
	}, map[string]ed25519.PublicKey{"base-key": privateKey.Public().(ed25519.PublicKey)}); err == nil {
		t.Fatal("tampered ParseBaseArtifact() error = nil")
	}
}

func TestParseBaseArtifactRejectsUnsafeArchiveAndSourceLimits(t *testing.T) {
	t.Parallel()

	privateKey := baseArtifactKey()
	if _, err := parseBase(t, []byte("not a zip"), privateKey); err == nil {
		t.Fatal("invalid ZIP error = nil")
	}
	valid := buildBaseArtifact(t, privateKey, baseNativeDefinition(), map[string][]byte{
		"payload/native.json": []byte(`{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`),
	})
	unsafe := appendZipEntry(t, valid.Archive, "../escape", []byte("x"))
	if _, err := parseBase(t, unsafe, privateKey); err == nil {
		t.Fatal("unsafe archive error = nil")
	}

	invalidUTF8 := buildBaseArtifact(t, privateKey, baseNativeDefinition(), map[string][]byte{
		"payload/native.json": []byte{0xff, 0xfe},
	})
	if _, err := parseBase(t, invalidUTF8.Archive, privateKey); err == nil {
		t.Fatal("invalid UTF-8 error = nil")
	}

	tooLarge := buildBaseArtifact(t, privateKey, baseNativeDefinition(), map[string][]byte{
		"payload/native.json": bytes.Repeat([]byte("a"), agent.MaxBaseSourceBytes+1),
	})
	if _, err := parseBase(t, tooLarge.Archive, privateKey); err == nil {
		t.Fatal("oversized source error = nil")
	}

	definition := baseNativeDefinition()
	definition.Runtime = contracts.CardRuntimeWeb
	definition.Entrypoint = "payload/web/index.html"
	definition.CatalogVersion = ""
	unknownSource := buildBaseArtifact(t, privateKey, definition, map[string][]byte{
		"payload/web/index.html":     []byte("<main></main>"),
		"source/web/src/card.tsx":    []byte("export const Card = () => null"),
		"source/web/src/unknown.tsx": []byte("export default 1"),
	})
	if _, err := parseBase(t, unknownSource.Archive, privateKey); err == nil {
		t.Fatal("unknown source error = nil")
	}
}

func baseArtifactKey() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("base-artifact-key"))
	return ed25519.NewKeyFromSeed(seed[:])
}

func baseNativeDefinition() contracts.CardDefinition {
	return contracts.CardDefinition{
		FormatVersion: 1, MinHostVersion: "1.0.0", CardID: "card_base", VersionID: "ver_base",
		DisplayVersion: "1.0.0", Runtime: contracts.CardRuntimeNative, StateSchemaVersion: 1,
		Title: "基线", Entrypoint: "payload/native.json", CatalogVersion: "1",
		MinSize: contracts.Size{Width: 240, Height: 160}, PreferredSize: contracts.Size{Width: 360, Height: 240},
		MaxSize: contracts.Size{Width: 720, Height: 480}, Capabilities: []string{"storage"},
		NetworkPolicy: contracts.NetworkPolicy{Mode: "none", Domains: []string{}},
		CreatedAt:     time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
	}
}

func buildBaseArtifact(t *testing.T, privateKey ed25519.PrivateKey, definition contracts.CardDefinition, files map[string][]byte) artifact.BuildOutput {
	t.Helper()
	built, err := artifact.NewBuilder("base-key", privateKey).Build(artifact.BuildInput{Definition: definition, Files: files})
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func parseBase(t *testing.T, archive []byte, privateKey ed25519.PrivateKey) (agent.BaseArtifact, error) {
	t.Helper()
	return agent.ParseBaseArtifact(archive, agent.BaseArtifactExpectation{
		CardID: "card_base", VersionID: "ver_base", KeyID: "base-key",
	}, map[string]ed25519.PublicKey{"base-key": privateKey.Public().(ed25519.PublicKey)})
}

func appendZipEntry(t *testing.T, original []byte, name string, content []byte) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data := new(bytes.Buffer)
		if _, err := data.ReadFrom(stream); err != nil {
			t.Fatal(err)
		}
		_ = stream.Close()
		target, err := writer.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := target.Write(data.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	target, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
