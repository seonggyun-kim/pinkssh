package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"pinkssh/internal/pinkssh"
	"pinkssh/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "list":
			return runList(args[1:])
		case "copy-key":
			return runCopyKey(args[1:])
		case "add":
			return runAdd(args[1:])
		case "edit":
			return runEdit(args[1:])
		case "delete", "rm":
			return runDelete(args[1:])
		case "-h", "--help", "help":
			printRootUsage(os.Stdout)
			return nil
		}
	}

	opts := pinkssh.DefaultOptions()
	fs := flag.NewFlagSet("pinkssh", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	finalizeFlags := addCommonFlags(fs, &opts)
	fs.Usage = func() { printRootUsage(os.Stderr) }
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	finalizeFlags()

	switch fs.NArg() {
	case 0:
		return tui.Run(opts)
	case 1:
		return pinkssh.RunConnect(fs.Arg(0), opts)
	default:
		fs.Usage()
		return errors.New("too many arguments")
	}
}

func runList(args []string) error {
	opts := pinkssh.DefaultOptions()
	var jsonOut bool
	fs := flag.NewFlagSet("pinkssh list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	finalizeFlags := addCommonFlags(fs, &opts)
	fs.BoolVar(&jsonOut, "json", false, "print JSON output")
	fs.Usage = func() { printListUsage(os.Stderr) }
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	finalizeFlags()
	if fs.NArg() != 0 {
		fs.Usage()
		return errors.New("list does not accept host arguments")
	}

	ctx := context.Background()
	hosts, _, err := pinkssh.LoadHosts(ctx, opts)
	if err != nil {
		return err
	}
	hosts = pinkssh.RefreshStatuses(ctx, hosts, opts)

	if jsonOut {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(hosts)
	}

	for _, host := range hosts {
		fmt.Printf("%-24s %-32s %s\n", host.Alias, host.Address(), strings.Join(host.Badges, " "))
	}
	return nil
}

func runCopyKey(args []string) error {
	opts := pinkssh.DefaultOptions()
	var pubkey string
	fs := flag.NewFlagSet("pinkssh copy-key", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	finalizeFlags := addCommonFlags(fs, &opts)
	fs.StringVar(&pubkey, "pubkey", "", "public key path")
	fs.Usage = func() { printCopyKeyUsage(os.Stderr) }
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	finalizeFlags()
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("copy-key requires exactly one host")
	}

	alias := fs.Arg(0)
	ctx := context.Background()
	host, err := resolveOneHost(ctx, opts, alias)
	if err != nil {
		return err
	}

	if pubkey == "" {
		pubkey, err = choosePublicKey(host)
	} else {
		pubkey, err = pinkssh.ResolvePublicKey(host, pubkey)
	}
	if err != nil {
		return err
	}

	if err := pinkssh.CopyPublicKey(ctx, host, opts, pubkey); err != nil {
		return err
	}

	host = pinkssh.RefreshHostStatus(ctx, host, opts)
	if host.Auth == pinkssh.AuthKeyOK {
		fmt.Fprintln(os.Stdout, "KEY OK")
	}
	return nil
}

type hostConfigFlagValues struct {
	alias        string
	hostName     string
	user         string
	port         string
	identityFile string
	proxyJump    string
	proxyCommand string
}

func runAdd(args []string) error {
	opts := pinkssh.DefaultOptions()
	var values hostConfigFlagValues
	fs := flag.NewFlagSet("pinkssh add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	finalizeFlags := addCommonFlags(fs, &opts)
	addHostConfigFlags(fs, &values, false)
	fs.Usage = func() { printAddUsage(os.Stderr) }
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	finalizeFlags()
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("add requires exactly one alias")
	}

	spec := pinkssh.HostConfig{
		Alias:         fs.Arg(0),
		HostName:      values.hostName,
		User:          values.user,
		Port:          values.port,
		IdentityFiles: identityFilesFromFlag(values.identityFile),
		ProxyJump:     values.proxyJump,
		ProxyCommand:  values.proxyCommand,
	}
	if err := pinkssh.AddHost(opts.ConfigPath, spec); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "added %s\n", spec.Alias)
	return nil
}

