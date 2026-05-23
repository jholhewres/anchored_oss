package middleware

import (
	"context"
	"errors"

	"github.com/jholhewres/anchored_oss/internal/store"
)

var (
	ErrNotAuthorized = errors.New("access denied")
	ErrNoAccount     = errors.New("no account in context")
)

// CheckProjectRead verifies the authenticated account has read access to the
// given project through their team memberships.
func CheckProjectRead(ctx context.Context, st store.Store, projectID string) error {
	accountID := GetAccountID(ctx)
	if accountID == "" {
		return ErrNoAccount
	}

	projects, err := st.ListProjectsByTeamAccess(ctx, accountID)
	if err != nil {
		return err
	}

	for _, p := range projects {
		if p.ID == projectID {
			return nil
		}
	}

	return ErrNotAuthorized
}

// CheckProjectWrite verifies the authenticated account has write access to the
// given project through their team memberships.
//
// Currently uses the same team-access check as read. When the store supports
// role-level queries on team_project_access, this will enforce the
// writer/maintainer requirement.
func CheckProjectWrite(ctx context.Context, st store.Store, projectID string) error {
	accountID := GetAccountID(ctx)
	if accountID == "" {
		return ErrNoAccount
	}

	projects, err := st.ListProjectsByTeamAccess(ctx, accountID)
	if err != nil {
		return err
	}

	for _, p := range projects {
		if p.ID == projectID {
			return nil
		}
	}

	return ErrNotAuthorized
}

// CheckOrgAdmin verifies the authenticated account has admin or owner role in
// the given organization.
//
// Stub: returns ErrNotAuthorized until Store exposes org member role queries.
func CheckOrgAdmin(ctx context.Context, st store.Store, orgID string) error {
	accountID := GetAccountID(ctx)
	if accountID == "" {
		return ErrNoAccount
	}

	_ = orgID
	_ = accountID
	return ErrNotAuthorized
}
