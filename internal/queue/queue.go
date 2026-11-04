package queue

import (
	"context"
	"github.com/nats-io/nats.go"
	"time"
)

const Stream = "SERVER_METRICS"
const Subject = "metrics.samples"

func Connect(url string) (*nats.Conn, nats.JetStreamContext, error) {
	nc, err := nats.Connect(url, nats.RetryOnFailedConnect(true), nats.MaxReconnects(-1), nats.ReconnectWait(time.Second))
	if err != nil {
		return nil, nil, err
	}
	js, err := nc.JetStream(nats.PublishAsyncMaxPending(1024))
	if err != nil {
		nc.Close()
		return nil, nil, err
	}
	_, err = js.AddStream(&nats.StreamConfig{Name: Stream, Subjects: []string{Subject}, Storage: nats.FileStorage, Retention: nats.LimitsPolicy, MaxAge: 48 * time.Hour, MaxBytes: 2 * 1024 * 1024 * 1024, Discard: nats.DiscardNew, Replicas: 1, Duplicates: 2 * time.Minute})
	if err != nil {
		if _, e := js.StreamInfo(Stream); e != nil {
			nc.Close()
			return nil, nil, err
		}
	}
	return nc, js, nil
}
func Publish(ctx context.Context, js nats.JetStreamContext, data []byte) error {
	_, err := js.PublishMsg(&nats.Msg{Subject: Subject, Data: data}, nats.Context(ctx))
	return err
}
