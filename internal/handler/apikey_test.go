package handler

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminAPIKeys_OrgSafeCreateAndRevoke(t *testing.T) {
	st, org, admin, _ := newPersonalAPIKeyTestStore(t)
	h := NewAPIKeyHandler(st, slog.Default())
	ctx := adminCtx(org.ID, admin.ID)

	otherOrg, err := st.CreateOrganization(context.Background(), "Other API key org", "other-api-key-org")
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := st.CreateAccount(context.Background(), "outsider@example.test", "Outsider", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddOrgMember(context.Background(), otherOrg.ID, outsider.ID, "member"); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(`{"name":"cross-org","scope":"sync","account_id":"`+outsider.ID+`"}`)).WithContext(ctx)
	createRec := httptest.NewRecorder()
	h.Create(createRec, create)
	if createRec.Code != http.StatusNotFound {
		t.Fatalf("cross-org create = %d: %s", createRec.Code, createRec.Body.String())
	}
	reserved := httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(`{"name":"session","scope":"admin","account_id":"`+admin.ID+`"}`)).WithContext(ctx)
	reservedRec := httptest.NewRecorder()
	h.Create(reservedRec, reserved)
	if reservedRec.Code != http.StatusBadRequest {
		t.Fatalf("reserved session create = %d: %s", reservedRec.Code, reservedRec.Body.String())
	}
	keys, err := st.ListAPIKeysByOrg(context.Background(), org.ID)
	if err != nil || len(keys) != 0 {
		t.Fatalf("cross-org create changed org keys: %#v, %v", keys, err)
	}

	foreign, err := st.CreateAPIKey(context.Background(), otherOrg.ID, outsider.ID, "foreign", "anc_live_", "foreign-hash", "sync", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{foreign.ID, "00000000-0000-0000-0000-000000000000"} {
		req := httptest.NewRequest(http.MethodDelete, "/v1/api-keys/"+id, nil).WithContext(ctx)
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		h.Revoke(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("cross-org revoke %s = %d: %s", id, rec.Code, rec.Body.String())
		}
	}
	foreignKeys, err := st.ListAPIKeysByOwner(context.Background(), otherOrg.ID, outsider.ID)
	if err != nil || len(foreignKeys) != 1 || foreignKeys[0].RevokedAt != nil {
		t.Fatalf("foreign key changed by org-A revoke: %#v, %v", foreignKeys, err)
	}

	if err := st.AddOrgMember(context.Background(), org.ID, outsider.ID, "member"); err != nil {
		t.Fatal(err)
	}
	create = httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(`{"name":"member","scope":"readonly","account_id":"`+outsider.ID+`"}`)).WithContext(ctx)
	createRec = httptest.NewRecorder()
	h.Create(createRec, create)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("member create = %d: %s", createRec.Code, createRec.Body.String())
	}
}
