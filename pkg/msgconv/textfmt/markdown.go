// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package textfmt

import (
	"html"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"maunium.net/go/mautrix/event"
)

func metaFormatToHTML(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	ranges := parseMetaFormatting(text)
	if len(ranges) == 0 {
		return event.TextToHTML(text)
	}
	sort.SliceStable(ranges, func(i, j int) bool {
		return ranges[i].start < ranges[j].start
	})
	var output strings.Builder
	prevEnd := 0
	for _, formatRange := range ranges {
		if formatRange.start < prevEnd {
			continue
		}
		output.WriteString(event.TextToHTML(text[prevEnd:formatRange.start]))
		output.WriteString(renderMetaFormattingRange(formatRange))
		prevEnd = formatRange.end
	}
	output.WriteString(event.TextToHTML(text[prevEnd:]))
	return output.String()
}

type metaTextFormat int

const (
	metaFormatCodeBlock metaTextFormat = iota
	metaFormatBlockQuote
	metaFormatInlineQuote
	metaFormatInlineCode
	metaFormatBold
	metaFormatItalic
	metaFormatStrikethrough
)

type metaFormatRange struct {
	format   metaTextFormat
	start    int
	end      int
	text     string
	language string
}

var metaFormatRules = []func(string) []metaFormatRange{
	findMetaCodeBlocks,
	findMetaBlockQuotes,
	findMetaInlineQuotes,
	func(text string) []metaFormatRange {
		return findMetaInlineDelimited(text, metaFormatInlineCode, '`', "*_~'\"(", "word*_~,.;:!?'\")", true)
	},
	func(text string) []metaFormatRange {
		return findMetaInlineDelimited(text, metaFormatBold, '*', "_~'\"(", "_~,.;:!?'\")", false)
	},
	func(text string) []metaFormatRange {
		return findMetaInlineDelimited(text, metaFormatItalic, '_', "*~'\"(", "*~,.;:!?'\")", false)
	},
	func(text string) []metaFormatRange {
		return findMetaInlineDelimited(text, metaFormatStrikethrough, '~', "*_'\"(", "*_,.;:!?'\")", false)
	},
}

func parseMetaFormatting(text string) []metaFormatRange {
	var ranges []metaFormatRange
	for _, ruleMatches := range metaFormatRules {
		for _, match := range ruleMatches(text) {
			if match.start >= match.end || overlapsMetaFormatting(ranges, match) {
				continue
			}
			if len(ranges) > 0 {
				last := &ranges[len(ranges)-1]
				if last.format == match.format && last.end+1 == match.start {
					last.end = match.end
					last.text += "\n" + match.text
					continue
				}
			}
			ranges = append(ranges, match)
		}
	}
	return ranges
}

func overlapsMetaFormatting(ranges []metaFormatRange, match metaFormatRange) bool {
	for _, existing := range ranges {
		if match.end > existing.start && match.start < existing.end {
			return true
		}
	}
	return false
}

func renderMetaFormattingRange(formatRange metaFormatRange) string {
	switch formatRange.format {
	case metaFormatCodeBlock:
		class := ""
		if formatRange.language != "" {
			class = ` class="language-` + html.EscapeString(formatRange.language) + `"`
		}
		return "<pre><code" + class + ">" + html.EscapeString(formatRange.text) + "</code></pre>"
	case metaFormatBlockQuote, metaFormatInlineQuote:
		return "<blockquote>" + event.TextToHTML(formatRange.text) + "</blockquote>"
	case metaFormatInlineCode:
		return "<code>" + html.EscapeString(formatRange.text) + "</code>"
	case metaFormatBold:
		return "<strong>" + event.TextToHTML(formatRange.text) + "</strong>"
	case metaFormatItalic:
		return "<em>" + event.TextToHTML(formatRange.text) + "</em>"
	case metaFormatStrikethrough:
		return "<del>" + event.TextToHTML(formatRange.text) + "</del>"
	default:
		return event.TextToHTML(formatRange.text)
	}
}

func findMetaInlineDelimited(text string, format metaTextFormat, delimiter byte, prefixChars, suffixChars string, code bool) []metaFormatRange {
	matches := make([]metaFormatRange, 0)
	for offset := 0; offset < len(text); {
		openRel := strings.IndexByte(text[offset:], delimiter)
		if openRel < 0 {
			break
		}
		open := offset + openRel
		if !hasMetaFormattingPrefix(text, open, prefixChars) {
			offset = open + 1
			continue
		}
		searchEnd := findMetaLineEnd(text, open+1)
		found := false
		for closeSearch := open + 1; closeSearch < searchEnd; {
			closeRel := strings.IndexByte(text[closeSearch:searchEnd], delimiter)
			if closeRel < 0 {
				break
			}
			close := closeSearch + closeRel
			content := text[open+1 : close]
			if isValidMetaInlineContent(content, code) && hasMetaFormattingSuffix(text, close+1, suffixChars) {
				matches = append(matches, metaFormatRange{
					format: format,
					start:  open,
					end:    close + 1,
					text:   content,
				})
				offset = close + 1
				found = true
				break
			}
			closeSearch = close + 1
		}
		if !found {
			offset = open + 1
		}
	}
	return matches
}

