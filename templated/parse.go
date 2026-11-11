package templated

import (
	"strings"
	"unicode"
)

// Entry is one parsed value with its comma-separated flags.
type Entry struct {
	Raw   string
	Value string
	Flags map[string]string
}

func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return s
	}

	quote := s[0]
	isQuote := quote == '"' || quote == '\''
	if !isQuote || s[len(s)-1] != quote {
		return s
	}

	for i := 1; i < len(s)-1; i++ {
		if s[i] == quote && s[i-1] != '\\' {
			return s
		}
	}

	return s[1 : len(s)-1]
}

func splitTokensQuoteAware(raw string) []string {
	var tokens []string
	var cur strings.Builder
	var inQuote rune

	runes := []rune(raw)
	n := len(runes)

	isAdjacentToPunct := func(idx int, forward bool) bool {
		step := 1
		if !forward {
			step = -1
		}
		for i := idx + step; i >= 0 && i < n; i += step {
			r := runes[i]
			if unicode.IsSpace(r) {
				continue
			}

			return r == ',' || r == ':'
		}

		return false
	}

	for i := 0; i < n; i++ {
		r := runes[i]

		if inQuote == 0 {
			quoteChar := r == '\'' || r == '"'
			if quoteChar {
				inQuote = r
				cur.WriteRune(r)

				continue
			}

			if r == ';' {
				if cur.Len() > 0 {
					tokens = append(tokens, cur.String())
					cur.Reset()
				}

				continue
			}

			if unicode.IsSpace(r) {
				adjacent := isAdjacentToPunct(i, false) || isAdjacentToPunct(i, true)
				if adjacent {
					continue
				}

				if cur.Len() > 0 {
					tokens = append(tokens, cur.String())
					cur.Reset()
				}

				continue
			}

			cur.WriteRune(r)
		} else {
			if r == inQuote {
				inQuote = 0
				cur.WriteRune(r)

				curStr := cur.String()
				isFullyQuoted := len(curStr) > 0 && rune(curStr[0]) == r
				if isFullyQuoted {
					k := i + 1
					for k < n && unicode.IsSpace(runes[k]) {
						k++
					}

					hasSemi := k < n && runes[k] == ';'
					if hasSemi {
						tokens = append(tokens, curStr)
						cur.Reset()
						k++
						for k < n && unicode.IsSpace(runes[k]) {
							k++
						}
						i = k - 1
					}

					hasComma := k < n && runes[k] == ','
					if hasComma {
						next := k + 1
						for next < n && unicode.IsSpace(runes[next]) {
							next++
						}

						nextIsQuote := next < n && (runes[next] == '"' || runes[next] == '\'')
						if nextIsQuote {
							tokens = append(tokens, curStr)
							cur.Reset()
							i = next - 1
						}
					}
				}

				continue
			}
			cur.WriteRune(r)
		}
	}

	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}

	return tokens
}

func splitPartsQuoteAware(token string) []string {
	var parts []string
	var cur strings.Builder
	var inQuote rune

	for _, r := range token {
		if inQuote == 0 {
			quoteChar := r == '\'' || r == '"'
			if quoteChar {
				inQuote = r
				cur.WriteRune(r)
			} else if r == ',' {
				p := strings.TrimSpace(cur.String())
				if p != "" {
					parts = append(parts, p)
				}
				cur.Reset()
			} else {
				cur.WriteRune(r)
			}
		} else {
			if r == inQuote {
				inQuote = 0
			}
			cur.WriteRune(r)
		}
	}

	p := strings.TrimSpace(cur.String())
	if p != "" {
		parts = append(parts, p)
	}

	return parts
}

// ParseEntries parses whitespace- or comma-separated entries with optional comma-separated flags.
func ParseEntries(raw string) []Entry {
	var entries []Entry
	tokens := splitTokensQuoteAware(raw)

	for _, token := range tokens {
		token = strings.Trim(token, ",")
		token = trimQuotes(token)
		if token == "" {
			continue
		}

		parts := splitPartsQuoteAware(token)
		var currentValue string
		var currentParts []string
		currentFlags := make(map[string]string)

		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			lower := strings.ToLower(part)
			if lower == "uri" {
				currentFlags["uri"] = "true"
				currentParts = append(currentParts, part)
			} else if lower == "no-uri" {
				currentFlags["uri"] = "false"
				currentParts = append(currentParts, part)
			} else if strings.HasPrefix(lower, "no-") && len(lower) > 3 && (currentValue != "" || len(currentFlags) > 0) {
				key := strings.TrimPrefix(lower, "no-")
				currentFlags[key] = "false"
				currentParts = append(currentParts, part)
			} else if strings.Contains(part, "=") {
				k, v, _ := strings.Cut(part, "=")
				key := trimQuotes(k)
				val := trimQuotes(v)
				isNewTarget := false
				if key == "instance" && currentFlags["instance"] != "" && currentFlags["project"] != "" {
					isNewTarget = true
				} else if key == "path" && currentFlags["path"] != "" && currentValue == "" {
					isNewTarget = true
				}

				if isNewTarget {
					entries = append(entries, Entry{
						Raw:   strings.Join(currentParts, ","),
						Value: currentValue,
						Flags: currentFlags,
					})
					currentValue = ""
					currentParts = nil
					currentFlags = make(map[string]string)
				}
				currentFlags[key] = val
				currentParts = append(currentParts, part)
			} else if currentValue == "" && len(currentFlags) == 0 {
				currentValue = trimQuotes(part)
				currentParts = append(currentParts, part)
			} else {
				entries = append(entries, Entry{
					Raw:   strings.Join(currentParts, ","),
					Value: currentValue,
					Flags: currentFlags,
				})
				currentValue = trimQuotes(part)
				currentParts = []string{part}
				currentFlags = make(map[string]string)
			}
		}

		if currentValue != "" || len(currentFlags) > 0 {
			entries = append(entries, Entry{
				Raw:   strings.Join(currentParts, ","),
				Value: currentValue,
				Flags: currentFlags,
			})
		}
	}

	return entries
}
