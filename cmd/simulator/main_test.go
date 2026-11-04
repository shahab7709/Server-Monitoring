package main

import (
	"reflect"
	"testing"
	"time"
)

func TestTC33DeterministicSample(t *testing.T) {
	at := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	for d := 0; d < 50; d++ {
		a, b := sample(d, at, 42), sample(d, at, 42)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("device %d changed between replays", d)
		}
		if err := a.Validate(at); err != nil {
			t.Fatalf("device %d: %v", d, err)
		}
		if a.DeviceID == sample(d+1, at, 42).DeviceID {
			t.Fatalf("device IDs collide at %d", d)
		}
	}
	if reflect.DeepEqual(sample(1, at, 42), sample(1, at.Add(time.Second), 42)) {
		t.Fatal("timestamp must advance with time")
	}
}

func TestSimulatorValuesInRangeAndSpike(t *testing.T) {
	for device := 0; device < 5; device++ {
		for kind := 0; kind < 7; kind++ {
			for ts := int64(0); ts < 1500; ts++ {
				v := value(device, ts, kind, 42)
				if v < 0 || v > 100 {
					t.Fatalf("device %d sensor %d time %d: %f", device, kind, ts, v)
				}
			}
			// The seeded spike condition guarantees a full-scale value once per 713-second cycle.
			spikeAt := (713 - (int64(device*17+kind*31)+42)%713) % 713
			if v := value(device, spikeAt, kind, 42); v != 100 {
				t.Fatalf("spike lost for device %d sensor %d: %v", device, kind, v)
			}
		}
	}
}

func TestSimulatorEnvironmentIntegerFallback(t *testing.T) {
	t.Setenv("DEVICE_COUNT", "50")
	if got := integer("DEVICE_COUNT", 2); got != 50 {
		t.Fatalf("got %d", got)
	}
	t.Setenv("DEVICE_COUNT", "invalid")
	if got := integer("DEVICE_COUNT", 2); got != 2 {
		t.Fatalf("got %d", got)
	}
}
