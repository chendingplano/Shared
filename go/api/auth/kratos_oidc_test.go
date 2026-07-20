package auth

import (
	"testing"

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
