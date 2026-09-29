package burst

import (
	"testing"
	"time"
)

func TestDetector(t *testing.T) {
	t0 := time.Unix(1_000_000, 0)
	d := &Detector{Threshold: 5, Window: 120 * time.Second, Cooldown: 300 * time.Second}
	d.Add(4, t0)
	if d.Ready(t0) {
		t.Fatal("4 errors is below the threshold")
	}
	d.Add(1, t0.Add(10*time.Second))
	if !d.Ready(t0.Add(10 * time.Second)) {
		t.Fatal("5 errors should trigger")
	}
	d.Acted(t0.Add(10 * time.Second))
	d.Add(10, t0.Add(60*time.Second))
	if d.Ready(t0.Add(60 * time.Second)) {
		t.Fatal("cooldown should suppress a second action")
	}
	if d.Count(t0.Add(310*time.Second)) != 0 {
		t.Fatal("old events should be pruned")
	}
	d.Add(5, t0.Add(400*time.Second))
	if !d.Ready(t0.Add(400 * time.Second)) {
		t.Fatal("new burst after cooldown should trigger")
	}
}