func runEdit(args []string) error {
	opts := pinkssh.DefaultOptions()
	var values hostConfigFlagValues
	fs := flag.NewFlagSet("pinkssh edit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	finalizeFlags := addCommonFlags(fs, &opts)
	addHostConfigFlags(fs, &values, true)
	fs.Usage = func() { printEditUsage(os.Stderr) }
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	finalizeFlags()
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("edit requires exactly one alias")
	}

	changed := visitedFlags(fs)
	if len(changed) == 0 {
		fs.Usage()
		return errors.New("edit requires at least one field flag")
	}

	alias := fs.Arg(0)
	spec, err := pinkssh.ReadHostConfig(opts.ConfigPath, alias)
	if err != nil {
		return err
	}
	if changed["alias"] {
		spec.Alias = values.alias
	}
	if changed["hostname"] {
		spec.HostName = values.hostName
	}
	if changed["user"] {
		spec.User = values.user
	}
	if changed["port"] {
		spec.Port = values.port
	}
	if changed["identity-file"] {
		spec.IdentityFiles = identityFilesFromFlag(values.identityFile)
	}
	if changed["proxy-jump"] {
		spec.ProxyJump = values.proxyJump
	}
	if changed["proxy-command"] {
		spec.ProxyCommand = values.proxyCommand
	}

	if err := pinkssh.UpdateHost(opts.ConfigPath, alias, spec); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "updated %s\n", spec.Alias)
	return nil
}

func runDelete(args []string) error {
	opts := pinkssh.DefaultOptions()
	var yes bool
	fs := flag.NewFlagSet("pinkssh delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	finalizeFlags := addCommonFlags(fs, &opts)
	fs.BoolVar(&yes, "yes", false, "delete without prompting")
	fs.Usage = func() { printDeleteUsage(os.Stderr) }
	if err := parseFlagSet(fs, args); err != nil {
		return err
	}
	finalizeFlags()
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("delete requires exactly one alias")
	}

	alias := fs.Arg(0)
	if !yes {
		ok, err := confirmDelete(alias)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(os.Stdout, "cancelled")
			return nil
		}
	}
	if err := pinkssh.DeleteHost(opts.ConfigPath, alias); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "deleted %s\n", alias)
	return nil
}

func resolveOneHost(ctx context.Context, opts pinkssh.Options, alias string) (pinkssh.Host, error) {
	resolved, err := pinkssh.ResolveHost(ctx, opts, alias)
	host := pinkssh.Host{
		Alias:  alias,
		Status: pinkssh.NetworkUnknown,
		Auth:   pinkssh.AuthUnknown,
	}
	if err != nil {
		host.Error = err.Error()
		return host, nil
	}
	host.HostName = resolved.HostName
	host.User = resolved.User
	host.Port = resolved.Port
	host.IdentityFiles = resolved.IdentityFiles
	host.ProxyJump = resolved.ProxyJump
	host.ProxyCommand = resolved.ProxyCommand
	host.LocalKey = len(pinkssh.PublicKeyCandidates(host)) > 0
	host.Badges = pinkssh.BuildBadges(host)
	return host, nil
}

func choosePublicKey(host pinkssh.Host) (string, error) {
	candidates := pinkssh.PublicKeyCandidates(host)
	if len(candidates) == 0 {
		return "", pinkssh.ErrNoPublicKeys
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}

	fmt.Fprintln(os.Stdout, "Public keys:")
	for i, candidate := range candidates {
		fmt.Fprintf(os.Stdout, "%d. %s\n", i+1, candidate)
	}
	fmt.Fprint(os.Stdout, "Choose key [1]: ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return candidates[0], nil
	}
	index, err := strconv.Atoi(line)
	if err != nil || index < 1 || index > len(candidates) {
		return "", errors.New("invalid public key selection")
	}
	return candidates[index-1], nil
}

