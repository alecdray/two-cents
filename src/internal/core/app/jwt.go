package app

import (
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwtCookieName = "twocents_token"
	// jwtTTL is a multi-week sliding window: the cookie is re-issued on every
	// authenticated request, so active use never expires while a long idle gap
	// does (ADR-0007). The session carries no bank secret, so a generous lifetime
	// trades convenience against a small blast radius.
	jwtTTL = 30 * 24 * time.Hour
)

// Claims is the signed session payload. UserID is the single local user's
// stored id (ADR-0007); nothing is partitioned by it, but carrying the real id
// keeps the request-context plumbing honest.
type Claims struct {
	jwt.RegisteredClaims
	UserID *string `json:"user_id"`
}

// NewClaims builds a fresh claim set for the given user, valid for jwtTTL.
func NewClaims(userID string) *Claims {
	now := time.Now()
	return &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(jwtTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "two-cents",
		},
		UserID: &userID,
	}
}

// jwt signs the claims with the HS256 secret.
func (c *Claims) jwt(secret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return token.SignedString([]byte(secret))
}

// save signs the claims (refreshing the expiry to slide the window) and writes
// the session cookie. Secure is set only in prod so the cookie works over plain
// HTTP in local dev.
func (c *Claims) save(cfg Config, w http.ResponseWriter) error {
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(jwtTTL))
	token, err := c.jwt(cfg.JwtSecret)
	if err != nil {
		return fmt.Errorf("failed to sign session token: %w", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     jwtCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(jwtTTL.Seconds()),
		HttpOnly: true,
		Secure:   cfg.Env == EnvProd,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// deleteCookie clears the session cookie (logout, or an invalid token).
func deleteCookie(cfg Config, w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     jwtCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.Env == EnvProd,
		SameSite: http.SameSiteLaxMode,
	})
}

// validateClaims parses and verifies a signed token, rejecting any unexpected
// signing method.
func validateClaims(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse session token: %w", err)
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid session token claims")
}

// validateClaimsFromRequest reads and validates the session token from the
// request cookie.
func validateClaimsFromRequest(r *http.Request, secret string) (*Claims, error) {
	cookie, err := r.Cookie(jwtCookieName)
	if err != nil {
		return nil, err
	}
	return validateClaims(cookie.Value, secret)
}
