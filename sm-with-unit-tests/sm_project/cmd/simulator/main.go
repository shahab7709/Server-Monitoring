package main

import (
	"bytes"
	"encoding/json"
	"example.com/server-monitoring/internal/model"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"
)

func env(k, v string) string {
	if s := os.Getenv(k); s != "" {
		return s
	}
	return v
}
func integer(k string, n int) int {
	x, err := strconv.Atoi(env(k, strconv.Itoa(n)))
	if err != nil {
		return n
	}
	return x
}
func value(device int, t int64, kind int, seed int64) float64 {
	// Deterministic, smooth baseline with deterministic spikes; replaying is safe.
	phase := float64(t)/float64(40+kind*23) + float64(device*3+kind)
	v := 38 + 25*math.Sin(phase) + float64((device*17+kind*11)%19)
	if (t+int64(device*17+kind*31)+seed)%713 == 0 {
		v = 100
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return math.Round(v*100) / 100
}
func sample(d int, t time.Time, seed int64) model.Sample {
	vals := [7]*float64{}
	for k := 0; k < 7; k++ {
		x := value(d, t.Unix(), k, seed)
		vals[k] = &x
	}
	return model.Sample{DeviceID: fmt.Sprintf("server-%05d", d), Time: t, CPU: vals[0], Memory: vals[1], DiskSpace: vals[2], DiskBusy: vals[3], NetIn: vals[4], NetOut: vals[5], Fan: vals[6]}
}
func main() {
	count := integer("DEVICE_COUNT", 2)
	days := integer("HISTORY_DAYS", 3)
	seed := int64(integer("SEED", 42))
	api := env("API_URL", "http://localhost:8080")
	if count < 1 || days < 0 {
		log.Fatal("invalid device count/history")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	send := func(batch []model.Sample) {
		payload, _ := json.Marshal(model.Batch{Samples: batch})
		for attempts := 0; ; attempts++ {
			resp, err := client.Post(api+"/api/v1/metrics/batch", "application/json", bytes.NewReader(payload))
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 202 {
					return
				}
				err = fmt.Errorf("status %d", resp.StatusCode)
			}
			if attempts%10 == 0 {
				log.Printf("retrying (%v)", err)
			}
			time.Sleep(time.Duration(1+attempts%5) * time.Second)
		}
	}
	start := time.Now().UTC().Truncate(time.Second)
	batch := make([]model.Sample, 0, 400)
	for t := start.Add(-time.Duration(days) * 24 * time.Hour); t.Before(start); t = t.Add(time.Second) {
		for d := 0; d < count; d++ {
			batch = append(batch, sample(d, t, seed))
			if len(batch) >= 400 {
				send(batch)
				batch = batch[:0]
			}
		}
	}
	if len(batch) > 0 {
		send(batch)
	}
	log.Printf("backfill queued %d devices, %d days", count, days)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for t := range tick.C {
		t = t.UTC().Truncate(time.Second)
		batch = batch[:0]
		for d := 0; d < count; d++ {
			batch = append(batch, sample(d, t, seed))
			if len(batch) >= 400 {
				send(batch)
				batch = batch[:0]
			}
		}
		if len(batch) > 0 {
			send(batch)
		}
	}
}
