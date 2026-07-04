package twitchauth

import "testing"

func TestTokenHasScopes(t *testing.T) {
	tok := &Token{Scope: []string{"user:read:chat", "channel:read:redemptions"}}

	if !tok.HasScopes([]string{"user:read:chat"}) {
		t.Fatal("expected a granted scope to be reported as present")
	}
	if !tok.HasScopes([]string{"user:read:chat", "channel:read:redemptions"}) {
		t.Fatal("expected all granted scopes to be reported as present")
	}
	if tok.HasScopes([]string{"channel:manage:redemptions"}) {
		t.Fatal("expected an ungranted scope to be reported as missing")
	}
	if tok.HasScopes([]string{"user:read:chat", "channel:manage:redemptions"}) {
		t.Fatal("expected HasScopes to require every scope, not just one")
	}
}

// TestTokenHasScopesGuardsAgainstUpgradeBug guards the exact bug this
// method exists to catch: a token cached by an older version of the app
// (fewer scopes) must be recognized as insufficient once a newer version
// starts requiring more, rather than being blindly reused and causing
// silent 401s against endpoints it was never authorized for.
func TestTokenHasScopesGuardsAgainstUpgradeBug(t *testing.T) {
	oldToken := &Token{Scope: []string{"user:read:chat"}}
	newRequirements := []string{"user:read:chat", "channel:read:redemptions", "channel:manage:redemptions"}

	if oldToken.HasScopes(newRequirements) {
		t.Fatal("expected an old-scope token to be insufficient for newly required scopes")
	}
}
