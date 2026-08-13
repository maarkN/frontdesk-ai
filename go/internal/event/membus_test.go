package event_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/maarkn/frontdesk/internal/event"
)

func TestMemoryBusPublishDedupAndFold(t *testing.T) {
	defer goleak.VerifyNone(t)

	var bus event.MemoryBus // zero value is ready
	var delivered []event.Envelope
	bus.Handle(func(_ context.Context, ev event.Envelope) error {
		delivered = append(delivered, ev)
		return nil
	})

	ctx := context.Background()
	fixture := callFixture(t)
	for _, ev := range fixture {
		require.NoError(t, bus.Publish(ctx, ev))
	}
	// At-least-once producer retries: duplicates are dropped by (call, seq).
	require.NoError(t, bus.Publish(ctx, fixture[0]))
	require.NoError(t, bus.Publish(ctx, fixture[6]))

	require.Len(t, delivered, len(fixture))
	require.Equal(t, bus.Events(), delivered)

	// The bus feeds the same deterministic fold.
	snap, err := event.Fold(bus.Events())
	require.NoError(t, err)
	require.Equal(t, event.StatusEnded, snap.Status)
	require.Equal(t, uint64(20), snap.LastSeq)
}

func TestMemoryBusSubscribeBlocksUntilCancel(t *testing.T) {
	defer goleak.VerifyNone(t)

	var bus event.MemoryBus
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(ctx, func(context.Context, event.Envelope) error { return nil })
	}()

	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe did not return after cancel")
	}
}
