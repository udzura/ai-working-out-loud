package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildMessage(t *testing.T) {
	t.Run("背景あり", func(t *testing.T) {
		got := buildMessage("U123", "", "本番反映してよいですか？", "PR #123 が承認済みです", "[ask-the-boss:abc]")
		for _, want := range []string{"<@U123>", "*【背景】*", "PR #123 が承認済みです", "*【質問】*", "本番反映してよいですか？", "[ask-the-boss:abc]"} {
			if !strings.Contains(got, want) {
				t.Errorf("メッセージに %q が含まれていません:\n%s", want, got)
			}
		}
	})
	t.Run("背景なしなら背景ブロックを省略", func(t *testing.T) {
		got := buildMessage("U123", "", "質問だけ", "", "[m]")
		if strings.Contains(got, "【背景】") {
			t.Errorf("背景が空なのに背景ブロックが出ています:\n%s", got)
		}
		if !strings.Contains(got, "【質問】") {
			t.Errorf("質問ブロックがありません:\n%s", got)
		}
	})
	t.Run("質問者名ありなら本文に含む", func(t *testing.T) {
		got := buildMessage("U123", "田中花子", "質問です", "", "[m]")
		if !strings.Contains(got, "質問者: 田中花子") {
			t.Errorf("質問者名が含まれていません:\n%s", got)
		}
	})
	t.Run("質問者名なしなら質問者表記を省略", func(t *testing.T) {
		got := buildMessage("U123", "", "質問です", "", "[m]")
		if strings.Contains(got, "質問者") {
			t.Errorf("質問者名が空なのに質問者表記が出ています:\n%s", got)
		}
	})
}

func TestResolveText(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bg.md")
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
	t.Run("両方空", func(t *testing.T) {
		got, err := resolveText("", "")
		if err != nil || got != "" {
			t.Errorf("got %q, err %v; want empty", got, err)
		}
	})
	t.Run("存在しないファイル", func(t *testing.T) {
		if _, err := resolveText("", filepath.Join(dir, "nope.md")); err == nil {
			t.Error("エラーを期待しましたが nil でした")
		}
	})
}
