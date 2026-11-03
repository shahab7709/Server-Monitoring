package model

import (
	"testing"
	"time"
)

func TestValidation(t *testing.T) {
	x := float64(0)
	s := Sample{DeviceID: "a", Time: time.Now(), CPU: &x, Memory: &x, DiskSpace: &x, DiskBusy: &x, NetIn: &x, NetOut: &x, Fan: &x}
	if err := s.Validate(time.Now()); err != nil {
		t.Fatal(err)
	}
	x = 101
	if s.Validate(time.Now()) == nil {
		t.Fatal("expected invalid")
	}
}
func TestMetrics(t *testing.T) {
	if _, e := ParseSensorSet("cpu,unknown"); e == nil {
		t.Fatal("unknown")
	}
	v, e := ParseSensorSet("cpu,cpu,fan")
	if e != nil || len(v) != 2 {
		t.Fatalf("%v %v", v, e)
	}
}
