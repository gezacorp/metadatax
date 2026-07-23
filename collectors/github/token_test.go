package github_test

import (
	"context"
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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gezacorp/metadatax/collectors/github"
)

const testKID = "test-key-1"

// oidcTestServer is a minimal self-hosted OIDC issuer: it serves a discovery
// document and a JWKS backed by a locally generated RSA key, and can sign
// tokens with that key. This exercises the real signature-verification path
// in NewOIDCTokenVerifier end to end, with no network access.
type oidcTestServer struct {
	server *httptest.Server
	key    *rsa.PrivateKey
}

func newOIDCTestServer(t *testing.T) *oidcTestServer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	ts := &oidcTestServer{key: key}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                ts.server.URL,
			"jwks_uri":                              ts.server.URL + "/.well-known/jwks",
			"response_types_supported":              []string{"id_token"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/.well-known/jwks", func(w http.ResponseWriter, r *http.Request) {
		pub := key.Public().(*rsa.PublicKey)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": testKID,
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			}},
		})
	})

	ts.server = httptest.NewServer(mux)
	t.Cleanup(ts.server.Close)

	return ts
}

func (ts *oidcTestServer) sign(t *testing.T, claims map[string]any) string {
	t.Helper()

	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": testKID}
	if claims["iss"] == nil {
		claims["iss"] = ts.server.URL
	}

	b64 := func(v any) string {
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(raw)
	}

	signingInput := b64(header) + "." + b64(claims)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, ts.key, crypto.SHA256, sum[:])
	require.NoError(t, err)

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func baseClaims(ts *oidcTestServer) map[string]any {
	now := time.Now()
	return map[string]any{
		"iss":                 ts.server.URL,
		"aud":                 "vso:38460d49-5296-4101-bdd0-d17987c84a6c",
		"iat":                 now.Add(-time.Minute).Unix(),
		"nbf":                 now.Add(-time.Minute).Unix(),
		"exp":                 now.Add(time.Hour).Unix(),
		"run_id":              "30012614543",
		"run_number":          "1806",
		"sha":                 "aa5c5dd6adc9b38638e8b7251716d0515b6df0c7",
		"repository_id":       "976787887",
		"repository_owner_id": "202861820",
		"job_id":              "e679fd3b-5ebe-5859-aeb4-7920aa158817",
		"runner_id":           "1000039223",
		"trust_tier":          "1",
		"oidc_sub":            "repo:riptideslabs/daemon:ref:refs/heads/aa",
		"oidc_extra":          `{"repository":"riptideslabs/daemon","actor":"waynz0r","ref":"refs/heads/aa","workflow":"build"}`,
	}
}

func TestOIDCVerifierValidToken(t *testing.T) {
	ts := newOIDCTestServer(t)
	verifier := github.NewOIDCTokenVerifier(ts.server.URL)

	token := ts.sign(t, baseClaims(ts))

	claims, err := verifier.Verify(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "30012614543", claims.RunID)
	assert.Equal(t, "976787887", claims.RepositoryID)
	assert.Equal(t, "e679fd3b-5ebe-5859-aeb4-7920aa158817", claims.JobID)
	assert.Equal(t, ts.server.URL, claims.Issuer)

	// oidc_extra parses into the attested identity.
	id, ok := claims.Identity()
	require.True(t, ok)
	assert.Equal(t, "riptideslabs/daemon", id.Repository)
	assert.Equal(t, "waynz0r", id.Actor)
	assert.Equal(t, "repo:riptideslabs/daemon:ref:refs/heads/aa", claims.Subject)
}

func TestOIDCVerifierExpiredToken(t *testing.T) {
	ts := newOIDCTestServer(t)
	verifier := github.NewOIDCTokenVerifier(ts.server.URL)

	claims := baseClaims(ts)
	claims["exp"] = time.Now().Add(-time.Hour).Unix()
	claims["iat"] = time.Now().Add(-2 * time.Hour).Unix()
	claims["nbf"] = time.Now().Add(-2 * time.Hour).Unix()

	_, err := verifier.Verify(context.Background(), ts.sign(t, claims))
	assert.Error(t, err)
}

func TestOIDCVerifierTamperedSignature(t *testing.T) {
	ts := newOIDCTestServer(t)
	verifier := github.NewOIDCTokenVerifier(ts.server.URL)

	token := ts.sign(t, baseClaims(ts))
	tampered := token[:len(token)-2] + "xx"

	_, err := verifier.Verify(context.Background(), tampered)
	assert.Error(t, err)
}

func TestOIDCVerifierWrongIssuerKey(t *testing.T) {
	// A token signed by a different server's key must fail verification
	// against this server's JWKS.
	ts := newOIDCTestServer(t)
	other := newOIDCTestServer(t)
	verifier := github.NewOIDCTokenVerifier(ts.server.URL)

	// Sign with `other`'s key but claim `ts`'s issuer.
	claims := baseClaims(ts)
	token := other.sign(t, claims)

	_, err := verifier.Verify(context.Background(), token)
	assert.Error(t, err)
}
