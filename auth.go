package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type credentials struct {
	Email 		string 	`json:"email"`
	Password 	string 	`json:"password"`
}


 
type userResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type ctxKey string
const userIDKey ctxKey = "userID"

const invalidCredentials = "invalid email or password"

func userIDFromContext(r *http.Request) (string, bool) {
	id, ok := r.Context().Value(userIDKey).(string)
	return id, ok && id != ""
}



func (s *Server) register(w http.ResponseWriter, r *http.Request){
	
	// New empty credentials struct
	var creds credentials
	// Decode the json to fill in the struct
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return 
	}
	// Normalise the email
	email := strings.ToLower(strings.TrimSpace(creds.Email))

	// Validate
	if email == "" || !strings.Contains(email, "@") {
		http.Error(w, "a valid email is required", http.StatusBadRequest)
		return
	}

	if len(creds.Password) < 8 || len(creds.Password) > 72 {
		http.Error(w, "password must be between 8 and 72 characters", http.StatusBadRequest)
		return 
	}

	// Hash the password
	hash, err := bcrypt.GenerateFromPassword([]byte(creds.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Println("register hash:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Insert the user
	id := uuid.NewString()
	_, err = s.db.Exec(
		"INSERT INTO users (id, email, passwordHash) VALUES (?, ?, ?)",
		id, email, hash,
	)

	// Handle insert errors
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			http.Error(w, "email already registered", http.StatusConflict)
			return
		}
		log.Println("register: insert:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := s.issueSession(w, id); err != nil {
		log.Println("register: issue session:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
 
	writeJSON(w, http.StatusCreated, map[string]userResponse{
		"user": {ID: id, Email: email},
	})

	// // Respond
	// w.Header().Set("Content-Type", "application/json")
	// w.WriteHeader(http.StatusCreated)
	// if err := json.NewEncoder(w).Encode(map[string]string {
	// 	"id": id,
	// 	"email": email,
	// }); err != nil {
	// 	log.Println("register: encode response:", err)
	// }
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {

	var creds credentials

	//Decode and normalise the email
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
	  http.Error(w, "invalid JSON", http.StatusBadRequest)
	  return
	}

	email := strings.ToLower(strings.TrimSpace(creds.Email))

	//Look up the user
	var userID string
	var hash []byte
	err := s.db.QueryRow("SELECT id, passwordHash FROM users WHERE email = ?",
	 email).Scan(&userID, &hash)

	// No user has that email
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, invalidCredentials, http.StatusUnauthorized)
		return
	}

	if err != nil {
		http.Error(w, "internal server error occurred", http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword(hash, []byte(creds.Password)); err != nil {
		http.Error(w, invalidCredentials, http.StatusUnauthorized)
		return
	}

	//Store the JWT in cookies
	if err := s.issueSession(w, userID); err != nil {
		log.Println("login: issue session:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]userResponse{
		"user": {ID: userID, Email: email},
	})

}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(authCookie)

     	if err != nil || cookie.Value == "" {
          http.Error(w, "unauthorized", http.StatusUnauthorized)
          return
      }
		tokenString := cookie.Value

		claims := &jwt.RegisteredClaims{}
		_, err = jwt.ParseWithClaims(
			tokenString,
			claims,
			func(t *jwt.Token) (any,error) { return s.jwtSecret, nil},
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithExpirationRequired(),
		)
		if err != nil || claims.Subject == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, claims.Subject)
		next(w, r.WithContext(ctx))
	}
}


func (s *Server) refresh( w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookie)
	if err != nil || cookie.Value == ""{
		clearSession(w)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	hash := hashToken(cookie.Value)
		// A transaction plus FOR UPDATE locks this row, so two refreshes
	// with the same token run one after the other, not interleaved.
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		log.Println("refresh: begin:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback() // does nothing once Commit has succeeded
 
	var (
		userID    string
		expiresAt time.Time
		revokedAt sql.NullTime
	)
	err = tx.QueryRow(
		`SELECT userId, expiresAt, revokedAt FROM refresh_tokens
		 WHERE tokenHash = ? FOR UPDATE`, hash,
	).Scan(&userID, &expiresAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		clearSession(w)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err != nil {
		log.Println("refresh: select:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
 
	now := time.Now()
 
	if revokedAt.Valid {
		if now.Sub(revokedAt.Time) < reuseGrace {
			// Rotated moments ago by another tab or request. The browser
			// already has the new cookies, so the client can just retry.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// An old token came back: assume it was stolen.
		if _, err := tx.Exec("DELETE FROM refresh_tokens WHERE userId = ?", userID); err != nil {
			log.Println("refresh: revoke all:", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			log.Println("refresh: commit:", err)
		}
		log.Printf("refresh: reused token for user %s, all sessions revoked", userID)
		clearSession(w)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
 
	if now.After(expiresAt) {
		clearSession(w)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
 
	// Rotate: mark this token used, then issue a fresh pair.
	if _, err := tx.Exec("UPDATE refresh_tokens SET revokedAt = ? WHERE tokenHash = ?", now, hash); err != nil {
		log.Println("refresh: revoke:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Println("refresh: commit:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
 
	if err := s.issueSession(w, userID); err != nil {
		log.Println("refresh: issue session:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}


func (s *Server) logout(w http.ResponseWriter, r *http.Request){
	if cookie, err := r.Cookie(refreshCookie); err == nil && cookie.Value != "" {
		if _, err := s.db.Exec(
			"DELETE FROM refresh_tokens WHERE tokenHash = ?", hashToken(cookie.Value),
		); err != nil {
			log.Println("logout: delete", err)
		}
	}
	clearSession(w)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}