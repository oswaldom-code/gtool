// Command mysql-app is a minimal sample application under test for gtool. On
// startup it reads a JSON array of books from a file and inserts them into a
// MySQL table, then serves a health check so a component test can validate the
// stored rows against the gtool MySQL mock.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type book struct {
	ISBN   string `json:"isbn"`
	Title  string `json:"title"`
	Copies int    `json:"copies"`
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func connect() (*sql.DB, error) {
	db, err := sql.Open("mysql", env("MYSQL_DSN", "root:root@tcp(localhost:3306)/test"))
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

// insertFile creates the books table and inserts every element of the JSON
// array in path as one row.
func insertFile(db *sql.DB, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var books []book
	if err := json.Unmarshal(raw, &books); err != nil {
		return err
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS books (
		isbn   VARCHAR(20) PRIMARY KEY,
		title  VARCHAR(255) NOT NULL,
		copies INT NOT NULL
	)`); err != nil {
		return err
	}
	for _, b := range books {
		if _, err := db.Exec("INSERT INTO books (isbn, title, copies) VALUES (?, ?, ?)", b.ISBN, b.Title, b.Copies); err != nil {
			return err
		}
	}
	log.Printf("inserted %d row(s) from %s", len(books), path)
	return nil
}

func main() {
	db, err := connect()
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer db.Close()

	if err := insertFile(db, env("DATA_FILE", "data/books.json")); err != nil {
		log.Fatalf("insert: %v", err)
	}

	http.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
