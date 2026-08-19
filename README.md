# ai-working-out-loud

AI（Claude Code）に「作業しながら声に出す」＝人間とのコミュニケーションを促させるための
コマンド群と Claude Code スキルを置くリポジトリです。

収録コマンド:

- **`ask-the-boss`** — 上司の判断・承認が必要になったとき、Slack で質問して回答を待つ
- **`wol-slack`** — 作業まとめ（WOL）を Slack に投げるだけの一方向コマンド（[wol-slack](#wol-slack) 参照）

## Claude Code プラグインとしてインストール

各自の Claude Code で、マーケットプレイスを追加してプラグインをインストールします。
プラグイン 1 つに 2 つのスキルが入っています。

```
/plugin marketplace add udzura/ai-working-out-loud
/plugin install ai-working-out-loud@udzura
```

| スキル | 発動する場面 | 明示的に呼ぶ場合 |
|---|---|---|
| `ask-the-boss` | 後戻りしにくい操作の承認、仕様の最終判断など上司の確認が必要なとき | `/ai-working-out-loud:ask-the-boss` |
| `wol-slack` | 「WOL を始めて」「N 分ごとに進捗を共有して」「今の進捗を共有して」と頼まれたとき | `/ai-working-out-loud:wol-slack` |

スキルの発動には、各セクションに書かれた環境変数と対応するバイナリ（`go install` 済み）が必要です。

## ask-the-boss

作業中の AI（Claude Code）が、**人間（上司）の判断・承認が必要になったとき**に、Slack 経由で
質問を投げて回答を待つためのツールと Claude Code スキルです。本番反映・破壊的操作・費用の発生する
操作の承認、仕様や優先順位の最終判断など、「AI が自分で決めるべきでないこと」を上司に確認できます。

- **CLI 本体** (`ask-the-boss`, Go 製) — Slack Incoming Webhook で質問を投稿し、Bot トークンで
  スレッド返信をポーリングして上司の回答を取得します。タイムアウトした質問は後から
  `resume` で取得を再開できます。
- **Claude Code スキル** (`ask-the-boss`) — 上記 CLI を Claude Code から使うためのスキル。
  プラグインとして各自の環境にインストールできます。

### 仕組み

1. ユニークな質問 ID を付けて、上司メンション付きで Incoming Webhook に質問を投稿
2. Bot トークンで `conversations.history` を検索し、投稿した自分のメッセージの `ts` を特定
3. `conversations.replies` をポーリングし、**上司の返信だけ**を取得（他人の割り込みは無視）
4. 回答内のメンション `<@U...>` は表示名に解決して出力

投稿は Incoming Webhook、読み取りは Bot トークンという二系統構成です。

### セットアップ

#### 1. Slack アプリの準備

- **Incoming Webhooks** を有効化し、投稿先チャンネルの Webhook URL を取得
- **Bot Token Scopes** に投稿先チャンネル種別に応じた履歴スコープ + `users:read` を付与
  - パブリックチャンネル: `channels:history`（プライベートは `groups:history`、DM は `im:history`）
  - `users:read`（回答内メンションの表示名解決に使用）
- Bot を **対象チャンネルに招待**（`/invite @<Botアプリ>`）。未参加だと履歴が読めません。

#### 2. 環境変数

| 変数 | 用途 |
|---|---|
| `SLACK_WEBHOOK_URL` | 質問の投稿先（Incoming Webhook）|
| `SLACK_BOT_TOKEN` | 返信の読み取り（`xoxb-...`）|
| `SLACK_CHANNEL_ID` | Webhook の投稿先チャンネル ID |
| `SLACK_BOSS_USER_ID` | 上司の Slack ユーザー ID（メンション & 返信判定）|

シェルのプロファイルや direnv 等で設定してください。ローカル開発時はリポジトリ直下の `.env`
（`.gitignore` 済み）に書いて `set -a; . ./.env; set +a` で読み込めます。

#### 3. CLI のインストール

```bash
go install github.com/udzura/ai-working-out-loud/cmd/ask-the-boss@latest
# $(go env GOPATH)/bin に ask-the-boss が入るので PATH に含める
```

### CLI の使い方

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

#### 返信を後から取得する（resume）

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

## wol-slack

作業のまとめ（Working Out Loud）を Slack に**投げるだけ**の最小コマンドです。返信は待たず、
投稿に成功したら正常終了します。必要な環境変数は Incoming Webhook URL のみです。

| 変数 | 用途 |
|---|---|
| `WOL_SLACK_WEBHOOK_URL` | 投稿先 Incoming Webhook URL（優先）|
| `SLACK_WEBHOOK_URL` | 上記が無い場合に使用（`ask-the-boss` と同じチャンネルに投げる場合）|

```bash
go install github.com/udzura/ai-working-out-loud/cmd/wol-slack@latest
```

```bash
# 見出し・投稿者つきで投稿
wol-slack --title "パフォーマンス調査" --from "山田太郎" \
  --summary "N+1 を 3 箇所修正し、レイテンシが 40% 改善しました。次はキャッシュ層を見ます。"

# ファイル・標準入力・位置引数でも渡せる
wol-slack --summary-file summary.md
echo "調査ログをまとめました" | wol-slack

# 投稿せず本文だけ確認する
wol-slack --dry-run -s "動作確認"
```

| flag | 説明 |
|---|---|
| `--summary`, `-s` / `--summary-file` | まとめ本文（インライン / ファイル）。必須 |
| `--title`, `-t` | 見出し。任意 |
| `--from` | 投稿者の名前（受け取った文字列をそのまま表示）。任意 |
| `--dry-run` | 投稿せず、組み立てた本文を標準出力に表示して終了 |

まとめ本文は `--summary` / `--summary-file` / 位置引数 / 標準入力 のいずれかで必須です。

### スキルとしての振る舞い

同梱の `wol-slack` スキルは、この CLI を使って**作業中に定期的に進捗を投稿し続ける**ためのものです。

1. 「WOL を始めて」等で発動し、最初に**何分ごとに投稿するか**を確認します（`--from` に載せる名前も）
2. 以降は作業の区切りごとに作業ログを状態ファイルへ溜めていきます
3. 決めた間隔が経過したら、前回投稿以降の差分をまとめて投稿し、ログをクリアします
4. 「WOL して」「今の進捗を共有して」と促されたときは、経過を待たずに差分を投稿します
5. 「WOL 止めて」と言われるまで継続します（停止時に未投稿分を投げるかは確認します）

状態は `${XDG_STATE_HOME:-~/.local/state}/wol-slack/<プロジェクト名>.md`（`WOL_SLACK_STATE_FILE`
で変更可）に保存され、Webhook URL などの秘匿情報は含みません。

**制約**: Claude Code は自分で時間が来たら起きることができないため、「N 分ごと」は実際には
「N 分経過後、次に作業が一段落したタイミングで投稿する」という意味になります。会話が止まって
いる間は投稿されません。厳密なタイマーが必要なら `/loop 30m /ai-working-out-loud:wol-slack`
のような定期実行の仕組みと併用してください。

## 開発

このリポジトリは複数のコマンドを置く構成で、各コマンドは `cmd/<コマンド名>/` に入っています。

```
cmd/ask-the-boss/   # ask-the-boss CLI（main パッケージ）
cmd/wol-slack/      # wol-slack CLI（main パッケージ）
internal/slack/     # コマンド間で共有する Slack クライアント
skills/             # Claude Code スキル（ask-the-boss / wol-slack）
.claude-plugin/     # プラグイン & マーケットプレイスの定義
```

```bash
go build -o ask-the-boss ./cmd/ask-the-boss   # 個別ビルド
go build -o wol-slack ./cmd/wol-slack
go build ./...                                # 全コマンドのビルド確認
go test ./...                                 # テスト
```

## ライセンス

MIT
