package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	hyprStart = "-- BEGIN OMARCHY SENSEI (managed by omarchy-sensei setup)"
	hyprEnd   = "-- END OMARCHY SENSEI"
	menuStart = "// BEGIN OMARCHY SENSEI (managed by omarchy-sensei setup)"
	menuEnd   = "// END OMARCHY SENSEI"
)

func setupIntegration(paths Paths) error {
	if err := installSelf(paths.LocalBinary); err != nil {
		return fmt.Errorf("install helper: %w", err)
	}
	if err := installHyprIntegration(paths); err != nil {
		return fmt.Errorf("install Hyprland integration: %w", err)
	}
	catalog, err := loadCatalog(paths)
	if err != nil {
		return fmt.Errorf("build coaching catalog: %w", err)
	}
	if err := installMenuIntegration(paths, catalog); err != nil {
		return fmt.Errorf("install menu integration: %w", err)
	}
	if err := installBindingCache(paths, catalog); err != nil {
		return fmt.Errorf("install click binding cache: %w", err)
	}
	if err := installRefreshWatcher(paths); err != nil {
		return fmt.Errorf("install catalog refresh watcher: %w", err)
	}
	return nil
}

func refreshIntegration(paths Paths) (Catalog, error) {
	catalog, err := loadCatalog(paths)
	if err != nil {
		return Catalog{}, err
	}
	if err := installMenuIntegration(paths, catalog); err != nil {
		return Catalog{}, err
	}
	if err := installBindingCache(paths, catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func installBindingCache(paths Paths, catalog Catalog) error {
	bindings := make([]Binding, 0, len(catalog.Matches)+len(catalog.UnmatchedBindings))
	seen := map[string]bool{}
	for _, match := range catalog.Matches {
		key := normalizedPhrase(match.Binding.Description)
		if !seen[key] {
			bindings = append(bindings, match.Binding)
			seen[key] = true
		}
	}
	for _, binding := range catalog.UnmatchedBindings {
		key := normalizedPhrase(binding.Description)
		if !seen[key] {
			bindings = append(bindings, binding)
			seen[key] = true
		}
	}
	data, err := json.Marshal(bindings)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(paths.BindingCache), 0o700); err != nil {
		return err
	}
	return writeAtomic(paths.BindingCache, append(data, '\n'), 0o600)
}

func uninstallIntegration(paths Paths) error {
	if err := removeManagedBlock(paths.HyprlandConfig, hyprStart, hyprEnd); err != nil {
		return err
	}
	if err := removeManagedBlock(paths.MenuExtension, menuStart, menuEnd); err != nil {
		return err
	}
	if err := uninstallRefreshWatcher(paths); err != nil {
		return err
	}
	err := os.Remove(paths.SenseiLua)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, path := range []string{paths.BindingCache, paths.LocalBinary} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func integrationInstalled(paths Paths) bool {
	content, err := os.ReadFile(paths.HyprlandConfig)
	return err == nil && strings.Contains(string(content), hyprStart)
}

func installSelf(destination string) error {
	source, err := os.Executable()
	if err != nil {
		return err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	if source == destination {
		return nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return writeAtomic(destination, data, 0o755)
}

func installHyprIntegration(paths Paths) error {
	content, err := os.ReadFile(paths.HyprlandConfig)
	if err != nil {
		return err
	}
	clean := stripManagedBlock(string(content), hyprStart, hyprEnd)
	needle := "-- Load Omarchy defaults."
	index := strings.Index(clean, needle)
	if index < 0 {
		return errors.New("could not find the Omarchy defaults marker in hyprland.lua")
	}
	block := hyprStart + "\nrequire(\"default.hypr.helpers\")\nrequire(\"hypr.sensei\")\n" + hyprEnd + "\n\n"
	updated := clean[:index] + block + clean[index:]
	if err := backupAndWrite(paths.HyprlandConfig, []byte(updated), 0o644); err != nil {
		return err
	}
	return writeAtomic(paths.SenseiLua, []byte(senseiLua()), 0o644)
}

func installMenuIntegration(paths Paths, catalog Catalog) error {
	content, err := os.ReadFile(paths.MenuExtension)
	if errors.Is(err, os.ErrNotExist) {
		content = []byte("{\n}\n")
	} else if err != nil {
		return err
	}
	clean := stripManagedBlock(string(content), menuStart, menuEnd)
	open := strings.Index(clean, "{")
	close := strings.LastIndex(clean, "}")
	if open < 0 || close <= open {
		return errors.New("menu extension is not a JSONC object")
	}
	block := menuOverrideBlock(catalog.Matches)
	separator := ""
	for index := close - 1; index > open; index-- {
		if strings.ContainsRune(" \t\r\n", rune(clean[index])) {
			continue
		}
		if clean[index] != ',' {
			separator = "  ,\n"
		}
		break
	}
	// Generated entries come last so they also instrument user-defined menu
	// actions instead of being shadowed by the original user object key.
	updated := clean[:close] + "\n  " + menuStart + "\n" + separator + block + "  " + menuEnd + "\n" + clean[close:]
	return backupAndWrite(paths.MenuExtension, []byte(updated), 0o644)
}

func senseiLua() string {
	var out strings.Builder
	out.WriteString("-- Generated by omarchy-sensei setup. Re-run setup instead of editing this file.\n")
	out.WriteString("if hl and not _G.omarchy_sensei_original_hl_bind then\n")
	out.WriteString("  _G.omarchy_sensei_original_hl_bind = hl.bind\n")
	out.WriteString(`  local function slug(value)
    return tostring(value):lower():gsub("[^%w]+", "-"):gsub("^-", ""):gsub("-$", "")
  end

  local function quote(value)
    return "'" .. tostring(value):gsub("'", "'\\''") .. "'"
  end

  local function coaching_identity(description)
    local text = tostring(description or "")
    if text:match("^Switch to workspace %d+$") or text == "Next workspace"
      or text == "Previous workspace" or text == "Former workspace" then
      return "workspace-switching", "Workspace switching"
    end
    if text:match("^Bar panel %d+$") then
      return "bar-panels", "Bar panels"
    end
    return slug(text), text
  end

  function hl.bind(keys, dispatcher, options)
    local original = _G.omarchy_sensei_original_hl_bind
    local description = options and (options.description or options.desc)
    local key_text = tostring(keys or "")
    local source_scan = type(dispatcher) == "table" and dispatcher.__omarchy_dispatcher
    if source_scan or not description or key_text:find("mouse", 1, true) or key_text:find("switch:", 1, true) then
      return original(keys, dispatcher, options)
    end

    local action, title = coaching_identity(description)
    local command = "omarchy-sensei complete --action " .. quote(action)
      .. " --title " .. quote(title)
    return original(keys, function()
      hl.dispatch(dispatcher)
      hl.exec_cmd(command)
    end, options)
  end
end
`)
	return out.String()
}

func menuOverrideBlock(matches []CatalogMatch) string {
	var out strings.Builder
	for _, match := range matches {
		actionID := actionID(match.Binding.Description)
		command := "omarchy-sensei run --action " + shellQuote(actionID) +
			" --title " + shellQuote(match.Binding.Description) +
			" --shortcut " + shellQuote(match.Binding.Shortcuts[0]) +
			" -- " + shellQuote(match.Menu.Action)
		item := menuOverride(match.Menu, command)
		encodedID, _ := json.Marshal(match.Menu.ID)
		encodedItem, _ := json.Marshal(item)
		fmt.Fprintf(&out, "  %s: %s,\n", encodedID, encodedItem)
	}
	return out.String()
}

func menuOverride(item MenuItem, command string) map[string]any {
	result := map[string]any{
		"parent": item.Parent, "label": item.Label, "action": command,
	}
	if item.Icon != "" {
		result["icon"] = item.Icon
	}
	if item.IconFont != "" {
		result["iconFont"] = item.IconFont
	}
	if item.Title != "" {
		result["title"] = item.Title
	}
	if item.Description != "" {
		result["description"] = item.Description
	}
	if len(item.Aliases) > 0 {
		result["aliases"] = item.Aliases
	}
	if item.When != "" {
		result["when"] = item.When
	}
	if item.Checked != "" {
		result["checked"] = item.Checked
	}
	return result
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func stripManagedBlock(content, start, end string) string {
	startIndex := strings.Index(content, start)
	if startIndex < 0 {
		return content
	}
	lineStart := strings.LastIndex(content[:startIndex], "\n") + 1
	endIndex := strings.Index(content[startIndex:], end)
	if endIndex < 0 {
		return content
	}
	endIndex += startIndex + len(end)
	if endIndex < len(content) && content[endIndex] == '\n' {
		endIndex++
	}
	return content[:lineStart] + content[endIndex:]
}

func removeManagedBlock(path, start, end string) error {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	updated := stripManagedBlock(string(content), start, end)
	if updated == string(content) {
		return nil
	}
	return backupAndWrite(path, []byte(updated), 0o644)
}

func backupAndWrite(path string, data []byte, mode os.FileMode) error {
	if current, err := os.ReadFile(path); err == nil && string(current) == string(data) {
		return nil
	} else if err == nil {
		backup := fmt.Sprintf("%s.sensei-backup-%s", path, time.Now().Format("20060102-150405"))
		if err := os.WriteFile(backup, current, mode); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeAtomic(path, data, mode)
}

func actionID(description string) string {
	return strings.Join(wordsPattern.FindAllString(strings.ToLower(description), -1), "-")
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".sensei-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
