package pinkssh

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"strings"
	"unicode/utf16"
)

func NewConnectCommand(alias string, opts Options) *exec.Cmd {
	ssh := opts.SSHPath
	if ssh == "" {
		ssh = "ssh"
	}
	cmd := exec.Command(ssh, sshArgsForHost(opts, alias)...)
	return cmd
}

func RunConnect(alias string, opts Options) error {
	cmd := NewConnectCommand(alias, opts)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func NewCopyPublicKeyCommand(host Host, opts Options, pubkeyPath string) (*exec.Cmd, error) {
	pubkey, err := ReadPublicKey(pubkeyPath)
	if err != nil {
		return nil, err
	}

	ssh := opts.SSHPath
	if ssh == "" {
		ssh = "ssh"
	}
	remote := remoteAppendKeyCommand(pubkey)
	cmd := exec.Command(ssh, sshArgsForHost(opts, host.Alias, remote)...)
	return cmd, nil
}

func CopyPublicKey(ctx context.Context, host Host, opts Options, pubkeyPath string) error {
	cmd, err := NewCopyPublicKeyCommand(host, opts, pubkeyPath)
	if err != nil {
		LogCopyDone(opts, host, err)
		return err
	}
	LogCopyStart(opts, host, pubkeyPath)
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	LogCopyDone(opts, host, err)
	return err
}

func remoteAppendKeyCommand(pubkey string) string {
	return "sh -c " + shellDoubleQuote(posixAppendKeyCommand(pubkey)) + " 2>NUL || " + windowsAppendKeyCommand(pubkey)
}

func posixAppendKeyCommand(pubkey string) string {
	quoted := shellQuote(pubkey)
	return strings.Join([]string{
		"key=" + quoted,
		"rm -f NUL 2>/dev/null || true",
		"umask 077",
		"append_key() { file=$1; dir=${file%/*}; mkdir -p \"$dir\" && touch \"$file\" || return 1; chmod 700 \"$dir\" 2>/dev/null || true; chmod 600 \"$file\" 2>/dev/null || true; grep -qxF \"$key\" \"$file\" 2>/dev/null || printf '%s\\n' \"$key\" >> \"$file\"; }",
		"ok=0",
		"if [ -d /etc/dropbear ] || [ -f /etc/dropbear/authorized_keys ]; then append_key /etc/dropbear/authorized_keys && ok=1; fi",
		"home=${HOME:-.}",
		"append_key \"$home/.ssh/authorized_keys\" && ok=1",
		"[ \"$ok\" -eq 1 ]",
	}, "; ")
}

func windowsAppendKeyCommand(pubkey string) string {
	key := powershellQuote(pubkey)
	script := strings.Join([]string{
		"$ErrorActionPreference='Stop'",
		"$ProgressPreference='SilentlyContinue'",
		"$VerbosePreference='SilentlyContinue'",
		"$InformationPreference='SilentlyContinue'",
		"$key=" + key,
		"function Add-Key([string]$file){$dir=Split-Path -Parent $file; New-Item -ItemType Directory -Force -Path $dir | Out-Null; if(-not (Test-Path -LiteralPath $file)){New-Item -ItemType File -Force -Path $file | Out-Null}; $existing=@(); if(Test-Path -LiteralPath $file){$existing=Get-Content -LiteralPath $file -ErrorAction SilentlyContinue}; if($existing -notcontains $key){Add-Content -LiteralPath $file -Value $key -Encoding ascii}}",
		"$identity=[System.Security.Principal.WindowsIdentity]::GetCurrent()",
		"$principal=New-Object System.Security.Principal.WindowsPrincipal($identity)",
		"$isAdmin=$principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)",
		"$userDir=Join-Path $env:USERPROFILE '.ssh'",
		"$userFile=Join-Path $userDir 'authorized_keys'",
		"Add-Key $userFile",
		"try{$acct=$identity.Name; icacls.exe $userDir /inheritance:r /grant \"${acct}:(OI)(CI)F\" /grant \"*S-1-5-18:(OI)(CI)F\" | Out-Null; icacls.exe $userFile /inheritance:r /grant \"${acct}:F\" /grant \"*S-1-5-18:F\" | Out-Null}catch{}",
		"if($isAdmin -and $env:ProgramData){$adminFile=Join-Path $env:ProgramData 'ssh\\administrators_authorized_keys'; Add-Key $adminFile; icacls.exe $adminFile /inheritance:r /grant '*S-1-5-32-544:F' /grant '*S-1-5-18:F' | Out-Null}",
	}, "; ")
	powershell := "powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -OutputFormat Text -EncodedCommand " + encodePowerShell(script)
	return `cmd.exe /d /s /c "` + powershell + ` 2>NUL"`
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func shellDoubleQuote(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		`$`, `\$`,
		"`", "\\`",
	)
	return `"` + replacer.Replace(value) + `"`
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func encodePowerShell(script string) string {
	encoded := utf16.Encode([]rune(script))
	bytes := make([]byte, len(encoded)*2)
	for i, value := range encoded {
		bytes[i*2] = byte(value)
		bytes[i*2+1] = byte(value >> 8)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
