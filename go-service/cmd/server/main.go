package main

import (
	"bufio"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const serviceVersion = "1.0.0"

type component struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type healthResponse struct {
	Status   string               `json:"status"`
	Services map[string]component `json:"services"`
}

func main() {
	port := valueOrDefault("BRAIN_PORT", "47180")
	pythonURL := valueOrDefault("PYTHON_SERVICE_URL", "http://localhost:47181")
	postgresAddress := net.JoinHostPort(valueOrDefault("POSTGRES_HOST", "localhost"), valueOrDefault("POSTGRES_PORT", "47182"))
	databaseURL := os.Getenv("DATABASE_URL")
	allowedOrigins := strings.Split(valueOrDefault("BRAIN_ALLOWED_ORIGINS", "http://127.0.0.1:4201"), ",")

	client := &http.Client{Timeout: 2 * time.Second}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("opening database connection: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		health := getHealth(r.Context(), client, pythonURL, postgresAddress, db)
		w.Header().Set("Content-Type", "application/json")
		if health.Status != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(health)
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"service": "brain-go", "version": serviceVersion})
	})
	mux.HandleFunc("GET /health/stream", func(w http.ResponseWriter, r *http.Request) {
		serveHealthStream(w, r, client, pythonURL, postgresAddress, db, allowedOrigins)
	})

	server := &http.Server{Addr: ":" + port, Handler: allowCORS(mux, allowedOrigins), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("Brain Go API listening on :%s", port)
	log.Fatal(server.ListenAndServe())
}

func getHealth(ctx context.Context, client *http.Client, pythonURL, postgresAddress string, db *sql.DB) healthResponse {
	services := map[string]component{"go": {Status: "ok", Version: serviceVersion}}
	pythonStatus := checkPython(ctx, client, pythonURL)
	databaseStatus := checkDatabase(ctx, postgresAddress, db)
	services["python"] = pythonStatus
	services["database"] = databaseStatus

	status := "ok"
	if pythonStatus.Status != "ok" || databaseStatus.Status != "ok" {
		status = "degraded"
	}

	return healthResponse{Status: status, Services: services}
}

func checkPython(parent context.Context, client *http.Client, url string) component {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/version", nil)
	if err != nil {
		return component{Status: "error", Error: err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return component{Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return component{Status: "error", Error: "unexpected status " + resp.Status}
	}
	var version struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil || version.Version == "" {
		return component{Status: "error", Error: "invalid version response"}
	}
	return component{Status: "ok", Version: version.Version}
}

func checkDatabase(parent context.Context, address string, db *sql.DB) component {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return component{Status: "error", Error: err.Error()}
	}
	connection.Close()

	var version string
	if err := db.QueryRowContext(ctx, "SELECT version FROM flyway_schema_history WHERE success = true ORDER BY installed_rank DESC LIMIT 1").Scan(&version); err != nil {
		return component{Status: "error", Error: err.Error()}
	}
	return component{Status: "ok", Version: version}
}

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func allowCORS(next http.Handler, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isAllowedOrigin(origin, allowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", http.MethodGet)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isAllowedOrigin(origin string, allowedOrigins []string) bool {
	for _, allowedOrigin := range allowedOrigins {
		allowedOrigin = strings.TrimSpace(allowedOrigin)
		if origin == allowedOrigin {
			return true
		}

		if !strings.HasPrefix(allowedOrigin, "https://*.") || !strings.HasPrefix(origin, "https://") {
			continue
		}

		domain := strings.TrimPrefix(allowedOrigin, "https://*.")
		host := strings.TrimPrefix(origin, "https://")
		if strings.HasSuffix(host, "."+domain) && strings.Count(host, ".") > strings.Count(domain, ".") {
			return true
		}
	}

	return false
}

func serveHealthStream(
	w http.ResponseWriter,
	r *http.Request,
	client *http.Client,
	pythonURL, postgresAddress string,
	db *sql.DB,
	allowedOrigins []string,
) {
	if origin := r.Header.Get("Origin"); origin != "" && !isAllowedOrigin(origin, allowedOrigins) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}

	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || r.Header.Get("Sec-WebSocket-Key") == "" {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket unsupported", http.StatusInternalServerError)
		return
	}

	connection, buffer, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer connection.Close()

	accept := websocketAccept(r.Header.Get("Sec-WebSocket-Key"))
	fmt.Fprintf(buffer, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err := buffer.Flush(); err != nil {
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		payload, err := json.Marshal(getHealth(r.Context(), client, pythonURL, postgresAddress, db))
		if err != nil || writeWebSocketText(connection, buffer, payload) != nil {
			return
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func websocketAccept(key string) string {
	hash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func writeWebSocketText(connection net.Conn, buffer *bufio.ReadWriter, payload []byte) error {
	connection.SetWriteDeadline(time.Now().Add(3 * time.Second))
	defer connection.SetWriteDeadline(time.Time{})

	if len(payload) > 65535 {
		return fmt.Errorf("health payload too large")
	}

	frameHeader := []byte{0x81, byte(len(payload))}
	if len(payload) > 125 {
		frameHeader = []byte{0x81, 126, byte(len(payload) >> 8), byte(len(payload))}
	}
	if _, err := buffer.Write(frameHeader); err != nil {
		return err
	}
	if _, err := buffer.Write(payload); err != nil {
		return err
	}
	return buffer.Flush()
}
