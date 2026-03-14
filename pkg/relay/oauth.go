// OAuth 2.0 / OIDC token validation for the relay server.
//
// Supports three modes, configured via environment variables:
//
//  1. OIDC JWT validation (Google, Auth0, any OIDC provider):
//     RELAY_OAUTH_ISSUER=https://accounts.google.com
//     RELAY_OAUTH_AUDIENCE=your-client-id
//     → Validates JWTs using the issuer's JWKS endpoint (auto-discovered)
//
//  2. GCP Identity Token (Cloud Run / IAP):
//     RELAY_OAUTH_ISSUER=https://accounts.google.com
//     RELAY_OAUTH_AUDIENCE=https://relay-xxxxx-uc.a.run.app
//     → Same flow — GCP identity tokens are standard OIDC JWTs
//
//  3. Static Bearer token fallback:
//     RELAY_AUTH_TOKEN=some-secret
//     → Simple string comparison (dev/testing)
//
// The middleware checks the Authorization header for "Bearer <token>". For
// OIDC mode it decodes the JWT, verifies the signature against the issuer's
// JWKS, checks exp/iss/aud claims, and optionally restricts to specific
// email addresses via RELAY_OAUTH_ALLOWED_EMAILS.
package relay

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// OAuthConfig holds OAuth 2.0 / OIDC configuration for the relay.
type OAuthConfig struct {
	// Issuer is the OIDC issuer URL (e.g., "https://accounts.google.com").
	// When set, the relay validates JWTs using the issuer's JWKS.
	Issuer string

	// Audiences is the list of accepted "aud" claims in the JWT.
	// Typically includes the OAuth client ID and the Cloud Run service URL.
	Audiences []string

	// AllowedEmails restricts access to specific email addresses found in
	// the "email" claim. If empty, any valid token from the issuer is accepted.
	// This is the static fallback — prefer Firestore for dynamic management.
	AllowedEmails []string

	// Firestore is an optional Firestore-backed email allowlist.
	// When set, email authorization is checked against Firestore first,
	// falling back to AllowedEmails if Firestore is unreachable.
	Firestore *FirestoreAllowlist

	// StaticToken is a fallback static Bearer token. If set and no Issuer
	// is configured, simple string comparison is used.
	StaticToken string
}

