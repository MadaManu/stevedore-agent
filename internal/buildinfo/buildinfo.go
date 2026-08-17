package buildinfo

import "strings"

var (
	Version   = "dev"
	Commit    = ""
	BuildDate = ""
)

func EffectiveVersion() string {
	if v := strings.TrimSpace(Version); v != "" {
		return v
	}
	return "dev"
}

func Summary() string {
	parts := []string{EffectiveVersion()}
	if commit := strings.TrimSpace(Commit); commit != "" {
		parts = append(parts, commit)
	}
	if buildDate := strings.TrimSpace(BuildDate); buildDate != "" {
		parts = append(parts, buildDate)
	}
	return strings.Join(parts, " ")
}
