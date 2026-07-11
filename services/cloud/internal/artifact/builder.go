package artifact

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
)

type BuildInput struct {
	Definition contracts.CardDefinition
	Files      map[string][]byte
}

type BuildOutput struct {
	Archive []byte
	SHA256  string
	KeyID   string
}

type Builder struct {
	keyID      string
	privateKey ed25519.PrivateKey
}

func NewBuilder(keyID string, privateKey ed25519.PrivateKey) *Builder {
	return &Builder{keyID: keyID, privateKey: privateKey}
}

func (builder *Builder) Build(input BuildInput) (BuildOutput, error) {
	if strings.TrimSpace(builder.keyID) == "" || len(builder.privateKey) != ed25519.PrivateKeySize {
		return BuildOutput{}, fmt.Errorf("artifact signer is not configured")
	}
	if len(input.Files) == 0 || len(input.Files) > 512 {
		return BuildOutput{}, fmt.Errorf("artifact file count is invalid")
	}
	names := make([]string, 0, len(input.Files))
	var expanded int
	for name, content := range input.Files {
		if err := validatePath(name); err != nil {
			return BuildOutput{}, err
		}
		if len(content) > 8*1024*1024 {
			return BuildOutput{}, fmt.Errorf("artifact file %q exceeds size limit", name)
		}
		expanded += len(content)
		if expanded > 32*1024*1024 {
			return BuildOutput{}, fmt.Errorf("artifact exceeds expanded size limit")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if _, exists := input.Files[input.Definition.Entrypoint]; !exists {
		return BuildOutput{}, fmt.Errorf("artifact entrypoint is missing")
	}
	definition := input.Definition
	definition.Files = make([]contracts.CardFile, 0, len(names))
	for _, name := range names {
		digest := sha256.Sum256(input.Files[name])
		definition.Files = append(definition.Files, contracts.CardFile{
			Path:   name,
			SHA256: hex.EncodeToString(digest[:]),
			Size:   int64(len(input.Files[name])),
		})
	}
	if err := definition.Validate(); err != nil {
		return BuildOutput{}, fmt.Errorf("validate card definition: %w", err)
	}
	encodedDefinition, err := json.Marshal(definition)
	if err != nil {
		return BuildOutput{}, fmt.Errorf("encode card definition: %w", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(encodedDefinition, &manifest); err != nil {
		return BuildOutput{}, fmt.Errorf("normalize card definition: %w", err)
	}
	manifest["keyId"] = builder.keyID
	message, err := CanonicalJSON(manifest)
	if err != nil {
		return BuildOutput{}, err
	}
	manifest["signature"] = base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(builder.privateKey, message),
	)
	manifestBytes, err := CanonicalJSON(manifest)
	if err != nil {
		return BuildOutput{}, err
	}

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	if err := writeZipFile(writer, "manifest.json", manifestBytes); err != nil {
		return BuildOutput{}, err
	}
	for _, name := range names {
		if err := writeZipFile(writer, name, input.Files[name]); err != nil {
			return BuildOutput{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return BuildOutput{}, fmt.Errorf("close artifact archive: %w", err)
	}
	if archive.Len() > 8*1024*1024 {
		return BuildOutput{}, fmt.Errorf("artifact archive exceeds compressed size limit")
	}
	digest := sha256.Sum256(archive.Bytes())
	return BuildOutput{
		Archive: append([]byte(nil), archive.Bytes()...),
		SHA256:  hex.EncodeToString(digest[:]),
		KeyID:   builder.keyID,
	}, nil
}

func writeZipFile(writer *zip.Writer, name string, content []byte) error {
	header := &zip.FileHeader{
		Name:   name,
		Method: zip.Store,
	}
	header.SetMode(0o644)
	header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
	stream, err := writer.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create artifact file %q: %w", name, err)
	}
	if _, err := stream.Write(content); err != nil {
		return fmt.Errorf("write artifact file %q: %w", name, err)
	}
	return nil
}

func validatePath(name string) error {
	if name == "" ||
		strings.HasPrefix(name, "/") ||
		strings.Contains(name, "\\") ||
		path.Clean(name) != name {
		return fmt.Errorf("unsafe artifact path %q", name)
	}
	parts := strings.Split(name, "/")
	if len(parts) > 8 {
		return fmt.Errorf("artifact path %q is too deep", name)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("unsafe artifact path %q", name)
		}
	}
	return nil
}

func CanonicalJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	if err := appendCanonical(&output, value); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func appendCanonical(output *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		output.WriteString("null")
	case bool:
		if typed {
			output.WriteString("true")
		} else {
			output.WriteString("false")
		}
	case string:
		output.WriteString(strconv.Quote(typed))
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return fmt.Errorf("canonical JSON number must be finite")
		}
		output.WriteString(strconv.FormatFloat(typed, 'g', -1, 64))
	case float32:
		return appendCanonical(output, float64(typed))
	case int:
		output.WriteString(strconv.Itoa(typed))
	case int64:
		output.WriteString(strconv.FormatInt(typed, 10))
	case json.Number:
		if _, err := strconv.ParseFloat(string(typed), 64); err != nil {
			return fmt.Errorf("invalid canonical JSON number")
		}
		output.WriteString(string(typed))
	case []any:
		output.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				output.WriteByte(',')
			}
			if err := appendCanonical(output, item); err != nil {
				return err
			}
		}
		output.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		output.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				output.WriteByte(',')
			}
			output.WriteString(strconv.Quote(key))
			output.WriteByte(':')
			if err := appendCanonical(output, typed[key]); err != nil {
				return err
			}
		}
		output.WriteByte('}')
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode canonical JSON: %w", err)
		}
		var normalized any
		if err := json.Unmarshal(encoded, &normalized); err != nil {
			return fmt.Errorf("normalize canonical JSON: %w", err)
		}
		return appendCanonical(output, normalized)
	}
	return nil
}
