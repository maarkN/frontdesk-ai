package asterisk

import (
	"context"
	"fmt"

	"github.com/maarkn/frontdesk/internal/audiobank"
	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/telnyx"
)

// TransferRecoveredOrphan is the transfer reason for channels found alive on
// boot without a snapshot: hand them to a human, never improvise (nota 07).
const TransferRecoveredOrphan event.TransferReason = "recovered_orphan"

// CallLoader loads the folded state of one call. The gateway keys calls by
// ARI channel id on this backend, so channelID == callID. *store.Mem and the
// Postgres store satisfy it via their GetCall method (this package depends
// only on this consumer-side interface, never on the store package).
type CallLoader interface {
	GetCall(ctx context.Context, callID string) (domain.Call, error)
}

// ClipSource looks up pre-synthesized phrases. *audiobank.Bank satisfies it
// (same contract as internal/turn.ClipSource).
type ClipSource interface {
	Lookup(key audiobank.Key, loc event.Locale) (audiobank.Clip, error)
}

// ClipPlayer plays one audio-bank clip on a channel. The wiring implements
// it over the call's media session (or an ARI sound URI); tests fake it.
type ClipPlayer interface {
	PlayClip(ctx context.Context, channelID string, clip audiobank.Clip) error
}

// Transferer is the slice of the call-control contract rehydration needs
// for the orphan path (telnyx.CallControl satisfies it).
type Transferer interface {
	Transfer(ctx context.Context, callControlID, to string) error
	Hangup(ctx context.Context, callControlID string) error
}

var _ Transferer = (telnyx.CallControl)(nil)

// RehydrateHooks observe the boot recovery; the wiring uses them to emit
// business events (transfer.initiated with reason recovered_orphan, metrics
// like recovery_orphan_rate). All fields are optional.
type RehydrateHooks struct {
	// OnResumed fires after a channel with a snapshot was re-anchored.
	OnResumed func(channelID string, snap event.CallSnapshot)
	// OnOrphan fires after a snapshotless channel was transferred to the
	// owner's cell.
	OnOrphan func(channelID string)
	// OnStale fires after a channel whose snapshot says the call already
	// ended was hung up.
	OnStale func(channelID string)
}

// RehydratorConfig wires a Rehydrator.
type RehydratorConfig struct {
	// ARI lists the live channels (only LiveChannels is used).
	ARI ARI
	// Calls loads snapshots (state = fold(events)).
	Calls CallLoader
	// Clips provides the re-anchoring phrase.
	Clips ClipSource
	// Player plays the re-anchoring clip on the channel.
	Player ClipPlayer
	// Control executes the orphan transfer / stale hangup.
	Control Transferer
	// OwnerCell is the human failover destination.
	OwnerCell string
	// ReanchorKey selects the audio-bank phrase played before listening
	// again ("never resume mid-turn"). Defaults to say_again.
	ReanchorKey audiobank.Key
	Hooks       RehydrateHooks
}

// Rehydrator implements the boot recovery of nota 07: Asterisk is the source
// of truth for which channels exist; our stores only say how far each call
// got.
type Rehydrator struct {
	cfg RehydratorConfig
}

// NewRehydrator validates the wiring and returns a Rehydrator.
func NewRehydrator(cfg RehydratorConfig) (*Rehydrator, error) {
	switch {
	case cfg.ARI == nil:
		return nil, fmt.Errorf("asterisk: rehydrator: ARI is required")
	case cfg.Calls == nil:
		return nil, fmt.Errorf("asterisk: rehydrator: Calls is required")
	case cfg.Clips == nil:
		return nil, fmt.Errorf("asterisk: rehydrator: Clips is required")
	case cfg.Player == nil:
		return nil, fmt.Errorf("asterisk: rehydrator: Player is required")
	case cfg.Control == nil:
		return nil, fmt.Errorf("asterisk: rehydrator: Control is required")
	case cfg.OwnerCell == "":
		return nil, fmt.Errorf("asterisk: rehydrator: OwnerCell is required")
	}
	if cfg.ReanchorKey == "" {
		cfg.ReanchorKey = audiobank.KeySayAgain
	}
	return &Rehydrator{cfg: cfg}, nil
}

