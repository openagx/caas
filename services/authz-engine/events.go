package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// EventProducer publishes authorization events to Redpanda/Kafka.
type EventProducer struct {
	client *kgo.Client
	topic  string
}

// NewEventProducer creates a Kafka producer. Returns nil (no-op) if connection fails.
func NewEventProducer(brokers []string, topic string) *EventProducer {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
	)
	if err != nil {
		log.Printf("WARN: Kafka unavailable, events disabled: %v", err)
		return nil
	}
	return &EventProducer{client: client, topic: topic}
}

// Emit publishes an event asynchronously.
func (p *EventProducer) Emit(eventType string, data map[string]any) {
	if p == nil {
		return
	}

	payload := map[string]any{
		"type":      eventType,
		"data":      data,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}

	b, err := json.Marshal(payload)
	if err != nil {
		log.Printf("WARN: failed to marshal event: %v", err)
		return
	}

	p.client.Produce(context.Background(), &kgo.Record{Value: b}, func(r *kgo.Record, err error) {
		if err != nil {
			log.Printf("WARN: failed to produce event: %v", err)
		}
	})
}

// Close flushes and closes the producer.
func (p *EventProducer) Close() {
	if p == nil {
		return
	}
	p.client.Flush(context.Background())
	p.client.Close()
}
