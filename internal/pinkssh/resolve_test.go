package pinkssh

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseSSHConfigOutput(t *testing.T) {
	cfg := ParseSSHConfigOutput([]byte(`
hostname resolved.example
user alice
port 2222
identityfile ~/.ssh/id_ed25519
identityfile C:\keys\work
proxyjump bastion
proxycommand none
`))

	if cfg.HostName != "resolved.example" {
		t.Fatalf("HostName = %q", cfg.HostName)
	}
	if cfg.User != "alice" {
		t.Fatalf("User = %q", cfg.User)
	}
	if cfg.Port != "2222" {
		t.Fatalf("Port = %q", cfg.Port)
	}
	if !reflect.DeepEqual(cfg.IdentityFiles, []string{"~/.ssh/id_ed25519", `C:\keys\work`}) {
		t.Fatalf("IdentityFiles = %#v", cfg.IdentityFiles)
	}
	if cfg.ProxyJump != "bastion" {
		t.Fatalf("ProxyJump = %q", cfg.ProxyJump)
	}
	if cfg.ProxyCommand != "" {
		t.Fatalf("ProxyCommand = %q", cfg.ProxyCommand)
	}
}

func TestResolveHostUsesFakeSSH(t *testing.T) {
	ssh := fakeSSH(t, `
hostname fake.example
user testuser
port 2200
identityfile /tmp/test_id
`)
	opts := Options{SSHPath: ssh, ConfigPath: ""}

	cfg, err := ResolveHost(context.Background(), opts, "fake")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HostName != "fake.example" || cfg.User != "testuser" || cfg.Port != "2200" {
		t.Fatalf("cfg = %#v", cfg)
	}
	if !reflect.DeepEqual(cfg.IdentityFiles, []string{filepath.FromSlash("/tmp/test_id")}) &&
		!reflect.DeepEqual(cfg.IdentityFiles, []string{"/tmp/test_id"}) {
		t.Fatalf("IdentityFiles = %#v", cfg.IdentityFiles)
	}
}
