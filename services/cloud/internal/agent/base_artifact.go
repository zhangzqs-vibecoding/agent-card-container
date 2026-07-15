package agent

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
)

const (
	maxBaseArchiveBytes  = 8 * 1024 * 1024
	maxBaseFileBytes     = 8 * 1024 * 1024
	maxBaseExpandedBytes = 32 * 1024 * 1024
	MaxBaseSourceBytes   = 512 * 1024
)

type BaseArtifactExpectation struct {
	CardID    string
	VersionID string
	KeyID     string
}

type BaseArtifact struct {
	Definition contracts.CardDefinition
	Sources    map[string]string
}

func ParseBaseArtifact(
	archive []byte,
	expectation BaseArtifactExpectation,
	trustedKeys map[string]ed25519.PublicKey,
) (BaseArtifact, error) {
	if len(archive) == 0 || len(archive) > maxBaseArchiveBytes {
		return BaseArtifact{}, fmt.Errorf("base artifact archive size is invalid")
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return BaseArtifact{}, fmt.Errorf("open base artifact: %w", err)
	}
	if len(reader.File) == 0 || len(reader.File) > 513 {
		return BaseArtifact{}, fmt.Errorf("base artifact file count is invalid")
	}
	files := make(map[string][]byte, len(reader.File))
	expanded := int64(0)
	for _, entry := range reader.File {
		if err := validateBaseArtifactPath(entry.Name); err != nil {
			return BaseArtifact{}, err
		}
		if _, exists := files[entry.Name]; exists {
			return BaseArtifact{}, fmt.Errorf("duplicate base artifact path %q", entry.Name)
		}
		if entry.UncompressedSize64 > maxBaseFileBytes {
			return BaseArtifact{}, fmt.Errorf("base artifact file %q exceeds size limit", entry.Name)
		}
		content, err := readBaseArtifactFile(entry, maxBaseFileBytes)
		if err != nil {
			return BaseArtifact{}, err
		}
		expanded += int64(len(content))
		if expanded > maxBaseExpandedBytes {
			return BaseArtifact{}, fmt.Errorf("base artifact exceeds expanded size limit")
		}
		files[entry.Name] = content
	}
	manifestBytes, exists := files["manifest.json"]
	if !exists {
		return BaseArtifact{}, fmt.Errorf("base artifact manifest is missing")
	}
	delete(files, "manifest.json")
	definition, keyID, err := verifyBaseManifest(manifestBytes, trustedKeys)
	if err != nil {
		return BaseArtifact{}, err
	}
	if definition.CardID != expectation.CardID ||
		definition.VersionID != expectation.VersionID ||
		keyID != expectation.KeyID {
		return BaseArtifact{}, fmt.Errorf("base artifact identity does not match requested version")
	}
	if err := verifyBaseFiles(definition.Files, files); err != nil {
		return BaseArtifact{}, err
	}
	sources, err := extractBaseSources(definition, files)
	if err != nil {
		return BaseArtifact{}, err
	}
	return BaseArtifact{Definition: definition, Sources: sources}, nil
}

