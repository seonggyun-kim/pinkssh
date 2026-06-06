package pinkssh

import (
	"os"
	"path/filepath"
	"time"
)

const AppName = "pinkssh"

type Options struct {
	ConfigPath     string
	SSHPath        string
	ConnectTimeout time.Duration
	Watch          bool
	LogPath        string
}

func DefaultOptions() Options {
	return Options{
		ConfigPath:     filepath.Join(HomeDir(), ".ssh", "config"),
		SSHPath:        "ssh",
		ConnectTimeout: 3 * time.Second,
		Watch:          true,
		LogPath:        DefaultLogPath(),
	}
}

func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func SSHDir() string {
	return filepath.Join(HomeDir(), ".ssh")
}

type Host struct {
	Alias         string        `json:"alias"`
	HostName      string        `json:"hostname,omitempty"`
	User          string        `json:"user,omitempty"`
	Port          string        `json:"port,omitempty"`
	IdentityFiles []string      `json:"identity_files,omitempty"`
	ProxyJump     string        `json:"proxy_jump,omitempty"`
	ProxyCommand  string        `json:"proxy_command,omitempty"`
	Source        string        `json:"source,omitempty"`
	LocalKey      bool          `json:"local_key"`
	Status        NetworkStatus `json:"status"`
	Auth          AuthStatus    `json:"auth_status"`
	Badges        []string      `json:"badges"`
	Error         string        `json:"error,omitempty"`
}

func (h Host) Address() string {
	host := h.HostName
	if host == "" {
		host = h.Alias
	}
	port := h.Port
	if port == "" {
		port = "22"
	}
	if h.User != "" {
		return h.User + "@" + host + ":" + port
	}
	return host + ":" + port
}

func (h Host) UsesProxy() bool {
	return isSet(h.ProxyJump) || isSet(h.ProxyCommand)
}

func isSet(value string) bool {
	switch value {
	case "", "none", "None", "NONE":
		return false
	default:
		return true
	}
}

type NetworkStatus string

const (
	NetworkUnknown NetworkStatus = "unknown"
	NetworkOnline  NetworkStatus = "online"
	NetworkOffline NetworkStatus = "offline"
	NetworkProxy   NetworkStatus = "proxy"
)

type AuthStatus string

const (
	AuthUnknown AuthStatus = "unknown"
	AuthKeyOK   AuthStatus = "key_ok"
	AuthCopyKey AuthStatus = "copy_key"
)

func BuildBadges(h Host) []string {
	var badges []string
	switch h.Status {
	case NetworkOnline:
		badges = append(badges, "ONLINE")
	case NetworkOffline:
		badges = append(badges, "OFFLINE")
	case NetworkProxy:
		badges = append(badges, "PROXY")
	}

	switch h.Auth {
	case AuthKeyOK:
		badges = append(badges, "KEY OK")
	case AuthCopyKey:
		badges = append(badges, "COPY KEY")
	}

	if h.LocalKey {
		badges = append(badges, "LOCAL KEY")
	}
	return badges
}