func hasMetaFormattingPrefix(text string, offset int, prefixChars string) bool {
	if offset == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(text[:offset])
	return unicode.IsSpace(prev) || strings.ContainsRune(prefixChars, prev)
}

func hasMetaFormattingSuffix(text string, offset int, suffixChars string) bool {
	if offset >= len(text) {
		return true
	}
	next, _ := utf8.DecodeRuneInString(text[offset:])
	if unicode.IsSpace(next) || strings.ContainsRune(suffixChars, next) {
		return true
	}
	return strings.HasPrefix(suffixChars, "word") && isASCIIWord(next)
}

func isASCIIWord(char rune) bool {
	return char == '_' || ('0' <= char && char <= '9') || ('A' <= char && char <= 'Z') || ('a' <= char && char <= 'z')
}

func isValidMetaInlineContent(content string, code bool) bool {
	if content == "" || strings.ContainsRune(content, '\n') || strings.ContainsRune(content, '\u2028') {
		return false
	}
	if code {
		return true
	}
	first, _ := utf8.DecodeRuneInString(content)
	last, _ := utf8.DecodeLastRuneInString(content)
	return !unicode.IsSpace(first) && !unicode.IsSpace(last)
}

func findMetaLineEnd(text string, offset int) int {
	for offset < len(text) {
		char, size := utf8.DecodeRuneInString(text[offset:])
		if char == '\n' || char == '\u2028' {
			return offset
		}
		offset += size
	}
	return len(text)
}

func findMetaCodeBlocks(text string) []metaFormatRange {
	matches := make([]metaFormatRange, 0)
	for offset := 0; offset < len(text); {
		startRel := strings.Index(text[offset:], "```")
		if startRel < 0 {
			break
		}
		start := offset + startRel
		fenceEnd := start
		for fenceEnd < len(text) && text[fenceEnd] == '`' {
			fenceEnd++
		}
		fence := text[start:fenceEnd]
		if match, ok := findMetaCodeBlockAt(text, start, fenceEnd, fence); ok {
			matches = append(matches, match)
			offset = match.end
		} else {
			offset = start + 1
		}
	}
	return matches
}

func findMetaCodeBlockAt(text string, start, fenceEnd int, fence string) (metaFormatRange, bool) {
	if lineEnd, lineBreakEnd, ok := findNextMetaLineBreak(text, fenceEnd); ok {
		if closeStart, closeEnd, ok := findMetaClosingFence(text, lineBreakEnd, fence); ok {
			infoLineWithBreak := text[fenceEnd:lineBreakEnd]
			language := strings.ToLower(strings.TrimSpace(text[fenceEnd:lineEnd]))
			code := text[lineBreakEnd:closeStart]
			if language != "" && !metaCodeBlockLanguages[language] {
				code = infoLineWithBreak + code
				language = ""
			}
			return metaFormatRange{
				format:   metaFormatCodeBlock,
				start:    start,
				end:      closeEnd,
				text:     code,
				language: language,
			}, true
		}
	}
	closeStart, closeEnd, ok := findMetaClosingFence(text, fenceEnd, fence)
	if !ok || closeStart == fenceEnd {
		return metaFormatRange{}, false
	}
	return metaFormatRange{
		format: metaFormatCodeBlock,
		start:  start,
		end:    closeEnd,
		text:   text[fenceEnd:closeStart],
	}, true
}

func findMetaClosingFence(text string, offset int, fence string) (start, end int, ok bool) {
	for {
		rel := strings.Index(text[offset:], fence)
		if rel < 0 {
			return 0, 0, false
		}
		start = offset + rel
		if end, ok = consumeMetaFormattingTail(text, start+len(fence)); ok {
			return start, end, true
		}
		offset = start + 1
	}
}

func consumeMetaFormattingTail(text string, offset int) (int, bool) {
	for offset < len(text) {
		char, size := utf8.DecodeRuneInString(text[offset:])
		if char == '\n' || char == '\u2028' {
			return offset + size, true
		} else if !unicode.IsSpace(char) {
			return 0, false
		}
		offset += size
	}
	return offset, true
}

