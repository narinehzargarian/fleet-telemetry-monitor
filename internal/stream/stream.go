// Package stream moves readings from the API to the processor through a Redis Stream.
//
// The API publishes and returns immediately, so a slow database never slows down
// ingestion. The processor reads with a consumer group and only acknowledges a
// message after it is stored, so a crash means redelivery, not data loss.
package stream

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/telemetry"
)

const (
	StreamKey = "telemetry:readings"
	Group     = "processors"
	field     = "reading"
)

// Publisher appends readings to the stream.
type Publisher struct{ rdb *redis.Client }

func NewPublisher(rdb *redis.Client) *Publisher { return &Publisher{rdb: rdb} }

// Publish adds one reading. MaxLen caps memory if the processor falls far behind.
func (p *Publisher) Publish(ctx context.Context, r telemetry.Reading) error {
	payload, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamKey,
		MaxLen: 100_000,
		Approx: true,
		Values: map[string]any{field: payload},
	}).Err()
}

// Message is one delivered reading plus the id needed to acknowledge it.
type Message struct {
	ID      string
	Reading telemetry.Reading
	// Malformed is set when the payload could not be decoded; the consumer should ack and skip it.
	Malformed bool
}

// Consumer reads from the stream as part of a consumer group.
type Consumer struct {
	rdb  *redis.Client
	name string
}

// NewConsumer creates the consumer group if needed.
func NewConsumer(ctx context.Context, rdb *redis.Client, name string) (*Consumer, error) {
	err := rdb.XGroupCreateMkStream(ctx, StreamKey, Group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return nil, err
	}
	return &Consumer{rdb: rdb, name: name}, nil
}

// Read blocks up to wait for new messages. It first re-reads anything this consumer
// received earlier but never acknowledged (for example before a crash).
func (c *Consumer) Read(ctx context.Context, count int64, wait time.Duration) ([]Message, error) {
	for _, start := range []string{"0", ">"} {
		block := time.Duration(-1) // do not block when checking our own pending list
		if start == ">" {
			block = wait
		}
		res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: Group, Consumer: c.name,
			Streams: []string{StreamKey, start},
			Count:   count, Block: block,
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, err
		}
		msgs := decode(res)
		if len(msgs) > 0 {
			return msgs, nil
		}
	}
	return nil, nil
}

// Ack confirms a message is fully processed so it is never delivered again.
func (c *Consumer) Ack(ctx context.Context, id string) error {
	return c.rdb.XAck(ctx, StreamKey, Group, id).Err()
}

func decode(res []redis.XStream) []Message {
	var msgs []Message
	for _, s := range res {
		for _, m := range s.Messages {
			msg := Message{ID: m.ID}
			raw, _ := m.Values[field].(string)
			if err := json.Unmarshal([]byte(raw), &msg.Reading); err != nil {
				msg.Malformed = true
			}
			msgs = append(msgs, msg)
		}
	}
	return msgs
}
