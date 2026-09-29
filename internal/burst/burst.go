// Package burst decides when a run of rate-limit errors is worth acting on.
package burst

import "time"

// Detector counts errors in a sliding window and enforces a cooldown
// between actions.
type Detector struct {
	Threshold int
	Window    time.Duration
	Cooldown  time.Duration

	events     []time.Time
	lastAction time.Time
}

// Add records n errors seen at now.
func (d *Detector) Add(n int, now time.Time) {
	for i := 0; i < n; i++ {
		d.events = append(d.events, now)
	}
}

// Count prunes events older than the window and returns what's left.
func (d *Detector) Count(now time.Time) int {
	keep := d.events[:0]
	for _, e := range d.events {
		if now.Sub(e) <= d.Window {
			keep = append(keep, e)
		}
	}
	d.events = keep
	return len(d.events)
}

// Ready reports whether there's a burst and the cooldown has passed.
func (d *Detector) Ready(now time.Time) bool {
	return d.Count(now) >= d.Threshold && now.Sub(d.lastAction) >= d.Cooldown
}

// Acted starts the cooldown and forgets the current burst.
func (d *Detector) Acted(now time.Time) {
	d.lastAction = now
	d.events = nil
}

// LastAction is when Acted was last called.
func (d *Detector) LastAction() time.Time { return d.lastAction }

// SetLastAction restores the cooldown after a restart.
func (d *Detector) SetLastAction(t time.Time) { d.lastAction = t }
