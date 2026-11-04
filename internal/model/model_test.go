package model

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func floatPointer(v float64) *float64 { return &v }

func validSample(now time.Time) Sample {
	return Sample{DeviceID: "server-1", Time: now.UTC().Add(-time.Second), CPU: floatPointer(0), Memory: floatPointer(100), DiskSpace: floatPointer(20), DiskBusy: floatPointer(30), NetIn: floatPointer(40), NetOut: floatPointer(50), Fan: floatPointer(60)}
}

func setSensor(s *Sample, field string, value *float64) {
	switch field {
	case "cpu":
		s.CPU = value
	case "memory":
		s.Memory = value
	case "disk_space":
		s.DiskSpace = value
	case "disk_busy":
		s.DiskBusy = value
	case "net_in":
		s.NetIn = value
	case "net_out":
		s.NetOut = value
	case "fan":
		s.Fan = value
	}
}

func TestSampleValidationBoundaries(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	s := validSample(now)
	if err := s.Validate(now); err != nil {
		t.Fatalf("valid boundary values rejected: %v", err)
	}
	for _, field := range Sensors {
		t.Run(field, func(t *testing.T) {
			for _, v := range []float64{-1, 101, math.NaN(), math.Inf(1), math.Inf(-1)} {
				bad := s
				setSensor(&bad, field, floatPointer(v))
				if err := bad.Validate(now); err == nil || !strings.Contains(err.Error(), field) {
					t.Errorf("%s=%v: expected named validation error, got %v", field, v, err)
				}
			}
			bad := s
			setSensor(&bad, field, nil)
			if err := bad.Validate(now); err == nil || !strings.Contains(err.Error(), field) {
				t.Errorf("missing %s: got %v", field, err)
			}
		})
	}
}

func TestSampleValidationIdentifiersAndTimes(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	valid := validSample(now)
	for _, tc := range []struct {
		name string
		edit func(*Sample)
	}{
		{"missing device", func(s *Sample) { s.DeviceID = "" }},
		{"too long device", func(s *Sample) { s.DeviceID = strings.Repeat("x", 129) }},
		{"whitespace device", func(s *Sample) { s.DeviceID = " server " }},
		{"missing time", func(s *Sample) { s.Time = time.Time{} }},
		{"too far future", func(s *Sample) { s.Time = now.Add(2*time.Minute + time.Nanosecond) }},
		{"too old", func(s *Sample) { s.Time = now.AddDate(-2, 0, 0).Add(-time.Nanosecond) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			tc.edit(&s)
			if err := s.Validate(now); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	for _, at := range []time.Time{now.Add(2 * time.Minute), now.AddDate(-2, 0, 0)} {
		s := valid
		s.Time = at
		if err := s.Validate(now); err != nil {
			t.Errorf("time at permitted boundary %v: %v", at, err)
		}
	}
}

func TestBatchValidation(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	s := validSample(now)
	for _, n := range []int{0, 1001} {
		if err := (Batch{Samples: make([]Sample, n)}).Validate(now); err == nil {
			t.Errorf("size %d accepted", n)
		}
	}
	if err := (Batch{Samples: []Sample{s}}).Validate(now); err != nil {
		t.Fatal(err)
	}
	full := make([]Sample, 1000)
	for i := range full {
		full[i] = s
	}
	if err := (Batch{Samples: full}).Validate(now); err != nil {
		t.Fatal(err)
	}
	full[527].CPU = nil
	err := (Batch{Samples: full}).Validate(now)
	if err == nil || !strings.Contains(err.Error(), "samples[527]") {
		t.Fatalf("expected offending sample index, got %v", err)
	}
}

func TestParseSensorSet(t *testing.T) {
	for _, input := range []string{"", "cpu,unknown", "CPU", "cpu,", ",cpu", "cpu, fan"} {
		if _, err := ParseSensorSet(input); err == nil {
			t.Errorf("%q: expected error", input)
		}
	}
	set, err := ParseSensorSet("cpu,fan,cpu,memory")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"cpu", "fan", "memory"}
	if len(set) != len(expected) {
		t.Fatalf("set %v, want %v", set, expected)
	}
	for i := range expected {
		if set[i] != expected[i] {
			t.Fatalf("set %v, want %v", set, expected)
		}
	}
	set, err = ParseSensorSet(strings.Join(Sensors, ","))
	if err != nil || len(set) != 7 {
		t.Fatalf("all seven sensors: %v, %v", set, err)
	}
}

func TestSampleJSON(t *testing.T) {
	s := validSample(time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC))
	got := SampleJSON(s)
	if got["device_id"] != s.DeviceID || got["cpu"] != float64(0) || got["memory"] != float64(100) {
		t.Fatalf("unexpected JSON: %v", got)
	}
	if _, ok := got["time"].(string); !ok {
		t.Fatalf("time should be RFC3339 string: %v", got["time"])
	}
}

func TestMalformedAndMissingSampleJSON(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	good := validSample(now)
	body, err := json.Marshal(Batch{Samples: []Sample{good}})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{`,
		`{"samples":[{"device_id":"a","time":"2026-11-04T09:59:59Z","cpu":"bad"}]}`,
	} {
		var b Batch
		if err := json.Unmarshal([]byte(raw), &b); err == nil {
			t.Errorf("malformed or mistyped JSON was accepted: %s", raw)
		}
	}
	for _, missing := range []string{"device_id", "time", "cpu", "memory", "disk_space", "disk_busy", "net_in", "net_out", "fan"} {
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			t.Fatal(err)
		}
		sample := obj["samples"].([]any)[0].(map[string]any)
		delete(sample, missing)
		raw, err := json.Marshal(obj)
		if err != nil {
			t.Fatal(err)
		}
		var b Batch
		if err := json.Unmarshal(raw, &b); err != nil {
			t.Fatal(err)
		}
		if err := b.Validate(now); err == nil {
			t.Errorf("missing %s accepted", missing)
		}
	}
}
