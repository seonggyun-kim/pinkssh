package pinkssh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
)

type ResolvedConfig struct {
	HostName      string
	User          string
	Port          string
	IdentityFiles []string
	ProxyJump     string
	ProxyCommand  string
}

func LoadHosts(ctx context.Context, opts Options) ([]Host, []string, error) {
	discovery, err := DiscoverHosts(opts.ConfigPath)
	if err != nil {
		return nil, WatchPaths(opts.ConfigPath, discovery.Files), err
	}

	hosts := ResolveHosts(ctx, opts, discovery.Entries)
	for i := range hosts {
		hosts[i].LocalKey = len(PublicKeyCandidates(hosts[i])) > 0
		hosts[i].Badges = BuildBadges(hosts[i])
	}
	return hosts, WatchPaths(opts.ConfigPath, discovery.Files), nil
}

func ResolveHosts(ctx context.Context, opts Options, entries []HostEntry) []Host {
	if len(entries) == 0 {
		return nil
	}

	hosts := make([]Host, len(entries))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	for i, entry := range entries {
		i, entry := i, entry
		hosts[i] = Host{
			Alias:  entry.Alias,
			Source: entry.Source,
			Status: NetworkUnknown,
			Auth:   AuthUnknown,
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resolved, err := ResolveHost(ctx, opts, entry.Alias)
			if err != nil {
				hosts[i].Error = err.Error()
				hosts[i].Badges = BuildBadges(hosts[i])
				return
			}
			hosts[i].HostName = resolved.HostName
			hosts[i].User = resolved.User
			hosts[i].Port = resolved.Port
			hosts[i].IdentityFiles = resolved.IdentityFiles
			hosts[i].ProxyJump = resolved.ProxyJump
			hosts[i].ProxyCommand = resolved.ProxyCommand
			hosts[i].Badges = BuildBadges(hosts[i])
		}()
	}

	wg.Wait()
	return hosts
}

func ResolveHost(ctx context.Context, opts Options, alias string) (ResolvedConfig, error) {
	ssh := opts.SSHPath
	if ssh == "" {
		ssh = "ssh"
	}

	args := sshArgsForConfig(opts, "-G", alias)
	cmd := exec.CommandContext(ctx, ssh, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ResolvedConfig{}, commandError(err, out)
	}
	return ParseSSHConfigOutput(out), nil
}

func ParseSSHConfigOutput(output []byte) ResolvedConfig {
	var cfg ResolvedConfig
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		switch key {
		case "hostname":
			cfg.HostName = value
		case "user":
			cfg.User = value
		case "port":
			cfg.Port = value
		case "identityfile":
			if value != "" && !strings.EqualFold(value, "none") {
				cfg.IdentityFiles = append(cfg.IdentityFiles, value)
			}
		case "proxyjump":
			if isSet(value) {
				cfg.ProxyJump = value
			}
		case "proxycommand":
			if isSet(value) {
				cfg.ProxyCommand = value
			}
		}
	}
	return cfg
}

func commandError(err error, output []byte) error {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return err
	}
	return errors.New(text)
}

func sshArgsForConfig(opts Options, args ...string) []string {
	out := sshConfigArgs(opts)
	out = append(out, args...)
	return out
}

func sshArgsForHost(opts Options, alias string, args ...string) []string {
	out := sshConfigArgs(opts)
	out = append(out, alias)
	out = append(out, args...)
	return out
}

func sshConfigArgs(opts Options) []string {
	if opts.ConfigPath == "" {
		return nil
	}
	return []string{"-F", opts.ConfigPath}
}
