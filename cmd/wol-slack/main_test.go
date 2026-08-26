package main

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestRunHelp(t *testing.T) {
	// --help / -h / help は投稿せず正常終了する（環境変数が無くてもエラーにしない）。
	t.Setenv("WOL_SLACK_WEBHOOK_URL", "")
	t.Setenv("SLACK_WEBHOOK_URL", "")
	for _, arg := range []string{"--help", "-h", "help"} {
		if err := run([]string{arg}); err != nil {
			t.Errorf("run(%q) = %v, want nil", arg, err)
		}
	}
}

func TestRunUnknownFlag(t *testing.T) {
	if err := run([]string{"--nope"}); err == nil {
		t.Error("未知のフラグでエラーを期待しましたが nil でした")
	}
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
