package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// EventProducer publishes trust events to Kafka/Redpanda.
type EventProducer struct {
	client *kgo.Client
	topic  string
}

func NewEventProducer(brokers []string, topic string) *EventProducer {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		log.Printf("WARN: Kafka unavailable, events disabled: %v", err)
		return nil
	}
	return &EventProducer{client: client, topic: topic}
}

func (p *EventProducer) Emit(eventType string, data map[string]any) {
	if p == nil || p.client == nil {
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"type":      eventType,
		"data":      data,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})

	p.client.Produce(nil, &kgo.Record{
		Topic: p.topic,
		Value: payload,
	}, nil)
}

func (p *EventProducer) Close() {
	if p != nil && p.client != nil {
		p.client.Close()
	}
}
