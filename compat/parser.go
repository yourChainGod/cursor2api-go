package compat

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	openJSONBlockPattern   = regexp.MustCompile("```json(?:\\s+action)?")
	toolNamePattern        = regexp.MustCompile(`"(?:tool|name)"\s*:\s*"([^"]+)"`)
	parametersStartPattern = regexp.MustCompile(`"(?:parameters|arguments|input)"\s*:\s*(\{[\s\S]*)`)
	fieldPattern           = regexp.MustCompile(`"([^"]+)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
)

func HasToolCalls(text string) bool {
	return strings.Contains(text, "```json")
}

func ParseToolCalls(responseText string) ([]ParsedToolCall, string) {
	toolCalls := make([]ParsedToolCall, 0)
	type span struct{ start, end int }
	remove := make([]span, 0)

	matches := openJSONBlockPattern.FindAllStringIndex(responseText, -1)
	for _, match := range matches {
		blockStart := match[0]
		contentStart := match[1]
		closingPos := findClosingFence(responseText, contentStart)

		var jsonContent string
		var endPos int
		if closingPos >= 0 {
			jsonContent = strings.TrimSpace(responseText[contentStart:closingPos])
			endPos = closingPos + 3
		} else {
			jsonContent = strings.TrimSpace(responseText[contentStart:])
			endPos = len(responseText)
		}

		if len(jsonContent) < 2 {
			continue
		}

		parsed, err := tolerantParse(jsonContent)
		if err != nil {
			continue
		}

		name, args := extractToolPayload(parsed)
		if name == "" {
			continue
		}
		args = fixToolCallArguments(name, args)
		toolCalls = append(toolCalls, ParsedToolCall{Name: name, Arguments: args})
		remove = append(remove, span{start: blockStart, end: endPos})
	}

	cleanText := responseText
	for i := len(remove) - 1; i >= 0; i-- {
		cleanText = cleanText[:remove[i].start] + cleanText[remove[i].end:]
	}

	return toolCalls, strings.TrimSpace(cleanText)
}

func findClosingFence(text string, start int) int {
	inJSONString := false
	escaped := false
	for pos := start; pos < len(text)-2; pos++ {
		ch := text[pos]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inJSONString {
			escaped = true
			continue
		}
		if ch == '"' {
			inJSONString = !inJSONString
			continue
		}
		if !inJSONString && text[pos:pos+3] == "```" {
			return pos
		}
	}
	return -1
}

func tolerantParse(input string) (map[string]interface{}, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(input), &parsed); err == nil {
		return parsed, nil
	}

	fixed := repairBrokenJSON(input)
	if err := json.Unmarshal([]byte(fixed), &parsed); err == nil {
		return parsed, nil
	}

	if last := strings.LastIndex(fixed, "}"); last > 0 {
		if err := json.Unmarshal([]byte(fixed[:last+1]), &parsed); err == nil {
			return parsed, nil
		}
	}

	fallback, err := tolerantRegexParse(input)
	if err == nil {
		return fallback, nil
	}

	return nil, fmt.Errorf("unable to parse tool payload")
}

func repairBrokenJSON(input string) string {
	var out strings.Builder
	inString := false
	escaped := false
	stack := make([]rune, 0)

	for _, ch := range input {
		switch {
		case ch == '\\' && !escaped:
			escaped = true
			out.WriteRune(ch)
		case ch == '"' && !escaped:
			inString = !inString
			out.WriteRune(ch)
		case inString && ch == '\n':
			out.WriteString(`\n`)
		case inString && ch == '\r':
			out.WriteString(`\r`)
		case inString && ch == '\t':
			out.WriteString(`\t`)
		default:
			if !inString {
				switch ch {
				case '{':
					stack = append(stack, '}')
				case '[':
					stack = append(stack, ']')
				case '}', ']':
					if len(stack) > 0 {
						stack = stack[:len(stack)-1]
					}
				}
			}
			out.WriteRune(ch)
			escaped = false
		}
		if ch != '\\' {
			escaped = false
		}
	}

	if inString {
		out.WriteRune('"')
	}
	for i := len(stack) - 1; i >= 0; i-- {
		out.WriteRune(stack[i])
	}

	fixed := out.String()
	trailingComma := regexp.MustCompile(`,\s*([}\]])`)
	return trailingComma.ReplaceAllString(fixed, `$1`)
}

