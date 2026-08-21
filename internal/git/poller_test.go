package git

import (
	"testing"

	"stevedore-agent/internal/config"
)

func TestInjectHTTPSCredentialsEscapesSpecialCharacters(t *testing.T) {
	got, err := injectHTTPSCredentials(
		"https://scm.example.com/stash/scm/~user/repo.git",
		"x-access-token",
		"abc/def:ghi@jkl",
	)
	if err != nil {
		t.Fatalf("injectHTTPSCredentials() error = %v", err)
	}

	want := "https://x-access-token:abc%2Fdef%3Aghi%40jkl@scm.example.com/stash/scm/~user/repo.git"
	if got != want {
		t.Fatalf("injectHTTPSCredentials() = %q, want %q", got, want)
	}
}

func TestAuthURLUsesEscapedTokenCredentials(t *testing.T) {
	got, err := authURL(config.GitSource{
		URL: "https://scm.example.com/stash/scm/~user/repo.git",
		Auth: config.GitAuth{
			Token: &config.TokenAuth{Value: "abc/def:ghi@jkl"},
		},
	})
	if err != nil {
		t.Fatalf("authURL() error = %v", err)
	}

	want := "https://x-access-token:abc%2Fdef%3Aghi%40jkl@scm.example.com/stash/scm/~user/repo.git"
	if got != want {
		t.Fatalf("authURL() = %q, want %q", got, want)
	}
}
