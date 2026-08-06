package main

import (
	"testing"
)

func TestPendingSaveLoadRemove(t *testing.T) {
	t.Setenv("ASK_THE_BOSS_STATE_DIR", t.TempDir())

	p := &pending{
		ID:         "abc123",
		Marker:     "[ask-the-boss:abc123]",
		ThreadTS:   "1700000000.000100",
		ChannelID:  "C123",
		BossUserID: "U999",
		Question:   "本番反映してよいですか？",
		From:       "uchio.kondo",
		CreatedAt:  "2026-08-06T10:00:00+09:00",
	}
	if err := savePending(p); err != nil {
		t.Fatalf("savePending: %v", err)
	}

	got, err := loadPending("abc123")
	if err != nil {
		t.Fatalf("loadPending: %v", err)
	}
	if *got != *p {
		t.Errorf("loadPending mismatch:\n got %+v\nwant %+v", *got, *p)
	}

	if err := removePending("abc123"); err != nil {
		t.Fatalf("removePending: %v", err)
	}
	if _, err := loadPending("abc123"); err == nil {
		t.Error("削除後も読み込めてしまいました")
	}
	// 存在しない ID の削除はエラーにならない。
	if err := removePending("nope"); err != nil {
		t.Errorf("存在しない ID の削除でエラー: %v", err)
	}
}

func TestListAndLatestPending(t *testing.T) {
	t.Setenv("ASK_THE_BOSS_STATE_DIR", t.TempDir())

	// 何も無い状態。
	if ps, err := listPending(); err != nil || len(ps) != 0 {
		t.Fatalf("空のはずが len=%d err=%v", len(ps), err)
	}
	if p, err := latestPending(); err != nil || p != nil {
		t.Fatalf("空のはずが p=%v err=%v", p, err)
	}

	older := &pending{ID: "old", CreatedAt: "2026-08-06T09:00:00+09:00", Question: "古い"}
	newer := &pending{ID: "new", CreatedAt: "2026-08-06T11:00:00+09:00", Question: "新しい"}
	// 保存順は逆にしておく（並び替えを検証するため）。
	if err := savePending(newer); err != nil {
		t.Fatal(err)
	}
	if err := savePending(older); err != nil {
		t.Fatal(err)
	}

	ps, err := listPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0].ID != "old" || ps[1].ID != "new" {
		t.Errorf("作成日時の昇順になっていません: %+v", ps)
	}

	latest, err := latestPending()
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.ID != "new" {
		t.Errorf("latestPending が最新を返していません: %+v", latest)
	}
}
