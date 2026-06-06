package pinkssh

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func DefaultLogPath() string {
	if runtime.GOOS == "windows" {
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, "pinkssh", "pinkssh.log")
		}
	}
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "pinkssh", "pinkssh.log")
	}
	home := HomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "state", "pinkssh", "pinkssh.log")
}

func LogEvent(opts Options, event string, fields map[string]string) {
	if opts.LogPath == "" {
		return
	}
	path := expandPath(opts.LogPath, "")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}

	var parts []string
	parts = append(parts, time.Now().Format(time.RFC3339), event)
	for key, value := range fields {
		value = strings.ReplaceAll(value, "\r", " ")
		value = strings.ReplaceAll(value, "\n", " ")
		parts = append(parts, key+"="+value)
	}
	_ = appendLogLine(path, strings.Join(parts, " ")+"\n")
}

func appendLogLine(path, line string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprint(file, line)
	return err
}

func LogCopyStart(opts Options, host Host, pubkeyPath string) {
	LogEvent(opts, "copy-key:start", map[string]string{
		"alias":  host.Alias,
		"target": host.Address(),
		"pubkey": pubkeyPath,
		"ssh":    resolvedSSHPath(opts),
		"config": opts.ConfigPath,
	})
}

func LogCopyDone(opts Options, host Host, err error) {
	fields := map[string]string{
		"alias": host.Alias,
	}
	if err != nil {
		fields["result"] = "error"
		fields["error"] = err.Error()
	} else {
		fields["result"] = "ok"
	}
	LogEvent(opts, "copy-key:done", fields)
}

func resolvedSSHPath(opts Options) string {
	if opts.SSHPath == "" {
		return "ssh"
	}
	return opts.SSHPath
}
