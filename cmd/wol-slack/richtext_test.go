package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// items は rich_text_list 要素の各項目を平文にして返す（検証用）。
func plainOf(t *testing.T, elements []any) string {
	t.Helper()
	var b strings.Builder
	for _, e := range elements {
		el, ok := e.(inlineElement)
		if !ok {
			t.Fatalf("inlineElement を期待しましたが %T でした", e)
		}
		if el.Type == "link" && el.Text == "" {
			b.WriteString(el.URL)
			continue
		}
		b.WriteString(el.Text)
	}
	return b.String()
}

func TestBuildPayloadHeader(t *testing.T) {
	t.Run("見出しと投稿者あり", func(t *testing.T) {
		p := buildPayload("パフォーマンス調査", "田中花子", "- 本文")
		head := p.Blocks[0].Elements[0]
		if head.Type != "rich_text_section" {
			t.Fatalf("先頭要素が section ではありません: %q", head.Type)
		}
		got := plainOf(t, head.Elements)
		if got != "📝 作業まとめ: パフォーマンス調査\nby 田中花子" {
			t.Errorf("見出しが期待と違います: %q", got)
		}
		first := head.Elements[0].(inlineElement)
		if first.Style == nil || !first.Style.Bold {
			t.Error("見出しが太字になっていません")
		}
		last := head.Elements[len(head.Elements)-1].(inlineElement)
		if last.Style == nil || !last.Style.Italic {
			t.Error("投稿者名が斜体になっていません")
		}
	})
	t.Run("見出しなし・投稿者なし", func(t *testing.T) {
		p := buildPayload("", "", "本文")
		got := plainOf(t, p.Blocks[0].Elements[0].Elements)
		if got != "📝 作業まとめ" {
			t.Errorf("既定の見出しになっていません: %q", got)
		}
	})
}

func TestParseBodyList(t *testing.T) {
	body := strings.Join([]string{
		"導入の段落です。",
		"- 項目 1",
		"- 項目 2",
		"  - ネスト項目",
		"1. 番号付き 1",
		"2. 番号付き 2",
		"締めの段落。",
	}, "\n")

	els := parseBody(body)
	if len(els) != 5 {
		t.Fatalf("要素数 = %d, want 5 (段落/箇条書き/ネスト/番号付き/段落): %+v", len(els), els)
	}

	if els[0].Type != "rich_text_section" || plainOf(t, els[0].Elements) != "導入の段落です。" {
		t.Errorf("1番目が導入段落になっていません: %+v", els[0])
	}

	if els[1].Type != "rich_text_list" || els[1].Style != "bullet" || els[1].Indent != 0 {
		t.Errorf("2番目が indent 0 の箇条書きになっていません: %+v", els[1])
	}
	if len(els[1].Elements) != 2 {
		t.Errorf("箇条書きの項目数 = %d, want 2", len(els[1].Elements))
	}

	if els[2].Type != "rich_text_list" || els[2].Style != "bullet" || els[2].Indent != 1 {
		t.Errorf("3番目が indent 1 のネストリストになっていません: %+v", els[2])
	}

	if els[3].Type != "rich_text_list" || els[3].Style != "ordered" {
		t.Errorf("4番目が番号付きリストになっていません: %+v", els[3])
	}

	if els[4].Type != "rich_text_section" || plainOf(t, els[4].Elements) != "締めの段落。" {
		t.Errorf("5番目が締めの段落になっていません: %+v", els[4])
	}
}

func TestParseInline(t *testing.T) {
	t.Run("インラインコード", func(t *testing.T) {
		els := parseInline("先頭 `go test ./...` 末尾")
		if len(els) != 3 {
			t.Fatalf("要素数 = %d, want 3: %+v", len(els), els)
		}
		code := els[1].(inlineElement)
		if code.Text != "go test ./..." || code.Style == nil || !code.Style.Code {
			t.Errorf("コード要素が期待と違います: %+v", code)
		}
	})
	t.Run("markdown 形式のリンク", func(t *testing.T) {
		els := parseInline("詳細は [PR #1](https://example.com/pull/1) を参照")
		link := els[1].(inlineElement)
		if link.Type != "link" || link.URL != "https://example.com/pull/1" || link.Text != "PR #1" {
			t.Errorf("リンク要素が期待と違います: %+v", link)
		}
	})
	t.Run("素の URL", func(t *testing.T) {
		els := parseInline("https://example.com/x を見て")
		link := els[0].(inlineElement)
		if link.Type != "link" || link.URL != "https://example.com/x" || link.Text != "" {
			t.Errorf("素の URL がリンクになっていません: %+v", link)
		}
	})
	t.Run("URL 末尾の句読点は URL に含めない", func(t *testing.T) {
		for _, tc := range []struct{ in, wantURL, wantRest string }{
			{"https://example.com/x。", "https://example.com/x", "。"},
			{"https://example.com/x)", "https://example.com/x", ")"},
			{"https://example.com/x", "https://example.com/x", ""},
		} {
			els := parseInline(tc.in)
			link := els[0].(inlineElement)
			if link.URL != tc.wantURL {
				t.Errorf("in=%q URL = %q, want %q", tc.in, link.URL, tc.wantURL)
			}
			rest := ""
			if len(els) > 1 {
				rest = els[1].(inlineElement).Text
			}
			if rest != tc.wantRest {
				t.Errorf("in=%q 残り = %q, want %q", tc.in, rest, tc.wantRest)
			}
		}
	})
}

func TestNotificationText(t *testing.T) {
	if got := notificationText("題名", "- 本文"); got != "📝 作業まとめ: 題名" {
		t.Errorf("got %q", got)
	}
	if got := notificationText("", "- 最初の項目\n- 次の項目"); got != "📝 作業まとめ: 最初の項目" {
		t.Errorf("見出しが無いとき先頭項目を使っていません: %q", got)
	}
	if got := notificationText("", ""); got != "📝 作業まとめ" {
		t.Errorf("got %q", got)
	}
}

func TestPayloadJSON(t *testing.T) {
	p := buildPayload("題名", "udzura", "- 項目\n  - ネスト")
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		`"type":"rich_text"`,
		`"type":"rich_text_list"`,
		`"style":"bullet"`,
		`"indent":1`,
		`"text":"📝 作業まとめ: 題名"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("JSON に %s が含まれていません:\n%s", want, got)
		}
	}
	// 通知用の text は blocks と併せて必ず送る。
	if !strings.Contains(got, `"text":"📝 作業まとめ: 題名"`) {
		t.Error("通知用の text が入っていません")
	}
	// indent 0 のリストでは indent を省略する（omitempty）。
	if strings.Contains(got, `"indent":0`) {
		t.Error("indent 0 が明示的に出力されています")
	}
}
