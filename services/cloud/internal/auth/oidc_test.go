package auth_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/auth"
)

func TestOIDCAuthenticatorVerifiesRS256ClaimsAndJWKS(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"keys": []any{jwk(&privateKey.PublicKey, "key-1")},
		})
	}))
	defer jwks.Close()
	now := time.Date(2026, 7, 12, 14, 0, 0, 0, time.UTC)
	authenticator, err := auth.NewOIDCAuthenticator(auth.OIDCConfig{
		Issuer:        "https://issuer.example",
		Audience:      "agent-card-api",
		JWKSURL:       jwks.URL,
		AllowInsecure: true,
		Now:           func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	token := signedToken(t, privateKey, "key-1", map[string]any{
		"iss": "https://issuer.example",
		"aud": []string{"other", "agent-card-api"},
		"sub": "user_01",
		"exp": now.Add(time.Hour).Unix(),
		"nbf": now.Add(-time.Minute).Unix(),
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/cards", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	userID, err := authenticator.Authenticate(request)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if userID != "user_01" {
		t.Fatalf("userID = %q", userID)
	}
}

func TestOIDCAuthenticatorRejectsExpiredAndWrongAudienceTokens(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []any{jwk(&privateKey.PublicKey, "key-1")}})
	}))
	defer jwks.Close()
	now := time.Now().UTC()
	authenticator, err := auth.NewOIDCAuthenticator(auth.OIDCConfig{
		Issuer:        "https://issuer.example",
		Audience:      "agent-card-api",
		JWKSURL:       jwks.URL,
		AllowInsecure: true,
		Now:           func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, claims := range []map[string]any{
		{"iss": "https://issuer.example", "aud": "agent-card-api", "sub": "user", "exp": now.Add(-time.Minute).Unix()},
		{"iss": "https://issuer.example", "aud": "wrong", "sub": "user", "exp": now.Add(time.Minute).Unix()},
	} {
		token := signedToken(t, privateKey, "key-1", claims)
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		if _, err := authenticator.Authenticate(request); err == nil {
			t.Fatalf("Authenticate() accepted claims %#v", claims)
		}
	}
}

func jwk(key *rsa.PublicKey, keyID string) map[string]any {
	exponent := big.NewInt(int64(key.E)).Bytes()
	return map[string]any{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": keyID,
		"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(exponent),
	}
}

func signedToken(t *testing.T, key *rsa.PrivateKey, keyID string, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT", "kid": keyID})
	payload, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := key.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}