func tolerantRegexParse(input string) (map[string]interface{}, error) {
	toolMatch := toolNamePattern.FindStringSubmatch(input)
	if len(toolMatch) < 2 {
		return nil, fmt.Errorf("tool name not found")
	}
	result := map[string]interface{}{"tool": toolMatch[1], "parameters": map[string]interface{}{}}

	paramsMatch := parametersStartPattern.FindStringSubmatch(input)
	if len(paramsMatch) < 2 {
		return result, nil
	}

	paramsStr, ok := extractJSONObject(paramsMatch[1])
	if !ok {
		return result, nil
	}

	params := map[string]interface{}{}
	if err := json.Unmarshal([]byte(paramsStr), &params); err == nil {
		result["parameters"] = params
		return result, nil
	}

	for _, match := range fieldPattern.FindAllStringSubmatch(paramsStr, -1) {
		params[match[1]] = strings.ReplaceAll(strings.ReplaceAll(match[2], `\n`, "\n"), `\t`, "\t")
	}
	result["parameters"] = params
	return result, nil
}

func extractJSONObject(input string) (string, bool) {
	depth := 0
	inString := false
	escaped := false
	for i, ch := range input {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return input[:i+1], true
			}
		}
	}
	return "", false
}

func extractToolPayload(parsed map[string]interface{}) (string, map[string]interface{}) {
	name := stringValue(parsed["tool"])
	if name == "" {
		name = stringValue(parsed["name"])
	}
	var args map[string]interface{}
	for _, key := range []string{"parameters", "arguments", "input"} {
		if raw, ok := parsed[key].(map[string]interface{}); ok {
			args = raw
			break
		}
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	return name, args
}

func fixToolCallArguments(toolName string, args map[string]interface{}) map[string]interface{} {
	args = normalizeToolArguments(args)
	return repairExactMatchToolArguments(toolName, args)
}

func normalizeToolArguments(args map[string]interface{}) map[string]interface{} {
	return args
}

func repairExactMatchToolArguments(toolName string, args map[string]interface{}) map[string]interface{} {
	lowerName := strings.ToLower(toolName)
	if !strings.Contains(lowerName, "str_replace") && !strings.Contains(lowerName, "search_replace") && !strings.Contains(lowerName, "strreplace") {
		return args
	}

	oldString := stringValue(args["old_string"])
	if oldString == "" {
		oldString = stringValue(args["old_str"])
	}
	if oldString == "" {
		return args
	}

	filePath := stringValue(args["path"])
	if filePath == "" {
		filePath = stringValue(args["file_path"])
	}
	if filePath == "" {
		return args
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return args
	}
	text := string(content)
	if strings.Contains(text, oldString) {
		return args
	}

	pattern := buildFuzzyPattern(oldString)
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return args
	}
	matches := regex.FindAllString(text, -1)
	if len(matches) != 1 {
		return args
	}

	matched := matches[0]
	if _, ok := args["old_string"]; ok {
		args["old_string"] = matched
	} else if _, ok := args["old_str"]; ok {
		args["old_str"] = matched
	}

	if _, ok := args["new_string"]; ok {
		args["new_string"] = replaceSmartQuotes(stringValue(args["new_string"]))
	} else if _, ok := args["new_str"]; ok {
		args["new_str"] = replaceSmartQuotes(stringValue(args["new_str"]))
	}
	return args
}

func buildFuzzyPattern(text string) string {
	var parts strings.Builder
	for _, ch := range text {
		switch {
		case isSmartDoubleQuote(ch) || ch == '"':
			parts.WriteString(`["\u00ab\u201c\u201d\u275e\u201f\u201e\u275d\u00bb]`)
		case isSmartSingleQuote(ch) || ch == '\'':
			parts.WriteString(`['\u2018\u2019\u201a\u201b]`)
		case ch == ' ' || ch == '\t':
			parts.WriteString(`\s+`)
		case ch == '\\':
			parts.WriteString(`\\{1,2}`)
		default:
			parts.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	return parts.String()
}

func replaceSmartQuotes(text string) string {
	runes := []rune(text)
	for i, ch := range runes {
		switch {
		case isSmartDoubleQuote(ch):
			runes[i] = '"'
		case isSmartSingleQuote(ch):
			runes[i] = '\''
		}
	}
	return string(runes)
}

func isSmartDoubleQuote(ch rune) bool {
	switch ch {
	case '\u00ab', '\u201c', '\u201d', '\u275e', '\u201f', '\u201e', '\u275d', '\u00bb':
		return true
	default:
		return false
	}
}

func isSmartSingleQuote(ch rune) bool {
	switch ch {
	case '\u2018', '\u2019', '\u201a', '\u201b':
		return true
	default:
		return false
	}
}
