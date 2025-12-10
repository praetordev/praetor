package core

import (
	"context"
	"log"

	"github.com/praetordev/praetor/pkg/events"
)

type EventSubscriber interface {
	SubscribeToJobEvents() (<-chan events.JobEvent, error)
}

type Consumer struct {
	Subscriber EventSubscriber
	Writer     *DBWriter
}

func NewConsumer(sub EventSubscriber, writer *DBWriter) *Consumer {
	return &Consumer{
		Subscriber: sub,
		Writer:     writer,
	}
}

func (c *Consumer) Start() error {
	eventChan, err := c.Subscriber.SubscribeToJobEvents()
	if err != nil {
		return err
	}

	log.Println("Consumer started, waiting for events...")

	for evt := range eventChan {
		// In a real system we would handle offsets/acks here
		if err := c.processEvent(evt); err != nil {
			log.Printf("Error processing event %d for run %s: %v", evt.Seq, evt.ExecutionRunID, err)
			continue
		}
		log.Printf("Processed event %s (Seq: %d) for Job %d", evt.EventType, evt.Seq, evt.UnifiedJobID)
	}
	return nil
}

func (c *Consumer) processEvent(evt events.JobEvent) error {
	// Delegate to DBWriter
	ctx := context.Background()
	return c.Writer.WriteEvent(ctx, evt)
}
