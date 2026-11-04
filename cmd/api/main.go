package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"example.com/server-monitoring/internal/model"
	"example.com/server-monitoring/internal/queue"
	"example.com/server-monitoring/internal/storage"
)

//go:embed index.html
var page embed.FS

func env(k, v string) string {
	if x := os.Getenv(k); x != "" {
		return x
	}
	return v
}

func fail(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://127.0.0.1:8080/health/ready")
		if err != nil {
			os.Exit(1)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	ctx := context.Background()
	var store *storage.Store
	var err error
	for i := 0; i < 90; i++ {
		store, err = storage.Open(ctx, env("DATABASE_URL", "postgres://monitor:monitor@localhost:5432/monitor?sslmode=disable"))
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
	if err = store.InstallRetention(ctx); err != nil {
		log.Fatal(err)
	}
	nc, js, err := queue.Connect(env("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		b, _ := page.ReadFile("index.html")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		c, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := store.DB.Ping(c); err != nil {
			fail(w, 503, "db_unavailable", err.Error())
			return
		}
		if !nc.IsConnected() {
			fail(w, 503, "queue_unavailable", "queue disconnected")
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /api/v1/metrics/batch", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			fail(w, 413, "body_too_large", "request exceeds 512KiB")
			return
		}
		var b model.Batch
		if err = json.Unmarshal(raw, &b); err != nil {
			fail(w, 400, "invalid_json", err.Error())
			return
		}
		if err = b.Validate(time.Now().UTC()); err != nil {
			fail(w, 400, "invalid_sample", err.Error())
			return
		}
		c, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err = queue.Publish(c, js, raw); err != nil {
			fail(w, 503, "queue_unavailable", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": len(b.Samples)})
	})
	mux.HandleFunc("GET /api/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		limit := 25
		if s := r.URL.Query().Get("limit"); s != "" {
			n, e := strconv.Atoi(s)
			if e != nil || n < 1 || n > 100 {
				fail(w, 400, "invalid_limit", "limit must be 1..100")
				return
			}
			limit = n
		}
		search := strings.TrimSpace(r.URL.Query().Get("search"))
		if len(search) > 128 {
			fail(w, 400, "invalid_search", "search too long")
			return
		}
		rows, err := store.Devices(r.Context(), search, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			fail(w, 500, "db_error", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"devices": rows})
	})
	mux.HandleFunc("GET /api/v1/metrics/query", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query()
		id := p.Get("device_id")
		if id == "" || len(id) > 128 {
			fail(w, 400, "invalid_device", "device_id required")
			return
		}
		names, err := model.ParseSensorSet(p.Get("metrics"))
		if err != nil {
			fail(w, 400, "invalid_metrics", err.Error())
			return
		}
		from, err1 := time.Parse(time.RFC3339Nano, p.Get("from"))
		to, err2 := time.Parse(time.RFC3339Nano, p.Get("to"))
		if err1 != nil || err2 != nil || !from.Before(to) || to.Sub(from) > 365*24*time.Hour || to.After(time.Now().Add(2*time.Minute)) {
			fail(w, 400, "invalid_range", "from/to must be RFC3339, ordered, within 365 days, and not future")
			return
		}
		mp := 1500
		if p.Get("max_points") != "" {
			mp, err = strconv.Atoi(p.Get("max_points"))
			if err != nil || mp < 100 || mp > 1500 {
				fail(w, 400, "invalid_max_points", "max_points must be 100..1500")
				return
			}
		}
		data, err := store.Query(r.Context(), id, names, from.UTC(), to.UTC(), mp)
		if err != nil {
			fail(w, 500, "query_failed", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(data)
	})
	server := &http.Server{Addr: env("HTTP_ADDR", ":8080"), Handler: requestLog(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second}
	log.Printf("api listening %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Print(fmt.Sprintf("%s %s latency=%s", r.Method, r.URL.Path, time.Since(start)))
		}
	})
}
