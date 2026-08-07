package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// HashManifests returns a stable SHA-256 digest over all manifest files under
// host-aware manifest directories. The digest changes whenever any manifest is
// added, removed, or edited, making it suitable for cheap change detection.
func HashManifests(repoRoot string) (string, error) {
	paths, err := DiscoverManifestPaths(repoRoot)
	if err != nil {
		return "", err
	}

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
