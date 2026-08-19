# ai-working-out-loud

AI（Claude Code）に「作業しながら声に出す」＝人間とのコミュニケーションを促させるための
コマンド群と Claude Code スキルを置くリポジトリです。現在収録しているのは `ask-the-boss` 一式で、
以降の説明はすべてこのコマンドについてのものです。

作業中の AI（Claude Code）が、**人間（上司）の判断・承認が必要になったとき**に、Slack 経由で
質問を投げて回答を待つためのツールと Claude Code スキルです。本番反映・破壊的操作・費用の発生する
操作の承認、仕様や優先順位の最終判断など、「AI が自分で決めるべきでないこと」を上司に確認できます。

- **CLI 本体** (`ask-the-boss`, Go 製) — Slack Incoming Webhook で質問を投稿し、Bot トークンで
  スレッド返信をポーリングして上司の回答を取得します。タイムアウトした質問は後から
  `resume` で取得を再開できます。
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
go install github.com/udzura/ai-working-out-loud/cmd/ask-the-boss@latest
# $(go env GOPATH)/bin に ask-the-boss が入るので PATH に含める
```

## Claude Code スキルとしてインストール

各自の Claude Code で、マーケットプレイスを追加してプラグインをインストールします。

```
/plugin marketplace add udzura/ai-working-out-loud
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
| `--from` | 質問者の名前（受け取った文字列をそのまま表示）。任意 |
| `--timeout` | 回答待ちの最大時間（既定 `30m`）|
| `--interval` | ポーリング間隔（既定 `10s`）|

上司の返信が付くとその回答テキストが標準出力に出力され、正常終了します。時間内に返信が
無ければタイムアウトで異常終了します。

### 返信を後から取得する（resume）

`ask` がタイムアウト等で返信を取れずに終了した場合、その質問は「保留」として保存され、
標準エラーに `後で再開するには: ask-the-boss resume <ID>` と表示されます。あとから
`resume` で**質問を投げ直さずに**返信だけを取りに行けます（上司への二重通知を防ぎます）。

```bash
# 保留中の質問を一覧表示（ID・作成日時・質問文）
ask-the-boss resume --list

# ID を指定して返信を待つ（ID を省略すると直近の保留が対象）
ask-the-boss resume <質問ID> --timeout 30m --interval 15s
```

返信が取れると回答を標準出力に出して保留を自動削除します。`resume` は投稿を行わないため、
`SLACK_BOT_TOKEN` だけあれば動作します。保留状態は `~/.local/state/ask-the-boss/`
（`ASK_THE_BOSS_STATE_DIR` / `XDG_STATE_HOME` で変更可）に保存され、Bot トークンは含みません。

## 開発

このリポジトリは複数のコマンドを置く構成で、各コマンドは `cmd/<コマンド名>/` に入っています。

```
cmd/ask-the-boss/   # ask-the-boss CLI（main パッケージ）
internal/slack/     # コマンド間で共有する Slack クライアント
skills/             # Claude Code スキル
```

```bash
go build -o ask-the-boss ./cmd/ask-the-boss   # ビルド
go build ./...                                # 全コマンドのビルド確認
go test ./...                                 # テスト
```

## ライセンス

MIT
