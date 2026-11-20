package model

import (
	"errors"
	"regexp"
	"strings"
)

var errReasoningTable = errors.New("unrecognized official reasoning table; keep last good snapshot")
var geminiID = regexp.MustCompile(`^gemini-[a-z0-9.-]+$`)
var claudeLink = regexp.MustCompile(`\[(Claude [^\]]+)\]\(https://platform\.claude\.com/docs/en/models/([a-z0-9-]+)/overview\)`)
var plainClaudeLabel = regexp.MustCompile(`^Claude [A-Za-z0-9 .-]+$`)
var claudeDefaults = regexp.MustCompile("Setting `effort` to the model[’']s default \\(`\"([a-z]+)\"` on (Claude [^,()]+(?:, Claude [^,()]+)*), `\"([a-z]+)\"` on other models\\)")

func cells(line string) []string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
func parseGeminiEfforts(text string) (officialProfiles, error) {
	_, table, ok := strings.Cut(text, "| Model | Default Thinking | Levels Supported |")
	if !ok {
		return nil, errReasoningTable
	}
	table, tail, complete := strings.Cut(strings.TrimSpace(table), "\n\n")
	if !complete || !strings.HasPrefix(strings.TrimSpace(tail), "### ") {
		return nil, errReasoningTable
	}
	lines := strings.Split(table, "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "| ---") {
		return nil, errReasoningTable
	}
	rows := officialProfiles{}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			break
		}
		c := cells(line)
		if len(c) != 3 || !geminiID.MatchString(c[0]) || rows[c[0]] != nil {
			return nil, errReasoningTable
		}
		p := &ReasoningProfile{SupportedLevels: []string{}}
		for _, v := range strings.Split(c[2], ",") {
			v = strings.TrimSpace(v)
			if !knownReasoningLevel(v) {
				return nil, errReasoningTable
			}
			p.SupportedLevels = append(p.SupportedLevels, v)
		}
		switch {
		case c[1] == "On" || c[1] == "Off": // No specific default effort is stated.
		case strings.HasPrefix(c[1], "On (") && strings.HasSuffix(c[1], ")"):
			p.DefaultLevel = strings.TrimSuffix(strings.TrimPrefix(c[1], "On ("), ")")
		default:
			return nil, errReasoningTable
		}
		rows[c[0]] = p
	}
	return rows, nil
}

func listedClaudeNames(s string) map[string]bool {
	s = strings.ReplaceAll(strings.ReplaceAll(s, ", and ", ", "), " and ", ", ")
	r := map[string]bool{}
	for _, n := range strings.Split(s, ", ") {
		r[n] = true
	}
	return r
}

// ClaudeAvailabilityIDs returns the complete availability row, including labels without an ID.
// A display name alone is never used to manufacture an API identifier.
func ClaudeAvailabilityIDs(text string) (map[string]string, error) {
	var names map[string]string
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "| Available | Claude API,") {
			continue
		}
		c := cells(line)
		if len(c) != 3 || names != nil {
			return nil, errReasoningTable
		}
		names = map[string]string{}
		for item := range listedClaudeNames(c[2]) {
			label, id := item, ""
			if m := claudeLink.FindStringSubmatch(item); m != nil && m[0] == item {
				label, id = m[1], "claude-"+m[2]
			} else if !plainClaudeLabel.MatchString(item) {
				return nil, errReasoningTable
			}
			if _, dup := names[label]; dup {
				return nil, errReasoningTable
			}
			names[label] = id
		}
	}
	if len(names) == 0 || len(names) > 4096 {
		return nil, errReasoningTable
	}
	return names, nil
}

func reconcileClaudeIDs(names, prior map[string]string) error {
	newUnknown := false
	for label, id := range names {
		if id != "" {
			continue
		}
		old, known := prior[label]
		names[label] = old // Only reuse an exact label whose hyperlink established its ID.
		newUnknown = newUnknown || !known
	}
	if newUnknown {
		for label, id := range prior {
			if _, present := names[label]; !present && id != "" {
				return errReasoningTable
			}
		}
	}
	return nil
}

func parseClaudeEfforts(text string, names map[string]string) (officialProfiles, error) {
	_, section, ok := strings.Cut(text, "### Effort levels")
	if !ok {
		return nil, errReasoningTable
	}
	section, _, _ = strings.Cut(section, "\n##")
	byLevel := map[string]string{}
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "| `") {
			c := cells(line)
			if len(c) != 3 {
				return nil, errReasoningTable
			}
			key := strings.Trim(c[0], "`")
			if byLevel[key] != "" {
				return nil, errReasoningTable
			}
			byLevel[key] = c[1]
		}
	}
	for _, l := range []string{"low", "medium", "high"} {
		if byLevel[l] == "" || strings.Contains(byLevel[l], "Available on") {
			return nil, errReasoningTable
		}
	}
	limits := map[string]map[string]bool{}
	for _, level := range []string{"xhigh", "max"} {
		_, list, found := strings.Cut(byLevel[level], "Available on ")
		if !found || !strings.HasSuffix(list, ".") {
			return nil, errReasoningTable
		}
		limits[level] = listedClaudeNames(strings.TrimSuffix(list, "."))
		for label := range limits[level] {
			if _, available := names[label]; !available {
				return nil, errReasoningTable
			}
		}
	}
	defaults := claudeDefaults.FindStringSubmatch(text)
	rows := officialProfiles{}
	for name, id := range names {
		if id == "" {
			continue
		}
		p := &ReasoningProfile{SupportedLevels: []string{"low", "medium", "high"}}
		for _, l := range []string{"xhigh", "max"} {
			if limits[l][name] {
				p.SupportedLevels = append(p.SupportedLevels, l)
			}
		}
		if defaults != nil {
			p.DefaultLevel = defaults[3]
			if listedClaudeNames(defaults[2])[name] {
				p.DefaultLevel = defaults[1]
			}
		}
		if rows[id] != nil {
			return nil, errReasoningTable
		}
		rows[id] = p
	}
	return rows, nil
}
