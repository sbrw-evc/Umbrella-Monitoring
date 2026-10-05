package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const telegramAPI = "https://api.telegram.org"

// chatID: a numeric chat ID (negative for groups) or the @name of a channel.
var chatID = regexp.MustCompile(`^(-?\d{1,20}|@[A-Za-z][A-Za-z0-9_]{3,31})$`)

func ValidChat(v string) bool { return chatID.MatchString(v) }

type telegramReply struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	ErrorCode   int    `json:"error_code"`
	Result      struct {
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
	} `json:"result"`
}

// errPermanent marks a failure retrying will not fix.
type errPermanent struct{ error }

func telegramCall(ctx context.Context, client *http.Client, api, token, method string, body any) (telegramReply, error) {
	var out telegramReply
	if api == "" {
		api = telegramAPI
	}
	if token == "" {
		return out, errPermanent{errors.New("the Telegram bot token is not set")}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(api, "/")+"/bot"+token+"/"+method, bytes.NewReader(raw))
	if err != nil {
		return out, errPermanent{errors.New("the Telegram API address is not valid")}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		// The error carries the URL with the token in it.
		return out, errors.New(strings.ReplaceAll(err.Error(), token, "…"))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(data, &out)
	if resp.StatusCode/100 != 2 || !out.OK {
		msg := out.Description
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		err := fmt.Errorf("Telegram: %d %s", resp.StatusCode, msg)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return out, errPermanent{err}
		}
		return out, err
	}
	return out, nil
}

func sendTelegram(ctx context.Context, client *http.Client, api, token, chat, html string) error {
	_, err := telegramCall(ctx, client, api, token, "sendMessage", map[string]any{
		"chat_id": chat, "text": html, "parse_mode": "HTML",
		"link_preview_options": map[string]bool{"is_disabled": true},
	})
	return err
}
