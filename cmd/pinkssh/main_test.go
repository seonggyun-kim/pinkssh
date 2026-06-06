package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCRUDCommandsAcceptFlagsAfterAlias(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config")

	if err := run([]string{"add", "demo", "--config", config, "--hostname", "demo.example", "--user", "alice"}); err != nil {
		t.Fatal(err)
	}
	content := readTestFile(t, config)
	if !strings.Contains(content, "Host demo") || !strings.Contains(content, "HostName demo.example") {
		t.Fatalf("add did not write expected host:\n%s", content)
	}

	if err := run([]string{"edit", "demo", "--config", config, "--hostname", "renamed.example"}); err != nil {
		t.Fatal(err)
	}
	content = readTestFile(t, config)
	if !strings.Contains(content, "HostName renamed.example") {
		t.Fatalf("edit did not update hostname:\n%s", content)
	}

	if err := run([]string{"delete", "demo", "--config", config, "--yes"}); err != nil {
		t.Fatal(err)
	}
	content = readTestFile(t, config)
	if strings.Contains(content, "Host demo") {
		t.Fatalf("delete did not remove host:\n%s", content)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
