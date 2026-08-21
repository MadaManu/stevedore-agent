package git

import (
	"fmt"
	urlpkg "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"stevedore-agent/internal/config"
)

// Pull runs a fast-forward-only pull in an existing checkout.
func Pull(repoRoot string) error {
	cmd := exec.Command("git", "-C", repoRoot, "pull", "--ff-only")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull --ff-only: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Sync ensures the configured git repository is present at its workdir and up to
// date on the configured branch. It clones on first run and fast-forwards on
// subsequent runs. Returns the resolved workdir path.
func Sync(g config.GitSource) (string, error) {
	workdir := g.Workdir
	if strings.TrimSpace(workdir) == "" {
		return "", fmt.Errorf("git workdir is empty")
	}

	env, cleanup, err := authEnv(g.Auth)
	if err != nil {
		return "", err
	}
	defer cleanup()

	url, err := authURL(g)
	if err != nil {
		return "", err
	}

	if !isGitRepo(workdir) {
		if err := os.MkdirAll(filepath.Dir(workdir), 0o755); err != nil {
			return "", fmt.Errorf("create workdir parent: %w", err)
		}
		if err := runGit(env, "clone", "--branch", g.Branch, url, workdir); err != nil {
			return "", err
		}
		return workdir, nil
	}

	// Ensure remote reflects the (possibly credentialed) URL, then fast-forward.
	if err := runGit(env, "-C", workdir, "remote", "set-url", "origin", url); err != nil {
		return "", err
	}
	if err := runGit(env, "-C", workdir, "fetch", "origin", g.Branch); err != nil {
		return "", err
	}
	if err := runGit(env, "-C", workdir, "checkout", g.Branch); err != nil {
		return "", err
	}
	if err := runGit(env, "-C", workdir, "reset", "--hard", "origin/"+g.Branch); err != nil {
		return "", err
	}
	return workdir, nil
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func runGit(env []string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// authEnv builds the process environment for git commands and returns a cleanup
// function for any temporary resources created.
func authEnv(auth config.GitAuth) ([]string, func(), error) {
	env := append([]string{}, os.Environ()...)
	// Never prompt interactively in a daemon.
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	cleanup := func() {}

	if auth.SSH != nil {
		key := auth.SSH.KeyPath
		if strings.TrimSpace(key) == "" {
			return nil, cleanup, fmt.Errorf("ssh auth requires keyPath")
		}
		sshCmd := fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new", key)
		env = append(env, "GIT_SSH_COMMAND="+sshCmd)
	}

	return env, cleanup, nil
}

// authURL injects token/basic credentials into an https URL where required.
// For SSH or none, the configured URL is returned unchanged.
func authURL(g config.GitSource) (string, error) {
	switch {
	case g.Auth.Token != nil:
		return injectHTTPSCredentials(g.URL, "x-access-token", g.Auth.Token.Value)
	case g.Auth.Basic != nil:
		return injectHTTPSCredentials(g.URL, g.Auth.Basic.Username, g.Auth.Basic.Password)
	default:
		return g.URL, nil
	}
}

func injectHTTPSCredentials(url, user, secret string) (string, error) {
	const prefix = "https://"
	if !strings.HasPrefix(url, prefix) {
		return "", fmt.Errorf("token/basic auth requires an https:// url, got %q", url)
	}
	parsed, err := urlpkg.Parse(url)
	if err != nil {
		return "", fmt.Errorf("parse git url %q: %w", url, err)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("token/basic auth requires an https:// url, got %q", url)
	}
	parsed.User = urlpkg.UserPassword(user, secret)
	return parsed.String(), nil
}
