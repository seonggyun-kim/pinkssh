package pinkssh

import (
	"context"
	"errors"
	"math"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CommandResult struct {
	ExitCode int
	Output   string
	TimedOut bool
}

func RefreshStatuses(ctx context.Context, hosts []Host, opts Options) []Host {
	out := append([]Host(nil), hosts...)
	if len(out) == 0 {
		return out
	}

	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for i := range out {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = RefreshHostStatus(ctx, out[i], opts)
		}()
	}
	wg.Wait()
	return out
}

func RefreshHostStatus(ctx context.Context, host Host, opts Options) Host {
	host.LocalKey = len(PublicKeyCandidates(host)) > 0
	host.Status = CheckNetwork(ctx, host, opts.ConnectTimeout)
	if host.Status == NetworkOnline || host.Status == NetworkProxy {
		host.Auth = CheckAuth(ctx, host, opts)
	} else {
		host.Auth = AuthUnknown
	}
	host.Badges = BuildBadges(host)
	return host
}

func CheckNetwork(ctx context.Context, host Host, timeout time.Duration) NetworkStatus {
	if host.UsesProxy() {
		return NetworkProxy
	}

	target := host.HostName
	if target == "" {
		target = host.Alias
	}
	if target == "" {
		return NetworkUnknown
	}

	port := host.Port
	if port == "" {
		port = "22"
	}

	dialer := net.Dialer{Timeout: normalizedTimeout(timeout)}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(trimIPv6Brackets(target), port))
	if err != nil {
		return NetworkOffline
	}
	_ = conn.Close()
	return NetworkOnline
}

func CheckAuth(ctx context.Context, host Host, opts Options) AuthStatus {
	ssh := opts.SSHPath
	if ssh == "" {
		ssh = "ssh"
	}

	timeout := normalizedTimeout(opts.ConnectTimeout)
	authCtx, cancel := context.WithTimeout(ctx, authProbeTimeout(timeout))
	defer cancel()

	args := sshArgsForConfig(opts,
		"-o", "BatchMode=yes",
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "PreferredAuthentications=publickey",
		"-o", "NumberOfPasswordPrompts=0",
		"-o", "ConnectTimeout="+strconv.Itoa(timeoutSeconds(timeout)),
		"-T",
		host.Alias,
		"exit",
	)
	cmd := exec.CommandContext(authCtx, ssh, args...)
	out, err := cmd.CombinedOutput()
	result := CommandResult{
		ExitCode: exitCode(err),
		Output:   string(out),
		TimedOut: authCtx.Err() != nil,
	}
	return InterpretAuthResult(result)
}

func InterpretAuthResult(result CommandResult) AuthStatus {
	if result.ExitCode == 0 && !result.TimedOut {
		return AuthKeyOK
	}

	output := strings.ToLower(result.Output)
	switch {
	case strings.Contains(output, "permission denied"):
		return AuthCopyKey
	case strings.Contains(output, "authentications that can continue"):
		return AuthCopyKey
	case strings.Contains(output, "no supported authentication methods"):
		return AuthCopyKey
	case strings.Contains(output, "publickey"):
		return AuthCopyKey
	default:
		return AuthUnknown
	}
}

func normalizedTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 3 * time.Second
	}
	return timeout
}

func timeoutSeconds(timeout time.Duration) int {
	return int(math.Max(1, math.Ceil(timeout.Seconds())))
}

func authProbeTimeout(timeout time.Duration) time.Duration {
	minimum := 10 * time.Second
	if timeout+5*time.Second > minimum {
		return timeout + 5*time.Second
	}
	return minimum
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func trimIPv6Brackets(host string) string {
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	return host
}
