// Package slack はこのリポジトリの各コマンドが使う最小限の Slack 連携を提供する。
// 投稿するだけのコマンド（wol-slack）は PostWebhook のみを使い、BotToken /
// ChannelID は空でよい。
//
// 投稿は Incoming Webhook 経由、返信の読み取りは Bot トークンの
// conversations.history / conversations.replies 経由という二系統構成になっている。
// Incoming Webhook は投稿したメッセージの ts を返さないため、投稿本文に埋め込んだ
// マーカーを履歴から探して ts を特定する、という流れを Client が担う。
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// mentionRE は Slack のユーザーメンション表記 <@U123> / <@U123|label> にマッチする。
var mentionRE = regexp.MustCompile(`<@([UW][A-Z0-9]+)(?:\|[^>]*)?>`)

const apiBase = "https://slack.com/api"

// Client は Slack への投稿・読み取りをまとめて扱う。
type Client struct {
	WebhookURL string
	BotToken   string
	ChannelID  string

	// BaseURL は Slack Web API のベース URL。通常は空で apiBase を使う（テストで差し替え可能）。
	BaseURL string
	HTTP    *http.Client
}

// New は必要な認証情報から Client を生成する。
func New(webhookURL, botToken, channelID string) *Client {
	return &Client{
		WebhookURL: webhookURL,
		BotToken:   botToken,
		ChannelID:  channelID,
		BaseURL:    apiBase,
		HTTP:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Message は conversations.history / conversations.replies が返すメッセージ。
type Message struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	BotID    string `json:"bot_id"`
}

// PostWebhook は Incoming Webhook にテキストを投稿する。
// Webhook はメッセージの ts を返さないため戻り値は error のみ。
func (c *Client) PostWebhook(ctx context.Context, text string) error {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(respBody)) != "ok" {
		return fmt.Errorf("webhook への投稿に失敗しました: status=%d body=%q", resp.StatusCode, string(respBody))
	}
	return nil
}

type apiResponse struct {
	OK       bool      `json:"ok"`
	Error    string    `json:"error"`
	Messages []Message `json:"messages"`
}

// fetch は Bot トークン付きで Slack Web API を GET し、レスポンスを out にデコードする。
func (c *Client) fetch(ctx context.Context, method string, params url.Values, out any) error {
	base := c.BaseURL
	if base == "" {
		base = apiBase
	}
	u := fmt.Sprintf("%s/%s?%s", base, method, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.BotToken)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s のレスポンス解析に失敗しました: %w", method, err)
	}
	return nil
}

func (c *Client) getAPI(ctx context.Context, method string, params url.Values) (*apiResponse, error) {
	var out apiResponse
	if err := c.fetch(ctx, method, params, &out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("%s が失敗しました: %s", method, out.Error)
	}
	return &out, nil
}

// FindMessageTS は channel の直近履歴から marker を含むメッセージを探し、その ts を返す。
// 見つからなければ空文字を返す（エラーではない）。
func (c *Client) FindMessageTS(ctx context.Context, marker string) (string, error) {
	params := url.Values{}
	params.Set("channel", c.ChannelID)
	params.Set("limit", "50")
	resp, err := c.getAPI(ctx, "conversations.history", params)
	if err != nil {
		return "", err
	}
	for _, m := range resp.Messages {
		if strings.Contains(m.Text, marker) {
			return m.TS, nil
		}
	}
	return "", nil
}

// ReplyFrom は threadTS のスレッドから userID による最初の返信を探して返す。
// まだ無ければ nil を返す（エラーではない）。
func (c *Client) ReplyFrom(ctx context.Context, threadTS, userID string) (*Message, error) {
	params := url.Values{}
	params.Set("channel", c.ChannelID)
	params.Set("ts", threadTS)
	resp, err := c.getAPI(ctx, "conversations.replies", params)
	if err != nil {
		return nil, err
	}
	for _, m := range resp.Messages {
		if m.TS == threadTS {
			continue // 親メッセージ（質問本体）はスキップ
		}
		if m.User == userID {
			reply := m
			return &reply, nil
		}
	}
	return nil, nil
}

type userResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	User  struct {
		Name    string `json:"name"`
		Profile struct {
			DisplayName string `json:"display_name"`
			RealName    string `json:"real_name"`
		} `json:"profile"`
	} `json:"user"`
}

// UserDisplayName は users.info で userID の表示名を取得する。
// display_name > real_name > name の優先順で返す。
func (c *Client) UserDisplayName(ctx context.Context, userID string) (string, error) {
	params := url.Values{}
	params.Set("user", userID)
	var out userResponse
	if err := c.fetch(ctx, "users.info", params, &out); err != nil {
		return "", err
	}
	if !out.OK {
		return "", fmt.Errorf("users.info が失敗しました: %s", out.Error)
	}
	if n := out.User.Profile.DisplayName; n != "" {
		return n, nil
	}
	if n := out.User.Profile.RealName; n != "" {
		return n, nil
	}
	return out.User.Name, nil
}

// ResolveMentions はテキスト中の <@U123> 表記を @表示名 に置換する。
// これはベストエフォートで、解決に失敗したメンションは元の表記のまま残す。
// 同一 ID は一度だけ問い合わせてキャッシュする。
func (c *Client) ResolveMentions(ctx context.Context, text string) string {
	cache := map[string]string{}
	return mentionRE.ReplaceAllStringFunc(text, func(match string) string {
		id := mentionRE.FindStringSubmatch(match)[1]
		if replaced, ok := cache[id]; ok {
			return replaced
		}
		name, err := c.UserDisplayName(ctx, id)
		if err != nil || name == "" {
			cache[id] = match // 解決失敗時は元のまま
			return match
		}
		replaced := "@" + name
		cache[id] = replaced
		return replaced
	})
}