func verifyBaseManifest(
	manifestBytes []byte,
	trustedKeys map[string]ed25519.PublicKey,
) (contracts.CardDefinition, string, error) {
	if len(manifestBytes) == 0 || len(manifestBytes) > 256*1024 || !utf8.Valid(manifestBytes) {
		return contracts.CardDefinition{}, "", fmt.Errorf("base artifact manifest is invalid")
	}
	var manifest map[string]any
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&manifest); err != nil {
		return contracts.CardDefinition{}, "", fmt.Errorf("decode base artifact manifest: %w", err)
	}
	if err := ensureBaseJSONEOF(decoder); err != nil {
		return contracts.CardDefinition{}, "", err
	}
	keyID, ok := manifest["keyId"].(string)
	if !ok || strings.TrimSpace(keyID) == "" {
		return contracts.CardDefinition{}, "", fmt.Errorf("base artifact keyId is invalid")
	}
	signatureText, ok := manifest["signature"].(string)
	if !ok {
		return contracts.CardDefinition{}, "", fmt.Errorf("base artifact signature is invalid")
	}
	delete(manifest, "signature")
	message, err := artifact.CanonicalJSON(manifest)
	if err != nil {
		return contracts.CardDefinition{}, "", fmt.Errorf("canonicalize base artifact manifest: %w", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(signatureText)
	if err != nil {
		return contracts.CardDefinition{}, "", fmt.Errorf("decode base artifact signature: %w", err)
	}
	publicKey := trustedKeys[keyID]
	if len(publicKey) != ed25519.PublicKeySize || !ed25519.Verify(publicKey, message, signature) {
		return contracts.CardDefinition{}, "", fmt.Errorf("base artifact signature verification failed")
	}
	delete(manifest, "keyId")
	definitionBytes, err := artifact.CanonicalJSON(manifest)
	if err != nil {
		return contracts.CardDefinition{}, "", err
	}
	definition, err := contracts.DecodeCardDefinition(bytes.NewReader(definitionBytes))
	if err != nil {
		return contracts.CardDefinition{}, "", fmt.Errorf("decode base card definition: %w", err)
	}
	return definition, keyID, nil
}

func verifyBaseFiles(expected []contracts.CardFile, actual map[string][]byte) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("base artifact file set does not match manifest")
	}
	seen := make(map[string]bool, len(expected))
	for _, file := range expected {
		if seen[file.Path] {
			return fmt.Errorf("duplicate base manifest path %q", file.Path)
		}
		seen[file.Path] = true
		content, exists := actual[file.Path]
		if !exists || int64(len(content)) != file.Size {
			return fmt.Errorf("base artifact file %q size does not match manifest", file.Path)
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != file.SHA256 {
			return fmt.Errorf("base artifact file %q hash does not match manifest", file.Path)
		}
	}
	return nil
}

func extractBaseSources(
	definition contracts.CardDefinition,
	files map[string][]byte,
) (map[string]string, error) {
	sources := make(map[string]string)
	if definition.Runtime == contracts.CardRuntimeNative {
		for name := range files {
			if strings.HasPrefix(name, "source/") {
				return nil, fmt.Errorf("base NativeCard source path %q is not allowed", name)
			}
		}
		content, exists := files["payload/native.json"]
		if !exists {
			return nil, fmt.Errorf("base NativeCard source is missing")
		}
		if err := addBaseSource(sources, "payload/native.json", content); err != nil {
			return nil, err
		}
	} else {
		allowed := map[string]string{
			"source/web/src/card.tsx":      "src/card.tsx",
			"source/web/src/card.css":      "src/card.css",
			"source/web/src/card.test.tsx": "src/card.test.tsx",
		}
		for name, content := range files {
			if !strings.HasPrefix(name, "source/") {
				continue
			}
			outputName, ok := allowed[name]
			if !ok {
				return nil, fmt.Errorf("base CodeCard source path %q is not allowed", name)
			}
			if err := addBaseSource(sources, outputName, content); err != nil {
				return nil, err
			}
		}
		if _, exists := sources["src/card.tsx"]; !exists {
			return nil, fmt.Errorf("base CodeCard source is missing src/card.tsx")
		}
	}
	total := 0
	for _, source := range sources {
		total += len(source)
	}
	if total > MaxBaseSourceBytes {
		return nil, fmt.Errorf("base card source exceeds total size limit")
	}
	return sources, nil
}

func addBaseSource(output map[string]string, name string, content []byte) error {
	if len(content) > MaxBaseSourceBytes || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return fmt.Errorf("base card source %q contains invalid text", name)
	}
	output[name] = string(content)
	return nil
}

func validateBaseArtifactPath(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != name {
		return fmt.Errorf("unsafe base artifact path %q", name)
	}
	parts := strings.Split(name, "/")
	if len(parts) > 8 {
		return fmt.Errorf("base artifact path %q is too deep", name)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("unsafe base artifact path %q", name)
		}
	}
	return nil
}

func readBaseArtifactFile(file *zip.File, maxBytes int64) ([]byte, error) {
	stream, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open base artifact file %q: %w", file.Name, err)
	}
	defer stream.Close()
	content, err := io.ReadAll(io.LimitReader(stream, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read base artifact file %q: %w", file.Name, err)
	}
	if int64(len(content)) > maxBytes {
		return nil, fmt.Errorf("base artifact file %q exceeds size limit", file.Name)
	}
	return content, nil
}

func ensureBaseJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("base artifact manifest must contain one JSON value")
	}
	return nil
}
