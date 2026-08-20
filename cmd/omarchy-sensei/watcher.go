package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func installRefreshWatcher(paths Paths) error {
	service := `[Unit]
Description=Refresh the Omarchy Sensei coaching catalog

[Service]
Type=oneshot
ExecStartPre=/usr/bin/sleep 1
ExecStart=%h/.local/bin/omarchy-sensei refresh
`
	pathUnit := fmt.Sprintf(`[Unit]
Description=Watch Omarchy actions and keybindings for Sensei

[Path]
PathChanged=%s
PathChanged=%%h/.config/omarchy/extensions/omarchy-menu.jsonc
PathChanged=%%h/.config/hypr/bindings.lua
Unit=omarchy-sensei-refresh.service

[Install]
WantedBy=default.target
`, paths.DefaultMenu)
	hook := `#!/usr/bin/env bash
if ! omarchy-sensei refresh; then
  logger -t omarchy-sensei "Catalog refresh skipped after Omarchy update"
fi
`
	for path, file := range map[string]struct {
		data string
		mode os.FileMode
	}{
		paths.RefreshService: {service, 0o644},
		paths.RefreshPath:    {pathUnit, 0o644},
		paths.PostUpdateHook: {hook, 0o755},
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := writeAtomic(path, []byte(file.data), file.mode); err != nil {
			return err
		}
	}
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return err
	}
	if output, err := exec.Command("systemctl", "--user", "enable", "--now", "omarchy-sensei-refresh.path").CombinedOutput(); err != nil {
		return fmt.Errorf("enable refresh watcher: %w: %s", err, output)
	}
	return nil
}

func uninstallRefreshWatcher(paths Paths) error {
	_ = exec.Command("systemctl", "--user", "disable", "--now", "omarchy-sensei-refresh.path").Run()
	for _, path := range []string{paths.RefreshPath, paths.RefreshService, paths.PostUpdateHook} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return exec.Command("systemctl", "--user", "daemon-reload").Run()
}
