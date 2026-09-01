package logging

import (
	"testing"
	"time"
)

func TestSamplerPassesBurstThenOnePerWindow(t *testing.T) {
	sampler := NewSampler(3, time.Minute)
	current := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	sampler.now = func() time.Time { return current }

	for range 3 {
		if !sampler.Allow() {
			t.Fatal("burst events must pass")
		}
	}
	if sampler.Allow() {
		t.Fatal("event beyond burst must be suppressed inside the window")
	}
	current = current.Add(30 * time.Second)
	if sampler.Allow() {
		t.Fatal("event before the window must be suppressed")
	}
	current = current.Add(31 * time.Second)
	if !sampler.Allow() {
		t.Fatal("first event after the window must pass")
	}
	if sampler.Allow() {
		t.Fatal("second event right after the window must be suppressed")
	}
}

func TestSamplerRejectsInvalidConfiguration(t *testing.T) {
	zero := NewSampler(0, 0)
	if !zero.Allow() {
		t.Fatal("sampler must clamp burst to at least one")
	}
	if zero.Allow() {
		t.Fatal("sampler must clamp window to a positive duration")
	}
}
