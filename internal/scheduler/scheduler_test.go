package scheduler

import (
	"context"
	"testing"
	"time"
)

func TestSchedulerRunsJobImmediatelyAndStopsOnCancel(t *testing.T) {
	clock := newTestClock()
	s := New(clock)

	ctx, cancel := context.WithCancel(context.Background())
	done := s.Start(ctx, []Job{
		{
			Interval: time.Second,
			Run: func(context.Context) {
				clock.runs <- struct{}{}
			},
		},
	})

	waitForSignal(t, clock.runs)

	ticker := clock.lastTicker()
	ticker.Tick(time.Unix(1, 0))
	waitForSignal(t, clock.runs)

	cancel()
	waitForSignal(t, done)

	ticker.Tick(time.Unix(2, 0))
	assertNoSignal(t, clock.runs)
}

type testClock struct {
	tickers []*testTicker
	runs    chan struct{}
}

func newTestClock() *testClock {
	return &testClock{
		runs: make(chan struct{}, 8),
	}
}

func (c *testClock) NewTicker(time.Duration) Ticker {
	ticker := &testTicker{
		ch: make(chan time.Time, 8),
	}
	c.tickers = append(c.tickers, ticker)
	return ticker
}

func (c *testClock) lastTicker() *testTicker {
	if len(c.tickers) == 0 {
		return nil
	}
	return c.tickers[len(c.tickers)-1]
}

type testTicker struct {
	ch chan time.Time
}

func (t *testTicker) C() <-chan time.Time {
	return t.ch
}

func (t *testTicker) Stop() {}

func (t *testTicker) Tick(at time.Time) {
	t.ch <- at
}

func waitForSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for signal")
	}
}

func assertNoSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("unexpected signal")
	case <-time.After(100 * time.Millisecond):
	}
}
