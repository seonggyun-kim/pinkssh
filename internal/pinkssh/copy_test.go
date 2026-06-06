package pinkssh

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestCopyPublicKeyRunsFakeSSH(t *testing.T) {
	dir := t.TempDir()
	pubkey := filepath.Join(dir, "id_ed25519.pub")
	logPath := filepath.Join(dir, "ssh.log")
	writeFile(t, pubkey, "ssh-ed25519 AAAATEST test\n")

	ssh := fakeSSH(t, "copy")
	t.Setenv("PINKSSH_FAKE_LOG", logPath)

	host := Host{Alias: "copy-target"}
	opts := Options{SSHPath: ssh, ConfigPath: ""}
	if err := CopyPublicKey(context.Background(), host, opts, pubkey); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	for _, fragment := range []string{"copy-target", "/etc/dropbear/authorized_keys", ".ssh/authorized_keys", "AAAATEST"} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("fake ssh log missing %q: %s", fragment, got)
		}
	}
}

func TestCopyPublicKeyWritesDebugLog(t *testing.T) {
	dir := t.TempDir()
	pubkey := filepath.Join(dir, "id_ed25519.pub")
	sshLogPath := filepath.Join(dir, "ssh.log")
	appLogPath := filepath.Join(dir, "pinkssh.log")
	writeFile(t, pubkey, "ssh-ed25519 AAAATEST test\n")

	ssh := fakeSSH(t, "copy")
	t.Setenv("PINKSSH_FAKE_LOG", sshLogPath)

	host := Host{Alias: "copy-target", HostName: "example.test", User: "alice", Port: "22"}
	opts := Options{SSHPath: ssh, ConfigPath: "", LogPath: appLogPath}
	if err := CopyPublicKey(context.Background(), host, opts, pubkey); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(appLogPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	for _, fragment := range []string{"copy-key:start", "copy-key:done", "alias=copy-target", "result=ok"} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("debug log missing %q: %s", fragment, got)
		}
	}
}

func TestPosixAppendKeyCommandSupportsDropbear(t *testing.T) {
	command := posixAppendKeyCommand(`ssh-ed25519 AAAATEST user'name`)
	for _, fragment := range []string{"key='ssh-ed25519 AAAATEST user'\\''name'", "/etc/dropbear/authorized_keys", "$home/.ssh/authorized_keys", "grep -qxF", "printf '%s\\n'"} {
		if !strings.Contains(command, fragment) {
			t.Fatalf("command missing %q: %s", fragment, command)
		}
	}
}

func TestWindowsAppendKeyCommandUsesPowerShellAuthorizedKeys(t *testing.T) {
	command := windowsAppendKeyCommand(`ssh-ed25519 AAAATEST user'name`)
	for _, fragment := range []string{"cmd.exe", "/d", "/s", "/c", "powershell.exe", "-NoLogo", "-NonInteractive", "-OutputFormat Text", "2>NUL"} {
		if !strings.Contains(command, fragment) {
			t.Fatalf("command missing %q: %s", fragment, command)
		}
	}

	parts := strings.Fields(command)
	var encoded string
	for i, part := range parts {
		if part == "-EncodedCommand" && i+1 < len(parts) {
			encoded = parts[i+1]
			break
		}
	}
	if encoded == "" {
		t.Fatalf("missing -EncodedCommand: %s", command)
	}

	script := decodePowerShellForTest(t, encoded)
	for _, fragment := range []string{
		"$ProgressPreference='SilentlyContinue'",
		"authorized_keys",
		"administrators_authorized_keys",
		"ProgramData",
		"IsInRole",
		"Add-Content",
		"AAAATEST",
		"user''name",
		"*S-1-5-32-544:F",
		"*S-1-5-18:F",
		"icacls.exe",
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("script missing %q: %s", fragment, script)
		}
	}
}

func decodePowerShellForTest(t *testing.T, encoded string) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(data)%2 != 0 {
		t.Fatalf("encoded PowerShell is not UTF-16LE")
	}
	words := make([]uint16, len(data)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	return string(utf16.Decode(words))
}
