package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
	"github.com/jholhewres/anchored_oss/internal/updater"
)

// applyDelay gives the HTTP response time to flush before pm2 kills the process
// during a self-update. Without it the client would see a dropped connection
// instead of the 202 acknowledgement.
const applyDelay = 500 * time.Millisecond

// updaterIface is the subset of *updater.Updater the handler needs, extracted
// so tests can substitute a fake without real network or binary swaps.
type updaterIface interface {
	CheckLatest(ctx context.Context) (current, latest string, available bool, err error)
	Apply(ctx context.Context) error
}

type UpdateHandler struct {
	updater updaterIface
	store   store.Store // nil-safe: only used for best-effort audit entries
	logger  *slog.Logger
	// applying guards against a concurrent apply racing the binary swap: a
	// second POST while one is in flight is rejected with 409.
	applying atomic.Bool
}

func NewUpdateHandler(st store.Store, logger *slog.Logger) *UpdateHandler {
	return &UpdateHandler{updater: updater.New(logger), store: st, logger: logger}
}

type updateCheckResponse struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
}

// Check reports the running version, the latest published version, and whether
// an update is available. Admin only.
func (h *UpdateHandler) Check(w http.ResponseWriter, r *http.Request) {
	current, latest, available, err := h.updater.CheckLatest(r.Context())
	if err != nil {
		h.logger.Error("update check failed", "error", err)
		jsonError(w, http.StatusBadGateway, "failed to check for updates")
		return
	}
	jsonResponse(w, http.StatusOK, updateCheckResponse{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: available,
	})
}

// Apply kicks off a self-update. It returns 202 immediately and performs the
// swap + pm2 restart in a background goroutine after a short delay so the
// response is flushed before pm2 stops this process. Admin only.
//
// Two guards run before the goroutine is spawned: an in-flight check rejects a
// concurrent apply with 409 (a second swap would race the first), and an
// availability check rejects 409 when there is nothing newer to install, so a
// 202 is only ever returned when an update will actually be applied.
func (h *UpdateHandler) Apply(w http.ResponseWriter, r *http.Request) {
	if !h.applying.CompareAndSwap(false, true) {
		jsonError(w, http.StatusConflict, "update already in progress")
		return
	}

	current, latest, available, err := h.updater.CheckLatest(r.Context())
	if err != nil {
		h.applying.Store(false)
		h.logger.Error("update check failed", "error", err)
		jsonError(w, http.StatusBadGateway, "failed to check for updates")
		return
	}
	if !available {
		h.applying.Store(false)
		jsonError(w, http.StatusConflict, "no update available")
		return
	}

	// Audit before the swap: a successful apply restarts this process, so a
	// post-success append would race the pm2 restart. Best-effort.
	if h.store != nil {
		if err := h.store.AppendAudit(r.Context(), &model.AuditEntry{
			OrgID:   middleware.GetOrgID(r.Context()),
			ActorID: middleware.GetAccountID(r.Context()),
			Action:  "server.updated", TargetType: "server",
			Metadata: map[string]string{"from_version": current, "to_version": latest},
		}); err != nil {
			h.logger.Error("audit server.updated failed", "error", err)
		}
	}

	go func() {
		defer h.applying.Store(false)
		time.Sleep(applyDelay)
		// Detached from the request context (the request is long gone); use a
		// generous standalone timeout for the download + swap.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := h.updater.Apply(ctx); err != nil && !errors.Is(err, updater.ErrNoUpdate) {
			h.logger.Error("self-update apply failed", "error", err)
		}
	}()
	jsonResponse(w, http.StatusAccepted, map[string]string{"status": "updating"})
}
