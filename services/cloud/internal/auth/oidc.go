package auth

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrUnauthenticated = errors.New("OIDC token is invalid")

type OIDCConfig struct {
	Issuer        string
	Audience      string
	JWKSURL       string
	AllowInsecure bool
	Now           func() time.Time
	HTTPClient    *http.Client
}

type OIDCAuthenticator struct {
	issuer   string
	audience string
	jwksURL  string
	now      func() time.Time
	client   *http.Client

	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey
	keysExpires time.Time
}

func NewOIDCAuthenticator(config OIDCConfig) (*OIDCAuthenticator, error) {
	issuer, err := url.Parse(config.Issuer)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" {
		return nil, fmt.Errorf("OIDC issuer must be an HTTPS URL")
	}
	jwksURL, err := url.Parse(config.JWKSURL)
	if err != nil || !jwksURL.IsAbs() || jwksURL.Host == "" {
		return nil, fmt.Errorf("OIDC JWKS URL is invalid")
	}
	if jwksURL.Scheme != "https" && !config.AllowInsecure {
		return nil, fmt.Errorf("OIDC JWKS URL must use HTTPS")
	}
	if strings.TrimSpace(config.Audience) == "" {
		return nil, fmt.Errorf("OIDC audience is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &OIDCAuthenticator{
		issuer:   strings.TrimRight(config.Issuer, "/"),
		audience: config.Audience,
		jwksURL:  config.JWKSURL,
		now:      config.Now,
		client:   config.HTTPClient,
		keys:     make(map[string]*rsa.PublicKey),
	}, nil
}

func (authenticator *OIDCAuthenticator) Authenticate(request *http.Request) (string, error) {
	const prefix = "Bearer "
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) || strings.Contains(strings.TrimPrefix(header, prefix), " ") {
		return "", ErrUnauthenticated
	}
	token := strings.TrimPrefix(header, prefix)
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(token) > 16*1024 {
		return "", ErrUnauthenticated
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrUnauthenticated
	}
	var tokenHeader struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &tokenHeader); err != nil ||
		tokenHeader.Algorithm != "RS256" ||
		tokenHeader.KeyID == "" {
		return "", ErrUnauthenticated
	}
	key, err := authenticator.key(request, tokenHeader.KeyID)
	if err != nil {
		return "", ErrUnauthenticated
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, cryptoHashSHA256, digest[:], signature); err != nil {
		return "", ErrUnauthenticated
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrUnauthenticated
	}
	var claims struct {
		Issuer    string          `json:"iss"`
		Audience  json.RawMessage `json:"aud"`
		Subject   string          `json:"sub"`
		ExpiresAt float64         `json:"exp"`
		NotBefore float64         `json:"nbf"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", ErrUnauthenticated
	}
	now := authenticator.now().UTC()
	const skew = 30 * time.Second
	if strings.TrimRight(claims.Issuer, "/") != authenticator.issuer ||
		strings.TrimSpace(claims.Subject) == "" ||
		claims.ExpiresAt == 0 ||
		now.After(time.Unix(int64(claims.ExpiresAt), 0).Add(skew)) ||
		(claims.NotBefore != 0 && now.Add(skew).Before(time.Unix(int64(claims.NotBefore), 0))) ||
		!containsAudience(claims.Audience, authenticator.audience) {
		return "", ErrUnauthenticated
	}
	return claims.Subject, nil
}

const cryptoHashSHA256 = 5

func (authenticator *OIDCAuthenticator) key(request *http.Request, keyID string) (*rsa.PublicKey, error) {
	authenticator.mu.RLock()
	key := authenticator.keys[keyID]
	valid := authenticator.now().Before(authenticator.keysExpires)
	authenticator.mu.RUnlock()
	if key != nil && valid {
		return key, nil
	}
	if err := authenticator.refreshKeys(request); err != nil {
		return nil, err
	}
	authenticator.mu.RLock()
	defer authenticator.mu.RUnlock()
	key = authenticator.keys[keyID]
	if key == nil {
		return nil, ErrUnauthenticated
	}
	return key, nil
}

func (authenticator *OIDCAuthenticator) refreshKeys(request *http.Request) error {
	httpRequest, err := http.NewRequestWithContext(request.Context(), http.MethodGet, authenticator.jwksURL, nil)
	if err != nil {
		return err
	}
	response, err := authenticator.client.Do(httpRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrUnauthenticated
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return ErrUnauthenticated
	}
	var set struct {
		Keys []struct {
			Type        string   `json:"kty"`
			Use         string   `json:"use"`
			Algorithm   string   `json:"alg"`
			KeyID       string   `json:"kid"`
			Modulus     string   `json:"n"`
			Exponent    string   `json:"e"`
			Certificate []string `json:"x5c"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(data, &set); err != nil {
		return ErrUnauthenticated
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, item := range set.Keys {
		if item.Type != "RSA" || item.Algorithm != "RS256" || item.KeyID == "" {
			continue
		}
		key, err := parseRSAKey(item.Modulus, item.Exponent, item.Certificate)
		if err == nil {
			keys[item.KeyID] = key
		}
	}
	if len(keys) == 0 {
		return ErrUnauthenticated
	}
	authenticator.mu.Lock()
	authenticator.keys = keys
	authenticator.keysExpires = authenticator.now().Add(5 * time.Minute)
	authenticator.mu.Unlock()
	return nil
}

func parseRSAKey(modulusText, exponentText string, certificates []string) (*rsa.PublicKey, error) {
	if len(certificates) > 0 {
		der, err := base64.StdEncoding.DecodeString(certificates[0])
		if err == nil {
			certificate, parseErr := x509.ParseCertificate(der)
			if parseErr == nil {
				if key, ok := certificate.PublicKey.(*rsa.PublicKey); ok {
					return key, nil
				}
			}
		}
	}
	modulus, err := base64.RawURLEncoding.DecodeString(modulusText)
	if err != nil {
		return nil, err
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(exponentText)
	if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
		return nil, ErrUnauthenticated
	}
	exponent := new(big.Int).SetBytes(exponentBytes)
	if !exponent.IsInt64() || exponent.Int64() < 3 {
		return nil, ErrUnauthenticated
	}
	key := &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: int(exponent.Int64())}
	if key.N.BitLen() < 2048 {
		return nil, ErrUnauthenticated
	}
	return key, nil
}

func containsAudience(raw json.RawMessage, expected string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == expected
	}
	var many []string
	if json.Unmarshal(raw, &many) != nil {
		return false
	}
	for _, audience := range many {
		if audience == expected {
			return true
		}
	}
	return false
}
