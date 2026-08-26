package main

import (
	"regexp"
	"strings"
)

// Slack の mrkdwn にはリスト記法が無いため、投稿は rich_text ブロックで組み立てる。
// 対応する記法は WOL の投稿で実際に使う分だけに絞っている:
//
//	行頭 "- " / "* "      箇条書き（先頭の空白 2 つで 1 段ネスト）
//	行頭 "1. " / "1) "    番号付きリスト
//	`code`               インラインコード
//	[表示文字](URL)       リンク
//	素の http(s) URL      リンク
//
// 太字・斜体などは本文では扱わない（見出しと投稿者名にのみ内部で使う）。

var (
	bulletRE  = regexp.MustCompile(`^(\s*)[-*]\s+(.*)$`)
	orderedRE = regexp.MustCompile(`^(\s*)\d+[.)]\s+(.*)$`)
	// インライン記法をまとめて拾う。グループ 1: コード, 2: リンク表示文字, 3: リンク URL。
	// いずれも空なら素の URL。
	inlineRE = regexp.MustCompile("`([^`]+)`" + `|\[([^\]]+)\]\((https?://[^)\s]+)\)|https?://[^\s<>|]+`)
)

// textStyle は text 要素の装飾。
type textStyle struct {
	Bold   bool `json:"bold,omitempty"`
	Italic bool `json:"italic,omitempty"`
	Code   bool `json:"code,omitempty"`
}

// inlineElement は rich_text_section の中身（text または link）。
type inlineElement struct {
	Type  string     `json:"type"` // "text" | "link"
	Text  string     `json:"text,omitempty"`
	URL   string     `json:"url,omitempty"` // link のみ
	Style *textStyle `json:"style,omitempty"`
}

// richTextElement は rich_text ブロック直下の要素（section または list）。
// section の Elements は inlineElement、list の Elements は section が入る。
type richTextElement struct {
	Type     string `json:"type"` // "rich_text_section" | "rich_text_list"
	Elements []any  `json:"elements"`
	Style    string `json:"style,omitempty"`  // list: "bullet" | "ordered"
	Indent   int    `json:"indent,omitempty"` // list のネスト段数
}

type block struct {
	Type     string            `json:"type"`
	Elements []richTextElement `json:"elements"`
}

// payload は Incoming Webhook に送る JSON。text は通知プレビュー用のフォールバック。
type payload struct {
	Text   string  `json:"text"`
	Blocks []block `json:"blocks"`
}

// buildPayload は投稿する Webhook ペイロードを組み立てる。
// title が空なら見出しは既定文言のみ、from が空なら投稿者表記を省略する。
func buildPayload(title, from, summary string) payload {
	head := []any{}
	heading := "📝 作業まとめ"
	if title != "" {
		heading += ": " + title
	}
	head = append(head, inlineElement{Type: "text", Text: heading, Style: &textStyle{Bold: true}})
	if from != "" {
		head = append(head,
			inlineElement{Type: "text", Text: "\n"},
			inlineElement{Type: "text", Text: "by " + from, Style: &textStyle{Italic: true}},
		)
	}

	elements := []richTextElement{{Type: "rich_text_section", Elements: head}}
	elements = append(elements, parseBody(summary)...)

	return payload{
		Text:   notificationText(title, summary),
		Blocks: []block{{Type: "rich_text", Elements: elements}},
	}
}

