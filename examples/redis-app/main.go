// Command redis-app is a minimal sample application under test for gtool. On
// startup it reads a JSON array of products from a file and stores each one in
// Redis under product:<id>, then serves a health check so a component test can
// validate the stored values against the gtool Redis mock.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/redis/go-redis/v9"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// storeFile stores every element of the JSON array in path as product:<id>,
// keeping the element's JSON as the value.
func storeFile(ctx context.Context, rdb *redis.Client, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var products []json.RawMessage
	if err := json.Unmarshal(raw, &products); err != nil {
		return err
	}

	for _, p := range products {
		var key struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(p, &key); err != nil {
			return err
		}
		if err := rdb.Set(ctx, "product:"+key.ID, string(p), 0).Err(); err != nil {
			return err
		}
	}
	log.Printf("stored %d product(s) from %s", len(products), path)
	return nil
}

func main() {
	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	defer rdb.Close()

	if err := storeFile(ctx, rdb, env("DATA_FILE", "data/products.json")); err != nil {
		log.Fatalf("store: %v", err)
	}

	http.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
