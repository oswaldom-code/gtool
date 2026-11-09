// Command s3-app is a minimal sample application under test for gtool. On
// startup it uploads a local file to an S3 bucket, then serves a health check
// so a component test can validate the stored object against the gtool Floci
// (AWS) mock.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// uploadFile copies the local file at path into bucket/key.
func uploadFile(ctx context.Context, client *s3.Client, path, bucket, key string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        f,
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return err
	}
	log.Printf("uploaded %s to s3://%s/%s", path, bucket, key)
	return nil
}

func main() {
	ctx := context.Background()

	// Region and credentials come from AWS_REGION, AWS_ACCESS_KEY_ID and
	// AWS_SECRET_ACCESS_KEY; S3_ENDPOINT points the client at the emulator.
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(env("S3_ENDPOINT", "http://localhost:4566"))
		o.UsePathStyle = true
	})

	err = uploadFile(ctx, client, env("DATA_FILE", "data/report.json"),
		env("BUCKET", "reports"), env("OBJECT_KEY", "orders/report.json"))
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
