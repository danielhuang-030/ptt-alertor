package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// SendTextMessage 透過 Discord Webhook 傳送純文字通知。
// 此函式模仿其他渠道 (如 Telegram) 的函式簽名。
func SendTextMessage(webhookURL string, message string) error {
	if webhookURL == "" {
		return fmt.Errorf("Discord webhook URL is empty")
	}

	payload := map[string]interface{}{
		"content": message,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord payload: %w", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to send Discord message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var responseBody bytes.Buffer
		_, readErr := responseBody.ReadFrom(resp.Body)
		if readErr != nil {
			// 如果讀取回應 body 也失敗，只回報原始狀態碼錯誤
			return fmt.Errorf("Discord webhook returned status %d (failed to read response body: %v)", resp.StatusCode, readErr)
		}
		return fmt.Errorf("Discord webhook returned status %d. Response: %s", resp.StatusCode, responseBody.String())
	}

	return nil
}
