package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// CardRuntime identifies the renderer used for a card.
type CardRuntime string

const (
	CardRuntimeNative CardRuntime = "native"
	CardRuntimeWeb    CardRuntime = "web"
)

// ErrInvalidRuntime is returned when a definition requests an unknown runtime.
var ErrInvalidRuntime = errors.New("invalid card runtime")

// Size is expressed in Flutter logical pixels.
type Size struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// NetworkPolicy controls card network access through the host proxy.
type NetworkPolicy struct {
	Mode    string   `json:"mode"`
	Domains []string `json:"domains"`
}

// CardFile records an immutable file in a card artifact.
type CardFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// CardDefinition describes one immutable version of a card.
type CardDefinition struct {
	FormatVersion      int           `json:"formatVersion"`
	MinHostVersion     string        `json:"minHostVersion"`
	CardID             string        `json:"cardId"`
	VersionID          string        `json:"versionId"`
	DisplayVersion     string        `json:"displayVersion"`
	Runtime            CardRuntime   `json:"runtime"`
	StateSchemaVersion int           `json:"stateSchemaVersion"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	Entrypoint         string        `json:"entrypoint"`
	CatalogVersion     string        `json:"catalogVersion,omitempty"`
	MinSize            Size          `json:"minSize"`
	PreferredSize      Size          `json:"preferredSize"`
	MaxSize            Size          `json:"maxSize"`
	Capabilities       []string      `json:"capabilities"`
	NetworkPolicy      NetworkPolicy `json:"networkPolicy"`
	Files              []CardFile    `json:"files"`
	CreatedAt          time.Time     `json:"createdAt"`
}

// DecodeCardDefinition strictly decodes and validates a card definition.
func DecodeCardDefinition(reader io.Reader) (CardDefinition, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var definition CardDefinition
	if err := decoder.Decode(&definition); err != nil {
		return CardDefinition{}, fmt.Errorf("decode card definition: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return CardDefinition{}, err
	}
	if err := definition.Validate(); err != nil {
		return CardDefinition{}, err
	}
	return definition, nil
}

// Validate checks invariants shared by every language implementation.
func (definition CardDefinition) Validate() error {
	if definition.FormatVersion != 1 {
		return fmt.Errorf("formatVersion must be 1")
	}
	if !strings.HasPrefix(definition.CardID, "card_") {
		return fmt.Errorf("cardId must start with card_")
	}
	if !strings.HasPrefix(definition.VersionID, "ver_") {
		return fmt.Errorf("versionId must start with ver_")
	}
	if definition.StateSchemaVersion < 1 {
		return fmt.Errorf("stateSchemaVersion must be positive")
	}
	if strings.TrimSpace(definition.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if err := validateSizes(definition.MinSize, definition.PreferredSize, definition.MaxSize); err != nil {
		return err
	}

	switch definition.Runtime {
	case CardRuntimeNative:
		if definition.Entrypoint != "payload/native.json" {
			return fmt.Errorf("native entrypoint must be payload/native.json")
		}
		if definition.CatalogVersion == "" {
			return fmt.Errorf("native catalogVersion is required")
		}
	case CardRuntimeWeb:
		if definition.Entrypoint != "payload/web/index.html" {
			return fmt.Errorf("web entrypoint must be payload/web/index.html")
		}
	default:
		return fmt.Errorf("%w: %q", ErrInvalidRuntime, definition.Runtime)
	}

	return nil
}

// HasCapability reports whether the manifest declares a capability.
func (definition CardDefinition) HasCapability(capability string) bool {
	for _, candidate := range definition.Capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}

func validateSizes(minimum, preferred, maximum Size) error {
	if minimum.Width <= 0 || minimum.Height <= 0 {
		return fmt.Errorf("minSize values must be positive")
	}
	if preferred.Width < minimum.Width || preferred.Height < minimum.Height {
		return fmt.Errorf("preferredSize must not be smaller than minSize")
	}
	if maximum.Width < preferred.Width || maximum.Height < preferred.Height {
		return fmt.Errorf("maxSize must not be smaller than preferredSize")
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode card definition: trailing JSON value")
		}
		return fmt.Errorf("decode card definition: %w", err)
	}
	return nil
}
