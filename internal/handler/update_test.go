package handler

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeUpdater is a controllable updaterIface for handler tests.
type fakeUpdater struct {
	available bool
	checkErr  error
	applyErr  error

	// block, when non-nil, holds Apply until released so a test can observe the
	// in-flight guard rejecting a concurrent request.
	block chan struct{}
}

func (f *fakeUpdater) CheckLatest(ctx context.Context) (string, string, bool, error) {
	return "v0.4.6", "v0.5.0", f.available, f.checkErr
}

func (f *fakeUpdater) Apply(ctx context.Context) error {
	if f.block != nil {
		<-f.block
	}
	return f.applyErr
}

func newUpdateHandler(f *fakeUpdater) *UpdateHandler {
	return &UpdateHandler{updater: f, logger: slog.Default()}
}

func postApply(h *UpdateHandler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/update/apply", nil)
	rec := httptest.NewRecorder()
	h.Apply(rec, req)
	return rec
}

func TestUpdateApply_AcceptedWhenAvailable(t *testing.T) {
	h := newUpdateHandler(&fakeUpdater{available: true})
	rec := postApply(h)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateApply_NoUpdateAvailable(t *testing.T) {
	h := newUpdateHandler(&fakeUpdater{available: false})
	rec := postApply(h)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateApply_CheckError(t *testing.T) {
	h := newUpdateHandler(&fakeUpdater{checkErr: context.DeadlineExceeded})
	rec := postApply(h)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", rec.Code, rec.Body.String())
	}
}

// TestUpdateApply_ConcurrentInProgress proves a second apply while one is still
// running returns 409 "update already in progress". The first apply blocks
// inside Apply (via the block channel) and holds the in-flight flag; the guard
// is set synchronously in Apply() before the goroutine is spawned, so the second
// request deterministically observes the conflict.
func TestUpdateApply_ConcurrentInProgress(t *testing.T) {
	f := &fakeUpdater{available: true, block: make(chan struct{})}
	h := newUpdateHandler(f)

	rec1 := postApply(h)
	if rec1.Code != http.StatusAccepted {
		t.Fatalf("first apply status = %d, want 202", rec1.Code)
	}

	rec2 := postApply(h)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second apply status = %d, want 409; body=%s", rec2.Code, rec2.Body.String())
	}

	// Release the first apply and wait for the guard to clear.
	close(f.block)
	for i := 0; i < 200; i++ {
		if !h.applying.Load() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("in-flight guard was never released after Apply finished")
}
