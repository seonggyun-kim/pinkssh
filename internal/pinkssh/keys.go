package pinkssh

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

var ErrNoPublicKeys = errors.New("no local public keys found")

func PublicKeyCandidates(host Host) []string {
	var candidates []string
	add := func(path string) {
		path = expandIdentityPath(path, host)
		if path == "" {
			return
		}
		if !strings.HasSuffix(strings.ToLower(path), ".pub") {
			path += ".pub"
		}
		if fileExists(path) {
			candidates = append(candidates, filepath.Clean(path))
		}
	}

	for _, identity := range host.IdentityFiles {
		add(identity)
	}
	add(filepath.Join(SSHDir(), "id_ed25519.pub"))

	pubKeys, _ := filepath.Glob(filepath.Join(SSHDir(), "*.pub"))
	sort.Strings(pubKeys)
	for _, path := range pubKeys {
		add(path)
	}

	return dedupePaths(candidates)
}

func IdentityFileChoices() []string {
	return identityFileChoicesFromDir(SSHDir())
}

func identityFileChoicesFromDir(sshDir string) []string {
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return nil
	}

	pubBacked := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".pub") {
			continue
		}
		privateName := strings.TrimSuffix(name, ".pub")
		if privateName != "" && fileExists(filepath.Join(sshDir, privateName)) {
			pubBacked[privateName] = true
		}
	}

	var choices []string
	add := func(name string) {
		if name == "" {
			return
		}
		path := filepath.Join(sshDir, name)
		if !fileExists(path) || !looksLikeIdentityFile(name, pubBacked[name]) {
			return
		}
		choices = append(choices, "~/.ssh/"+filepath.ToSlash(name))
	}

	defaults := []string{"id_ed25519", "id_ecdsa", "id_rsa", "id_dsa", "id_ed25519_sk", "id_ecdsa_sk"}
	for _, name := range defaults {
		add(name)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		add(entry.Name())
	}

	return dedupeStrings(choices)
}

func looksLikeIdentityFile(name string, hasPublicKey bool) bool {
	if strings.HasSuffix(name, ".pub") {
		return false
	}
	switch strings.ToLower(name) {
	case "config", "known_hosts", "known_hosts.old", "authorized_keys", "environment", "rc":
		return false
	}
	if strings.HasPrefix(strings.ToLower(name), "known_hosts.") {
		return false
	}
	if hasPublicKey {
		return true
	}
	if strings.HasPrefix(name, "id_") {
		return true
	}
	if strings.HasSuffix(strings.ToLower(name), ".pem") {
		return true
	}
	return false
}

func ResolvePublicKey(host Host, requested string) (string, error) {
	if requested != "" {
		path := expandIdentityPath(requested, host)
		if fileExists(path) {
			return filepath.Clean(path), nil
		}
		return "", errors.New("public key does not exist: " + path)
	}

	candidates := PublicKeyCandidates(host)
	if len(candidates) == 0 {
		return "", ErrNoPublicKeys
	}
	return candidates[0], nil
}

func ReadPublicKey(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", errors.New("public key is empty: " + path)
}

func expandIdentityPath(path string, host Host) string {
	path = strings.ReplaceAll(path, "%%", "\x00")
	path = strings.ReplaceAll(path, "%d", HomeDir())
	path = strings.ReplaceAll(path, "%h", host.HostName)
	path = strings.ReplaceAll(path, "%n", host.Alias)
	path = strings.ReplaceAll(path, "%p", defaultPort(host.Port))
	path = strings.ReplaceAll(path, "%r", host.User)
	path = strings.ReplaceAll(path, "\x00", "%")
	return expandPath(path, "")
}

func defaultPort(port string) string {
	if port == "" {
		return "22"
	}
	return port
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dedupePaths(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, path := range paths {
		key := filepath.Clean(path)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, filepath.Clean(path))
	}
	return out
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		key := value
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}
