package pinkssh

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const fakeSSHOutputEnv = "PINKSSH_TEST_FAKE_SSH_OUTPUT"

func TestMain(m *testing.M) {
	if output, ok := os.LookupEnv(fakeSSHOutputEnv); ok {
		os.Exit(runFakeSSH(output))
	}
	os.Exit(m.Run())
}

func fakeSSH(t *testing.T, output string) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeSSHOutputEnv, output)
	return path
}

func runFakeSSH(output string) int {
	switch output {
	case "auth":
		if os.Getenv("PINKSSH_FAKE_MODE") == "ok" {
			return 0
		}
		fmt.Fprintln(os.Stderr, "Permission denied (publickey).")
		return 255
	case "copy":
		logPath := os.Getenv("PINKSSH_FAKE_LOG")
		if logPath == "" {
			return 0
		}
		if err := os.WriteFile(logPath, []byte(strings.Join(os.Args[1:], " ")), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	default:
		fmt.Println(strings.TrimSpace(output))
		return 0
	}
}
