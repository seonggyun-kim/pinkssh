# pinkssh

`pinkssh` is a cross-platform SSH host picker for the terminal. It reads aliases from your OpenSSH config, resolves each alias through `ssh -G`, shows reachability and key-auth badges, and connects by running the native `ssh` client.

Passwords and private keys are never stored by `pinkssh`. Interactive prompts belong to OpenSSH.

## Commands

```sh
pinkssh
pinkssh <host>
pinkssh list --json
pinkssh copy-key <host> [--pubkey path]
pinkssh add <host> [--hostname name] [--user user]
pinkssh edit <host> [--alias name] [--hostname name]
pinkssh delete <host> [--yes]
```

Common flags:

```sh
--config <path>
--ssh <path>
--log <path>
--no-log
--connect-timeout <seconds>
--no-watch
```

## TUI

The interface uses a centered floating panel that adapts to the terminal width. Wide terminals keep the panel capped for easier scanning, while narrow terminals use nearly the full shell width. The host table uses `Host`, `Address`, `Status`, `Key`, and `Via` columns.

`Key` states are `checking`, `accepted`, `needs copy`, `local only`, and `no key`.

`Via` shows the SSH routing method: `direct` means no proxy setting, `jump` means `ProxyJump`/`ssh -J`, and `cmd` means `ProxyCommand`.

Add/edit forms, fuzzy search, key selection, and delete confirmation open as floating modal boxes inside the panel. The selected row uses magenta foreground text without a background fill. Press `Enter` to connect, `/` to fuzzy-search hosts and addresses, `c` to copy a public key, `a` to add a host, `e` to edit the selected host, `d` to delete the selected host, `r` to reload, and `q` to quit.

In add/edit forms, the `IdentityFile` row shows existing local keys as `~/.ssh/<key>` choices when focused. Use `Up`/`Down` on that row to select a key, and `Tab` to move to the next field.

The host list reloads when `~/.ssh/config`, included config files, or `~/.ssh/*.pub` change.

## Key Copy

The copy flow appends a selected `.pub` key to the remote authorized keys file using the native `ssh` client. POSIX-style targets use `sh`, `mkdir`, `grep`, and `printf`; OpenWrt/Dropbear targets also get `/etc/dropbear/authorized_keys` when available. Windows OpenSSH targets are handled with PowerShell and write to `%USERPROFILE%\.ssh\authorized_keys`; administrator accounts also get `%ProgramData%\ssh\administrators_authorized_keys`.

Public key selection order:

1. Configured `IdentityFile.pub`
2. `~/.ssh/id_ed25519.pub`
3. Other `~/.ssh/*.pub`

## Config Editing

`add` appends a new `Host` block to the configured SSH config file. `edit` updates the concrete host block where the alias was discovered, preserving comments and unknown options in that block. `delete` removes the alias from a multi-host line, or removes the whole block when it was the only alias.
