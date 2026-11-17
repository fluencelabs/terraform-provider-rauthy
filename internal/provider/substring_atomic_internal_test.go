package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fluencelabs/rauthy-go"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Every entry can introduce a conflicting name or perform a guarded rewrite.
// Null input lets the call finish without HTTP once the shared lock is released.
func TestSubstringMutationParticipants(t *testing.T) {
	for _, tc := range []struct {
		r   resource.Resource
		ops []string
	}{
		{&scopeResource{}, []string{"Create", "Update", "Delete"}},
		{&roleResource{}, []string{"Create", "Delete"}},
		{&groupResource{}, []string{"Create", "Delete"}},
		{&userAttributeResource{}, []string{"Create", "Update", "Delete"}},
		{&userResource{}, []string{"Create", "Update"}},
		{&clientResource{}, []string{"Create", "Update"}},
	} {
		for _, op := range tc.ops {
			t.Run(reflect.TypeOf(tc.r).Elem().Name()+"/"+op, func(t *testing.T) {
				testMutationParticipant(t, tc.r, op)
			})
		}
	}
}

func testMutationParticipant(t *testing.T, r resource.Resource, op string) {
	t.Helper()
	null := emptyState(t, r)
	done := make(chan struct{})
	substringMutationMu.Lock()
	go func() {
		defer close(done)
		switch op {
		case "Create":
			r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan(null)}, &resource.CreateResponse{})
		case "Update":
			r.Update(
				context.Background(),
				resource.UpdateRequest{Plan: tfsdk.Plan(null), State: null},
				&resource.UpdateResponse{},
			)
		case "Delete":
			r.Delete(context.Background(), resource.DeleteRequest{State: null}, &resource.DeleteResponse{})
		}
	}()
	blocked := waitForMutation(t, reflect.TypeOf(r).Elem().Name()+")."+op, done, nil)
	substringMutationMu.Unlock()
	<-done
	if !blocked {
		t.Fatal("resource mutation bypassed the shared lock")
	}
}

// Observe the contender at the shared mutex or a controlled server write.
// The server signal also drives the corrupting interleaving on the unfixed code.
func waitForMutation(t *testing.T, frame string, done, reached <-chan struct{}) bool {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		stack := make([]byte, 1<<20)
		stack = stack[:runtime.Stack(stack, true)]
		for goroutine := range strings.SplitSeq(string(stack), "\n\n") {
			if strings.Contains(goroutine, frame) &&
				strings.Contains(goroutine, "sync.(*Mutex).Lock") {
				return true
			}
		}
		select {
		case <-reached:
			return true
		case <-done:
			return false
		default:
			runtime.Gosched()
		}
	}
	return false
}

func TestUserAttributeConcurrentScopeMapping(t *testing.T) {
	f, api := newFakeRauthy(t)
	// Distinct configured clients in one process must share the provider lock too.
	writerAPI, err := rauthy.New(api.BaseURL().Reveal(), "test$key")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	mapping := ""
	snapshot := make(chan struct{})
	releaseSnapshot := make(chan struct{})
	deleted := make(chan struct{})
	putStarted := make(chan struct{})
	var once sync.Once
	f.answer("GET /users/attr", map[string]any{"values": []rauthy.UserAttrResponse{{Name: "dev"}, {Name: "developer"}}})
	f.handle("GET /scopes", func(w http.ResponseWriter, _ *http.Request) {
		first := false
		once.Do(func() { first = true; close(snapshot) })
		if first {
			<-releaseSnapshot
		}
		_ = json.NewEncoder(w).Encode([]rauthy.ScopeResponse{{ID: "s", Name: "mapped"}})
	})
	f.handle("PUT /scopes/s", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		mapping = "developer"
		mu.Unlock()
		close(putStarted)
		<-deleted
		_ = json.NewEncoder(w).
			Encode(rauthy.ScopeResponse{ID: "s", Name: "mapped", AttrIncludeAccess: []string{"developer"}})
	})
	f.handle("DELETE /users/attr/dev", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		mapping = strings.ReplaceAll(strings.ReplaceAll(mapping, "dev,", ""), "dev", "")
		mu.Unlock()
		close(deleted)
		w.WriteHeader(http.StatusOK)
	})
	target := &userAttributeResource{api: api}
	state := resourceState(
		t,
		target,
		userAttributeResourceModel{ID: types.StringValue("dev"), Name: types.StringValue("dev")},
	)
	deletion := make(chan resource.DeleteResponse, 1)
	go func() {
		resp := resource.DeleteResponse{State: state}
		target.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
		deletion <- resp
	}()
	<-snapshot
	writer := &scopeResource{api: writerAPI}
	plan := resourcePlan(t, writer, scopeResourceModel{
		ID:   types.StringValue("s"),
		Name: types.StringValue("mapped"),
		AttrIncludeAccess: setOf(
			t,
			"developer",
		),
		AttrIncludeID: types.SetNull(types.StringType),
		ClaimsAtRoot:  types.BoolValue(false),
	})
	written := make(chan resource.UpdateResponse, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		resp := resource.UpdateResponse{State: emptyState(t, writer)}
		writer.Update(context.Background(), resource.UpdateRequest{Plan: plan}, &resp)
		written <- resp
	}()
	observed := waitForMutation(t, "scopeResource).Update", done, putStarted)
	close(releaseSnapshot)
	dr, wr := <-deletion, <-written
	if !observed {
		t.Fatal("did not observe concurrent scope update")
	}
	if dr.Diagnostics.HasError() || wr.Diagnostics.HasError() {
		t.Fatalf("delete=%v update=%v", dr.Diagnostics, wr.Diagnostics)
	}
	mu.Lock()
	defer mu.Unlock()
	if mapping != "developer" {
		t.Fatalf("concurrent mapping corrupted: %q", mapping)
	}
}
