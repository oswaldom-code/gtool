// Command mongodb-app is a minimal sample application under test for gtool. On
// startup it reads a JSON array of customers from a file and inserts them into
// a MongoDB collection, then serves a health check so a component test can
// validate the stored documents against the gtool MongoDB mock.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// insertFile inserts every element of the JSON array in path as one document.
func insertFile(ctx context.Context, coll *mongo.Collection, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var docs []bson.D
	if err := bson.UnmarshalExtJSON(raw, false, &docs); err != nil {
		return err
	}

	if _, err := coll.InsertMany(ctx, docs); err != nil {
		return err
	}
	log.Printf("inserted %d document(s) from %s", len(docs), path)
	return nil
}

func main() {
	ctx := context.Background()

	client, err := mongo.Connect(options.Client().ApplyURI(env("MONGO_URI", "mongodb://localhost:27017")))
	if err != nil {
		log.Fatalf("mongo: %v", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	coll := client.Database(env("MONGO_DATABASE", "test")).Collection(env("COLLECTION", "customers"))
	if err := insertFile(ctx, coll, env("DATA_FILE", "data/customers.json")); err != nil {
		log.Fatalf("insert: %v", err)
	}

	http.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
