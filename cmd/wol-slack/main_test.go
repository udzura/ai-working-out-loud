package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildMessage(t *testing.T) {
	t.Run("見出しと投稿者あり", func(t *testing.T) {
		got := buildMessage("パフォーマンス調査", "田中花子", "N+1 を 3 箇所見つけました")
		for _, want := range []string{"*📝 作業まとめ: パフォーマンス調査*", "_by 田中花子_", "N+1 を 3 箇所見つけました"} {
			if !strings.Contains(got, want) {
				t.Errorf("メッセージに %q が含まれていません:\n%s", want, got)
			}
		}
	})
	t.Run("見出しなしなら既定の見出しのみ", func(t *testing.T) {
		got := buildMessage("", "", "まとめ本文")
		if !strings.HasPrefix(got, "*📝 作業まとめ*\n") {
			t.Errorf("既定の見出しになっていません:\n%s", got)
		}
		if strings.Contains(got, ":") {
			t.Errorf("見出しが空なのにコロンが出ています:\n%s", got)
		}
	})
	t.Run("投稿者なしなら投稿者表記を省略", func(t *testing.T) {
		got := buildMessage("題名", "", "まとめ本文")
		if strings.Contains(got, "_by") {
			t.Errorf("投稿者名が空なのに投稿者表記が出ています:\n%s", got)
		}
	})
}

func TestResolveText(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(file, []byte("  ファイルの内容  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("インライン優先", func(t *testing.T) {
		got, err := resolveText("  inline  ", file)
		if err != nil {
			t.Fatal(err)
		}
		if got != "inline" {
			t.Errorf("got %q, want %q", got, "inline")
		}
	})
	t.Run("ファイルから読み込み trim", func(t *testing.T) {
		got, err := resolveText("", file)
		if err != nil {
			t.Fatal(err)
		}
		if got != "ファイルの内容" {
			t.Errorf("got %q, want %q", got, "ファイルの内容")
		}
	})
	t.Run("存在しないファイル", func(t *testing.T) {
		if _, err := resolveText("", filepath.Join(dir, "nope.md")); err == nil {
			t.Error("エラーを期待しましたが nil でした")
		}
	})
}

func TestResolveSummary(t *testing.T) {
	t.Run("位置引数を結合", func(t *testing.T) {
		got, err := resolveSummary("", "", []string{"まとめ", "本文"})
		if err != nil {
			t.Fatal(err)
		}
		if got != "まとめ 本文" {
			t.Errorf("got %q, want %q", got, "まとめ 本文")
		}
	})
	t.Run("どこにも本文が無ければエラー", func(t *testing.T) {
		// 標準入力を空にして、フォールバックが最終的にエラーになることを確認する。
		orig := os.Stdin
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		w.Close()
		os.Stdin = r
		defer func() { os.Stdin = orig; r.Close() }()

		if _, err := resolveSummary("", "", nil); err == nil {
			t.Error("エラーを期待しましたが nil でした")
		}
	})
}

func TestWebhookURL(t *testing.T) {
	t.Run("WOL_SLACK_WEBHOOK_URL を優先", func(t *testing.T) {
		t.Setenv("WOL_SLACK_WEBHOOK_URL", "https://hooks.example.com/wol")
		t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.example.com/boss")
		got, err := webhookURL()
		if err != nil {
			t.Fatal(err)
		}
		if got != "https://hooks.example.com/wol" {
			t.Errorf("got %q, want wol 用の URL", got)
		}
	})
	t.Run("無ければ SLACK_WEBHOOK_URL にフォールバック", func(t *testing.T) {
		t.Setenv("WOL_SLACK_WEBHOOK_URL", "")
		t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.example.com/boss")
		got, err := webhookURL()
		if err != nil {
			t.Fatal(err)
		}
		if got != "https://hooks.example.com/boss" {
			t.Errorf("got %q, want フォールバック先の URL", got)
		}
	})
	t.Run("両方未設定ならエラー", func(t *testing.T) {
		t.Setenv("WOL_SLACK_WEBHOOK_URL", "")
		t.Setenv("SLACK_WEBHOOK_URL", "")
		if _, err := webhookURL(); err == nil {
			t.Error("エラーを期待しましたが nil でした")
		}
	})
}
