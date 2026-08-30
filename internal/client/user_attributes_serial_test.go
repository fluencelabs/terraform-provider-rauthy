package client_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fluencelabs/terraform-provider-rauthy/internal/client"
)

// Rauthy rewrites the whole user attribute list on every write without locking,
// so two overlapping writes lose one another — see the note in
// user_attributes.go. The client serialises them; this pins that it really
// does, because the symptom on a live instance is silent (a 200 and a missing
// attribute) and Terraform creates independent resources ten at a time.
func TestUserAttrWritesAreSerialised(t *testing.T) {
	t.Parallel()

	var inFlight, overlaps atomic.Int32

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if inFlight.Add(1) > 1 {
			overlaps.Add(1)
		}
		// Wide enough that an unserialised caller would certainly overlap.
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"department","user_editable":false}`))
	})

	const writers = 8
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			var err error
			switch i % 3 {
			case 0:
				_, err = c.CreateUserAttr(context.Background(),
					client.UserAttrRequest{Name: "department"})
			case 1:
				err = c.UpdateUserAttr(context.Background(), "department",
					client.UserAttrRequest{Name: "department"})
			default:
				err = c.DeleteUserAttr(context.Background(), "department")
			}
			if err != nil {
				t.Errorf("write: %v", err)
			}
		})
	}
	wg.Wait()

	if got := overlaps.Load(); got != 0 {
		t.Errorf("%d user attribute writes overlapped; they must be serialised", got)
	}
}

// Reads are deliberately not serialised: they do not mutate the list, and
// holding the lock over them would make a parallel refresh needlessly slow.
func TestUserAttrReadsAreNotSerialised(t *testing.T) {
	t.Parallel()

	var inFlight, overlaps atomic.Int32

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if inFlight.Add(1) > 1 {
			overlaps.Add(1)
		}
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"values":[]}`))
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := c.ListUserAttrs(context.Background()); err != nil {
				t.Errorf("list: %v", err)
			}
		})
	}
	wg.Wait()

	if overlaps.Load() == 0 {
		t.Error("no reads overlapped; the read path may have been put behind the write lock")
	}
}
