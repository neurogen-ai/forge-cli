package config

import (
	"fmt"
	"strings"
)

// ParseTOML parses exactly the PRD §7 subset of TOML: [section] headers and
// key = value pairs where value is a double-quoted string, an integer, or a
// bare word. No arrays, no multiline strings, no nesting beyond one level.
//
// Ints are returned as decimal strings; callers convert. Full-line comments
// start with #; trailing comments follow values (a # inside quotes is kept).
// Re-opening a [section] merges into it; redefining the same key inside one
// section is an error.
func ParseTOML(src []byte) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	section := "" // "" means no section opened yet

	lines := strings.Split(string(src), "\n")
	for i, raw := range lines {
		lineNo := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			end := strings.Index(line, "]")
			if end < 0 {
				return nil, fmt.Errorf("config: line %d: malformed section header %q", lineNo, line)
			}
			rest := strings.TrimSpace(line[end+1:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return nil, fmt.Errorf("config: line %d: unexpected text after section header", lineNo)
			}
			name := strings.TrimSpace(line[1:end])
			if name == "" || strings.ContainsAny(name, "[]") {
				return nil, fmt.Errorf("config: line %d: invalid section name %q", lineNo, name)
			}
			if out[name] == nil {
				out[name] = map[string]string{}
			}
			section = name
			continue
		}

		eq := strings.Index(line, "=")
		if eq < 0 {
			return nil, fmt.Errorf("config: line %d: expected key = value", lineNo)
		}
		key := strings.TrimSpace(line[:eq])
		if key == "" {
			return nil, fmt.Errorf("config: line %d: empty key", lineNo)
		}
		if section == "" {
			return nil, fmt.Errorf("config: line %d: key %q outside any [section]", lineNo, key)
		}
		if _, dup := out[section][key]; dup {
			return nil, fmt.Errorf("config: line %d: duplicate key %q in [%s]", lineNo, key, section)
		}

		val, err := parseValue(strings.TrimSpace(line[eq+1:]), lineNo)
		if err != nil {
			return nil, err
		}
		out[section][key] = val
	}
	return out, nil
}

// parseValue decodes one RHS: quoted string (trailing comment allowed),
// integer, or bare word. Ints pass through as their decimal text.
func parseValue(s string, lineNo int) (string, error) {
	if s == "" {
		return "", fmt.Errorf("config: line %d: missing value", lineNo)
	}
	if s[0] == '"' {
		end := strings.Index(s[1:], `"`)
		if end < 0 {
			return "", fmt.Errorf("config: line %d: unterminated string", lineNo)
		}
		val := s[1 : 1+end]
		rest := strings.TrimSpace(s[2+end:])
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return "", fmt.Errorf("config: line %d: unexpected text after closing quote", lineNo)
		}
		return val, nil
	}
	// Bare word or integer: cut any trailing comment, keep the first token.
	if hash := strings.Index(s, "#"); hash >= 0 {
		s = strings.TrimSpace(s[:hash])
	}
	fields := strings.Fields(s)
	if len(fields) != 1 {
		return "", fmt.Errorf("config: line %d: unquoted value must be a single word", lineNo)
	}
	return fields[0], nil
}
