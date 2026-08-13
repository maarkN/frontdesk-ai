package event

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

// JetStreamPublisher publishes envelopes to NATS JetStream. Each message is
// published with Nats-Msg-Id = EventID so the server's dedup window drops
// producer-side retries; consumers still dedup by (CallID, Seq) because the
// bus remains at-least-once end to end.
type JetStreamPublisher struct {
	js            jetstream.JetStream
	subjectPrefix string
}

var _ Publisher = (*JetStreamPublisher)(nil)

// NewJetStreamPublisher returns a publisher writing to
// "<subjectPrefix>.<tenantId>.<callId>".
func NewJetStreamPublisher(js jetstream.JetStream, subjectPrefix string) *JetStreamPublisher {
	return &JetStreamPublisher{js: js, subjectPrefix: subjectPrefix}
}

// Publish implements Publisher.
func (p *JetStreamPublisher) Publish(ctx context.Context, ev Envelope) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event %s: %w", ev.EventID, err)
	}
	subject := fmt.Sprintf("%s.%s.%s", p.subjectPrefix, ev.TenantID, ev.CallID)
	if _, err := p.js.Publish(ctx, subject, body, jetstream.WithMsgID(ev.EventID)); err != nil {
		return fmt.Errorf("publish %s to %s: %w", ev.EventID, subject, err)
	}
	return nil
}

// JetStreamSubscriber consumes envelopes from a JetStream consumer, dropping
// duplicate (CallID, Seq) deliveries before invoking the handler.
type JetStreamSubscriber struct {
	consumer jetstream.Consumer
}

var _ Subscriber = (*JetStreamSubscriber)(nil)

// NewJetStreamSubscriber returns a subscriber reading from consumer.
func NewJetStreamSubscriber(consumer jetstream.Consumer) *JetStreamSubscriber {
	return &JetStreamSubscriber{consumer: consumer}
}

// Subscribe implements Subscriber. It blocks until ctx is done. Handler
// errors leave the message unacknowledged so JetStream redelivers it.
func (s *JetStreamSubscriber) Subscribe(ctx context.Context, h Handler) error {
	var dedup seqDedup
	cons, err := s.consumer.Consume(func(msg jetstream.Msg) {
		var ev Envelope
		if err := json.Unmarshal(msg.Data(), &ev); err != nil {
			// Malformed payloads would redeliver forever; drop them.
			_ = msg.Term()
			return
		}
		if dedup.isDuplicate(ev.CallID, ev.Seq) {
			_ = msg.Ack()
			return
		}
		if err := h(ctx, ev); err != nil {
			_ = msg.Nak()
			return
		}
		if ev.Type == TypeCallEnded {
			dedup.forget(ev.CallID)
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	defer cons.Stop()

	<-ctx.Done()
	return ctx.Err()
}
