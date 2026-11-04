package main

import (
	"bytes"
	"encoding/json"
	"example.com/server-monitoring/internal/model"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type answer struct {
	Points []struct {
		Metrics map[string]*struct {
			Avg   float64 `json:"avg"`
			Min   float64 `json:"min"`
			Max   float64 `json:"max"`
			Count int64   `json:"count"`
		} `json:"metrics"`
	} `json:"points"`
}

func must(b bool, msg string) {
	if !b {
		log.Fatal("FAIL: " + msg)
	}
}
func main() {
	base := os.Getenv("API_URL")
	if base == "" {
		base = "http://localhost:8080"
	}
	c := &http.Client{Timeout: 6 * time.Second}
	ready := false
	for i := 0; i < 90; i++ {
		r, e := c.Get(base + "/health/ready")
		if e == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(time.Second)
	}
	must(ready, "readiness")
	r, e := c.Get(base + "/")
	must(e == nil, "HTML request")
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	must(r.StatusCode == 200 && bytes.Contains(b, []byte("<canvas")), "HTML dashboard")
	id := fmt.Sprintf("smoke-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Second).Add(-3 * time.Minute)
	ptr := func(x float64) *float64 { return &x }
	one := model.Sample{DeviceID: id, Time: now, CPU: ptr(20), Memory: ptr(100), DiskSpace: ptr(0), DiskBusy: ptr(40), NetIn: ptr(50), NetOut: ptr(60), Fan: ptr(70)}
	two := one
	two.Time = now.Add(time.Minute)
	two.CPU = ptr(60)
	post := func(batch model.Batch) int {
		b, _ := json.Marshal(batch)
		resp, e := c.Post(base+"/api/v1/metrics/batch", "application/json", bytes.NewReader(b))
		must(e == nil, "HTTP ingest")
		defer resp.Body.Close()
		return resp.StatusCode
	}
	batch := model.Batch{Samples: []model.Sample{one, two}}
	must(post(batch) == 202 && post(batch) == 202, "publish and duplicate accepted")
	bad := one
	bad.CPU = ptr(101)
	must(post(model.Batch{Samples: []model.Sample{bad}}) == 400, "invalid metrics rejected")
	query := fmt.Sprintf("%s/api/v1/metrics/query?device_id=%s&metrics=cpu,memory&from=%s&to=%s&max_points=100", base, id, now.Add(-time.Minute).Format(time.RFC3339), now.Add(2*time.Minute).Format(time.RFC3339))
	passed := false
	for i := 0; i < 90; i++ {
		r, e = c.Get(query)
		if e == nil {
			var a answer
			e = json.NewDecoder(r.Body).Decode(&a)
			r.Body.Close()
			if e == nil {
				values := map[float64]bool{}
				for _, p := range a.Points {
					if v := p.Metrics["cpu"]; v != nil && v.Count == 1 {
						values[v.Avg] = true
					}
				}
				if values[20] && values[60] && len(values) == 2 {
					passed = true
					break
				}
			}
		}
		time.Sleep(time.Second)
	}
	must(passed, "storage, aggregates, idempotency, query")
	r, e = c.Get(base + "/api/v1/devices?search=" + id)
	must(e == nil, "search")
	b, _ = io.ReadAll(r.Body)
	r.Body.Close()
	must(strings.Contains(string(b), id), "device discovered")

	// Three distinct raw samples in a completed minute must yield a weighted rollup.
	minuteTime := now.Add(-24 * time.Hour).Truncate(time.Minute)
	m1 := one
	m1.Time = minuteTime
	m1.CPU = ptr(20)
	m2 := m1
	m2.Time = minuteTime.Add(1 * time.Second)
	m2.CPU = ptr(40)
	m3 := m1
	m3.Time = minuteTime.Add(2 * time.Second)
	m3.CPU = ptr(60)
	old := one
	old.Time = now.Add(-60 * 24 * time.Hour).Truncate(time.Hour)
	old.CPU = ptr(95)
	must(post(model.Batch{Samples: []model.Sample{m1, m2, m3, old}}) == 202, "historical publish")
	minuteURL := fmt.Sprintf("%s/api/v1/metrics/query?device_id=%s&metrics=cpu&from=%s&to=%s&max_points=1500", base, id, minuteTime.Add(-time.Hour).Format(time.RFC3339), minuteTime.Add(24*time.Hour).Format(time.RFC3339))
	rolled := false
	for i := 0; i < 90; i++ {
		r, e = c.Get(minuteURL)
		if e == nil {
			var a answer
			e = json.NewDecoder(r.Body).Decode(&a)
			r.Body.Close()
			if e == nil {
				for _, p := range a.Points {
					if v := p.Metrics["cpu"]; v != nil && v.Count == 3 && v.Avg == 40 && v.Min == 20 && v.Max == 60 {
						rolled = true
						break
					}
				}
			}
		}
		if rolled {
			break
		}
		time.Sleep(time.Second)
	}
	must(rolled, "minute rollup min/max/weighted average")
	start := now.Add(-365 * 24 * time.Hour)
	uri := fmt.Sprintf("%s/api/v1/metrics/query?device_id=%s&metrics=cpu,memory,disk_space,disk_busy,net_in,net_out,fan&from=%s&to=%s&max_points=1500", base, id, start.Format(time.RFC3339), now.Format(time.RFC3339))
	r, e = c.Get(uri)
	must(e == nil, "year query")
	var a answer
	e = json.NewDecoder(r.Body).Decode(&a)
	r.Body.Close()
	must(e == nil && r.StatusCode == 200 && len(a.Points) <= 1500, "bounded one-year query")
	oldFound := false
	for _, p := range a.Points {
		if v := p.Metrics["cpu"]; v != nil && v.Max == 95 {
			oldFound = true
			break
		}
	}
	must(oldFound, "60-day historical hour rollup accessible in year chart")
	log.Println("PASS readiness, HTML, ingestion, validation, idempotency, search, minute min/max/avg, 60d hour rollup, bounded year query")
}
