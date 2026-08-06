# ask-the-boss

作業中の AI（Claude Code）が、**人間（上司）の判断・承認が必要になったとき**に、Slack 経由で
質問を投げて回答を待つためのツールと Claude Code スキルです。本番反映・破壊的操作・費用の発生する
操作の承認、仕様や優先順位の最終判断など、「AI が自分で決めるべきでないこと」を上司に確認できます。

- **CLI 本体** (`ask-the-boss`, Go 製) — Slack Incoming Webhook で質問を投稿し、Bot トークンで
  スレッド返信をポーリングして上司の回答を取得します。
- **Claude Code スキル** (`ask-the-boss`) — 上記 CLI を Claude Code から使うためのスキル。
  プラグインとして各自の環境にインストールできます。

## 仕組み

1. ユニークな質問 ID を付けて、上司メンション付きで Incoming Webhook に質問を投稿
2. Bot トークンで `conversations.history` を検索し、投稿した自分のメッセージの `ts` を特定
3. `conversations.replies` をポーリングし、**上司の返信だけ**を取得（他人の割り込みは無視）
4. 回答内のメンション `<@U...>` は表示名に解決して出力

投稿は Incoming Webhook、読み取りは Bot トークンという二系統構成です。

## セットアップ

### 1. Slack アプリの準備

- **Incoming Webhooks** を有効化し、投稿先チャンネルの Webhook URL を取得
- **Bot Token Scopes** に投稿先チャンネル種別に応じた履歴スコープ + `users:read` を付与
  - パブリックチャンネル: `channels:history`（プライベートは `groups:history`、DM は `im:history`）
  - `users:read`（回答内メンションの表示名解決に使用）
- Bot を **対象チャンネルに招待**（`/invite @<Botアプリ>`）。未参加だと履歴が読めません。

### 2. 環境変数

| 変数 | 用途 |
|---|---|
| `SLACK_WEBHOOK_URL` | 質問の投稿先（Incoming Webhook）|
| `SLACK_BOT_TOKEN` | 返信の読み取り（`xoxb-...`）|
| `SLACK_CHANNEL_ID` | Webhook の投稿先チャンネル ID |
| `SLACK_BOSS_USER_ID` | 上司の Slack ユーザー ID（メンション & 返信判定）|

シェルのプロファイルや direnv 等で設定してください。ローカル開発時はリポジトリ直下の `.env`
（`.gitignore` 済み）に書いて `set -a; . ./.env; set +a` で読み込めます。

### 3. CLI のインストール

```bash
go install github.com/udzura/ask-the-boss@latest
# $(go env GOPATH)/bin に ask-the-boss が入るので PATH に含める
```

## Claude Code スキルとしてインストール

各自の Claude Code で、マーケットプレイスを追加してプラグインをインストールします。

```
/plugin marketplace add udzura/ask-the-boss
/plugin install ask-the-boss@udzura
```

インストール後は、後戻りしにくい操作の承認などが必要な場面で自動的に呼び出されます
（明示的に呼ぶ場合は `/ask-the-boss:ask-the-boss`）。スキルの発動には上記の環境変数と
`ask-the-boss` バイナリ（`go install` 済み）が必要です。

## CLI の使い方

```bash
# 背景と質問を分けて指定
ask-the-boss ask \
  --background "PR #123 のレビューが完了し、CI もグリーンです。" \
  --question "本番に反映してよいですか？（はい／いいえ／保留 でお答えください）" \
  --timeout 30m --interval 15s

# ファイルから渡すこともできる
ask-the-boss ask --question-file q.md --background-file bg.md

# 質問だけなら位置引数・標準入力でも可
echo "この方針で進めてよいですか？" | ask-the-boss ask
```

| flag | 説明 |
|---|---|
| `--question`, `-q` / `--question-file` | 質問内容（インライン / ファイル）。必須 |
| `--background`, `-b` / `--background-file` | 質問の背景（インライン / ファイル）。任意 |
| `--timeout` | 回答待ちの最大時間（既定 `30m`）|
| `--interval` | ポーリング間隔（既定 `10s`）|

上司の返信が付くとその回答テキストが標準出力に出力され、正常終了します。時間内に返信が
無ければタイムアウトで異常終了します。

## 開発

```bash
go build -o ask-the-boss .   # ビルド
go test ./...                # テスト
```

## ライセンス

MIT
