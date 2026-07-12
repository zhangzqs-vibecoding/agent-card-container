package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"

	"github.com/zzq/agent-card-container/services/cloud/internal/artifact"
)

func main() {
	seed := sha256.Sum256([]byte("agent-card-cross-language-fixture"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	message := map[string]any{
		"files": []any{
			map[string]any{
				"path":   "payload/native.json",
				"sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"size":   float64(128),
			},
		},
		"keyId": "fixture-key",
		"title": "离线卡片",
	}
	canonical, err := artifact.CanonicalJSON(message)
	if err != nil {
		panic(err)
	}
	output := map[string]any{
		"message":   string(canonical),
		"publicKey": base64.RawURLEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
		"signature": base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)),
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		panic(err)
	}
}
