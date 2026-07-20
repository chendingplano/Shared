package auth

import (
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
	ory "github.com/ory/client-go"
)

func TestIsSessionAuthenticatedViaOIDCFromAuthenticationMethods(t *testing.T) {
	method := "oidc"
	session := &ory.Session{
		AuthenticationMethods: []ory.SessionAuthenticationMethod{
			{Method: &method},
		},
	}

	if !isSessionAuthenticatedViaOIDC(session) {
		t.Fatalf("expected session with oidc authentication method to be detected")
	}
}

func TestIsSessionAuthenticatedViaOIDCFromIdentityCredentials(t *testing.T) {
	credType := "oidc"
	session := &ory.Session{
		Identity: &ory.Identity{
			Credentials: &map[string]ory.IdentityCredentials{
				"oidc": {Type: &credType},
			},
		},
	}

	if !isSessionAuthenticatedViaOIDC(session) {
		t.Fatalf("expected session with oidc identity credentials to be detected")
	}
}

func TestIsSessionAuthenticatedViaOIDCFalseWithoutOIDCSignals(t *testing.T) {
	passwordType := "password"
	session := &ory.Session{
		AuthenticationMethods: []ory.SessionAuthenticationMethod{
			{Method: &passwordType},
		},
		Identity: &ory.Identity{
			Credentials: &map[string]ory.IdentityCredentials{
				"password": {Type: &passwordType},
			},
		},
	}

	if isSessionAuthenticatedViaOIDC(session) {
		t.Fatalf("expected non-oidc session to return false")
	}
}

func TestBuildUserInfoFromKratosSessionTreatsAdminRoleAsAdmin(t *testing.T) {
	session := &ory.Session{
		Id: "sess-1",
		Identity: &ory.Identity{
			Id: "identity-1",
			Traits: map[string]interface{}{
				"email": "admin@example.com",
				"name": map[string]interface{}{
					"first": "Admin",
					"last":  "User",
				},
			},
			MetadataPublic: map[string]interface{}{
				"roles": []interface{}{"admin", "dev"},
			},
		},
	}

	userInfo, err := buildUserInfoFromKratosSession(loggerutil.CreateDefaultLogger("SHD_TEST_OIDC_001"), session)
	if err != nil {
		t.Fatalf("buildUserInfoFromKratosSession returned error: %v", err)
	}
	if !userInfo.Admin {
		t.Fatalf("expected admin role to set userInfo.Admin=true")
	}
	if len(userInfo.Roles) != 2 || userInfo.Roles[0] != "admin" || userInfo.Roles[1] != "dev" {
		t.Fatalf("unexpected roles: %#v", userInfo.Roles)
	}
}
