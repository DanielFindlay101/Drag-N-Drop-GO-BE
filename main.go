package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Struct to give handlers access to shared things
type Server struct {
	db        *sql.DB
	jwtSecret []byte
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	// Load and check the secret
	secret := os.Getenv("JWT_SECRET")

	if len(secret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters")
	}

	// Connection url to docker
	db, err := sql.Open("mysql", "user:password@tcp(127.0.0.1:3306)/go-todo-app?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	srv := &Server{db: db, jwtSecret: []byte(secret)}

	// Remove expired refresh tokens once an hour so the table doesn't grow forever.
	go func() {
		for range time.Tick(time.Hour) {
			if _, err := db.Exec("DELETE FROM refresh_tokens WHERE expiresAt < ?", time.Now()); err != nil {
				log.Println("cleanup refresh tokens:", err)
			}
		}
	}()

	mux := http.NewServeMux()

	// public: these authenticate with the refresh cookie or a password
	mux.HandleFunc("POST /auth/register", srv.register)
	mux.HandleFunc("POST /auth/login", srv.login)
	mux.HandleFunc("POST /auth/refresh", srv.refresh)
	mux.HandleFunc("POST /auth/logout", srv.logout)

	// protected: need a valid access JWT
	mux.HandleFunc("GET /todos", srv.requireAuth(srv.fetchAllTasks))
	mux.HandleFunc("POST /todos", srv.requireAuth(srv.createTask))
	mux.HandleFunc("PATCH /todos/{id}", srv.requireAuth(srv.updateTask))
	mux.HandleFunc("GET /tags", srv.requireAuth(srv.fetchAllTags))

	addr := ":" + envOr("PORT", "8080")
	origin := envOr("FRONTEND_ORIGIN", "http://localhost:5173")

	log.Printf("listening on %s (CORS origin %s)", addr, origin)
	log.Fatal(http.ListenAndServe(addr, withCORS(origin, mux)))
}
