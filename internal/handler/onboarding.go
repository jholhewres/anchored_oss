package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/model"
	projectpkg "github.com/jholhewres/anchored_oss/internal/project"
	"github.com/jholhewres/anchored_oss/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type OnboardingHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewOnboardingHandler(st store.Store, logger *slog.Logger) *OnboardingHandler {
	return &OnboardingHandler{store: st, logger: logger}
}

type onboardingOrgInput struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type onboardingAdminInput struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type onboardingProjectInput struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	// RepoURL is an optional git remote URL (ssh or https). When set, the
	// project's remote_key is derived from it so the repo's sync resolves to
	// this project. When empty, the project is "manual" (a synthetic key).
	RepoURL string `json:"repo_url"`
}

type onboardingRequest struct {
	Org      onboardingOrgInput      `json:"org"`
	Admin    onboardingAdminInput    `json:"admin"`
	Projects []onboardingProjectInput `json:"projects"`
}

type onboardingResponse struct {
	APIKey   string            `json:"api_key"`
	Org      *model.Organization `json:"org"`
	Admin    *model.Account    `json:"admin"`
	Projects []*model.Project  `json:"projects"`
}

// Complete is a single-shot endpoint that bootstraps an org, admin, and
// projects in one call. 409 if any org already exists.
func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	count, err := h.store.CountOrganizations(r.Context())
	if err != nil {
		h.logger.Error("onboarding: count orgs failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if count > 0 {
		jsonError(w, http.StatusConflict, "organization already exists; onboarding is a first-run-only endpoint")
		return
	}

	var req onboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Org.Name == "" {
		jsonError(w, http.StatusBadRequest, "org.name is required")
		return
	}
	if req.Admin.Email == "" || req.Admin.Password == "" || req.Admin.DisplayName == "" {
		jsonError(w, http.StatusBadRequest, "admin.email, admin.password, and admin.display_name are required")
		return
	}
	if len(req.Admin.Password) < 8 {
		jsonError(w, http.StatusBadRequest, "admin.password must be at least 8 characters")
		return
	}
	if len(req.Projects) > 10 {
		jsonError(w, http.StatusBadRequest, "at most 10 projects allowed per onboarding call")
		return
	}

	orgSlug := req.Org.Slug
	if orgSlug == "" {
		orgSlug = slugify(req.Org.Name)
	}
	if orgSlug == "" {
		jsonError(w, http.StatusBadRequest, "org.slug could not be derived from org.name")
		return
	}

	org, err := h.store.CreateOrganization(r.Context(), req.Org.Name, orgSlug)
	if err != nil {
		h.logger.Error("onboarding: create org failed", "slug", orgSlug, "error", err)
		jsonError(w, http.StatusConflict, "organization slug already taken")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Admin.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("onboarding: hash password failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	admin, err := h.store.CreateAccount(r.Context(), req.Admin.Email, req.Admin.DisplayName, string(passwordHash))
	if err != nil {
		h.logger.Error("onboarding: create admin account failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to create admin account")
		return
	}

	if err := h.store.AddOrgMember(r.Context(), org.ID, admin.ID, "admin"); err != nil {
		h.logger.Error("onboarding: add org member failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.store.EnsureDefaultTeamMembership(r.Context(), org.ID, admin.ID); err != nil {
		h.logger.Error("onboarding: ensure default team failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	fullKey, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		h.logger.Error("onboarding: generate api key failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := h.store.CreateAPIKey(r.Context(), org.ID, admin.ID, "default", prefix, hash, "admin", nil); err != nil {
		h.logger.Error("onboarding: store api key failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	projects := make([]*model.Project, 0, len(req.Projects))
	for _, pi := range req.Projects {
		if pi.Name == "" {
			continue
		}
		pSlug := slugify(pi.Name)
		if pSlug == "" {
			continue
		}
		// Derive the key from the repo URL (ssh/https) so the repo's sync lands
		// here; fall back to a unique synthetic key for manual (no-repo) projects.
		remoteKey := projectpkg.DeriveRemoteKey(pi.RepoURL)
		if remoteKey == "" {
			remoteKey = pSlug + "-" + randomSuffix(8)
		}
		cat := model.NormalizeCategory(pi.Category)
		proj, err := h.store.CreateProject(r.Context(), org.ID, pi.Name, pSlug, remoteKey, admin.ID, cat)
		if err != nil {
			h.logger.Warn("onboarding: create project failed", "name", pi.Name, "error", err)
			continue
		}
		if err := h.store.EnsureCreatorProjectAccess(r.Context(), org.ID, admin.ID, proj.ID); err != nil {
			h.logger.Warn("onboarding: grant project access failed", "project_id", proj.ID, "error", err)
		}
		projects = append(projects, proj)
	}

	jsonResponse(w, http.StatusCreated, onboardingResponse{
		APIKey:   fullKey,
		Org:      org,
		Admin:    admin,
		Projects: projects,
	})
}

func randomSuffix(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
