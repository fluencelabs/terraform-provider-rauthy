package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSubstringGuardDeletionAdvice(t *testing.T) {
	for _, tc := range []struct {
		name, kind, short, other, want, forbidden string
	}{
		{
			name: "protected role", kind: "role", short: "rauthy", other: "rauthy_admin",
			want:      "The built-in rauthy_admin role cannot be deleted. Keep this role managed without deleting it.",
			forbidden: `Delete "rauthy_admin"`,
		},
		{
			name: "custom role", kind: "role", short: "rauthy", other: "rauthy_custom",
			want: `Delete "rauthy_custom" in a separate apply first, then retry.`,
		},
		{
			name: "group", kind: "group", short: "rauthy", other: "rauthy_admin",
			want: `Delete "rauthy_admin" in a separate apply first, then retry.`,
		},
		{
			name: "protected scope", kind: "scope", short: "id", other: "openid",
			want:      "The built-in openid scope cannot be removed or renamed. Keep this scope managed without deleting it.",
			forbidden: `Delete "openid"`,
		},
		{
			name: "mutable default scope", kind: "scope", short: "file", other: "profile",
			want: `Delete "profile" in a separate apply first, then retry.`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, api := newFakeRauthy(t)
			f.answer("GET /"+tc.kind+"s", []map[string]string{
				{"id": "target", "name": tc.short}, {"id": "other", "name": tc.other},
			})
			r, model := guardedResource(tc.kind, api, types.StringValue(tc.short))
			state := resourceState(t, r, model)
			ctx := context.Background()
			var planResp resource.ModifyPlanResponse
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: state, Plan: tfsdk.Plan(emptyState(t, r))}, &planResp)
			checkDeletionAdvice(t, planResp.Diagnostics, tc.want, tc.forbidden)
			var deleteResp resource.DeleteResponse
			r.Delete(ctx, resource.DeleteRequest{State: state}, &deleteResp)
			checkDeletionAdvice(t, deleteResp.Diagnostics, tc.want, tc.forbidden)
			if len(f.bodies("DELETE /"+tc.kind+"s/target")) != 0 {
				t.Fatal("unsafe DELETE was sent")
			}
		})
	}
}

func checkDeletionAdvice(t *testing.T, diags diag.Diagnostics, want, forbidden string) {
	t.Helper()
	if !diags.HasError() || len(diags) != 1 {
		t.Fatalf("expected one guard error: %v", diags)
	}
	detail := diags[0].Detail()
	if !strings.Contains(detail, want) {
		t.Errorf("advice = %q, want %q", detail, want)
	}
	if forbidden != "" && strings.Contains(detail, forbidden) {
		t.Errorf("impossible advice: %s", detail)
	}
}
