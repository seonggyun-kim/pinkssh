package pinkssh

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicKeyCandidatesPreferConfiguredIdentity(t *testing.T) {
	dir := t.TempDir()
	privateKey := filepath.Join(dir, "work_id")
	publicKey := privateKey + ".pub"
	writeFile(t, publicKey, "ssh-ed25519 AAAATEST configured\n")

	host := Host{
		Alias:         "work",
		HostName:      "example.com",
		User:          "alice",
		Port:          "22",
		IdentityFiles: []string{privateKey},
	}
	candidates := PublicKeyCandidates(host)
	if len(candidates) == 0 {
		t.Fatal("no candidates")
	}
	if candidates[0] != filepath.Clean(publicKey) {
		t.Fatalf("first candidate = %q, want %q", candidates[0], publicKey)
	}
}

func TestIdentityFileChoicesFromDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "id_ed25519"), "private")
	writeFile(t, filepath.Join(dir, "id_ed25519.pub"), "public")
	writeFile(t, filepath.Join(dir, "work"), "private")
	writeFile(t, filepath.Join(dir, "work.pub"), "public")
	writeFile(t, filepath.Join(dir, "known_hosts"), "host")
	writeFile(t, filepath.Join(dir, "lonely.pub"), "public")

	choices := identityFileChoicesFromDir(dir)
	want := []string{"~/.ssh/id_ed25519", "~/.ssh/work"}
	for _, value := range want {
		if indexOfTestString(choices, value) < 0 {
			t.Fatalf("choices missing %q: %#v", value, choices)
		}
	}
	for _, value := range []string{"~/.ssh/id_ed25519.pub", "~/.ssh/known_hosts", "~/.ssh/lonely"} {
		if indexOfTestString(choices, value) >= 0 {
			t.Fatalf("choices should not include %q: %#v", value, choices)
		}
	}
}

func TestRemoteAppendKeyCommandQuotesPublicKey(t *testing.T) {
	command := remoteAppendKeyCommand(`ssh-ed25519 AAAATEST user'name`)
	if !containsAll(command, "sh -c", "2>NUL || cmd.exe", "powershell.exe", "-EncodedCommand", "/etc/dropbear/authorized_keys", "grep -qxF", "AAAATEST") {
		t.Fatalf("remote command did not quote expected fragments: %s", command)
	}
	if strings.Contains(command, "sh -lc") {
		t.Fatalf("remote command should not use a login shell: %s", command)
	}
}

func containsAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			return false
		}
	}
	return true
}

func indexOfTestString(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
