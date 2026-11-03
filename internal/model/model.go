package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var Sensors = []string{"cpu", "memory", "disk_space", "disk_busy", "net_in", "net_out", "fan"}

type Sample struct {
	DeviceID  string    `json:"device_id"`
	Time      time.Time `json:"time"`
	CPU       *float64  `json:"cpu"`
	Memory    *float64  `json:"memory"`
	DiskSpace *float64  `json:"disk_space"`
	DiskBusy  *float64  `json:"disk_busy"`
	NetIn     *float64  `json:"net_in"`
	NetOut    *float64  `json:"net_out"`
	Fan       *float64  `json:"fan"`
}
type Batch struct {
	Samples []Sample `json:"samples"`
}

func (s Sample) Values() []*float64 {
	return []*float64{s.CPU, s.Memory, s.DiskSpace, s.DiskBusy, s.NetIn, s.NetOut, s.Fan}
}
func (s Sample) Validate(now time.Time) error {
	if len(s.DeviceID) == 0 || len(s.DeviceID) > 128 || strings.TrimSpace(s.DeviceID) != s.DeviceID {
		return errors.New("invalid device_id")
	}
	if s.Time.IsZero() || s.Time.After(now.Add(2*time.Minute)) || s.Time.Before(now.AddDate(-2, 0, 0)) {
		return errors.New("invalid time")
	}
	for i, v := range s.Values() {
		if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > 100 {
			return fmt.Errorf("invalid %s", Sensors[i])
		}
	}
	return nil
}
func (b Batch) Validate(now time.Time) error {
	if len(b.Samples) == 0 || len(b.Samples) > 1000 {
		return errors.New("samples must contain 1..1000 items")
	}
	for i, s := range b.Samples {
		if err := s.Validate(now); err != nil {
			return fmt.Errorf("samples[%d]: %w", i, err)
		}
	}
	return nil
}
func ParseSensorSet(v string) ([]string, error) {
	if v == "" {
		return nil, errors.New("metrics required")
	}
	seen := map[string]bool{}
	out := []string{}
	for _, s := range strings.Split(v, ",") {
		valid := false
		for _, x := range Sensors {
			if x == s {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("unknown metric %q", s)
		}
		if !seen[s] {
			out = append(out, s)
			seen[s] = true
		}
	}
	return out, nil
}
func SampleJSON(s Sample) map[string]any {
	b, _ := json.Marshal(s)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}
