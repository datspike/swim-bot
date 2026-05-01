// Package chatconfig содержит правила настройки чатов swim-bot.
package chatconfig

import (
	"errors"
	"strings"
	"time"
)

// Config содержит конфигурацию бота для конкретного чата.
type Config struct {
	ChatID              int64
	TrackedBot          string
	IsActive            bool
	DailyLimit          int
	TestMode            bool
	RingBufferSize      int
	RingBufferThreshold int
	CommunityBanEnabled bool
	SpamLogChatID       int64
	SpamDeleteTTLSec    int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// NormalizeTrackedBot нормализует username inline-бота для спам-детекции.
func NormalizeTrackedBot(username string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(username)), "@")
}

// NormalizeBotUsername нормализует username bot-аккаунта для правил автоудаления.
func NormalizeBotUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	username = strings.TrimPrefix(username, "@")
	username = strings.ToLower(username)
	if username == "" {
		return "", errors.New("username bot-аккаунта не должен быть пустым")
	}
	return username, nil
}

// ValidateRateLimitConfig проверяет настройки лимитов спам-детекции.
func ValidateRateLimitConfig(daily, ringBufferSize, ringBufferThreshold int) error {
	if daily < 1 {
		return errors.New("daily должен быть не меньше 1")
	}
	if ringBufferSize < 1 {
		return errors.New("rb_size должен быть не меньше 1")
	}
	if ringBufferThreshold < 0 {
		return errors.New("rb_threshold должен быть не меньше 0")
	}
	return nil
}

// RBStealStatus возвращает текстовый статус reactive-штрафа.
func RBStealStatus(ringBufferThreshold int) string {
	if ringBufferThreshold <= 0 {
		return "off"
	}
	return "on"
}

// IsActive возвращает активность чата по включённым runtime-правилам.
func IsActive(trackedBot string, communityBanEnabled bool, hasBotDeleteRules bool) bool {
	return trackedBot != "" || communityBanEnabled || hasBotDeleteRules
}
