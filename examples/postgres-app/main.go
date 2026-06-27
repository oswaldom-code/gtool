// Command postgres-app is a minimal HTTP service backed by PostgreSQL, used as
// a sample application under test for gtool. It exposes a health check and a
// users resource so a component test can exercise the app against the gtool
// PostgreSQL mock.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
)

type user struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type server struct {
	db *sql.DB
}

func main() {
	db, err := connect()
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	srv := &server{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", srv.health)
	mux.HandleFunc("GET /users", srv.listUsers)
	mux.HandleFunc("POST /users", srv.createUser)

	addr := ":" + env("PORT", "8080")
	httpSrv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		log.Printf("listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	log.Println("shut down")
}

// connect opens the database and waits for it to accept connections, since the
// mock may still be starting up.
func connect() (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		env("DB_HOST", "localhost"),
		env("DB_PORT", "5432"),
		env("DB_USER", "postgres"),
		env("DB_PASSWORD", "postgres"),
		env("DB_NAME", "postgres"),
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	const maxAttempts = 30
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = db.Ping(); err == nil {
			log.Printf("connected to database after %d attempt(s)", attempt)
			return db, nil
		}
		log.Printf("waiting for database (attempt %d/%d): %v", attempt, maxAttempts, err)
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("database not reachable: %w", err)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT id, name FROM users ORDER BY id")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	users := []user{}
	for rows.Next() {
		var u user
		if err := rows.Scan(&u.ID, &u.Name); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *server) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}

	var id int
	err := s.db.QueryRowContext(r.Context(),
		"INSERT INTO users (name) VALUES ($1) RETURNING id", in.Name).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, user{ID: id, Name: in.Name})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
