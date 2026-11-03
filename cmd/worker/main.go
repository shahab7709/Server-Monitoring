package main

import (
	"context"
	"example.com/server-monitoring/internal/queue"
	"example.com/server-monitoring/internal/storage"
	"github.com/nats-io/nats.go"
	"log"
	"os"
	"time"
)

func env(k, v string) string {
	if s := os.Getenv(k); s != "" {
		return s
	}
	return v
}
func main() {
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
	nc, js, err := queue.Connect(env("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()
	sub, err := js.PullSubscribe(queue.Subject, "db-writers", nats.BindStream(queue.Stream), nats.ManualAck(), nats.AckWait(120*time.Second), nats.MaxDeliver(8))
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()
	log.Print("worker ready")
	for {
		msgs, err := sub.Fetch(8, nats.MaxWait(2*time.Second))
		if err != nil {
			if err != nats.ErrTimeout {
				log.Printf("fetch: %v", err)
			}
			continue
		}
		for _, msg := range msgs {
			c, cancel := context.WithTimeout(ctx, 90*time.Second)
			err = store.InsertBatch(c, msg.Data)
			cancel()
			if err != nil {
				log.Printf("insert retry: %v", err)
				_ = msg.NakWithDelay(5 * time.Second)
				continue
			}
			if err = msg.AckSync(); err != nil {
				log.Printf("ack: %v", err)
			}
		}
	}
}
