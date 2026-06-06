package pinkssh

import (
	"context"
	"testing"
)

func TestInterpretAuthResult(t *testing.T) {
	tests := []struct {
		name string
		in   CommandResult
		want AuthStatus
	}{
		{name: "success", in: CommandResult{ExitCode: 0}, want: AuthKeyOK},
		{name: "permission denied", in: CommandResult{ExitCode: 255, Output: "Permission denied (publickey)."}, want: AuthCopyKey},
		{name: "publickey hint", in: CommandResult{ExitCode: 255, Output: "Authentications that can continue: publickey,password"}, want: AuthCopyKey},
		{name: "network", in: CommandResult{ExitCode: 255, Output: "Connection timed out"}, want: AuthUnknown},
		{name: "timeout", in: CommandResult{ExitCode: -1, TimedOut: true}, want: AuthUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InterpretAuthResult(tt.in); got != tt.want {
				t.Fatalf("InterpretAuthResult() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckAuthUsesFakeSSH(t *testing.T) {
	ssh := fakeSSH(t, "auth")
	host := Host{Alias: "fake"}
	opts := Options{SSHPath: ssh, ConfigPath: ""}

	t.Setenv("PINKSSH_FAKE_MODE", "ok")
	if got := CheckAuth(context.Background(), host, opts); got != AuthKeyOK {
		t.Fatalf("CheckAuth ok = %q", got)
	}

	t.Setenv("PINKSSH_FAKE_MODE", "denied")
	if got := CheckAuth(context.Background(), host, opts); got != AuthCopyKey {
		t.Fatalf("CheckAuth denied = %q", got)
	}
}
