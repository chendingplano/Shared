package llm

import (
	"context"
	"testing"
	"time"
)

type fakeModelPermitTimer struct {
	stopped bool
	fire    func()
}

func (t *fakeModelPermitTimer) Stop() bool {
	wasActive := !t.stopped
	t.stopped = true
	return wasActive
}

func (t *fakeModelPermitTimer) Fire() {
	if t.stopped || t.fire == nil {
		return
	}
	t.stopped = true
	t.fire()
}

type fakeModelPermitScheduler struct {
	timers []*fakeModelPermitTimer
}

func (s *fakeModelPermitScheduler) Schedule(_ time.Duration, fn func()) modelPermitTimer {
	timer := &fakeModelPermitTimer{fire: fn}
	s.timers = append(s.timers, timer)
	return timer
}

func (s *fakeModelPermitScheduler) FireNext() {
	if len(s.timers) == 0 {
		return
	}
	timer := s.timers[0]
	s.timers = s.timers[1:]
	timer.Fire()
}

func TestModelPermitControllerBlocksPerModel(t *testing.T) {
	controller := newModelPermitController(modelPermitControllerConfig{
		DefaultLimit: 1,
		LeaseTTL:     320 * time.Second,
	})

	first, err := controller.Acquire(context.Background(), "deepseek-v4-flash")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Release()

	done := make(chan error, 1)
	go func() {
		second, err := controller.Acquire(context.Background(), "deepseek-v4-flash")
		if err == nil && second != nil {
			second.Release()
		}
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("second acquire should block until release, got early err=%v", err)
	case <-time.After(150 * time.Millisecond):
	}

	first.Release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second Acquire after release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocked acquire to continue after release")
	}
}

func TestModelPermitControllerLeaseExpiryReleasesPermit(t *testing.T) {
	scheduler := &fakeModelPermitScheduler{}
	controller := newModelPermitController(modelPermitControllerConfig{
		DefaultLimit: 1,
		LeaseTTL:     320 * time.Second,
		Schedule:     scheduler.Schedule,
	})

	first, err := controller.Acquire(context.Background(), "deepseek-v4-flash")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		second, err := controller.Acquire(context.Background(), "deepseek-v4-flash")
		if err == nil && second != nil {
			second.Release()
		}
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("second acquire should block before lease expiry, got early err=%v", err)
	case <-time.After(150 * time.Millisecond):
	}

	scheduler.FireNext()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second Acquire after lease expiry: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for lease expiry to release permit")
	}

	first.Release()
}
