// Package webhook содержит transport-адаптер Telegram webhook для telebot.
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	tele "gopkg.in/telebot.v4"
)

// defaultReadHeaderTimeout ограничивает чтение HTTP-заголовков webhook-запроса.
const defaultReadHeaderTimeout = 5 * time.Second

// Config содержит настройки transport-адаптера webhook.
type Config struct {
	Port              int
	Listen            string
	PublicURL         string
	SecretToken       string
	MaxConnections    int
	AllowedUpdates    []string
	IP                string
	DropUpdates       bool
	TLS               *tele.WebhookTLS
	ReadHeaderTimeout time.Duration
}

// SafeWebhook реализует telebot Poller без повторного закрытия stop-канала.
type SafeWebhook struct {
	Listen            string
	MaxConnections    int
	AllowedUpdates    []string
	IP                string
	DropUpdates       bool
	SecretToken       string
	TLS               *tele.WebhookTLS
	Endpoint          *tele.WebhookEndpoint
	ReadHeaderTimeout time.Duration

	dest chan<- tele.Update
	bot  *tele.Bot
}

// New создаёт transport-адаптер webhook для telebot.
func New(cfg Config) *SafeWebhook {
	listen := cfg.Listen
	if listen == "" {
		listen = fmt.Sprintf(":%d", cfg.Port)
	}
	readHeaderTimeout := cfg.ReadHeaderTimeout
	if readHeaderTimeout == 0 {
		readHeaderTimeout = defaultReadHeaderTimeout
	}

	return &SafeWebhook{
		Listen:            listen,
		MaxConnections:    cfg.MaxConnections,
		AllowedUpdates:    cfg.AllowedUpdates,
		IP:                cfg.IP,
		DropUpdates:       cfg.DropUpdates,
		SecretToken:       cfg.SecretToken,
		TLS:               cfg.TLS,
		ReadHeaderTimeout: readHeaderTimeout,
		Endpoint: &tele.WebhookEndpoint{
			PublicURL: cfg.PublicURL,
		},
	}
}

func (h *SafeWebhook) getParams() map[string]string {
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

// Poll настраивает Telegram webhook и запускает HTTP endpoint.
func (h *SafeWebhook) Poll(b *tele.Bot, dest chan tele.Update, stop chan struct{}) {
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
		ReadHeaderTimeout: h.ReadHeaderTimeout,
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

func (h *SafeWebhook) waitForStop(stop chan struct{}) {
	<-stop
}

// ServeHTTP принимает Telegram update и передаёт его в telebot.
func (h *SafeWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.SecretToken != "" && r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != h.SecretToken {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	var update tele.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	normalizeRichMessages(&update)
	h.dest <- update
}
