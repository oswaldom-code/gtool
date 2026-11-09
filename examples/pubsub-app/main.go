// Command pubsub-app is a minimal sample application under test for gtool. On
// startup it reads a JSON array of messages from a file and publishes each one
// to a Pub/Sub topic, then serves a health check so a component test can
// validate what was published against the gtool Pub/Sub emulator mock.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/pubsub/v2"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// publishFile publishes every element of the JSON array in path as one message
// and waits for the broker to acknowledge each of them.
func publishFile(ctx context.Context, publisher *pubsub.Publisher, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return err
	}

	for _, m := range messages {
		if _, err := publisher.Publish(ctx, &pubsub.Message{Data: m}).Get(ctx); err != nil {
			return err
		}
	}
	log.Printf("published %d message(s) from %s", len(messages), path)
	return nil
}

func main() {
	ctx := context.Background()

	// The client targets the emulator automatically when PUBSUB_EMULATOR_HOST is set.
	client, err := pubsub.NewClient(ctx, env("PUBSUB_PROJECT_ID", "test-project"))
	if err != nil {
		log.Fatalf("pubsub: %v", err)
	}
	defer client.Close()

	publisher := client.Publisher(env("TOPIC", "orders"))
	defer publisher.Stop()

	if err := publishFile(ctx, publisher, env("DATA_FILE", "data/orders.json")); err != nil {
		log.Fatalf("publish: %v", err)
	}

	http.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