// RehydrateReport summarizes one boot recovery.
type RehydrateReport struct {
	// Resumed channels had a snapshot and were re-anchored.
	Resumed []string
	// Orphaned channels had no usable snapshot and went to the owner's cell.
	Orphaned []string
	// Stale channels had an already-ended snapshot and were hung up.
	Stale []string
}

// Rehydrate lists the live channels of the Stasis app and resumes each one:
//
//   - snapshot found → play the re-anchoring phrase (audio bank, in the
//     call's locale) and hand the channel back to the turn loop. Never
//     resume mid-turn, never re-run a side-effecting node.
//   - no snapshot (or any load/clip failure) → transfer to the owner's cell
//     as recovered_orphan. When in doubt, a human — never improvisation.
//
// A failing channel never aborts the others; the first error is returned
// after every channel was handled.
func (r *Rehydrator) Rehydrate(ctx context.Context) (RehydrateReport, error) {
	var report RehydrateReport

	channels, err := r.cfg.ARI.LiveChannels()
	if err != nil {
		return report, fmt.Errorf("asterisk: rehydrate: list live channels: %w", err)
	}

	var firstErr error
	for _, chID := range channels {
		outcome, err := r.rehydrateChannel(ctx, chID)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		switch outcome {
		case outcomeResumed:
			report.Resumed = append(report.Resumed, chID)
		case outcomeOrphaned:
			report.Orphaned = append(report.Orphaned, chID)
		case outcomeStale:
			report.Stale = append(report.Stale, chID)
		}
	}
	return report, firstErr
}

type rehydrateOutcome int

const (
	outcomeResumed rehydrateOutcome = iota
	outcomeOrphaned
	outcomeStale
)

// rehydrateChannel recovers one channel. Any failure on the resume path
// degrades to the orphan transfer — the call must never sit in silence.
func (r *Rehydrator) rehydrateChannel(ctx context.Context, channelID string) (rehydrateOutcome, error) {
	call, err := r.cfg.Calls.GetCall(ctx, channelID)
	if err != nil {
		// Not found or store failure: either way there is no state to
		// resume from — recovered_orphan.
		return outcomeOrphaned, r.orphan(ctx, channelID)
	}
	snap := call.Snapshot

	if snap.Status == event.StatusEnded || snap.Status == event.StatusTransferred {
		// The call already finished; the channel is a leftover.
		if err := r.cfg.Control.Hangup(ctx, channelID); err != nil {
			return outcomeStale, fmt.Errorf("asterisk: rehydrate %s: hangup stale channel: %w", channelID, err)
		}
		if r.cfg.Hooks.OnStale != nil {
			r.cfg.Hooks.OnStale(channelID)
		}
		return outcomeStale, nil
	}

	locale := snap.Locale
	if locale == "" {
		locale = event.LocaleENCA
	}
	clip, err := r.cfg.Clips.Lookup(r.cfg.ReanchorKey, locale)
	if err != nil {
		// The audio bank is critical infrastructure; without the phrase we
		// cannot re-anchor honestly — degrade to a human.
		return outcomeOrphaned, r.orphan(ctx, channelID)
	}
	if err := r.cfg.Player.PlayClip(ctx, channelID, clip); err != nil {
		return outcomeOrphaned, r.orphan(ctx, channelID)
	}
	if r.cfg.Hooks.OnResumed != nil {
		r.cfg.Hooks.OnResumed(channelID, snap)
	}
	return outcomeResumed, nil
}

// orphan transfers a channel to the owner's cell (reason recovered_orphan).
func (r *Rehydrator) orphan(ctx context.Context, channelID string) error {
	if err := r.cfg.Control.Transfer(ctx, channelID, r.cfg.OwnerCell); err != nil {
		return fmt.Errorf("asterisk: rehydrate %s: orphan transfer: %w", channelID, err)
	}
	if r.cfg.Hooks.OnOrphan != nil {
		r.cfg.Hooks.OnOrphan(channelID)
	}
	return nil
}