// parseBody は本文を rich_text の要素列（段落とリスト）に変換する。
func parseBody(summary string) []richTextElement {
	var (
		out       []richTextElement
		para      []string // 連続する非リスト行
		items     []any    // 連続する同種リスト項目
		listStyle string
		listInden int
	)

	flushPara := func() {
		if len(para) > 0 {
			out = append(out, section(parseInline(strings.Join(para, "\n"))))
			para = nil
		}
	}
	flushList := func() {
		if len(items) > 0 {
			out = append(out, richTextElement{
				Type:     "rich_text_list",
				Style:    listStyle,
				Indent:   listInden,
				Elements: items,
			})
			items = nil
		}
	}
	addItem := func(style string, indent int, text string) {
		flushPara()
		if listStyle != style || listInden != indent {
			flushList()
			listStyle, listInden = style, indent
		}
		items = append(items, section(parseInline(text)))
	}

	for _, raw := range strings.Split(summary, "\n") {
		line := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(line) == "" {
			flushList()
			flushPara()
			continue
		}
		if m := bulletRE.FindStringSubmatch(line); m != nil {
			addItem("bullet", indentLevel(m[1]), m[2])
			continue
		}
		if m := orderedRE.FindStringSubmatch(line); m != nil {
			addItem("ordered", indentLevel(m[1]), m[2])
			continue
		}
		flushList()
		para = append(para, line)
	}
	flushList()
	flushPara()
	return out
}

func section(elements []any) richTextElement {
	return richTextElement{Type: "rich_text_section", Elements: elements}
}

// indentLevel は行頭の空白からネスト段数を求める（空白 2 つ、またはタブ 1 つで 1 段）。
func indentLevel(prefix string) int {
	spaces := 0
	for _, r := range prefix {
		if r == '\t' {
			spaces += 2
		} else {
			spaces++
		}
	}
	return spaces / 2
}

// parseInline は 1 行（または改行を含む段落）をインライン要素列に変換する。
func parseInline(s string) []any {
	var out []any
	addText := func(text string) {
		if text != "" {
			out = append(out, inlineElement{Type: "text", Text: text})
		}
	}

	last := 0
	for _, m := range inlineRE.FindAllStringSubmatchIndex(s, -1) {
		addText(s[last:m[0]])
		switch {
		case m[2] >= 0: // `code`
			out = append(out, inlineElement{Type: "text", Text: s[m[2]:m[3]], Style: &textStyle{Code: true}})
		case m[4] >= 0: // [表示文字](URL)
			out = append(out, inlineElement{Type: "link", URL: s[m[6]:m[7]], Text: s[m[4]:m[5]]})
		default: // 素の URL
			url, trailing := splitTrailingPunct(s[m[0]:m[1]])
			out = append(out, inlineElement{Type: "link", URL: url})
			addText(trailing)
		}
		last = m[1]
	}
	addText(s[last:])

	if len(out) == 0 {
		return []any{inlineElement{Type: "text", Text: ""}}
	}
	return out
}

// splitTrailingPunct は素の URL の末尾に付いた句読点を URL から切り離す。
// 「... https://example.com/x 。」のような文でリンクが壊れないようにする。
func splitTrailingPunct(url string) (string, string) {
	const punct = "。、，．,.;:!?)]｝）」』】>"
	i := len(url)
	for i > 0 {
		r := url[i-1]
		if r < 0x80 && strings.IndexByte(punct, r) >= 0 {
			i--
			continue
		}
		// マルチバイトの句読点（。、）などを削る
		trimmed := strings.TrimRight(url[:i], "。、，．）」』】")
		if len(trimmed) < i {
			i = len(trimmed)
			continue
		}
		break
	}
	return url[:i], url[i:]
}

// notificationText は通知プレビュー用の短い平文を返す（blocks を使う場合も text は必要）。
func notificationText(title, summary string) string {
	head := "📝 作業まとめ"
	detail := title
	if detail == "" {
		for _, line := range strings.Split(summary, "\n") {
			s := strings.TrimSpace(line)
			if m := bulletRE.FindStringSubmatch(line); m != nil {
				s = strings.TrimSpace(m[2])
			} else if m := orderedRE.FindStringSubmatch(line); m != nil {
				s = strings.TrimSpace(m[2])
			}
			if s != "" {
				detail = s
				break
			}
		}
	}
	if detail == "" {
		return head
	}
	if r := []rune(detail); len(r) > 80 {
		detail = string(r[:80]) + "…"
	}
	return head + ": " + detail
}
