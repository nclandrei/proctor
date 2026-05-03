// internal/proctor/secrets.go
package proctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ResolveSecret expands a credential reference to its concrete value.
//
// Supported reference forms:
//   - ""              → ""           (empty stays empty)
//   - "env:NAME"      → value of $NAME, errors if unset
//   - "op://path"     → value from `op read`, errors if op fails or is missing
//   - anything else   → returned as a literal
//
// Storing references instead of literals lets a profile travel between
// machines without baking secrets into profile.json.
func ResolveSecret(ref string) (string, error) {
	switch {
	case ref == "":
		return "", nil
	case strings.HasPrefix(ref, "env:"):
		name := strings.TrimPrefix(ref, "env:")
		if name == "" {
			return "", fmt.Errorf("env reference has empty variable name")
		}
		val, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("env var %s not set", name)
		}
		return val, nil
	case strings.HasPrefix(ref, "op://"):
		out, err := exec.Command("op", "read", ref).Output()
		if err != nil {
			return "", fmt.Errorf("op read %s failed: %w", ref, err)
		}
		return strings.TrimRight(string(out), "\n"), nil
	default:
		return ref, nil
	}
}
