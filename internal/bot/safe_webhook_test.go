package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tele "gopkg.in/telebot.v3"
)

func TestSafeWebhookStopDoesNotPanic(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botTEST_TOKEN/setWebhook" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
	}))
	defer api.Close()

	poller := &safeWebhook{
		Endpoint:    &tele.WebhookEndpoint{PublicURL: "https://example.com/swim-bot/webhook"},
		SecretToken: "secret",
	}

	tb, err := tele.NewBot(tele.Settings{
		Token:   "TEST_TOKEN",
		URL:     api.URL,
		Poller:  poller,
		Offline: true,
		OnError: func(err error, c tele.Context) {
			t.Fatalf("unexpected telebot error: %v", err)
		},
	})
	if err != nil {
		t.Fatalf("NewBot failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		tb.Start()
	}()

	time.Sleep(50 * time.Millisecond)
	tb.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bot did not stop in time")
	}
}