// oauthMiddleware returns an HTTP middleware that validates OAuth 2.0 / OIDC
// tokens on every request (except /health).
func oauthMiddleware(cfg *OAuthConfig, next http.Handler) http.Handler {
	var jwks *jwksCache
	if cfg.Issuer != "" {
		jwks = newJWKSCache(cfg.Issuer)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health check is always public (for load balancer probes).
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"missing Bearer token"}`, http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")

		// OIDC JWT validation mode.
		if cfg.Issuer != "" && jwks != nil {
			// Try each accepted audience until one validates.
			var claims map[string]any
			var lastErr error
			for _, aud := range cfg.Audiences {
				claims, lastErr = validateJWT(token, jwks, cfg.Issuer, aud)
				if lastErr == nil {
					break
				}
			}
			// If no audiences configured, validate without audience check.
			if len(cfg.Audiences) == 0 {
				claims, lastErr = validateJWT(token, jwks, cfg.Issuer, "")
			}
			if lastErr != nil {
				http.Error(w, fmt.Sprintf(`{"error":"token validation failed: %s"}`, lastErr), http.StatusUnauthorized)
				return
			}

			// Check email authorization.
			email, _ := claims["email"].(string)
			if cfg.Firestore != nil || len(cfg.AllowedEmails) > 0 {
				authorized := false

				// Check Firestore first (dynamic allowlist).
				if cfg.Firestore != nil {
					authorized = cfg.Firestore.IsAuthorized(email)
				}

				// Fall back to static allowlist.
				if !authorized && len(cfg.AllowedEmails) > 0 {
					authorized = containsString(cfg.AllowedEmails, email)
				}

				if !authorized {
					http.Error(w, `{"error":"email not authorized"}`, http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
			return
		}

		// Static token fallback.
		if cfg.StaticToken != "" {
			if token != cfg.StaticToken {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// No auth configured — pass through.
		next.ServeHTTP(w, r)
	})
}

// --------------------------------------------------------------------------
// JWT validation (zero dependencies — stdlib only)
// --------------------------------------------------------------------------

// jwtHeader is the decoded JOSE header.
type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

// validateJWT decodes and validates a JWT against the issuer's JWKS.
func validateJWT(tokenStr string, cache *jwksCache, issuer, audience string) (map[string]any, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed JWT")
	}

	// Decode header.
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}

	// Decode payload (claims).
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	// Verify expiration.
	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token expired")
		}
	} else {
		return nil, fmt.Errorf("missing exp claim")
	}

	// Verify issuer.
	if iss, _ := claims["iss"].(string); iss != issuer {
		return nil, fmt.Errorf("issuer mismatch: got %q, want %q", iss, issuer)
	}

	// Verify audience.
	if audience != "" {
		if !audienceMatches(claims, audience) {
			return nil, fmt.Errorf("audience mismatch")
		}
	}

	// Verify signature.
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	key, err := cache.GetKey(header.Kid)
	if err != nil {
		return nil, fmt.Errorf("get signing key: %w", err)
	}

	signedContent := parts[0] + "." + parts[1]
	if err := verifySignature(header.Alg, key, []byte(signedContent), sigBytes); err != nil {
		return nil, fmt.Errorf("signature: %w", err)
	}

	return claims, nil
}

// audienceMatches checks the "aud" claim which can be a string or []string.
func audienceMatches(claims map[string]any, expected string) bool {
	switch aud := claims["aud"].(type) {
	case string:
		return aud == expected
	case []any:
		for _, a := range aud {
			if s, ok := a.(string); ok && s == expected {
				return true
			}
		}
	}
	return false
}

// verifySignature verifies a JWT signature using the public key.
func verifySignature(alg string, key crypto.PublicKey, signed, sig []byte) error {
	hash := sha256.Sum256(signed)

	switch alg {
	case "RS256":
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("key is not RSA")
		}
		return rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, hash[:], sig)

	case "ES256":
		ecKey, ok := key.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("key is not ECDSA")
		}
		// ES256 signature is r || s, each 32 bytes.
		if len(sig) != 64 {
			return fmt.Errorf("invalid ES256 signature length: %d", len(sig))
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(ecKey, hash[:], r, s) {
			return fmt.Errorf("ECDSA verification failed")
		}
		return nil

	default:
		return fmt.Errorf("unsupported algorithm: %s", alg)
	}
}

// --------------------------------------------------------------------------
// JWKS cache — auto-discovers and caches the issuer's signing keys
// --------------------------------------------------------------------------

// jwksCache fetches and caches JWKS from the OIDC issuer.
type jwksCache struct {
	issuer string
	mu     sync.RWMutex
	keys   map[string]crypto.PublicKey
	expiry time.Time
}

func newJWKSCache(issuer string) *jwksCache {
	return &jwksCache{
		issuer: strings.TrimRight(issuer, "/"),
		keys:   make(map[string]crypto.PublicKey),
	}
}

// GetKey returns the public key for the given key ID, fetching/refreshing
// the JWKS if needed.
func (c *jwksCache) GetKey(kid string) (crypto.PublicKey, error) {
	c.mu.RLock()
	if time.Now().Before(c.expiry) {
		if key, ok := c.keys[kid]; ok {
			c.mu.RUnlock()
			return key, nil
		}
	}
	c.mu.RUnlock()

	// Refresh the JWKS.
	if err := c.refresh(); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	key, ok := c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("key %q not found in JWKS", kid)
	}
	return key, nil
}

// refresh fetches the JWKS from the issuer's discovery endpoint.
func (c *jwksCache) refresh() error {
	// Discover the JWKS URI from OpenID configuration.
	configURL := c.issuer + "/.well-known/openid-configuration"
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(configURL)
	if err != nil {
		return fmt.Errorf("fetch OIDC config: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var config struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return fmt.Errorf("parse OIDC config: %w", err)
	}
	if config.JWKSURI == "" {
		return fmt.Errorf("no jwks_uri in OIDC config")
	}

	// Fetch the JWKS.
	resp2, err := client.Get(config.JWKSURI)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(io.LimitReader(resp2.Body, 1<<20))
	var jwks struct {
		Keys []jwkKey `json:"keys"`
	}
	if err := json.Unmarshal(body2, &jwks); err != nil {
		return fmt.Errorf("parse JWKS: %w", err)
	}

	// Parse keys.
	keys := make(map[string]crypto.PublicKey)
	for _, k := range jwks.Keys {
		pub, err := k.toPublicKey()
		if err != nil {
			continue // Skip keys we can't parse.
		}
		keys[k.Kid] = pub
	}

	c.mu.Lock()
	c.keys = keys
	c.expiry = time.Now().Add(1 * time.Hour) // Cache for 1 hour.
	c.mu.Unlock()

	return nil
}

// jwkKey is a single JWK entry.
type jwkKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`   // RSA modulus
	E   string `json:"e"`   // RSA exponent
	X   string `json:"x"`   // EC x coordinate
	Y   string `json:"y"`   // EC y coordinate
	Crv string `json:"crv"` // EC curve
}

// toPublicKey converts a JWK to a Go crypto.PublicKey.
func (k *jwkKey) toPublicKey() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, fmt.Errorf("decode RSA n: %w", err)
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, fmt.Errorf("decode RSA e: %w", err)
		}
		n := new(big.Int).SetBytes(nBytes)
		e := new(big.Int).SetBytes(eBytes)
		return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil

	case "EC":
		xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, fmt.Errorf("decode EC x: %w", err)
		}
		yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, fmt.Errorf("decode EC y: %w", err)
		}
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		default:
			return nil, fmt.Errorf("unsupported curve: %s", k.Crv)
		}
		return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(xBytes), Y: new(big.Int).SetBytes(yBytes)}, nil

	default:
		return nil, fmt.Errorf("unsupported key type: %s", k.Kty)
	}
}

// containsString checks if a slice contains a string.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
