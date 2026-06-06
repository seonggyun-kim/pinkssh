package pinkssh

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverHostsParsesIncludesAndConcreteAliases(t *testing.T) {
	dir := t.TempDir()
	mainConfig := filepath.Join(dir, "config")
	includeConfig := filepath.Join(dir, "included config")

	writeFile(t, includeConfig, `
Host gamma delta*
  HostName included.example
`)
	writeFile(t, mainConfig, `
# leading comment
Host alpha beta *.wild !blocked
  HostName example.com # inline comment
Include "`+includeConfig+`"
Host "quoted"
  User alice
`)

	discovery, err := DiscoverHosts(mainConfig)
	if err != nil {
		t.Fatal(err)
	}

	var aliases []string
	for _, entry := range discovery.Entries {
		aliases = append(aliases, entry.Alias)
	}
	want := []string{"alpha", "beta", "gamma", "quoted"}
	if !reflect.DeepEqual(aliases, want) {
		t.Fatalf("aliases = %#v, want %#v", aliases, want)
	}

	if len(discovery.Files) != 2 {
		t.Fatalf("files = %#v, want main and include", discovery.Files)
	}
}

func TestDiscoverHostsGlobInclude(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config.d")
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mainConfig := filepath.Join(dir, "config")
	writeFile(t, filepath.Join(configDir, "one"), "Host one\n")
	writeFile(t, filepath.Join(configDir, "two"), "Host two\n")
	writeFile(t, mainConfig, "Include config.d/*\n")

	discovery, err := DiscoverHosts(mainConfig)
	if err != nil {
		t.Fatal(err)
	}

	var aliases []string
	for _, entry := range discovery.Entries {
		aliases = append(aliases, entry.Alias)
	}
	want := []string{"one", "two"}
	if !reflect.DeepEqual(aliases, want) {
		t.Fatalf("aliases = %#v, want %#v", aliases, want)
	}
}

func TestSplitSSHWordsHandlesQuotesAndComments(t *testing.T) {
	got, err := splitSSHWords(`Include "dir with spaces/*.conf" plain\ value # comment`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Include", "dir with spaces/*.conf", "plain value"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
