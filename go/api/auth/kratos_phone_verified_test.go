package auth

import (
	"testing"

	ory "github.com/ory/client-go"
)

func TestIdentityPhoneProvenByCode(t *testing.T) {
	cases := []struct {
		name string
		id   *ory.Identity
		want bool
	}{
		{"nil", nil, false},
		{"E.164 CN phone trait (whoami identity has no credentials)",
			&ory.Identity{Traits: map[string]interface{}{"phone": "+8618625008130"}}, true},
		{"no phone trait",
			&ory.Identity{Traits: map[string]interface{}{"email": "a@b.com"}}, false},
		{"empty phone trait",
			&ory.Identity{Traits: map[string]interface{}{"phone": ""}}, false},
		{"bare national number is not accepted (schema stores E.164)",
			&ory.Identity{Traits: map[string]interface{}{"phone": "18625008130"}}, false},
		{"non-CN E.164 number",
			&ory.Identity{Traits: map[string]interface{}{"phone": "+14155550123"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := identityPhoneProvenByCode(tc.id); got != tc.want {
				t.Fatalf("identityPhoneProvenByCode = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsIdentityVerifiedAcceptsPhoneCodeOrVerifiedEmail(t *testing.T) {
	emailOnlyVerified := &ory.Identity{
		Traits:              map[string]interface{}{"email": "a@b.com"},
		VerifiableAddresses: []ory.VerifiableIdentityAddress{{Verified: true}},
	}
	if !isIdentityVerified(emailOnlyVerified) {
		t.Fatal("verified email identity should pass isIdentityVerified")
	}

	phoneOnly := &ory.Identity{Traits: map[string]interface{}{"phone": "+8618625008130"}}
	if !isIdentityVerified(phoneOnly) {
		t.Fatal("phone-code identity should pass isIdentityVerified")
	}

	unverified := &ory.Identity{
		Traits:              map[string]interface{}{"email": "a@b.com"},
		VerifiableAddresses: []ory.VerifiableIdentityAddress{{Verified: false}},
	}
	if isIdentityVerified(unverified) {
		t.Fatal("identity with only an unverified email must not pass isIdentityVerified")
	}
}
