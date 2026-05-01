package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	tele "gopkg.in/telebot.v3"
)

// safeWebhook повторяет контракт tele.Webhook, но не закрывает stop-канал повторно.
// В telebot v3.3.8 стандартный Webhook закрывает stop внутри waitForStop,
// а Bot.Stop() закрывает тот же канал снаружи, что приводит к panic при shutdown.
const webhookReadHeaderTimeout = 5 * time.Second

type safeWebhook struct {
	Listen         string
	MaxConnections int
	AllowedUpdates []string
	IP             string
	DropUpdates    bool
	SecretToken    string
	TLS            *tele.WebhookTLS
	Endpoint       *tele.WebhookEndpoint

	dest chan<- tele.Update
	bot  *tele.Bot
}

func newSafeWebhook(cfg Config) *safeWebhook {
	return &safeWebhook{
		Listen: fmt.Sprintf(":%d", cfg.Port),
		Endpoint: &tele.WebhookEndpoint{
			PublicURL: cfg.WebhookURL,
		},
		SecretToken: cfg.WebhookSecret,
	}
}

func (h *safeWebhook) getParams() map[string]string {
	params := make(map[string]string)

	if h.MaxConnections != 0 {
		params["max_connections"] = strconv.Itoa(h.MaxConnections)
	}
	if len(h.AllowedUpdates) > 0 {
		data, err := json.Marshal(h.AllowedUpdates)
		if err == nil {
			params["allowed_updates"] = string(data)
		}
	}
	if h.IP != "" {
		params["ip_address"] = h.IP
	}
	if h.DropUpdates {
		params["drop_pending_updates"] = strconv.FormatBool(h.DropUpdates)
	}
	if h.SecretToken != "" {
		params["secret_token"] = h.SecretToken
	}

	if h.TLS != nil {
		params["url"] = "https://" + h.Listen
	} else {
		params["url"] = "http://" + h.Listen
	}
	if h.Endpoint != nil {
		params["url"] = h.Endpoint.PublicURL
	}

	return params
}

func (h *safeWebhook) Poll(b *tele.Bot, dest chan tele.Update, stop chan struct{}) {
	if _, err := b.Raw("setWebhook", h.getParams()); err != nil {
		b.OnError(err, nil)
		return
	}

	h.dest = dest
	h.bot = b

	if h.Listen == "" {
		h.waitForStop(stop)
		return
	}

	s := &http.Server{
		Addr:              h.Listen,
		Handler:           h,
		ReadHeaderTimeout: webhookReadHeaderTimeout,
	}

	go func() {
		h.waitForStop(stop)
		if err := s.Shutdown(context.Background()); err != nil {
			b.OnError(err, nil)
		}
	}()

	var err error
	if h.TLS != nil {
		err = s.ListenAndServeTLS(h.TLS.Cert, h.TLS.Key)
	} else {
		err = s.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		b.OnError(err, nil)
	}
}

func (h *safeWebhook) waitForStop(stop chan struct{}) {
	<-stop
}

func (h *safeWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.SecretToken != "" && r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != h.SecretToken {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	var update tele.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	h.dest <- update
}
