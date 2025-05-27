// channels/discord.go
package channels

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// SendDiscord 透過 Discord Webhook 傳送純文字通知
func SendDiscord(webhookURL, content string) error {
	payload := map[string]interface{}{
		"content": content,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 嘗試讀取回應內容以獲得更多錯誤資訊
		var responseBody bytes.Buffer
		_, _ = responseBody.ReadFrom(resp.Body) // 忽略讀取錯誤，因為主要錯誤是狀態碼
		return fmt.Errorf("Discord webhook returned status %d. Response: %s", resp.StatusCode, responseBody.String())
	}
	return nil
}