func addCommonFlags(fs *flag.FlagSet, opts *pinkssh.Options) func() {
	timeoutSeconds := int(opts.ConnectTimeout / time.Second)
	noWatch := false
	noLog := false
	fs.StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "SSH config path")
	fs.StringVar(&opts.SSHPath, "ssh", opts.SSHPath, "ssh executable path")
	fs.StringVar(&opts.LogPath, "log", opts.LogPath, "debug log path")
	fs.IntVar(&timeoutSeconds, "connect-timeout", timeoutSeconds, "connection timeout in seconds")
	fs.BoolVar(&noWatch, "no-watch", false, "disable SSH config auto-reload")
	fs.BoolVar(&noLog, "no-log", false, "disable debug logging")
	return func() {
		if timeoutSeconds < 1 {
			timeoutSeconds = 1
		}
		opts.ConnectTimeout = time.Duration(timeoutSeconds) * time.Second
		if noWatch {
			opts.Watch = false
		}
		if noLog {
			opts.LogPath = ""
		}
	}
}

func parseFlagSet(fs *flag.FlagSet, args []string) error {
	return fs.Parse(reorderFlags(fs, args))
}

func reorderFlags(fs *flag.FlagSet, args []string) []string {
	var flagArgs []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !isFlagArg(arg) {
			positional = append(positional, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		if strings.Contains(arg, "=") || !flagNeedsValue(fs, arg) {
			continue
		}
		if i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return append(flagArgs, positional...)
}

func isFlagArg(arg string) bool {
	return strings.HasPrefix(arg, "-") && arg != "-"
}

func flagNeedsValue(fs *flag.FlagSet, arg string) bool {
	name := strings.TrimLeft(arg, "-")
	name, _, _ = strings.Cut(name, "=")
	flag := fs.Lookup(name)
	if flag == nil {
		return false
	}
	if boolFlag, ok := flag.Value.(interface{ IsBoolFlag() bool }); ok {
		return !boolFlag.IsBoolFlag()
	}
	return true
}

func addHostConfigFlags(fs *flag.FlagSet, values *hostConfigFlagValues, includeAlias bool) {
	if includeAlias {
		fs.StringVar(&values.alias, "alias", "", "new host alias")
	}
	fs.StringVar(&values.hostName, "hostname", "", "HostName value")
	fs.StringVar(&values.user, "user", "", "User value")
	fs.StringVar(&values.port, "port", "", "Port value")
	fs.StringVar(&values.identityFile, "identity-file", "", "IdentityFile value")
	fs.StringVar(&values.proxyJump, "proxy-jump", "", "ProxyJump value")
	fs.StringVar(&values.proxyCommand, "proxy-command", "", "ProxyCommand value")
}

func identityFilesFromFlag(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func visitedFlags(fs *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	common := map[string]bool{
		"config":          true,
		"ssh":             true,
		"log":             true,
		"connect-timeout": true,
		"no-watch":        true,
		"no-log":          true,
	}
	fs.Visit(func(f *flag.Flag) {
		if !common[f.Name] {
			visited[f.Name] = true
		}
	})
	return visited
}

func confirmDelete(alias string) (bool, error) {
	fmt.Fprintf(os.Stdout, "Delete %s from SSH config? [y/N] ", alias)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false, err
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes", nil
}

func printRootUsage(out *os.File) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  pinkssh [flags]")
	fmt.Fprintln(out, "  pinkssh [flags] <host>")
	fmt.Fprintln(out, "  pinkssh list --json [flags]")
	fmt.Fprintln(out, "  pinkssh copy-key <host> [--pubkey path] [flags]")
	fmt.Fprintln(out, "  pinkssh add <host> [--hostname name] [--user user] [flags]")
	fmt.Fprintln(out, "  pinkssh edit <host> [--alias name] [--hostname name] [flags]")
	fmt.Fprintln(out, "  pinkssh delete <host> [--yes] [flags]")
}

func printListUsage(out *os.File) {
	fmt.Fprintln(out, "Usage: pinkssh list --json [flags]")
}

func printCopyKeyUsage(out *os.File) {
	fmt.Fprintln(out, "Usage: pinkssh copy-key <host> [--pubkey path] [flags]")
}

func printAddUsage(out *os.File) {
	fmt.Fprintln(out, "Usage: pinkssh add <host> [--hostname name] [--user user] [--port port] [--identity-file path] [flags]")
}

func printEditUsage(out *os.File) {
	fmt.Fprintln(out, "Usage: pinkssh edit <host> [--alias name] [--hostname name] [--user user] [--port port] [--identity-file path] [flags]")
}

func printDeleteUsage(out *os.File) {
	fmt.Fprintln(out, "Usage: pinkssh delete <host> [--yes] [flags]")
}
