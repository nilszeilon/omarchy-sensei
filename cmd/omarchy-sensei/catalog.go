package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

type MenuItem struct {
	ID          string   `json:"id"`
	Parent      string   `json:"parent,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	IconFont    string   `json:"iconFont,omitempty"`
	Label       string   `json:"label"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Action      string   `json:"action"`
	Aliases     []string `json:"aliases,omitempty"`
	When        string   `json:"when,omitempty"`
	Checked     string   `json:"checked,omitempty"`
}

type Binding struct {
	Description string   `json:"description"`
	Shortcuts   []string `json:"shortcuts"`
	Dispatcher  string   `json:"dispatcher,omitempty"`
	Argument    string   `json:"argument,omitempty"`
}

type CatalogMatch struct {
	Menu       MenuItem `json:"menu"`
	Binding    Binding  `json:"binding"`
	Confidence string   `json:"confidence"`
}

type Catalog struct {
	Matches           []CatalogMatch `json:"matches"`
	UnmatchedMenu     []MenuItem     `json:"unmatchedMenu"`
	UnmatchedBindings []Binding      `json:"unmatchedBindings"`
}

var trailingComma = regexp.MustCompile(`,(\s*[}\]])`)
var wordsPattern = regexp.MustCompile(`[[:alnum:]]+`)

func loadCatalog(paths Paths) (Catalog, error) {
	menu, err := loadMergedMenu(paths)
	if err != nil {
		return Catalog{}, err
	}
	output, err := resolvedKeybindingRecords()
	if err != nil {
		return Catalog{}, fmt.Errorf("resolve Super+K bindings: %w", err)
	}
	return matchCatalog(menu, bindingsFromRecords(output)), nil
}

func resolvedKeybindingRecords() ([]byte, error) {
	path, err := exec.LookPath("omarchy-menu-keybindings")
	if err != nil {
		return nil, err
	}
	script, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	marker := []byte("\nif [[ $1 == \"--print\"")
	index := strings.LastIndex(string(script), string(marker))
	if index < 0 {
		return exec.Command(path, "--print").Output()
	}
	// Catalog generation is infrequent and needs dispatcher metadata, so avoid
	// a stale display-only cache produced by an older Sensei integration.
	source := string(script[:index]) + "\noutput_binding_records_uncached\n"
	command := exec.Command("bash")
	command.Stdin = strings.NewReader(source)
	return command.Output()
}

