package EchoFactory

import (
	"reflect"
	"testing"
)

func TestResolveUpdatedRolesPreservesExistingRolesAndAddsAdmin(t *testing.T) {
	got := resolveUpdatedRoles([]string{"dev"}, nil, true)
	want := []string{"admin", "dev"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected roles: got %v want %v", got, want)
	}
}

func TestResolveUpdatedRolesRemovesAdminWhenLegacyFlagFalse(t *testing.T) {
	got := resolveUpdatedRoles([]string{"admin", "dev"}, nil, false)
	want := []string{"dev"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected roles: got %v want %v", got, want)
	}
}

func TestResolveUpdatedRolesUsesExplicitRolesButKeepsLegacyAdminCompatible(t *testing.T) {
	got := resolveUpdatedRoles([]string{"guest"}, []string{"trial", "admin"}, false)
	want := []string{"trial"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected roles: got %v want %v", got, want)
	}
}
