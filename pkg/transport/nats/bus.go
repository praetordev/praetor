package nats

import (
	"fmt"

	"github.com/praetordev/praetor/pkg/events"
	"github.com/nats-io/nats.go"
)

const (
	SubjectExecutionRequest = "job.requests"
	SubjectJobEvent         = "job.events"
	SubjectLogChunk         = "job.logs"
	QueueGroupExecutor      = "executor-group"
	QueueGroupConsumer      = "consumer-group"
)

type NatsBus struct {
	Conn *nats.Conn
	Enc  *nats.EncodedConn
}

func NewNatsBus(url string) (*NatsBus, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect failed: %w", err)
	}

	ec, err := nats.NewEncodedConn(nc, nats.JSON_ENCODER)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats encoded conn failed: %w", err)
	}

	return &NatsBus{
		Conn: nc,
		Enc:  ec,
	}, nil
}

func (b *NatsBus) Close() {
	b.Enc.Close()
	b.Conn.Close()
}

// -- Publisher Implementation --

func (b *NatsBus) PublishExecutionRequest(req *events.ExecutionRequest) error {
	return b.Enc.Publish(SubjectExecutionRequest, req)
}

func (b *NatsBus) PublishJobEvent(event *events.JobEvent) error {
	return b.Enc.Publish(SubjectJobEvent, event)
}

func (b *NatsBus) PublishLogChunk(chunk *events.LogChunk) error {
	return b.Enc.Publish(SubjectLogChunk, chunk)
}

// -- Subscriber Implementation --

func (b *NatsBus) SubscribeToExecutionRequests() (<-chan events.ExecutionRequest, error) {
	ch := make(chan events.ExecutionRequest, 100)
	// Queue Subscribe ensures load balancing if we run multiple executors
	_, err := b.Enc.QueueSubscribe(SubjectExecutionRequest, QueueGroupExecutor, func(req *events.ExecutionRequest) {
		ch <- *req
	})
	if err != nil {
		return nil, err
	}
	return ch, nil
}

func (b *NatsBus) SubscribeToJobEvents() (<-chan events.JobEvent, error) {
	ch := make(chan events.JobEvent, 100)
	// Queue Subscribe ensures only one consumer processes each event (if we scale consumers)
	_, err := b.Enc.QueueSubscribe(SubjectJobEvent, QueueGroupConsumer, func(event *events.JobEvent) {
		ch <- *event
	})
	if err != nil {
		return nil, err
	}
	return ch, nil
}
