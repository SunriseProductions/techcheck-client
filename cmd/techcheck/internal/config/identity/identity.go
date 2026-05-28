// Package identity persists the user's name and email across runs so the
// identify screen can pre-fill the form. Stored as a plain JSON file in the
// OS-native user-config directory — this is convenience data, not a secret.
package identity

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type Identity struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
}

// Path returns the absolute path to the identity file under the OS user
// config dir, creating the parent dir on demand. An error here is fatal —
// callers can't persist without it.
func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "SunriseTechCheck")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "identity.json"), nil
}

// Load returns the saved identity, or a zero-value Identity if no file
// exists yet. Parse errors are ignored — a corrupt file behaves like no file.
func Load() Identity {
	p, err := Path()
	if err != nil {
		return Identity{}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Identity{}
		}
		return Identity{}
	}
	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return Identity{}
	}
	return id
}

// Save writes the identity to disk. Non-atomic — good enough for a file this
// small; a truncated write still parses or silently returns zero values on
// next load.
func Save(id Identity) error {
	p, err := Path()
	if err != nil {
		return err
	}
	data, err := json.Marshal(id)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}
