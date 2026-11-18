package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	authCookie = "auth_token"
	refreshCookie = "refresh_token"

	accessTTL = 15 * time.Minute
	refreshTTL = 7 * 24 * time.Hour
	reuseGrace = 10 * time.Second
)

//Creates a 32 random byte string
func newRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func setCookie(w http.ResponseWriter, name, value, path string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
		// Secure: true, // turn on once you serve over HTTPS
	})
}

func (s *Server) issueSession(w http.ResponseWriter, userID string) error {

//Creates the JWT and stores in the cookies under "auth_token"

now := time.Now()

	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		return err
	}

	//Create new token and add into into the refresh_tokens table
	refreshToken, err := newRefreshToken()
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		"INSERT into refresh_tokens (tokenHash, userId, expiresAt) VALUES (?, ?, ?)",
		hashToken(refreshToken), userID, now.Add(refreshTTL),
	)
	if err != nil {
		return err
	}

	setCookie(w, authCookie, signed, "/", int(accessTTL.Seconds()))
	setCookie(w, refreshCookie, refreshToken, "/auth", int(refreshTTL.Seconds()))
	return nil
}

func clearSession(w http.ResponseWriter) {
	setCookie(w, authCookie, "", "/", -1)
	setCookie(w, refreshCookie, "", "/auth", -1)
}