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
	"strings"
	"testing"
)

func TestMetaFormatToHTMLInlineFormatting(t *testing.T) {
	input := `hello *bold* _italic_ ~strike~ ` + "`code <x>`"
	expected := `hello <strong>bold</strong> <em>italic</em> <del>strike</del> <code>code &lt;x&gt;</code>`
	if output := metaFormatToHTML(input); output != expected {
		t.Fatalf("unexpected html:\nexpected: %s\nactual:   %s", expected, output)
	}
}

func TestMetaFormatPriorityPreventsNestedMatches(t *testing.T) {
	input := "`*not bold*` and *_bold wins_*"
	expected := "<code>*not bold*</code> and <strong>_bold wins_</strong>"
	if output := metaFormatToHTML(input); output != expected {
		t.Fatalf("unexpected html:\nexpected: %s\nactual:   %s", expected, output)
	}
}

func TestMetaFormatToHTMLCodeBlock(t *testing.T) {
	input := "before\n```python\nprint(\"<hi>\")\n```\nafter"
	expected := "before<br/><pre><code class=\"language-python\">print(&#34;&lt;hi&gt;&#34;)\n</code></pre>after"
	if output := metaFormatToHTML(input); output != expected {
		t.Fatalf("unexpected html:\nexpected: %s\nactual:   %s", expected, output)
	}
}

func TestMetaFormatToHTMLStripsCarriageReturnsBeforeParsing(t *testing.T) {
	input := "before\r\n*bold*\r\n```go\r\nfmt.Println(\"hi\")\r\n```\r\nafter"
	expected := "before<br/><strong>bold</strong><br/><pre><code class=\"language-go\">fmt.Println(&#34;hi&#34;)\n</code></pre>after"
	if output := metaFormatToHTML(input); output != expected {
		t.Fatalf("unexpected html:\nexpected: %s\nactual:   %s", expected, output)
	}
}

func TestMetaFormatToHTMLUnsupportedCodeBlockLanguage(t *testing.T) {
	input := "```notreal\nbody\n```"
	expected := "<pre><code>notreal\nbody\n</code></pre>"
	if output := metaFormatToHTML(input); output != expected {
		t.Fatalf("unexpected html:\nexpected: %s\nactual:   %s", expected, output)
	}
}

func TestMetaFormatToHTMLMergesAdjacentInlineQuotes(t *testing.T) {
	input := "> first\n> second"
	output := metaFormatToHTML(input)
	if !strings.HasPrefix(output, "<blockquote>") || !strings.HasSuffix(output, "</blockquote>") {
		t.Fatalf("expected one blockquote, got %s", output)
	}
	if strings.Count(output, "<blockquote>") != 1 || !strings.Contains(output, "first") || !strings.Contains(output, "second") {
		t.Fatalf("unexpected blockquote html: %s", output)
	}
}

func TestMetaFormatToHTMLKeepsMentionPlaceholdersInFormatting(t *testing.T) {
	input := "*MENTIONPLACEHOLDER*"
	expected := "<strong>MENTIONPLACEHOLDER</strong>"
	if output := metaFormatToHTML(input); output != expected {
		t.Fatalf("unexpected html:\nexpected: %s\nactual:   %s", expected, output)
	}
}
