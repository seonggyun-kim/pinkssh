package pinkssh

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddHostAppendsBlock(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config")
	if err := AddHost(config, HostConfig{
		Alias:         "new-host",
		HostName:      "example.com",
		User:          "alice",
		Port:          "2222",
		IdentityFiles: []string{"~/.ssh/work key"},
	}); err != nil {
		t.Fatal(err)
	}

	content := readFile(t, config)
	for _, fragment := range []string{
		"Host new-host",
		"HostName example.com",
		"User alice",
		"Port 2222",
		`IdentityFile "~/.ssh/work key"`,
	} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("config missing %q:\n%s", fragment, content)
		}
	}
}

func TestUpdateHostPreservesUnknownOptionsAndComments(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config")
	writeFile(t, config, `Host old other
    HostName old.example
    # keep comment
    ForwardAgent yes
    IdentityFile ~/.ssh/old
`)

	if err := UpdateHost(config, "old", HostConfig{
		Alias:         "renamed",
		HostName:      "new.example",
		User:          "root",
		IdentityFiles: []string{"~/.ssh/new"},
	}); err != nil {
		t.Fatal(err)
	}

	content := readFile(t, config)
	for _, fragment := range []string{
		"Host renamed other",
		"HostName new.example",
		"# keep comment",
		"ForwardAgent yes",
		"User root",
		"IdentityFile ~/.ssh/new",
	} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("config missing %q:\n%s", fragment, content)
		}
	}
	if strings.Contains(content, "old.example") {
		t.Fatalf("old value still present:\n%s", content)
	}
}

func TestDeleteHostRemovesSingleAliasFromMultiHostLine(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config")
	writeFile(t, config, `Host one two
    HostName example
Host three
    HostName three
`)

	if err := DeleteHost(config, "one"); err != nil {
		t.Fatal(err)
	}

	content := readFile(t, config)
	if !strings.Contains(content, "Host two") {
		t.Fatalf("remaining alias missing:\n%s", content)
	}
	if strings.Contains(content, "Host one") {
		t.Fatalf("deleted alias still present:\n%s", content)
	}
}

func TestDeleteHostRemovesWholeBlock(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config")
	writeFile(t, config, `Host one
    HostName one
Host two
    HostName two
`)

	if err := DeleteHost(config, "one"); err != nil {
		t.Fatal(err)
	}

	content := readFile(t, config)
	if strings.Contains(content, "HostName one") {
		t.Fatalf("deleted block still present:\n%s", content)
	}
	if !strings.Contains(content, "Host two") {
		t.Fatalf("next block missing:\n%s", content)
	}
}

func TestReadHostConfigFromIncludedFile(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config")
	include := filepath.Join(dir, "included")
	writeFile(t, config, "Include included\n")
	writeFile(t, include, "Host inc\n    HostName included.example\n    User alice\n")

	spec, err := ReadHostConfig(config, "inc")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Alias != "inc" || spec.HostName != "included.example" || spec.User != "alice" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestAddHostRejectsDuplicate(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config")
	writeFile(t, config, "Host existing\n")
	err := AddHost(config, HostConfig{Alias: "existing"})
	if !errors.Is(err, ErrHostExists) {
		t.Fatalf("err = %v, want ErrHostExists", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
