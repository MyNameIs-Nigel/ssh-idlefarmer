package game

import (
	"errors"
	"sync"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/sim"
)

// ErrSessionClosed is returned when an intent arrives after the session's
// actor has stopped (the player was kicked or the server is shutting down).
var ErrSessionClosed = errors.New("session is closed")

// Session is one terminal's handle onto a save actor. The UI sends intents
// through it; every game mutation happens on the actor goroutine against
// authoritative state, and the UI only ever sees deep-copied snapshots.
type Session struct {
	id       uint64
	actor    *actor
	kicked   chan string
	kickFn   func(reason string)
	kickOnce sync.Once
}

// Snapshot is a point-in-time deep copy of the save for rendering. Mutating
// it has no effect on the game.
type Snapshot struct {
	State *sim.State
	Now   int64
}

// Kicked yields the takeover/shutdown notice for this session. The channel
// delivers at most one reason and is then closed; it also closes (without a
// reason) when the session detaches normally, so listeners never leak.
func (s *Session) Kicked() <-chan string { return s.kicked }

// deliverKick is safe from any goroutine; an empty reason just closes the
// channel (normal detach).
func (s *Session) deliverKick(reason string) {
	s.kickOnce.Do(func() {
		if reason != "" {
			s.kicked <- reason
			if s.kickFn != nil {
				s.kickFn(reason)
			}
		}
		close(s.kicked)
	})
}

// Detach persists the save and releases this session. The actor stops once
// no session is attached. Safe to call more than once.
func (s *Session) Detach() {
	s.actor.mgr.detach(s)
	s.deliverKick("")
}

// Advance simulates up to now and returns a fresh snapshot plus whatever
// happened. This is the live tick; it is the same engine path as offline
// catch-up.
func (s *Session) Advance(now int64) (Snapshot, sim.Events, error) {
	var snap Snapshot
	var ev sim.Events
	ok := s.actor.do(func() {
		ev = sim.Advance(s.actor.state, s.actor.content(), now)
		if ev.Elapsed > 0 {
			s.actor.dirty = true
		}
		snap = Snapshot{State: s.actor.state.Clone(), Now: now}
	})
	if !ok {
		return Snapshot{}, sim.Events{}, ErrSessionClosed
	}
	return snap, ev, nil
}

// intent runs a mutating action on the actor goroutine: advance to now,
// apply, re-check achievements, snapshot. Returns the achievements newly
// earned by the action itself.
func (s *Session) intent(now int64, apply func(st *sim.State) error) (Snapshot, []string, error) {
	var snap Snapshot
	var newly []string
	var actErr error
	ok := s.actor.do(func() {
		c := s.actor.content()
		ev := sim.Advance(s.actor.state, c, now)
		if ev.Elapsed > 0 {
			s.actor.dirty = true
		}
		actErr = apply(s.actor.state)
		if actErr == nil {
			s.actor.dirty = true
			newly = s.actor.state.CheckAchievements(c, now)
		}
		snap = Snapshot{State: s.actor.state.Clone(), Now: now}
	})
	if !ok {
		return Snapshot{}, nil, ErrSessionClosed
	}
	return snap, newly, actErr
}

// Plant plants cropID on the given plot.
func (s *Session) Plant(now int64, plot int, cropID string) (Snapshot, []string, error) {
	return s.intent(now, func(st *sim.State) error {
		return sim.Plant(st, s.actor.content(), plot, cropID, now)
	})
}

// Harvest gathers the given plot.
func (s *Session) Harvest(now int64, plot int) (sim.HarvestResult, Snapshot, []string, error) {
	var res sim.HarvestResult
	snap, newly, err := s.intent(now, func(st *sim.State) error {
		var herr error
		res, herr = sim.Harvest(st, s.actor.content(), plot, now)
		return herr
	})
	return res, snap, newly, err
}

// BuyPlot purchases the next plot, returning what it cost.
func (s *Session) BuyPlot(now int64) (int64, Snapshot, []string, error) {
	var cost int64
	snap, newly, err := s.intent(now, func(st *sim.State) error {
		var berr error
		cost, berr = sim.BuyPlot(st, s.actor.content())
		return berr
	})
	return cost, snap, newly, err
}

// BuyTool purchases a run-scoped automation tool.
func (s *Session) BuyTool(now int64, id string) (Snapshot, []string, error) {
	return s.intent(now, func(st *sim.State) error {
		return sim.BuyTool(st, s.actor.content(), id)
	})
}

// BuyZone purchases a zone expansion.
func (s *Session) BuyZone(now int64, id string) (Snapshot, []string, error) {
	return s.intent(now, func(st *sim.State) error {
		return sim.BuyZone(st, s.actor.content(), id)
	})
}

// BuyUpgrade spends prestige currency on a permanent upgrade.
func (s *Session) BuyUpgrade(now int64, id string) (Snapshot, []string, error) {
	return s.intent(now, func(st *sim.State) error {
		return sim.BuyUpgrade(st, s.actor.content(), id)
	})
}

// Rebirth resets the run for prestige currency, returning what was gained.
func (s *Session) Rebirth(now int64) (int64, Snapshot, []string, error) {
	var gain int64
	snap, newly, err := s.intent(now, func(st *sim.State) error {
		var rerr error
		gain, rerr = sim.Rebirth(st, s.actor.content(), now)
		return rerr
	})
	return gain, snap, newly, err
}

// SetFlavor toggles ambient discoveries for this save.
func (s *Session) SetFlavor(now int64, enabled bool) (Snapshot, error) {
	snap, _, err := s.intent(now, func(st *sim.State) error {
		sim.SetFlavor(st, enabled)
		return nil
	})
	return snap, err
}
