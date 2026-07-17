package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// HashManifests returns a stable SHA-256 digest over all manifest files under
// <repoRoot>/apps/*/stevedore.yml. The digest changes whenever any manifest is
// added, removed, or edited, making it suitable for cheap change detection.
func HashManifests(repoRoot string) (string, error) {
	pattern := filepath.Join(repoRoot, "apps", "*", "stevedore.yml")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob manifests: %w", err)
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("read manifest %s: %w", p, err)
		}
		// Include the path so renames are detected as changes.
		fmt.Fprintf(h, "%s\n", p)
		h.Write(b)
		fmt.Fprint(h, "\x00")
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
