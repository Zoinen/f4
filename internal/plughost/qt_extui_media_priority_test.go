package plughost

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMediaAdmissionPromotesQueuedResource(t *testing.T) {
	var admission extUiMediaAdmission
	for range 7 {
		release, err := admission.acquire(t.Context(), "background")
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	entered := make(chan func(), 1)
	go func() {
		release, err := admission.acquire(ctx, "opened")
		if err == nil {
			entered <- release
		}
	}()
	select {
	case release := <-entered:
		release()
		t.Fatal("background request used the reserved slot")
	case <-time.After(10 * time.Millisecond):
	}
	admission.setFocus("opened")
	select {
	case release := <-entered:
		admission.setFocus("")
		release()
	case <-ctx.Done():
		t.Fatal("opened image did not use reserved slot while background reads stayed active")
	}
	if admission.active != 7 || admission.background != 7 {
		t.Fatalf("focus change corrupted accounting: %+v", &admission)
	}
}

func TestMediaAdmissionCancelledWaiterDoesNotConsumePermit(t *testing.T) {
	var admission extUiMediaAdmission
	for range 7 {
		release, err := admission.acquire(t.Context(), "background")
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if _, err := admission.acquire(ctx, "waiting"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting admission: %v", err)
	}
	admission.setFocus("opened")
	release, err := admission.acquire(t.Context(), "opened")
	if err != nil {
		t.Fatal(err)
	}
	release()
	if admission.active != 7 || admission.background != 7 {
		t.Fatalf("cancelled waiter consumed a permit")
	}
}
