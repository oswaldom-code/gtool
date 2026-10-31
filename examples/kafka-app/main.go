// Command kafka-app is a minimal sample application under test for gtool. On
// startup it reads a JSON array of messages from a file and publishes each one
// to a Kafka topic, then serves a health check so a component test can validate
// what was published against the gtool Kafka mock.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/segmentio/kafka-go"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// publishFile publishes every element of the JSON array in path as one message
// and waits for the broker to acknowledge them.
func publishFile(ctx context.Context, writer *kafka.Writer, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return err
	}

	batch := make([]kafka.Message, 0, len(messages))
	for _, m := range messages {
		batch = append(batch, kafka.Message{Value: m})
	}
	if err := writer.WriteMessages(ctx, batch...); err != nil {
		return err
	}
	log.Printf("published %d message(s) from %s", len(messages), path)
	return nil
}

func main() {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(env("KAFKA_BROKERS", "localhost:9092")),
		Topic:        env("TOPIC", "orders"),
		RequiredAcks: kafka.RequireAll,
	}
	defer writer.Close()

	if err := publishFile(context.Background(), writer, env("DATA_FILE", "data/orders.json")); err != nil {
		log.Fatalf("publish: %v", err)
	}

	http.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
