package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient は users.info をモックする httptest サーバに向けた Client を返す。
// profiles は userID -> display_name のマップ。マップに無い ID は user_not_found を返す。
func newTestClient(t *testing.T, profiles map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("user")
		w.Header().Set("Content-Type", "application/json")
		name, ok := profiles[id]
		if !ok {
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "user_not_found"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"user": map[string]any{"name": "fallback", "profile": map[string]any{"display_name": name}},
		})
	}))
	t.Cleanup(srv.Close)
	c := New("", "xoxb-test", "C123")
	c.BaseURL = srv.URL
	return c
}

func TestResolveMentions(t *testing.T) {
	c := newTestClient(t, map[string]string{
		"UEXAMPLE001": "山田上司",
		"UEXAMPLE002": "鈴木",
	})
	ctx := context.Background()

	t.Run("単一メンションを表示名へ", func(t *testing.T) {
		got := c.ResolveMentions(ctx, "了解です <@UEXAMPLE001> さん")
		want := "了解です @山田上司 さん"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("複数メンション", func(t *testing.T) {
		got := c.ResolveMentions(ctx, "<@UEXAMPLE001> と <@UEXAMPLE002> に確認")
		want := "@山田上司 と @鈴木 に確認"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("ラベル付きメンション表記", func(t *testing.T) {
		got := c.ResolveMentions(ctx, "<@UEXAMPLE001|yamada> です")
		want := "@山田上司 です"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("解決失敗は元の表記を維持", func(t *testing.T) {
		got := c.ResolveMentions(ctx, "不明 <@UNKNOWN00> は残す")
		want := "不明 <@UNKNOWN00> は残す"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("メンションなしはそのまま", func(t *testing.T) {
		got := c.ResolveMentions(ctx, "答えは42です")
		if got != "答えは42です" {
			t.Errorf("got %q", got)
		}
	})
}

func TestUserDisplayNameFallback(t *testing.T) {
	// display_name が空でも real_name にフォールバックすることを確認する。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"user": map[string]any{"name": "uname", "profile": map[string]any{"display_name": "", "real_name": "本名太郎"}},
		})
	}))
	defer srv.Close()
	c := New("", "xoxb-test", "C123")
	c.BaseURL = srv.URL

	got, err := c.UserDisplayName(context.Background(), "U1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "本名太郎" {
		t.Errorf("got %q, want %q", got, "本名太郎")
	}
}
