package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// The project gates. Every route that acts on a project by id goes through
// one of them:
//   - requireProjectAccess, for routes any key may call: org, then team.
//   - projectInCallerOrg alone, for admin-only routes (requireAdmin already
//     ran), where the team step would always pass.
//   - admin-only writes that the store already scopes (PATCH and DELETE
//     /v1/projects/{id}) pass the caller's org to UpdateProject or
//     SoftDeleteProject, which answer ErrNotFound for another org's project.
//
// Self-registration makes anyone an admin of some org, so the org step always
// comes before the admin bypass. Each gate writes the response on failure.

// projectInCallerOrg loads an active project of the caller's org. A project of
// another org answers 404, the same as a missing one, so other orgs' project
// ids cannot be probed.
func projectInCallerOrg(w http.ResponseWriter, r *http.Request, st store.Store, logger *slog.Logger, projectID string) (*model.Project, bool) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "missing org context")
		return nil, false
	}
	project, err := st.GetActiveProjectByID(r.Context(), projectID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && project.OrgID != orgID) {
		jsonError(w, http.StatusNotFound, "project not found")
		return nil, false
	}
	if err != nil {
		logger.Error("project lookup failed", "error", err, "project_id", projectID)
		jsonError(w, http.StatusInternalServerError, "project lookup failed")
		return nil, false
	}
	return project, true
}

// requireProjectAccess is projectInCallerOrg, then an admin key passes and any
// other key needs team access to the project (403).
func requireProjectAccess(w http.ResponseWriter, r *http.Request, st store.Store, logger *slog.Logger, projectID string) (*model.Project, bool) {
	project, ok := projectInCallerOrg(w, r, st, logger, projectID)
	if !ok {
		return nil, false
	}
	if middleware.GetScope(r.Context()) == "admin" {
		return project, true
	}
	accountID := middleware.GetAccountID(r.Context())
	if accountID == "" {
		jsonError(w, http.StatusUnauthorized, "missing account context")
		return nil, false
	}
	ok, err := st.HasProjectAccess(r.Context(), project.OrgID, accountID, projectID)
	if err != nil {
		logger.Error("project access check failed", "error", err, "project_id", projectID)
		jsonError(w, http.StatusInternalServerError, "access check failed")
		return nil, false
	}
	if !ok {
		jsonError(w, http.StatusForbidden, "no team access to this project")
		return nil, false
	}
	return project, true
}
