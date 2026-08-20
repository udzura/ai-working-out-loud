// Command wol-slack は作業まとめ（Working Out Loud）を Slack に投稿する CLI。
//
// 使い方:
//
//	wol-slack --summary "調査結果をまとめました。次はキャッシュ層を見ます。"
//	wol-slack --title "パフォーマンス調査" --summary-file summary.md
//	echo "長めのまとめ" | wol-slack
//
// Incoming Webhook で投稿するだけの一方向のコマンドで、返信は待たない。
// Slack の mrkdwn にはリスト記法が無いため、本文は rich_text ブロックに変換して送る
// （対応する記法は richtext.go 冒頭のコメントを参照）。
// 必要な環境変数は Webhook URL のみ（WOL_SLACK_WEBHOOK_URL があればそれを使い、
// 無ければ SLACK_WEBHOOK_URL を使う）。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/udzura/ai-working-out-loud/internal/slack"
)

const usage = `wol-slack - 作業まとめ (Working Out Loud) を Slack に投稿する

使い方:
  wol-slack --summary "調査結果をまとめました。次はキャッシュ層を見ます。"
  wol-slack --title "パフォーマンス調査" --summary-file summary.md
  echo "まとめ本文" | wol-slack        # 標準入力から渡す
  wol-slack "まとめ本文"               # 位置引数でも渡せる

flags:
  --summary, -s     まとめ本文 (インライン)
  --summary-file    まとめ本文をファイルから読み込む
  --title, -t       見出し (任意)
  --from            投稿者の名前 (任意, 受け取った文字列をそのまま表示)
  --dry-run         投稿せず、送信する JSON (blocks) を標準出力に表示して終了

まとめ本文は --summary / --summary-file / 位置引数 / 標準入力 のいずれかで必須。
投稿するだけで返信は待たない（投稿に成功すると正常終了する）。

環境変数:
  WOL_SLACK_WEBHOOK_URL  投稿先 Incoming Webhook URL (優先)
  SLACK_WEBHOOK_URL      投稿先 Incoming Webhook URL (上記が無い場合に使用)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// help は正常終了として扱いたいので、標準出力に出して終わる。
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		fmt.Print(usage)
		return nil
	}

	fs := flag.NewFlagSet("wol-slack", flag.ContinueOnError)
	fs.Usage = func() {} // usage の出し分けは自前で行う
	var summary, summaryFile, title string
	fs.StringVar(&summary, "summary", "", "まとめ本文 (インライン)")
	fs.StringVar(&summary, "s", "", "まとめ本文 (インライン, 短縮形)")
	fs.StringVar(&summaryFile, "summary-file", "", "まとめ本文をファイルから読み込む")
	fs.StringVar(&title, "title", "", "見出し (任意)")
	fs.StringVar(&title, "t", "", "見出し (任意, 短縮形)")
	from := fs.String("from", "", "投稿者の名前 (任意, そのまま表示)")
	dryRun := fs.Bool("dry-run", false, "投稿せず送信する JSON を標準出力に表示して終了")
	if err := fs.Parse(args); err != nil {
		// -h / --help がフラグの後ろに来た場合もここに入る（正常終了させる）。
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(usage)
			return nil
		}
		fmt.Fprint(os.Stderr, usage)
		return err
	}

	body, err := resolveSummary(summary, summaryFile, fs.Args())
	if err != nil {
		return err
	}
	msg := buildPayload(strings.TrimSpace(title), strings.TrimSpace(*from), body)

	if *dryRun {
		out, err := json.MarshalIndent(msg, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}

	hookURL, err := webhookURL()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := slack.New(hookURL, "", "")
	if err := client.PostWebhookPayload(ctx, msg); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "作業まとめを投稿しました")
	return nil
}

// webhookURL は投稿先の Incoming Webhook URL を環境変数から取得する。
// ask-the-boss と別チャンネルに投げられるよう WOL_SLACK_WEBHOOK_URL を優先する。
func webhookURL() (string, error) {
	for _, key := range []string{"WOL_SLACK_WEBHOOK_URL", "SLACK_WEBHOOK_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v, nil
		}
	}
	return "", errors.New("必要な環境変数が未設定です: WOL_SLACK_WEBHOOK_URL (または SLACK_WEBHOOK_URL)")
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

// resolveSummary は --summary / --summary-file / 位置引数 / 標準入力 の順にまとめ本文を解決する。
func resolveSummary(inline, file string, args []string) (string, error) {
	s, err := resolveText(inline, file)
	if err != nil {
		return "", fmt.Errorf("まとめ本文の読み込みに失敗しました: %w", err)
	}
	if s != "" {
		return s, nil
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
	if s := strings.TrimSpace(string(data)); s != "" {
		return s, nil
	}
	return "", errors.New("まとめ本文が空です。--summary / --summary-file / 位置引数 / 標準入力 のいずれかで渡してください")
}
