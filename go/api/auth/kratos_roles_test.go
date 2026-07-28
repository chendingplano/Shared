package auth

import (
	"reflect"
	"testing"

	ory "github.com/ory/client-go"
)

func TestExtractIdentityInfoDerivesAdminFromRoles(t *testing.T) {
	identity := &ory.Identity{
		Traits: map[string]interface{}{
			"email": "user@example.com",
			"name": map[string]interface{}{
				"first": "Ada",
				"last":  "Lovelace",
			},
		},
		MetadataPublic: map[string]interface{}{
			"admin":    false,
			"is_owner": true,
			"avatar":   "avatar.png",
			"roles":    []interface{}{"DEV", " admin ", "dev", "", 42},
		},
	}

	info := extractIdentityInfo(identity)

	if !info.IsAdmin {
		t.Fatalf("expected admin to be derived from roles")
	}
	if !info.IsOwner {
		t.Fatalf("expected owner to be preserved")
	}
	if info.Avatar != "avatar.png" {
		t.Fatalf("expected avatar to be preserved, got %q", info.Avatar)
	}

	wantRoles := []string{"admin", "dev"}
	if !reflect.DeepEqual(info.Roles, wantRoles) {
		t.Fatalf("unexpected roles: got %v want %v", info.Roles, wantRoles)
	}
}

func TestExtractIdentityInfoFallsBackToLegacyAdminFlag(t *testing.T) {
	identity := &ory.Identity{
		MetadataPublic: map[string]interface{}{
			"admin": true,
		},
	}

	info := extractIdentityInfo(identity)

	if !info.IsAdmin {
		t.Fatalf("expected legacy admin flag to be honored")
	}
	wantRoles := []string{"admin"}
	if !reflect.DeepEqual(info.Roles, wantRoles) {
		t.Fatalf("unexpected roles: got %v want %v", info.Roles, wantRoles)
	}
}

func TestKratosIdentityToUserInfoReadsRolesAndProjectsAdmin(t *testing.T) {
	identity := map[string]interface{}{
		"id": "kratos-user-1",
		"traits": map[string]interface{}{
			"email": "user@example.com",
			"name": map[string]interface{}{
				"first": "Grace",
				"last":  "Hopper",
			},
		},
		"metadata_public": map[string]interface{}{
			"admin":    false,
			"is_owner": true,
			"avatar":   "pic.jpg",
			"roles":    []interface{}{"trial", "ADMIN", "trial"},
		},
		"state": "active",
	}

	userInfo := KratosIdentityToUserInfo(identity)

	if !userInfo.Admin {
		t.Fatalf("expected admin to be projected from roles")
	}
	if !userInfo.IsOwner {
		t.Fatalf("expected owner to be preserved")
	}
	wantRoles := []string{"admin", "trial"}
	if !reflect.DeepEqual(userInfo.Roles, wantRoles) {
		t.Fatalf("unexpected roles: got %v want %v", userInfo.Roles, wantRoles)
	}
}

func TestProjectSignupRolesDefaultsGuestForNonAdmin(t *testing.T) {
	got := projectSignupRoles(false)
	want := []string{"guest"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected signup roles: got %v want %v", got, want)
	}
}

func TestSignupMetadataPublicUsesProvisionedRoles(t *testing.T) {
	got := signupMetadataPublic(false, true)
	want := map[string]interface{}{
		"admin":    false,
		"roles":    []string{"guest"},
		"is_owner": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected signup metadata_public: got %#v want %#v", got, want)
	}
}
