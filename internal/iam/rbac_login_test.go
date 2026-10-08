package iam_test

import (
	"testing"

	"capper/internal/iam"
)

// TestResolveSSOUserRejectsUnknown verifies there is no self-registration: an
// unknown SSO email is denied, and access requires an existing active user.
func TestResolveSSOUserRejectsUnknown(t *testing.T) {
	mgr, _ := openTestManager(t)
	if err := mgr.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if _, err := mgr.ResolveSSOUser("stranger@example.com"); err == nil {
		t.Fatalf("unknown SSO email should be rejected")
	}
	// Admin provisions the user (no self-service).
	if _, err := mgr.CreateManagedUser("alice@example.com", "alice@example.com", "google"); err != nil {
		t.Fatalf("CreateManagedUser: %v", err)
	}
	u, err := mgr.ResolveSSOUser("alice@example.com")
	if err != nil {
		t.Fatalf("ResolveSSOUser after provisioning: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("resolved wrong user: %+v", u)
	}
	// Disabled users are denied.
	if err := mgr.IAMStore().SetUserStatus(u.ID, iam.UserStatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := mgr.ResolveSSOUser("alice@example.com"); err == nil {
		t.Fatalf("disabled user should be denied")
	}
}

// TestPasswordLogin verifies local username/password authentication.
func TestPasswordLogin(t *testing.T) {
	mgr, _ := openTestManager(t)
	if err := mgr.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	u, err := mgr.CreateManagedUser("alice", "", "local")
	if err != nil {
		t.Fatalf("CreateManagedUser: %v", err)
	}
	if err := mgr.SetPassword(u.ID, "s3cret-pw"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if _, err := mgr.VerifyPassword("alice", "s3cret-pw"); err != nil {
		t.Fatalf("VerifyPassword good: %v", err)
	}
	if _, err := mgr.VerifyPassword("alice", "wrong"); err == nil {
		t.Fatalf("wrong password should fail")
	}
	if _, err := mgr.VerifyPassword("ghost", "whatever"); err == nil {
		t.Fatalf("unknown user should fail")
	}
}

// TestEnsureAdminUserBootstrap verifies the deploy bootstrap path makes an email
// an active admin authorized for everything.
func TestEnsureAdminUserBootstrap(t *testing.T) {
	mgr, s := openTestManager(t)
	if err := mgr.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	u, err := mgr.EnsureAdminUser("admin@example.com", "google")
	if err != nil {
		t.Fatalf("EnsureAdminUser: %v", err)
	}
	if u.Status != iam.UserStatusActive {
		t.Fatalf("bootstrap admin status = %q, want active", u.Status)
	}
	if dec, _, _ := s.Evaluate(iam.PrincipalUser, u.ID, "instance:run", "*"); dec != iam.DecisionAllow {
		t.Fatalf("bootstrap admin not authorized: %v", dec)
	}
	// Idempotent.
	if _, err := mgr.EnsureAdminUser("admin@example.com", "google"); err != nil {
		t.Fatalf("EnsureAdminUser repeat: %v", err)
	}
}

// TestMemberRoleEnforcement verifies an approved member can run workloads but
// cannot perform IAM administration.
func TestMemberRoleEnforcement(t *testing.T) {
	mgr, s := openTestManager(t)
	if err := mgr.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	u, err := mgr.CreateManagedUser("member@example.com", "member@example.com", "google")
	if err != nil {
		t.Fatalf("CreateManagedUser: %v", err)
	}
	if err := mgr.AssignRole(u.ID, iam.RoleMember); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if dec, _, _ := s.Evaluate(iam.PrincipalUser, u.ID, "instance:run", "*"); dec != iam.DecisionAllow {
		t.Fatalf("member denied workload action: %v", dec)
	}
	if dec, _, _ := s.Evaluate(iam.PrincipalUser, u.ID, "iam:user:update", "iam:system"); dec == iam.DecisionAllow {
		t.Fatalf("member wrongly allowed IAM admin action")
	}
}
