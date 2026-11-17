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
	substringMutationMu <- struct{}{}
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
	unlockSubstringMutation()
	<-done
	if !blocked {
		t.Fatal("resource mutation bypassed the shared lock")
	}
}

// Observe the contender at the shared lock or a controlled server write.
// The server signal also drives the corrupting interleaving on the unfixed code.
func waitForMutation(t *testing.T, frame string, done, reached <-chan struct{}) bool {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		stack := make([]byte, 1<<20)
		stack = stack[:runtime.Stack(stack, true)]
		for goroutine := range strings.SplitSeq(string(stack), "\n\n") {
			if strings.Contains(goroutine, frame) &&
				strings.Contains(goroutine, "lockSubstringMutation") {
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

func TestRoleDeleteHoldsSubstringLock(t *testing.T) {
	f, api := newFakeRauthy(t)
	f.answer("GET /roles", []rauthy.RoleResponse{{ID: "r", Name: "dev"}})
	deleteStarted := make(chan struct{})
	completeDelete := make(chan struct{})
	f.handle("DELETE /roles/r", func(w http.ResponseWriter, _ *http.Request) {
		close(deleteStarted)
		<-completeDelete
		w.WriteHeader(http.StatusOK)
	})
	postStarted := make(chan struct{})
	f.handle("POST /roles", func(w http.ResponseWriter, _ *http.Request) {
		close(postStarted)
		_ = json.NewEncoder(w).Encode(rauthy.RoleResponse{ID: "new", Name: "developer"})
	})

	r := &roleResource{api: api}
	ctx := t.Context()
	state := resourceState(t, r, roleResourceModel{ID: types.StringValue("r"), Name: types.StringValue("dev")})
	plan := resourcePlan(t, r, roleResourceModel{Name: types.StringValue("developer")})
	deletion := resource.DeleteResponse{State: state}
	deleteDone := make(chan struct{})
	go func() {
		defer close(deleteDone)
		r.Delete(ctx, resource.DeleteRequest{State: state}, &deletion)
	}()
	select {
	case <-deleteStarted:
	case <-deleteDone:
		t.Fatalf("role deletion failed before DELETE: %v", deletion.Diagnostics)
	}

	creation := resource.CreateResponse{State: emptyState(t, r)}
	createDone := make(chan struct{})
	go func() {
		defer close(createDone)
		r.Create(ctx, resource.CreateRequest{Plan: plan}, &creation)
	}()
	observed := waitForMutation(t, "roleResource).Create", createDone, postStarted)
	earlyPost := len(f.bodies("POST /roles")) != 0
	close(completeDelete)
	<-deleteDone
	<-createDone
	if !observed || earlyPost {
		t.Fatalf("role creation did not wait for DELETE: observed=%v early POST=%v", observed, earlyPost)
	}
	if deletion.Diagnostics.HasError() || creation.Diagnostics.HasError() || len(f.bodies("POST /roles")) != 1 {
		t.Fatalf("delete=%v create=%v", deletion.Diagnostics, creation.Diagnostics)
	}
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
	f.handle("PUT /scopes/s", func(w http.ResponseWriter, r *http.Request) {
		var req rauthy.ScopeRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			http.Error(w, decodeErr.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		mapping = strings.Join(req.AttrIncludeAccess, ",")
		mu.Unlock()
		close(putStarted)
		<-deleted
		_ = json.NewEncoder(w).
			Encode(rauthy.ScopeResponse{ID: "s", Name: "mapped", AttrIncludeAccess: req.AttrIncludeAccess})
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

func TestSubstringMutationCancellation(t *testing.T) {
	f, api := newFakeRauthy(t)
	r := &scopeResource{api: api}
	plan := resourcePlan(t, r, scopeResourceModel{
		Name:              types.StringValue("mapped"),
		AttrIncludeAccess: types.SetNull(types.StringType),
		AttrIncludeID:     types.SetNull(types.StringType),
	})
	f.answer("POST /scopes", rauthy.ScopeResponse{ID: "s", Name: "mapped"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	resp := resource.CreateResponse{State: emptyState(t, r)}
	substringMutationMu <- struct{}{}
	go func() {
		defer close(done)
		r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	}()
	waiting := waitForMutation(t, "scopeResource).Create", done, nil)
	cancel()
	returned := false
	select {
	case <-done:
		returned = true
	case <-time.After(time.Second):
	}
	unlockSubstringMutation()
	<-done
	if !waiting || !returned {
		t.Fatal("scope creation did not return on cancellation while the lock was held")
	}
	if !resp.Diagnostics.HasError() || resp.Diagnostics[0].Detail() != context.Canceled.Error() {
		t.Fatalf("expected cancellation diagnostic, got %v", resp.Diagnostics)
	}
	if len(f.bodies("POST /scopes")) != 0 {
		t.Fatal("canceled scope creation issued POST")
	}
}

func TestUserAttributeConcurrentScopeCreation(t *testing.T) {
	f, api := newFakeRauthy(t)
	writerAPI, err := rauthy.New(api.BaseURL().Reveal(), "test$key")
	if err != nil {
		t.Fatal(err)
	}
	f.answer("GET /users/attr", map[string]any{"values": []rauthy.UserAttrResponse{{Name: "dev"}}})
	f.answer("DELETE /users/attr/dev", nil)
	var scope rauthy.ScopeResponse
	postStarted := make(chan struct{})
	releasePost := make(chan struct{})
	created := make(chan struct{})
	listed := make(chan struct{})
	f.handle("POST /scopes", func(w http.ResponseWriter, r *http.Request) {
		var req rauthy.ScopeRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			http.Error(w, decodeErr.Error(), http.StatusBadRequest)
			return
		}
		close(postStarted)
		<-releasePost
		scope = rauthy.ScopeResponse{
			ID:                "s",
			Name:              req.Scope,
			AttrIncludeAccess: req.AttrIncludeAccess,
			AttrIncludeID:     req.AttrIncludeID,
			ClaimsAtRoot:      req.ClaimsAtRoot,
		}
		close(created)
		_ = json.NewEncoder(w).Encode(scope)
	})
	f.handle("GET /scopes", func(w http.ResponseWriter, _ *http.Request) {
		var scopes []rauthy.ScopeResponse
		select {
		case <-created:
			scopes = []rauthy.ScopeResponse{scope}
		default:
		}
		close(listed)
		_ = json.NewEncoder(w).Encode(scopes)
	})
	writer := &scopeResource{api: writerAPI}
	plan := resourcePlan(t, writer, scopeResourceModel{
		Name:              types.StringValue("mapped"),
		AttrIncludeAccess: setOf(t, "developer"),
		AttrIncludeID:     types.SetNull(types.StringType),
	})
	writeResp := resource.CreateResponse{State: emptyState(t, writer)}
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		writer.Create(context.Background(), resource.CreateRequest{Plan: plan}, &writeResp)
	}()
	select {
	case <-postStarted:
	case <-writeDone:
		t.Fatalf("scope creation failed before POST: %v", writeResp.Diagnostics)
	}
	target := &userAttributeResource{api: api}
	state := resourceState(t, target, userAttributeResourceModel{
		ID: types.StringValue("dev"), Name: types.StringValue("dev"),
	})
	deleteResp := resource.DeleteResponse{State: state}
	deleteDone := make(chan struct{})
	go func() {
		defer close(deleteDone)
		target.Delete(context.Background(), resource.DeleteRequest{State: state}, &deleteResp)
	}()
	observed := waitForMutation(t, "userAttributeResource).Delete", deleteDone, listed)
	close(releasePost)
	<-writeDone
	<-deleteDone
	if !observed || writeResp.Diagnostics.HasError() {
		t.Fatalf("concurrent scope creation failed: %v", writeResp.Diagnostics)
	}
	if !deleteResp.Diagnostics.HasError() || len(f.bodies("DELETE /users/attr/dev")) != 0 {
		t.Fatalf("deletion missed the new scope mapping: %v", deleteResp.Diagnostics)
	}
	<-created
	if len(scope.AttrIncludeAccess) != 1 || scope.AttrIncludeAccess[0] != "developer" {
		t.Fatalf("scope creation sent unexpected access mappings: %v", scope.AttrIncludeAccess)
	}
}
