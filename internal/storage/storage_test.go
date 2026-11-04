package storage

import (
	"math"
	"strings"
	"testing"

	"example.com/server-monitoring/internal/model"
)

func TestTC10RollupStatistics(t *testing.T) {
	// Three input values (20, 40, 60) aggregate to the persisted columns below.
	got := statsFromAggregate(20+40+60, 20, 60, 3)
	if got == nil || got.Avg != 40 || got.Min != 20 || got.Max != 60 || got.Count != 3 {
		t.Fatalf("unexpected aggregate: %+v", got)
	}
}

func TestTC11CountWeightedAggregation(t *testing.T) {
	// Two pre-aggregated buckets: [20] and [40, 60, 80].
	// Averaging bucket means produces 40 (incorrect), while sum/count produces 50.
	aSum, aCount := float64(20), int64(1)
	bSum, bCount := float64(180), int64(3)
	got := statsFromAggregate(aSum+bSum, math.Min(20, 40), math.Max(20, 80), aCount+bCount)
	if got == nil || got.Avg != 50 || got.Min != 20 || got.Max != 80 || got.Count != 4 {
		t.Fatalf("weighted aggregate: %+v", got)
	}
}

func TestEmptyAggregateIsGap(t *testing.T) {
	if got := statsFromAggregate(0, 0, 0, 0); got != nil {
		t.Fatalf("empty aggregate must be null, got %+v", got)
	}
}

func TestRollupSQLForAllSensors(t *testing.T) {
	for _, tc := range []struct{ table, interval, alias string }{
		{"minute_rollup", "1 minute", "m"}, {"hour_rollup", "1 hour", "h"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			sql := rollupCTE(tc.table, tc.interval, tc.alias)
			for _, want := range []string{
				"FROM inserted", "GROUP BY device_id,time_bucket('" + tc.interval + "',time)",
				"ON CONFLICT(device_id,time) DO UPDATE", "count = " + tc.table + ".count + EXCLUDED.count",
			} {
				if !strings.Contains(sql, want) {
					t.Errorf("missing %q in %s", want, sql)
				}
			}
			for _, sensor := range model.Sensors {
				for _, want := range []string{
					"sum(" + sensor + ")", "min(" + sensor + ")", "max(" + sensor + ")",
					sensor + "_sum = " + tc.table + "." + sensor + "_sum + EXCLUDED." + sensor + "_sum",
					sensor + "_min = LEAST(" + tc.table + "." + sensor + "_min,EXCLUDED." + sensor + "_min)",
					sensor + "_max = GREATEST(" + tc.table + "." + sensor + "_max,EXCLUDED." + sensor + "_max)",
				} {
					if !strings.Contains(sql, want) {
						t.Errorf("%s: missing %q", sensor, want)
					}
				}
			}
		})
	}
}

func TestSensorSQLTypes(t *testing.T) {
	sql := sensorTypes()
	for _, s := range model.Sensors {
		if !strings.Contains(sql, s+" REAL") {
			t.Errorf("missing %s", s)
		}
	}
	if got := len(strings.Split(sql, ",")); got != 7 {
		t.Errorf("got %d sensor columns", got)
	}
}
