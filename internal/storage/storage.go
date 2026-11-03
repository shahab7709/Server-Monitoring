package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"example.com/server-monitoring/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ DB *pgxpool.Pool }

func Open(ctx context.Context, uri string) (*Store, error) {
	p, err := pgxpool.New(ctx, uri)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Store{DB: p}, nil
}
func (s *Store) Close() { s.DB.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(804020)"); err != nil {
		return err
	}
	statements := []string{
		`CREATE EXTENSION IF NOT EXISTS timescaledb`,
		`CREATE TABLE IF NOT EXISTS devices (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE TABLE IF NOT EXISTS samples (device_id TEXT NOT NULL, time TIMESTAMPTZ NOT NULL,
 cpu REAL NOT NULL CHECK(cpu BETWEEN 0 AND 100), memory REAL NOT NULL CHECK(memory BETWEEN 0 AND 100),
 disk_space REAL NOT NULL CHECK(disk_space BETWEEN 0 AND 100), disk_busy REAL NOT NULL CHECK(disk_busy BETWEEN 0 AND 100),
 net_in REAL NOT NULL CHECK(net_in BETWEEN 0 AND 100), net_out REAL NOT NULL CHECK(net_out BETWEEN 0 AND 100),
 fan REAL NOT NULL CHECK(fan BETWEEN 0 AND 100), PRIMARY KEY (device_id,time))`,
		`SELECT create_hypertable('samples','time',if_not_exists=>TRUE,chunk_time_interval=>INTERVAL '1 day')`,
		`CREATE INDEX IF NOT EXISTS samples_device_time_desc ON samples(device_id,time DESC)`,
	}
	for _, sql := range statements {
		if _, err = tx.Exec(ctx, sql); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}
	for _, table := range []string{"minute_rollup", "hour_rollup"} {
		parts := []string{"device_id TEXT NOT NULL", "time TIMESTAMPTZ NOT NULL", "count BIGINT NOT NULL CHECK(count>0)"}
		for _, m := range model.Sensors {
			parts = append(parts, fmt.Sprintf("%s_sum DOUBLE PRECISION NOT NULL, %s_min REAL NOT NULL, %s_max REAL NOT NULL", m, m, m))
		}
		create := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s, PRIMARY KEY(device_id,time))", table, strings.Join(parts, ","))
		if _, err = tx.Exec(ctx, create); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "SELECT create_hypertable($1::regclass,'time',if_not_exists=>TRUE,chunk_time_interval=>INTERVAL '7 days')", table); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Upserts use only INSERT RETURNING rows. A duplicated device/timestamp never changes aggregates.
// The raw and both resolutions commit atomically. Historical backfill writes all three levels.
func (s *Store) InsertBatch(ctx context.Context, data []byte) error {
	var b model.Batch
	if err := json.Unmarshal(data, &b); err != nil {
		return err
	}
	if err := b.Validate(time.Now().UTC()); err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// External IDs are intentionally text to support discoverable device names and stable idempotency.
	deviceSet := map[string]struct{}{}
	for _, sample := range b.Samples {
		deviceSet[sample.DeviceID] = struct{}{}
	}
	for id := range deviceSet {
		if _, err = tx.Exec(ctx, "INSERT INTO devices(id,name) VALUES($1,$1) ON CONFLICT DO NOTHING", id); err != nil {
			return err
		}
	}
	cols := append([]string{"device_id", "time"}, model.Sensors...)
	raw := fmt.Sprintf("WITH inserted AS (INSERT INTO samples (%s) SELECT %s FROM jsonb_to_recordset($1::jsonb -> 'samples') AS x(%s) ON CONFLICT (device_id,time) DO NOTHING RETURNING *), ",
		strings.Join(cols, ","), strings.Join(cols, ","), "device_id TEXT,time TIMESTAMPTZ,"+sensorTypes())
	// Both aggregates are computed from accepted rows only, and merged additively.
	stmt := raw + rollupCTE("minute_rollup", "1 minute", "m") + ", " + rollupCTE("hour_rollup", "1 hour", "h") + " SELECT (SELECT count(*) FROM inserted)"
	if _, err = tx.Exec(ctx, stmt, string(data)); err != nil {
		return fmt.Errorf("batch insert: %w", err)
	}
	return tx.Commit(ctx)
}
func sensorTypes() string {
	a := []string{}
	for _, v := range model.Sensors {
		a = append(a, v+" REAL")
	}
	return strings.Join(a, ",")
}
func rollupCTE(table, interval, alias string) string {
	cols := []string{"device_id", "time", "count"}
	selects := []string{"device_id", fmt.Sprintf("time_bucket('%s',time)", interval), "count(*)"}
	updates := []string{"count = " + table + ".count + EXCLUDED.count"}
	for _, v := range model.Sensors {
		cols = append(cols, v+"_sum", v+"_min", v+"_max")
		selects = append(selects, "sum("+v+")", "min("+v+")", "max("+v+")")
		updates = append(updates, v+"_sum = "+table+"."+v+"_sum + EXCLUDED."+v+"_sum", v+"_min = LEAST("+table+"."+v+"_min,EXCLUDED."+v+"_min)", v+"_max = GREATEST("+table+"."+v+"_max,EXCLUDED."+v+"_max)")
	}
	return fmt.Sprintf("%s AS (INSERT INTO %s (%s) SELECT %s FROM inserted GROUP BY device_id,time_bucket('%s',time) ON CONFLICT(device_id,time) DO UPDATE SET %s RETURNING 1)", alias, table, strings.Join(cols, ","), strings.Join(selects, ","), interval, strings.Join(updates, ","))
}