func loadMergedMenu(paths Paths) ([]MenuItem, error) {
	defaults, err := os.ReadFile(paths.DefaultMenu)
	if err != nil {
		return nil, fmt.Errorf("read default Omarchy menu: %w", err)
	}
	defaultItems, err := parseMenuJSONC(defaults)
	if err != nil {
		return nil, fmt.Errorf("parse default Omarchy menu: %w", err)
	}

	var userItems []MenuItem
	user, err := os.ReadFile(paths.MenuExtension)
	if err == nil {
		clean := stripManagedBlock(string(user), menuStart, menuEnd)
		userItems, err = parseMenuJSONC([]byte(clean))
		if err != nil {
			return nil, fmt.Errorf("parse user Omarchy menu: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read user Omarchy menu: %w", err)
	}

	order := make([]string, 0, len(defaultItems)+len(userItems))
	merged := map[string]MenuItem{}
	for _, source := range [][]MenuItem{defaultItems, userItems} {
		for _, item := range source {
			if _, exists := merged[item.ID]; !exists {
				order = append(order, item.ID)
			}
			// Omarchy normalizes every source before merging, so an explicit
			// user entry replaces every normalized field for the same id.
			merged[item.ID] = item
		}
	}

	result := make([]MenuItem, 0, len(order))
	for _, id := range order {
		if item := merged[id]; item.Action != "" {
			result = append(result, item)
		}
	}
	return result, nil
}

func parseMenuJSONC(data []byte) ([]MenuItem, error) {
	lines := strings.Split(string(data), "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	clean := trailingComma.ReplaceAllString(strings.Join(kept, "\n"), "$1")
	if strings.TrimSpace(clean) == "" {
		return nil, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(clean), &raw); err != nil {
		return nil, err
	}
	if items, ok := raw["items"]; ok {
		if err := json.Unmarshal(items, &raw); err != nil {
			return nil, err
		}
	}

	result := make([]MenuItem, 0, len(raw))
	for id, encoded := range raw {
		var value struct {
			Parent      *string         `json:"parent"`
			Icon        string          `json:"icon"`
			IconFont    string          `json:"iconFont"`
			Label       string          `json:"label"`
			Title       string          `json:"title"`
			Description string          `json:"description"`
			Action      string          `json:"action"`
			Aliases     json.RawMessage `json:"aliases"`
			When        string          `json:"when"`
			Checked     string          `json:"checked"`
		}
		if err := json.Unmarshal(encoded, &value); err != nil {
			continue
		}
		parent := "root"
		if dot := strings.LastIndex(id, "."); dot >= 0 {
			parent = id[:dot]
		}
		if value.Parent != nil {
			parent = *value.Parent
		}
		label := value.Label
		if label == "" {
			label = id
		}
		result = append(result, MenuItem{
			ID: id, Parent: parent, Icon: value.Icon, IconFont: value.IconFont,
			Label: label, Title: value.Title, Description: value.Description,
			Action: value.Action, Aliases: decodeAliases(value.Aliases),
			When: value.When, Checked: value.Checked,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func decodeAliases(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var one string
	if json.Unmarshal(raw, &one) == nil && one != "" {
		return []string{one}
	}
	return nil
}

func bindingsFromKeybindings(data []byte) []Binding {
	return bindingsFromRecords(data)
}

func bindingsFromRecords(data []byte) []Binding {
	byDescription := map[string]*Binding{}
	var order []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, "\t")
		shortcut, description, found := strings.Cut(fields[0], "→")
		shortcut, description = strings.TrimSpace(shortcut), strings.TrimSpace(description)
		if !found || shortcut == "" || description == "" {
			continue
		}
		key := normalizedPhrase(description)
		binding := byDescription[key]
		if binding == nil {
			binding = &Binding{Description: description}
			byDescription[key] = binding
			order = append(order, key)
		}
		binding.Shortcuts = mergeShortcuts(binding.Shortcuts, shortcut)
		if len(fields) > 1 && binding.Dispatcher == "" {
			binding.Dispatcher = strings.TrimSpace(fields[1])
		}
		if len(fields) > 2 && binding.Argument == "" {
			binding.Argument = strings.TrimSpace(strings.Join(fields[2:], "\t"))
		}
	}
	result := make([]Binding, 0, len(order))
	for _, key := range order {
		result = append(result, *byDescription[key])
	}
	return result
}

func matchCatalog(menu []MenuItem, bindings []Binding) Catalog {
	result := Catalog{}
	matchedBindings := map[string]bool{}
	for _, item := range menu {
		binding, confidence, ok := matchMenuItem(item, bindings)
		if !ok {
			result.UnmatchedMenu = append(result.UnmatchedMenu, item)
			continue
		}
		result.Matches = append(result.Matches, CatalogMatch{Menu: item, Binding: binding, Confidence: confidence})
		matchedBindings[normalizedPhrase(binding.Description)] = true
	}
	for _, binding := range bindings {
		if !matchedBindings[normalizedPhrase(binding.Description)] {
			result.UnmatchedBindings = append(result.UnmatchedBindings, binding)
		}
	}
	return result
}

func matchMenuItem(item MenuItem, bindings []Binding) (Binding, string, bool) {
	menuCommand := normalizedCommand(item.Action)
	if menuCommand != "" {
		var commandMatches []Binding
		for _, binding := range bindings {
			if binding.Dispatcher == "exec" && normalizedCommand(binding.Argument) == menuCommand {
				commandMatches = append(commandMatches, binding)
			}
		}
		if len(commandMatches) == 1 {
			return commandMatches[0], "command-exact", true
		}
	}

	if !coachableMenuNamespace(item.ID) {
		return Binding{}, "", false
	}
	phrases := []string{item.Label, item.Title, item.Description}
	phrases = append(phrases, item.Aliases...)
	for _, phrase := range phrases {
		if phrase == "" {
			continue
		}
		for _, binding := range bindings {
			if normalizedPhrase(phrase) == normalizedPhrase(binding.Description) {
				return binding, "exact", true
			}
		}
	}

	candidates := strongSemanticTokenSets(item)
	bestScore := 0
	var best []Binding
	for _, binding := range bindings {
		bindingTokens := tokenSet(binding.Description)
		score := 0
		for _, candidate := range candidates {
			if setsEqual(candidate, bindingTokens) {
				score = max(score, 300+len(candidate))
			}
		}
		if score > bestScore {
			bestScore, best = score, []Binding{binding}
		} else if score > 0 && score == bestScore {
			best = append(best, binding)
		}
	}
	if len(best) == 1 {
		return best[0], "token-exact", true
	}

	return Binding{}, "", false
}

func strongSemanticTokenSets(item MenuItem) []map[string]bool {
	values := []string{item.Label, item.Title, item.Description}
	values = append(values, item.Aliases...)
	segments := strings.Split(item.ID, ".")
	for start := 0; start < len(segments); start++ {
		values = append(values, strings.Join(segments[start:], " "))
	}
	result := make([]map[string]bool, 0, len(values))
	for _, value := range values {
		if tokens := tokenSet(value); len(tokens) > 0 {
			result = append(result, tokens)
		}
	}
	return result
}

func coachableMenuNamespace(id string) bool {
	for _, prefix := range []string{"trigger.", "system.", "style."} {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

func normalizedCommand(command string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(command)), " ")
}

func normalizedPhrase(value string) string {
	return strings.Join(normalizedWords(value), " ")
}

func tokenSet(value string) map[string]bool {
	result := map[string]bool{}
	for _, word := range normalizedWords(value) {
		result[word] = true
	}
	return result
}

func normalizedWords(value string) []string {
	words := wordsPattern.FindAllString(strings.ToLower(value), -1)
	for index, word := range words {
		if len(word) > 3 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") {
			words[index] = strings.TrimSuffix(word, "s")
		}
	}
	return words
}

func setsEqual(left, right map[string]bool) bool {
	return len(left) == len(right) && isSubset(left, right)
}

func isSubset(left, right map[string]bool) bool {
	for key := range left {
		if !right[key] {
			return false
		}
	}
	return true
}
