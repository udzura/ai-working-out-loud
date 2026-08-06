// Command ask-the-boss は Slack 経由で上司に質問し、その返信を受け取る CLI。
//
// 使い方:
//
//	ask-the-boss ask --question "本番反映してよいですか？" --background "PR #123 のレビューが完了しました"
//	ask-the-boss ask --question-file q.md --background-file bg.md
//	echo "長めの質問文" | ask-the-boss ask
//
// 質問内容（--question / --question-file / 位置引数 / 標準入力）と、任意の背景
// （--background / --background-file）、任意の質問者名（--from）を分けて渡せる。
// Incoming Webhook で質問を投稿し、Bot トークンでスレッド返信をポーリングして
// 上司（SLACK_BOSS_USER_ID）の回答が付くまで待つ。回答内のメンションは表示名に
// 解決し、標準出力に出す。
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/udzura/ask-the-boss/internal/slack"
)

// config は環境変数から読み込む設定。
type config struct {
	WebhookURL string // SLACK_WEBHOOK_URL
	BotToken   string // SLACK_BOT_TOKEN
	ChannelID  string // SLACK_CHANNEL_ID
	BossUserID string // SLACK_BOSS_USER_ID
}

func loadConfig() (*config, error) {
	c := &config{
		WebhookURL: os.Getenv("SLACK_WEBHOOK_URL"),
		BotToken:   os.Getenv("SLACK_BOT_TOKEN"),
		ChannelID:  os.Getenv("SLACK_CHANNEL_ID"),
		BossUserID: os.Getenv("SLACK_BOSS_USER_ID"),
	}
	var missing []string
	if c.WebhookURL == "" {
		missing = append(missing, "SLACK_WEBHOOK_URL")
	}
	if c.BotToken == "" {
		missing = append(missing, "SLACK_BOT_TOKEN")
	}
	if c.ChannelID == "" {
		missing = append(missing, "SLACK_CHANNEL_ID")
	}
	if c.BossUserID == "" {
		missing = append(missing, "SLACK_BOSS_USER_ID")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("必要な環境変数が未設定です: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

const usage = `ask-the-boss - Slack 経由で上司に質問し、返信を受け取る

使い方:
  ask-the-boss ask --question "本番反映してよいですか？" --background "PR #123 のレビューが完了しました"
  ask-the-boss ask --question-file q.md --background-file bg.md
  echo "質問文" | ask-the-boss ask   # 質問内容を標準入力から渡す

flags:
  --question, -q        質問内容 (インライン)
  --question-file       質問内容をファイルから読み込む
  --background, -b      質問の背景 (インライン, 任意)
  --background-file     質問の背景をファイルから読み込む (任意)
  --from                質問者の名前 (任意, 受け取った文字列をそのまま表示)
  --timeout             回答待ちの最大時間 (デフォルト 30m)
  --interval            ポーリング間隔 (デフォルト 10s)

質問内容は --question / --question-file / 位置引数 / 標準入力 のいずれかで必須。
背景は --background / --background-file、質問者名は --from で任意に指定できる。

環境変数:
  SLACK_WEBHOOK_URL   質問の投稿先 Incoming Webhook URL
  SLACK_BOT_TOKEN     返信読み取り用 Bot トークン (xoxb-)
  SLACK_CHANNEL_ID    Webhook の投稿先チャンネル ID
  SLACK_BOSS_USER_ID  上司の Slack ユーザー ID (メンション & 返信判定に使用)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "ask":
		if err := runAsk(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "不明なコマンド: %s\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func runAsk(args []string) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	var question, questionFile, background, backgroundFile string
	fs.StringVar(&question, "question", "", "質問内容 (インライン)")
	fs.StringVar(&question, "q", "", "質問内容 (インライン, 短縮形)")
	fs.StringVar(&questionFile, "question-file", "", "質問内容をファイルから読み込む")
	fs.StringVar(&background, "background", "", "質問の背景 (インライン, 任意)")
	fs.StringVar(&background, "b", "", "質問の背景 (インライン, 短縮形)")
	fs.StringVar(&backgroundFile, "background-file", "", "質問の背景をファイルから読み込む (任意)")
	from := fs.String("from", "", "質問者の名前 (任意, そのまま表示)")
	timeout := fs.Duration("timeout", 30*time.Minute, "回答待ちの最大時間")
	interval := fs.Duration("interval", 10*time.Second, "ポーリング間隔")
	if err := fs.Parse(args); err != nil {
		return err
	}

	q, err := resolveQuestion(question, questionFile, fs.Args())
	if err != nil {
		return err
	}
	bg, err := resolveText(background, backgroundFile)
	if err != nil {
		return fmt.Errorf("背景の読み込みに失敗しました: %w", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	client := slack.New(cfg.WebhookURL, cfg.BotToken, cfg.ChannelID)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	answer, err := ask(ctx, client, cfg, q, bg, strings.TrimSpace(*from), *timeout, *interval)
	if err != nil {
		return err
	}
	fmt.Println(answer)
	return nil
}

// resolveText はインライン文字列を優先し、無ければファイルから内容を読み込む。
// どちらも空なら空文字を返す（呼び出し側で必須判定する）。
func resolveText(inline, file string) (string, error) {
	if strings.TrimSpace(inline) != "" {
		return strings.TrimSpace(inline), nil
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("ファイル %q: %w", file, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return "", nil
}

// resolveQuestion は --question / --question-file / 位置引数 / 標準入力 の順に質問内容を解決する。
func resolveQuestion(inline, file string, args []string) (string, error) {
	q, err := resolveText(inline, file)
	if err != nil {
		return "", fmt.Errorf("質問内容の読み込みに失敗しました: %w", err)
	}
	if q != "" {
		return q, nil
	}
	if len(args) > 0 {
		if joined := strings.TrimSpace(strings.Join(args, " ")); joined != "" {
			return joined, nil
		}
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	if q := strings.TrimSpace(string(data)); q != "" {
		return q, nil
	}
	return "", errors.New("質問内容が空です。--question / --question-file / 位置引数 / 標準入力 のいずれかで渡してください")
}

// buildMessage は Slack に投稿する質問メッセージ本文を組み立てる。
// background が空なら背景ブロックを、from が空なら質問者表記を省略する。
func buildMessage(bossUserID, from, question, background, marker string) string {
	var b strings.Builder
	if from != "" {
		fmt.Fprintf(&b, "<@%s> 質問です（質問者: %s）:\n\n", bossUserID, from)
	} else {
		fmt.Fprintf(&b, "<@%s> 質問です:\n\n", bossUserID)
	}
	if background != "" {
		fmt.Fprintf(&b, "*【背景】*\n%s\n\n", background)
	}
	fmt.Fprintf(&b, "*【質問】*\n%s\n\n", question)
	fmt.Fprintf(&b, "_このスレッドに返信してください_ %s", marker)
	return b.String()
}

// newQuestionID は履歴から自分の投稿を特定するためのユニーク ID を生成する。
func newQuestionID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ask は質問を投稿し、上司の返信が付くまでポーリングして回答テキストを返す。
func ask(ctx context.Context, client *slack.Client, cfg *config, question, background, from string, timeout, interval time.Duration) (string, error) {
	id := newQuestionID()
	marker := fmt.Sprintf("[ask-the-boss:%s]", id)
	text := buildMessage(cfg.BossUserID, from, question, background, marker)

	if err := client.PostWebhook(ctx, text); err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "質問を投稿しました (id=%s)。上司の返信を待っています...\n", id)

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var threadTS string
	for {
		// まだ ts が特定できていなければ履歴から探す。
		if threadTS == "" {
			ts, err := client.FindMessageTS(ctx, marker)
			if err != nil {
				return "", err
			}
			threadTS = ts
		}
		// ts が特定できていれば返信を確認する。
		if threadTS != "" {
			reply, err := client.ReplyFrom(ctx, threadTS, cfg.BossUserID)
			if err != nil {
				return "", err
			}
			if reply != nil {
				text := client.ResolveMentions(ctx, reply.Text)
				return strings.TrimSpace(text), nil
			}
		}

		if time.Now().After(deadline) {
			return "", fmt.Errorf("タイムアウトしました (%s 以内に上司の返信がありませんでした)", timeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}
