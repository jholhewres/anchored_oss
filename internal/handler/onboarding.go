package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
	// RepoURL is the Git remote used to derive the project's canonical identity.
	RepoURL string `json:"repo_url"`
}

type preparedOnboardingProject struct {
	name        string
	slug        string
	category    string
	repoURL     string
	remoteKey   string
	remoteKeyV1 string
}

type onboardingRequest struct {
	Org      onboardingOrgInput       `json:"org"`
	Admin    onboardingAdminInput     `json:"admin"`
	Projects []onboardingProjectInput `json:"projects"`
}

type onboardingResponse struct {
	APIKey   string              `json:"api_key"`
	Org      *model.Organization `json:"org"`
	Admin    *model.Account      `json:"admin"`
	Projects []*model.Project    `json:"projects"`
}

// maxOnboardingBodyBytes caps the unauthenticated onboarding body: an org,
// an admin and at most 10 projects fit in a few KiB.
const maxOnboardingBodyBytes = 16 << 10

// Complete is a single-shot endpoint that bootstraps an org, admin, and
// projects in one call. 409 if any org already exists.
func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	// The route is unauthenticated: an instance that is already set up
	// answers 409 before decoding the body or hashing the password.
	// BootstrapOnboarding checks again inside its transaction, for the race
	// between two first-run calls.
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOnboardingBodyBytes)).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			jsonError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
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
	// Postgres TEXT refuses NUL. Caught here, before bcrypt, such a body is a
	// 400 rather than a 500 that leaves the first-run window open for the
	// next try.
	if field, found := firstNUL(
		[2]string{"org.name", req.Org.Name}, [2]string{"org.slug", req.Org.Slug},
		[2]string{"admin.email", req.Admin.Email}, [2]string{"admin.display_name", req.Admin.DisplayName},
	); found {
		jsonError(w, http.StatusBadRequest, field+" must not contain NUL characters")
		return
	}
	if len(req.Projects) == 0 {
		jsonError(w, http.StatusBadRequest, "at least one project is required")
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
	projects, status, validationMessage := prepareOnboardingProjects(req.Projects)
	if validationMessage != "" {
		jsonError(w, status, validationMessage)
		return
	}

	// Complete all validation and fallible local work before the first database
	// write. This prevents malformed or duplicate project input from leaving a
	// partly bootstrapped organization behind.
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Admin.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("onboarding: hash password failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	fullKey, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		h.logger.Error("onboarding: generate api key failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	bootstrapProjects := make([]store.OnboardingProject, 0, len(projects))
	for _, project := range projects {
		bootstrapProjects = append(bootstrapProjects, store.OnboardingProject{
			Name:        project.name,
			Slug:        project.slug,
			Category:    project.category,
			RepoURL:     project.repoURL,
			RemoteKey:   project.remoteKey,
			RemoteKeyV1: project.remoteKeyV1,
		})
	}
	result, err := h.store.BootstrapOnboarding(r.Context(), store.OnboardingBootstrapInput{
		OrgName:           req.Org.Name,
		OrgSlug:           orgSlug,
		AdminEmail:        req.Admin.Email,
		AdminDisplayName:  req.Admin.DisplayName,
		AdminPasswordHash: string(passwordHash),
		// Still an admin key with no expiry: released CLIs create projects
		// through admin-only POST /v1/projects with it. Handing developers
		// expiring personal keys instead belongs to the v0.7 auth redesign
		// (see docs/migration-v0.7.md).
		APIKeyName:   "default",
		APIKeyPrefix: prefix,
		APIKeyHash:   hash,
		APIKeyScope:  "admin",
		Projects:     bootstrapProjects,
	})
	if err != nil {
		h.logger.Error("onboarding bootstrap failed", "error", err)
		if errors.Is(err, store.ErrConflict) {
			jsonError(w, http.StatusConflict, "onboarding data conflicts with an existing organization, account, or project")
		} else {
			jsonError(w, http.StatusInternalServerError, "failed to complete onboarding")
		}
		return
	}

	jsonResponse(w, http.StatusCreated, onboardingResponse{
		APIKey:   fullKey,
		Org:      result.Organization,
		Admin:    result.Admin,
		Projects: result.Projects,
	})
}

func prepareOnboardingProjects(inputs []onboardingProjectInput) ([]preparedOnboardingProject, int, string) {
	projects := make([]preparedOnboardingProject, 0, len(inputs))
	seenRemoteKeys := make(map[string]int, len(inputs))
	seenSlugs := make(map[string]int, len(inputs))
	for i, input := range inputs {
		name := strings.TrimSpace(input.Name)
		prefix := "projects[" + strconv.Itoa(i) + "]."
		if field, found := firstNUL(
			[2]string{prefix + "name", input.Name}, [2]string{prefix + "category", input.Category},
			[2]string{prefix + "repo_url", input.RepoURL},
		); found {
			return nil, http.StatusBadRequest, field + " must not contain NUL characters"
		}
		if name == "" {
			return nil, http.StatusBadRequest, "projects[" + strconv.Itoa(i) + "].name is required"
		}
		slug := slugify(name)
		if slug == "" {
			return nil, http.StatusBadRequest, "projects[" + strconv.Itoa(i) + "].name cannot produce a valid slug"
		}
		if previous, exists := seenSlugs[slug]; exists {
			return nil, http.StatusConflict, "projects[" + strconv.Itoa(i) + "].name duplicates projects[" + strconv.Itoa(previous) + "]"
		}
		repoURL := strings.TrimSpace(input.RepoURL)
		if !projectpkg.IsValidGitRemote(repoURL) {
			return nil, http.StatusBadRequest, "projects[" + strconv.Itoa(i) + "].repo_url must be a valid HTTPS or SSH git remote"
		}
		remoteKey := projectpkg.DeriveRemoteKey(repoURL)
		if previous, exists := seenRemoteKeys[remoteKey]; exists {
			return nil, http.StatusConflict, "projects[" + strconv.Itoa(i) + "].repo_url duplicates projects[" + strconv.Itoa(previous) + "]"
		}
		seenSlugs[slug] = i
		seenRemoteKeys[remoteKey] = i
		projects = append(projects, preparedOnboardingProject{
			name:        name,
			slug:        slug,
			category:    model.NormalizeCategory(input.Category),
			repoURL:     repoURL,
			remoteKey:   remoteKey,
			remoteKeyV1: projectpkg.DeriveLegacyRemoteKey(repoURL),
		})
	}
	return projects, 0, ""
}

// firstNUL names the first of the given (name, value) fields whose value holds
// a NUL, which Postgres TEXT refuses.
func firstNUL(fields ...[2]string) (string, bool) {
	for _, f := range fields {
		if strings.ContainsRune(f[1], 0) {
			return f[0], true
		}
	}
	return "", false
}
