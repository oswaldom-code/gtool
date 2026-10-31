// Command gcs-app is a minimal sample application under test for gtool. On
// startup it uploads a local file to a Google Cloud Storage bucket, then serves
// a health check so a component test can validate the stored object against the
// gtool GCS emulator mock.
package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/storage"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// uploadFile copies the local file at path into bucket/object.
func uploadFile(ctx context.Context, client *storage.Client, path, bucket, object string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := client.Bucket(bucket).Object(object).NewWriter(ctx)
	w.ContentType = "application/json"
	if _, err := io.Copy(w, f); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	log.Printf("uploaded %s to gs://%s/%s", path, bucket, object)
	return nil
}

func main() {
	ctx := context.Background()

	// The client targets the emulator automatically when STORAGE_EMULATOR_HOST is set.
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer client.Close()

	err = uploadFile(ctx, client, env("DATA_FILE", "data/report.json"),
		env("BUCKET", "reports"), env("OBJECT", "orders/report.json"))
	if err != nil {
		log.Fatalf("upload: %v", err)
	}

	http.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