type Point struct {
	Time    time.Time         `json:"time"`
	Metrics map[string]*Stats `json:"metrics"`
}
type Stats struct {
	Avg   float64 `json:"avg"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Count int64   `json:"count"`
}
type QueryResult struct {
	DeviceID      string  `json:"device_id"`
	Resolution    string  `json:"resolution"`
	BucketSeconds int64   `json:"bucket_seconds"`
	Points        []Point `json:"points"`
}

func (s *Store) Query(ctx context.Context, id string, metrics []string, from, to time.Time, maxPoints int) (QueryResult, error) {
	duration := to.Sub(from)
	source := "samples"
	res := "raw"
	base := int64(1)
	if duration > 2*time.Hour {
		source = "minute_rollup"
		res = "minute"
		base = 60
	}
	if duration > 45*24*time.Hour {
		source = "hour_rollup"
		res = "hour"
		base = 3600
	}
	bucket := int64(math.Ceil(duration.Seconds()/float64(maxPoints)/float64(base))) * base
	if bucket < base {
		bucket = base
	}
	// Query only selected columns, all drawn on the same 0..100 axis.
	fields := []string{}
	for _, m := range metrics {
		if source == "samples" {
			fields = append(fields, fmt.Sprintf("sum(%s)::float8, min(%s)::float8, max(%s)::float8, count(*)::bigint", m, m, m))
		} else {
			fields = append(fields, fmt.Sprintf("sum(%s_sum)::float8, min(%s_min)::float8, max(%s_max)::float8, sum(count)::bigint", m, m, m))
		}
	}
	sql := fmt.Sprintf(`SELECT (floor(extract(epoch FROM time)/$4::bigint)::bigint*$4::bigint) AS bucket,%s FROM %s WHERE device_id=$1 AND time >= $2 AND time < $3 GROUP BY bucket ORDER BY bucket`, strings.Join(fields, ","), source)
	rows, err := s.DB.Query(ctx, sql, id, from, to, bucket)
	if err != nil {
		return QueryResult{}, err
	}
	defer rows.Close()
	found := map[int64]map[string]*Stats{}
	for rows.Next() {
		var epoch int64
		values := make([]any, 1+len(metrics)*4)
		values[0] = &epoch
		sums := make([]float64, len(metrics))
		mins := make([]float64, len(metrics))
		maxs := make([]float64, len(metrics))
		counts := make([]int64, len(metrics))
		for i := range metrics {
			values[1+i*4] = &sums[i]
			values[2+i*4] = &mins[i]
			values[3+i*4] = &maxs[i]
			values[4+i*4] = &counts[i]
		}
		if err = rows.Scan(values...); err != nil {
			return QueryResult{}, err
		}
		found[epoch] = map[string]*Stats{}
		for i, m := range metrics {
			found[epoch][m] = &Stats{Avg: sums[i] / float64(counts[i]), Min: mins[i], Max: maxs[i], Count: counts[i]}
		}
	}
	if err = rows.Err(); err != nil {
		return QueryResult{}, err
	}
	result := QueryResult{DeviceID: id, Resolution: res, BucketSeconds: bucket, Points: []Point{}}
	start := from.Unix() / bucket * bucket
	// Dense bucket sequence leaves nil gaps to ensure charts never treat missing data as zero.
	for t := start; t < to.Unix(); t += bucket {
		result.Points = append(result.Points, Point{Time: time.Unix(t, 0).UTC(), Metrics: found[t]})
	}
	return result, nil
}
func (s *Store) Devices(ctx context.Context, search, cursor string, limit int) ([]map[string]string, error) {
	rows, err := s.DB.Query(ctx, `SELECT id,name FROM devices WHERE id > $1 AND (id ILIKE '%'||$2||'%' OR name ILIKE '%'||$2||'%') ORDER BY id LIMIT $3`, cursor, search, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	a := []map[string]string{}
	for rows.Next() {
		var id, name string
		if err = rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		a = append(a, map[string]string{"id": id, "name": name})
	}
	return a, rows.Err()
}

// Raw retention is safe because independent rollups are written in the same transaction.
// One-hour history is retained for 400 days; deletes are chunk-level via TimescaleDB policies.
func (s *Store) InstallRetention(ctx context.Context) error {
	for _, entry := range []struct{ table, age string }{{"samples", "7 days"}, {"minute_rollup", "45 days"}, {"hour_rollup", "400 days"}} {
		if _, err := s.DB.Exec(ctx, "SELECT add_retention_policy($1::regclass, $2::interval, if_not_exists => TRUE)", entry.table, entry.age); err != nil {
			return err
		}
	}
	return nil
}
