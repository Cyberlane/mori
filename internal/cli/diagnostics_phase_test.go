package cli

import (
	"testing"
	"time"

	"github.com/Cyberlane/mori/internal/analyzer"
)

func TestDiagnosticsPreservesObservedZeroDurationPhases(t *testing.T) {
	// On coarse clocks an executed phase may measure exactly zero nanoseconds.
	// Presence records execution, independently of the clock's resolution.
	capture := scanDiagnostics{started: time.Now(), discoveryObserved: true, timings: analyzer.PhaseTimings{ParseObserved: true, CompareObserved: true}}
	session := capture.session(exitSuccess)
	if len(session.Phases) != 4 {
		t.Fatalf("executed phases omitted: %+v", session.Phases)
	}
	for i, name := range []string{"total", "discovery", "parse", "compare"} {
		if session.Phases[i].Name != name {
			t.Fatalf("phase %d: %+v", i, session.Phases[i])
		}
		if i > 0 && session.Phases[i].Milliseconds != 0 {
			t.Fatalf("zero duration changed: %+v", session.Phases[i])
		}
	}
	// No observations means early failure or cached results. Neither case is
	// evidence that discovery/parsing/comparison happened in this invocation.
	capture.discoveryObserved = false
	capture.timings = analyzer.PhaseTimings{}
	session = capture.session(exitSuccess)
	if len(session.Phases) != 1 || session.Phases[0].Name != "total" {
		t.Fatalf("unexecuted phases invented: %+v", session.Phases)
	}
}
