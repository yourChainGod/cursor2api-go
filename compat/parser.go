package compat

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	openJSONBlockPattern   = regexp.MustCompile("```json(?:\\s+action)?")
	toolNamePattern        = regexp.MustCompile(`"(?:tool|name)"\s*:\s*"([^"]+)"`)
	parametersStartPattern = regexp.MustCompile(`"(?:parameters|arguments|input)"\s*:\s*(\{[\s\S]*)`)
	fieldPattern           = regexp.MustCompile(`"([^"]+)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
)

const (
	thinkingStartTag  = "<thinking>"
	thinkingEndTag    = "</thinking>"
	toolFenceStartTag = "```json"
	streamTailRunes   = 24
)

func HasToolCalls(text string) bool {
	return strings.Contains(text, "```json")
}

// ParseResponseSegments splits a model response into ordered text / thinking /
// tool_use segments. Tool blocks are parsed into structured calls. Thinking
// blocks are emitted in-order and stripped from plain text.
func ParseResponseSegments(responseText string) []ResponseSegment {
	segments := make([]ResponseSegment, 0)
	pos := 0
	for pos < len(responseText) {
		nextThinking := strings.Index(responseText[pos:], thinkingStartTag)
		nextTool := -1
		if loc := openJSONBlockPattern.FindStringIndex(responseText[pos:]); loc != nil {
			nextTool = loc[0]
		}

		markerType := ""
		markerRel := -1
		switch {
		case nextThinking >= 0 && (nextTool == -1 || nextThinking < nextTool):
			markerType = "thinking"
			markerRel = nextThinking
		case nextTool >= 0:
			markerType = "tool"
			markerRel = nextTool
		default:
			if tail := responseText[pos:]; tail != "" {
				segments = appendTextSegment(segments, tail)
			}
			pos = len(responseText)
			continue
		}

		markerAbs := pos + markerRel
		if markerAbs > pos {
			segments = appendTextSegment(segments, responseText[pos:markerAbs])
		}

		if markerType == "thinking" {
			start := markerAbs + len(thinkingStartTag)
			endRel := strings.Index(responseText[start:], thinkingEndTag)
			if endRel == -1 {
				segments = append(segments, ResponseSegment{Type: "thinking", Thinking: strings.TrimSpace(responseText[start:])})
				pos = len(responseText)
				continue
			}
			end := start + endRel
			segments = append(segments, ResponseSegment{Type: "thinking", Thinking: strings.TrimSpace(responseText[start:end])})
			pos = end + len(thinkingEndTag)
			continue
		}

		openLoc := openJSONBlockPattern.FindStringIndex(responseText[markerAbs:])
		if openLoc == nil {
			segments = appendTextSegment(segments, responseText[markerAbs:])
			pos = len(responseText)
			continue
		}
		contentStart := markerAbs + openLoc[1]
		closingPos := findClosingFence(responseText, contentStart)
		endPos := len(responseText)
		jsonContent := strings.TrimSpace(responseText[contentStart:])
		if closingPos >= 0 {
			jsonContent = strings.TrimSpace(responseText[contentStart:closingPos])
			endPos = closingPos + 3
		}
		if parsed, err := tolerantParse(jsonContent); err == nil {
			name, args := extractToolPayload(parsed)
			if name != "" {
				args = fixToolCallArguments(name, args)
				call := ParsedToolCall{Name: name, Arguments: args}
				segments = append(segments, ResponseSegment{Type: "tool_use", ToolCall: &call})
				pos = endPos
				continue
			}
		}
		segments = appendTextSegment(segments, responseText[markerAbs:endPos])
		pos = endPos
	}
	return mergeAdjacentTextSegments(segments)
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

	// Sort and merge overlapping spans to prevent slice-bounds panic when
	// a tool argument contains nested code fences.
	if len(remove) > 1 {
		sort.Slice(remove, func(i, j int) bool { return remove[i].start < remove[j].start })
		mergedSpans := remove[:1]
		for _, s := range remove[1:] {
			last := &mergedSpans[len(mergedSpans)-1]
			if s.start <= last.end {
				if s.end > last.end {
					last.end = s.end
				}
			} else {
				mergedSpans = append(mergedSpans, s)
			}
		}
		remove = mergedSpans
	}

	cleanText := responseText
	for i := len(remove) - 1; i >= 0; i-- {
		s := remove[i]
		if s.start < 0 || s.end > len(cleanText) || s.start > s.end {
			continue
		}
		cleanText = cleanText[:s.start] + cleanText[s.end:]
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

type StreamResponseParser struct {
	toolsEnabled    bool
	thinkingEnabled bool
	mode            string
	normalBuffer    string
	thinkingBuffer  string
	toolBuffer      string
	toolInString    bool
	toolEscaped     bool
	events          []ResponseSegment
}

func NewStreamResponseParser(toolsEnabled, thinkingEnabled bool) *StreamResponseParser {
	return &StreamResponseParser{toolsEnabled: toolsEnabled, thinkingEnabled: thinkingEnabled, mode: "normal", events: make([]ResponseSegment, 0, 8)}
}

func (p *StreamResponseParser) Feed(delta string) {
	for _, r := range delta {
		p.feedRune(r)
	}
	if p.mode == "normal" {
		p.flushNormalPartial(false)
	}
}

func (p *StreamResponseParser) Finish() {
	switch p.mode {
	case "thinking":
		if strings.TrimSpace(p.thinkingBuffer) != "" {
			p.events = append(p.events, ResponseSegment{Type: "thinking", Thinking: strings.TrimSpace(p.thinkingBuffer)})
		}
	case "tool":
		if calls, clean := ParseToolCalls(p.toolBuffer); len(calls) > 0 {
			if strings.TrimSpace(clean) != "" {
				p.events = appendTextSegment(p.events, clean)
			}
			for _, call := range calls {
				callCopy := call
				p.events = append(p.events, ResponseSegment{Type: "tool_use", ToolCall: &callCopy})
			}
		} else if p.toolBuffer != "" {
			p.events = appendTextSegment(p.events, p.toolBuffer)
		}
	}
	p.mode = "normal"
	p.thinkingBuffer = ""
	p.toolBuffer = ""
	p.toolInString = false
	p.toolEscaped = false
	p.flushNormalPartial(true)
}

func (p *StreamResponseParser) ConsumeEvents() []ResponseSegment {
	pending := p.events
	p.events = nil
	return pending
}

func (p *StreamResponseParser) feedRune(r rune) {
	switch p.mode {
	case "thinking":
		p.thinkingBuffer += string(r)
		if strings.HasSuffix(p.thinkingBuffer, thinkingEndTag) {
			content := strings.TrimSpace(strings.TrimSuffix(p.thinkingBuffer, thinkingEndTag))
			if content != "" {
				p.events = append(p.events, ResponseSegment{Type: "thinking", Thinking: content})
			}
			p.thinkingBuffer = ""
			p.mode = "normal"
		}
		return
	case "tool":
		p.toolBuffer += string(r)
		if p.toolEscaped {
			p.toolEscaped = false
			return
		}
		if p.toolInString {
			switch r {
			case '\\':
				p.toolEscaped = true
			case '"':
				p.toolInString = false
			}
			return
		}
		if r == '"' {
			p.toolInString = true
			return
		}
		if strings.HasSuffix(p.toolBuffer, "```") && len([]rune(p.toolBuffer)) > len(toolFenceStartTag)+3 {
			if calls, clean := ParseToolCalls(p.toolBuffer); len(calls) > 0 {
				if strings.TrimSpace(clean) != "" {
					p.events = appendTextSegment(p.events, clean)
				}
				for _, call := range calls {
					callCopy := call
					p.events = append(p.events, ResponseSegment{Type: "tool_use", ToolCall: &callCopy})
				}
			} else {
				p.events = appendTextSegment(p.events, p.toolBuffer)
			}
			p.toolBuffer = ""
			p.toolInString = false
			p.toolEscaped = false
			p.mode = "normal"
		}
		return
	default:
		p.normalBuffer += string(r)
		if p.thinkingEnabled && strings.HasSuffix(p.normalBuffer, thinkingStartTag) {
			prefix := strings.TrimSuffix(p.normalBuffer, thinkingStartTag)
			if prefix != "" {
				p.events = appendTextSegment(p.events, prefix)
			}
			p.normalBuffer = ""
			p.mode = "thinking"
			return
		}
		if p.toolsEnabled && strings.HasSuffix(p.normalBuffer, toolFenceStartTag) {
			prefix := strings.TrimSuffix(p.normalBuffer, toolFenceStartTag)
			if prefix != "" {
				p.events = appendTextSegment(p.events, prefix)
			}
			p.normalBuffer = ""
			p.toolBuffer = toolFenceStartTag
			p.mode = "tool"
			p.toolInString = false
			p.toolEscaped = false
			return
		}
	}
}

func (p *StreamResponseParser) flushNormalPartial(force bool) {
	if p.normalBuffer == "" {
		return
	}
	runes := []rune(p.normalBuffer)
	if !force && len(runes) <= streamTailRunes {
		return
	}
	cut := len(runes)
	if !force {
		cut -= streamTailRunes
	}
	if cut <= 0 {
		return
	}
	text := string(runes[:cut])
	if text != "" {
		p.events = appendTextSegment(p.events, text)
	}
	p.normalBuffer = string(runes[cut:])
}

func appendTextSegment(segments []ResponseSegment, text string) []ResponseSegment {
	if text == "" {
		return segments
	}
	return append(segments, ResponseSegment{Type: "text", Text: text})
}

func mergeAdjacentTextSegments(segments []ResponseSegment) []ResponseSegment {
	if len(segments) <= 1 {
		return segments
	}
	merged := make([]ResponseSegment, 0, len(segments))
	for _, seg := range segments {
		if seg.Type == "text" && len(merged) > 0 && merged[len(merged)-1].Type == "text" {
			merged[len(merged)-1].Text += seg.Text
			continue
		}
		merged = append(merged, seg)
	}
	return merged
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
