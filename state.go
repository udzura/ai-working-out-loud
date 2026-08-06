package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// pending は返信待ちのまま終了した質問を、あとで resume するために保存する状態。
// Bot トークンなどの秘匿情報は保存しない（resume 時に環境変数から取得する）。
type pending struct {
	ID         string `json:"id"`
	Marker     string `json:"marker"`
	ThreadTS   string `json:"thread_ts"`
	ChannelID  string `json:"channel_id"`
	BossUserID string `json:"boss_user_id"`
	Question   string `json:"question"`
	From       string `json:"from"`
	CreatedAt  string `json:"created_at"`
}

// stateDir は状態ファイルの保存先を返す。
// ASK_THE_BOSS_STATE_DIR > $XDG_STATE_HOME/ask-the-boss > $HOME/.local/state/ask-the-boss の優先順。
func stateDir() string {
	if d := os.Getenv("ASK_THE_BOSS_STATE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "ask-the-boss")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "state", "ask-the-boss")
}

func pendingDir() string { return filepath.Join(stateDir(), "pending") }

func pendingPath(id string) string { return filepath.Join(pendingDir(), id+".json") }

// savePending は保留中の質問を保存する（同じ ID は上書き）。
func savePending(p *pending) error {
	if err := os.MkdirAll(pendingDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pendingPath(p.ID), data, 0o600)
}

// loadPending は ID を指定して保留中の質問を読み込む。
func loadPending(id string) (*pending, error) {
	data, err := os.ReadFile(pendingPath(id))
	if err != nil {
		return nil, err
	}
	var p pending
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// listPending は保留中の質問を作成日時の昇順で返す。1件も無ければ空スライス。
func listPending() ([]pending, error) {
	entries, err := os.ReadDir(pendingDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ps []pending
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		p, err := loadPending(strings.TrimSuffix(e.Name(), ".json"))
		if err == nil {
			ps = append(ps, *p)
		}
	}
	// CreatedAt は RFC3339 なので文字列比較で時系列順になる。
	sort.Slice(ps, func(i, j int) bool { return ps[i].CreatedAt < ps[j].CreatedAt })
	return ps, nil
}

// latestPending は最も新しい保留中の質問を返す。無ければ nil。
func latestPending() (*pending, error) {
	ps, err := listPending()
	if err != nil || len(ps) == 0 {
		return nil, err
	}
	p := ps[len(ps)-1]
	return &p, nil
}

// removePending は保留中の質問を削除する（存在しなくてもエラーにしない）。
func removePending(id string) error {
	err := os.Remove(pendingPath(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
