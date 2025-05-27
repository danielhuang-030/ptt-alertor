package channels

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendDiscord_Success(t *testing.T) {
	expectedContent := "測試 Discord 通知成功"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("預期請求方法為 POST，實際為 %s", r.Method)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Errorf("預期 Content-Type 為 application/json，實際為 %s", contentType)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("解碼請求 body 失敗：%v", err)
		}
		defer r.Body.Close()

		content, ok := payload["content"].(string)
		if !ok {
			t.Errorf("請求 payload 中缺少 'content' 欄位或其型別不為 string")
		}
		if content != expectedContent {
			t.Errorf("預期的 content 為 '%s'，實際為 '%s'", expectedContent, content)
		}
		w.WriteHeader(http.StatusNoContent) // Discord Webhook 成功時通常返回 204
	}))
	defer server.Close()

	err := SendDiscord(server.URL, expectedContent)
	if err != nil {
		t.Errorf("預期 SendDiscord 成功，但收到錯誤：%v", err)
	}
}

func TestSendDiscord_HttpError(t *testing.T) {
	expectedContent := "測試 Discord HTTP 錯誤"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // 模擬錯誤狀態碼
		fmt.Fprintln(w, "{\"message\": \"模擬的 Discord API 錯誤\", \"code\": 50001}")
	}))
	defer server.Close()

	err := SendDiscord(server.URL, expectedContent)
	if err == nil {
		t.Fatalf("預期 SendDiscord 返回錯誤，但實際為 nil")
	}

	expectedErrorSubstring := "Discord webhook returned status 400"
	if !strings.Contains(err.Error(), expectedErrorSubstring) {
		t.Errorf("錯誤訊息 '%s' 中未包含預期的子字串 '%s'", err.Error(), expectedErrorSubstring)
	}
    
    expectedResponseBodySubstring := "模擬的 Discord API 錯誤"
	if !strings.Contains(err.Error(), expectedResponseBodySubstring) {
		t.Errorf("錯誤訊息 '%s' 中未包含預期的回應 body 子字串 '%s'", err.Error(), expectedResponseBodySubstring)
	}
}

func TestSendDiscord_InvalidUrl(t *testing.T) {
	// 使用一個無效的 URL (例如，包含無法解析的字符或不存在的協議)
	// 注意：直接使用 "http://:" 會在 Go 1.17+ 中被 net/http.Post 清理掉，
	// 導致一個不同的錯誤 (如 "http: no Host in request URL")。
	// 為了更穩定地觸發 Post 本身的錯誤，可以使用一個格式正確但無法連線的 URL，
	// 或者一個會導致 net.Dial 錯誤的 URL。
	// 這裡我們用一個明顯錯誤的協議。
	invalidURL := "invalid-protocol://localhost"
	err := SendDiscord(invalidURL, "測試無效 URL")
	if err == nil {
		t.Fatalf("預期 SendDiscord 因無效 URL 返回錯誤，但實際為 nil")
	}
	// 錯誤訊息可能因作業系統和 Go 版本而異，但應指示 URL 問題
	// 例如 "unsupported protocol scheme" 或類似的網路錯誤
	t.Logf("收到的無效 URL 錯誤：%v (這是預期的)", err) // 記錄實際錯誤以供參考
    if !strings.Contains(err.Error(), "unsupported protocol scheme") && !strings.Contains(err.Error(), "no such host") && !strings.Contains(err.Error(), "dial tcp") {
        // 這裡的檢查比較寬鬆，因為具體的網路錯誤訊息可能多樣
        t.Errorf("預期錯誤與 URL 無效或網路問題相關，但收到的錯誤是：%v", err)
    }
}

func TestSendDiscord_EmptyWebhookURL(t *testing.T) {
	err := SendDiscord("", "測試空 Webhook URL")
	if err == nil {
		t.Fatalf("預期 SendDiscord 因空 Webhook URL 返回錯誤，但實際為 nil")
	}
    // http.Post 對於空 URL 的行為可能是返回 "http: no Host in request URL" 或類似錯誤
	t.Logf("收到的空 URL 錯誤：%v (這是預期的)", err)
    if !strings.Contains(err.Error(), "no Host in request URL") && !strings.Contains(err.Error(), "unsupported protocol scheme \"\"") {
         t.Errorf("預期錯誤與空 URL 或無效協議相關，但收到的錯誤是：%v", err)
    }
}