func findMetaBlockQuotes(text string) []metaFormatRange {
	matches := make([]metaFormatRange, 0)
	for offset := 0; offset < len(text); {
		start := findMetaLinePrefix(text, offset, ">>>")
		if start < 0 {
			break
		}
		contentStart := start + 3
		if contentStart < len(text) && text[contentStart] == ' ' {
			contentStart++
		}
		closeStart, closeEnd, ok := findMetaBlockQuoteClose(text, contentStart)
		if !ok {
			offset = contentStart
			continue
		}
		content := text[contentStart:closeStart]
		if !containsNonWhitespace(content) {
			offset = closeEnd
			continue
		}
		matches = append(matches, metaFormatRange{
			format: metaFormatBlockQuote,
			start:  start,
			end:    closeEnd,
			text:   content,
		})
		offset = closeEnd
	}
	return matches
}

func findMetaBlockQuoteClose(text string, offset int) (int, int, bool) {
	for {
		closeStart := findMetaLinePrefix(text, offset, "<<<")
		if closeStart < 0 {
			return 0, 0, false
		}
		lineEnd, nextLineStart := findMetaLineBounds(text, closeStart)
		if strings.TrimSpace(text[closeStart+3:lineEnd]) == "" {
			return closeStart, nextLineStart, true
		}
		offset = nextLineStart
	}
}

func findMetaInlineQuotes(text string) []metaFormatRange {
	if strings.TrimSpace(text) == ">" {
		return nil
	}
	matches := make([]metaFormatRange, 0)
	for lineStart := 0; lineStart < len(text); {
		lineEnd, nextLineStart := findMetaLineBounds(text, lineStart)
		if strings.HasPrefix(text[lineStart:lineEnd], ">") {
			contentStart := lineStart + 1
			if contentStart < lineEnd && text[contentStart] == ' ' {
				contentStart++
			}
			matches = append(matches, metaFormatRange{
				format: metaFormatInlineQuote,
				start:  lineStart,
				end:    lineEnd,
				text:   text[contentStart:lineEnd],
			})
		}
		lineStart = nextLineStart
	}
	return matches
}

func findMetaLinePrefix(text string, offset int, prefix string) int {
	for offset < len(text) {
		rel := strings.Index(text[offset:], prefix)
		if rel < 0 {
			return -1
		}
		idx := offset + rel
		if isMetaLineStart(text, idx) {
			return idx
		}
		offset = idx + 1
	}
	return -1
}

func isMetaLineStart(text string, offset int) bool {
	if offset == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(text[:offset])
	return prev == '\n' || prev == '\u2028'
}

func findMetaLineBounds(text string, offset int) (lineEnd, nextLineStart int) {
	for offset < len(text) {
		char, size := utf8.DecodeRuneInString(text[offset:])
		if char == '\n' || char == '\u2028' {
			return offset, offset + size
		}
		offset += size
	}
	return len(text), len(text)
}

func findNextMetaLineBreak(text string, offset int) (lineEnd, nextLineStart int, ok bool) {
	for offset < len(text) {
		char, size := utf8.DecodeRuneInString(text[offset:])
		if char == '\n' || char == '\u2028' {
			return offset, offset + size, true
		}
		offset += size
	}
	return 0, 0, false
}

func containsNonWhitespace(text string) bool {
	for _, char := range text {
		if !unicode.IsSpace(char) {
			return true
		}
	}
	return false
}

var metaCodeBlockLanguages = map[string]bool{
	"asp": true, "aspx": true, "bash": true, "c": true, "clj": true,
	"cljc": true, "cljx": true, "clojure": true, "coffeescript": true,
	"cplusplus": true, "cpp": true, "c++": true, "cql": true, "cs": true,
	"csharp": true, "css": true, "curl": true, "d": true, "dart": true,
	"diff": true, "dockerfile": true, "ecmascript": true, "erl": true,
	"erlang": true, "go": true, "gql": true, "gradle": true, "graphql": true,
	"groovy": true, "handlebars": true, "hbs": true, "html": true, "http": true,
	"java": true, "javascript": true, "jl": true, "jruby": true, "js": true,
	"json": true, "jsx": true, "julia": true, "kotlin": true, "kt": true,
	"less": true, "liquid": true, "lua": true, "macruby": true, "markdown": true,
	"ml": true, "mssql": true, "mysql": true, "node": true, "objc": true,
	"objc++": true, "objcpp": true, "objectivec": true,
	"objectivecplusplus": true, "objectivecpp": true, "ocaml": true, "perl": true,
	"pgsql": true, "php": true, "pl": true, "plsql": true, "postgres": true,
	"postgresql": true, "powershell": true, "ps1": true, "py": true,
	"python": true, "r": true, "rake": true, "rb": true, "rbx": true, "rs": true,
	"ruby": true, "rust": true, "sass": true, "scala": true, "scss": true,
	"sh": true, "shell": true, "sol": true, "solidity": true, "sql": true,
	"sqlite": true, "styl": true, "stylus": true, "swift": true, "ts": true,
	"typescript": true, "xhtml": true, "xml": true, "yaml": true, "yml": true,
	"zsh": true,
}
