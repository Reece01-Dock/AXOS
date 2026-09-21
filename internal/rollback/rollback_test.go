package rollback

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestArmConfirm_KeepsChange(t *testing.T) {
	e := New()
	var restored int32

	id, err := e.Arm(60, "test change", func(ctx context.Context) error {
		atomic.AddInt32(&restored, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("Arm: %v", err)
	}

	st := e.Status()
	if !st.Pending || st.ID != id {
		t.Fatalf("Status after Arm = %+v, want pending id=%s", st, id)
	}

	if err := e.Confirm(id); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	if st := e.Status(); st.Pending {
		t.Fatalf("Status after Confirm = %+v, want not pending", st)
	}

	// Restore must never fire after a successful confirm, even if we wait
	// past where the original deadline would have been.
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&restored); got != 0 {
		t.Fatalf("restore called %d times after Confirm, want 0", got)
	}
}

func TestArmExpiry_AutoRestores(t *testing.T) {
	e := New()
	restoredCh := make(chan struct{}, 1)

	id, err := e.Arm(1, "risky change", func(ctx context.Context) error {
		restoredCh <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("Arm: %v", err)
	}

	select {
	case <-restoredCh:
		// good: auto-restored
	case <-time.After(3 * time.Second):
		t.Fatal("restore was not called within expected window after timeout")
	}

	if st := e.Status(); st.Pending {
		t.Fatalf("Status after expiry = %+v, want not pending", st)
	}

	// Confirming after expiry must fail — the AI cannot "confirm" its way
	// out of a change that already got reverted.
	if err := e.Confirm(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm after expiry = %v, want ErrNotFound", err)
	}
}

func TestArm_RejectsConcurrentTransaction(t *testing.T) {
	e := New()
	noop := func(ctx context.Context) error { return nil }

	id1, err := e.Arm(60, "first", noop)
	if err != nil {
		t.Fatalf("first Arm: %v", err)
	}

	_, err = e.Arm(60, "second", noop)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("second Arm = %v, want ErrBusy", err)
	}

	// Confirming the first must free things up for a new Arm.
	if err := e.Confirm(id1); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if _, err := e.Arm(60, "third", noop); err != nil {
		t.Fatalf("Arm after Confirm: %v", err)
	}
}

func TestCancel_RestoresImmediately(t *testing.T) {
	e := New()
	var restored int32

	id, err := e.Arm(120, "will be cancelled", func(ctx context.Context) error {
		atomic.AddInt32(&restored, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("Arm: %v", err)
	}

	if err := e.Cancel(context.Background(), id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got := atomic.LoadInt32(&restored); got != 1 {
		t.Fatalf("restore called %d times after Cancel, want 1", got)
	}
	if st := e.Status(); st.Pending {
		t.Fatalf("Status after Cancel = %+v, want not pending", st)
	}
}

func TestConfirm_UnknownID(t *testing.T) {
	e := New()
	if err := e.Confirm("does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm(unknown) = %v, want ErrNotFound", err)
	}
}

func TestArm_ValidatesInputs(t *testing.T) {
	e := New()
	if _, err := e.Arm(0, "x", func(context.Context) error { return nil }); err == nil {
		t.Fatal("Arm with zero timeout should fail")
	}
	if _, err := e.Arm(10, "x", nil); err == nil {
		t.Fatal("Arm with nil restore func should fail")
	}
}

func TestExpiry_RestoreFailureIsReported(t *testing.T) {
	e := New()
	var gotEvent Event
	done := make(chan struct{})
	e.OnEvent(func(ev Event) {
		if ev.Kind == EventExpiredRestoreFailed {
			gotEvent = ev
			close(done)
		}
	})

	wantErr := errors.New("management interface unreachable")
	_, err := e.Arm(1, "bad change", func(ctx context.Context) error {
		return wantErr
	})
	if err != nil {
		t.Fatalf("Arm: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("expected EventExpiredRestoreFailed within timeout window")
	}

	if gotEvent.Detail != wantErr.Error() {
		t.Fatalf("event detail = %q, want %q", gotEvent.Detail, wantErr.Error())
	}
}

// TestOnlyOutsideConfirmMatters exercises the core safety property described
// in docs/safety-rollback.md: if the entity that armed the transaction loses
// the ability to confirm (simulated here by simply never calling Confirm),
// the system reverts on its own without any other actor's intervention.
func TestOnlyOutsideConfirmMatters(t *testing.T) {
	e := New()
	restored := make(chan struct{}, 1)

	_, err := e.Arm(1, "AI locks itself out", func(ctx context.Context) error {
		restored <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("Arm: %v", err)
	}

	// Simulate "AI never confirms" by doing nothing and waiting.
	select {
	case <-restored:
		// Connectivity (simulated) is restored automatically.
	case <-time.After(3 * time.Second):
		t.Fatal("expected automatic restore when nobody confirms")
	}
}
