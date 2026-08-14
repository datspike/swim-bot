package webhook

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tele "gopkg.in/telebot.v4"
)

func TestSafeWebhookReportsListenError(t *testing.T) {
	// ошибка запуска listener передаётся в telebot OnError
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botTEST_TOKEN/setWebhook" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
	}))
	defer api.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer listener.Close()

	errorCh := make(chan error, 1)
	poller := New(Config{
		Listen:    listener.Addr().String(),
		PublicURL: "https://example.com/swim-bot/webhook",
	})

	tb, err := tele.NewBot(tele.Settings{
		Token:   "TEST_TOKEN",
		URL:     api.URL,
		Poller:  poller,
		Offline: true,
		OnError: func(err error, _ tele.Context) {
			errorCh <- err
		},
	})
	if err != nil {
		t.Fatalf("NewBot failed: %v", err)
	}

	poller.Poll(tb, make(chan tele.Update), make(chan struct{}))

	select {
	case err := <-errorCh:
		if err == nil {
			t.Fatal("ожидалась ошибка запуска listener")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ожидалась ошибка запуска listener")
	}
}

func TestSafeWebhookStopDoesNotPanic(t *testing.T) {
	// остановка telebot не вызывает повторное закрытие stop-канала
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botTEST_TOKEN/setWebhook" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
	}))
	defer api.Close()

	poller := New(Config{
		PublicURL:   "https://example.com/swim-bot/webhook",
		SecretToken: "secret",
	})

	tb, err := tele.NewBot(tele.Settings{
		Token:   "TEST_TOKEN",
		URL:     api.URL,
		Poller:  poller,
		Offline: true,
		OnError: func(err error, _ tele.Context) {
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

func TestNewSetsWebhookParams(t *testing.T) {
	// config явно задаёт public url и secret token
	poller := New(Config{
		Port:        8080,
		PublicURL:   "https://example.com/swim-bot/webhook",
		SecretToken: "secret",
	})

	params := poller.getParams()
	if params["url"] != "https://example.com/swim-bot/webhook" {
		t.Fatalf("url = %q, want public url", params["url"])
	}
	if params["secret_token"] != "secret" {
		t.Fatalf("secret_token = %q, want secret", params["secret_token"])
	}
}
