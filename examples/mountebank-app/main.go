// Command mountebank-app is a minimal sample application under test for gtool.
// It exposes a greeting endpoint backed by an external HTTP API, so a component
// test can replace that API with a gtool Mountebank imposter.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// greeting fetches the message from the external API and returns it wrapped in
// the app's own response shape.
func greeting(client *http.Client, apiURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, apiURL+"/greeting", nil)
		resp, err := client.Do(req)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		defer resp.Body.Close()

		var upstream struct {
			Message string `json:"message"`
		}
		if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&upstream) != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "unexpected greeting API response"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"greeting": upstream.Message})
	}
}

func main() {
	client := &http.Client{Timeout: 5 * time.Second}

	http.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("GET /greeting", greeting(client, env("GREETING_API_URL", "http://localhost:4545")))

	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
