package chatconfig

import "testing"

func TestNormalizeTrackedBot(t *testing.T) {
	// нормализация username inline-бота для отслеживания
	got := NormalizeTrackedBot(" @MLVerseBot ")
	if got != "mlversebot" {
		t.Fatalf("NormalizeTrackedBot() = %q, want %q", got, "mlversebot")
	}
}

func TestNormalizeBotUsername(t *testing.T) {
	// нормализация username bot-аккаунта для правила автоудаления
	got, err := NormalizeBotUsername(" @Clown_Alert_Bot ")
	if err != nil {
		t.Fatalf("NormalizeBotUsername() error = %v", err)
	}
	if got != "clown_alert_bot" {
		t.Fatalf("NormalizeBotUsername() = %q, want %q", got, "clown_alert_bot")
	}
}

func TestNormalizeBotUsernameEmptyReturnsError(t *testing.T) {
	// ошибка для пустого username bot-аккаунта
	_, err := NormalizeBotUsername(" @ ")
	if err == nil {
		t.Fatal("NormalizeBotUsername() error = nil, want error")
	}
}

func TestValidateRateLimitConfig(t *testing.T) {
	// проверка допустимых и недопустимых настроек лимитов
	tests := []struct {
		name                string
		daily               int
		ringBufferSize      int
		ringBufferThreshold int
		wantErr             bool
	}{
		{name: "valid", daily: 6, ringBufferSize: 30, ringBufferThreshold: 3},
		{name: "invalid daily", daily: 0, ringBufferSize: 30, ringBufferThreshold: 3, wantErr: true},
		{name: "invalid ring buffer size", daily: 6, ringBufferSize: 0, ringBufferThreshold: 3, wantErr: true},
		{name: "invalid ring buffer threshold", daily: 6, ringBufferSize: 30, ringBufferThreshold: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRateLimitConfig(tt.daily, tt.ringBufferSize, tt.ringBufferThreshold)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateRateLimitConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRBStealStatus(t *testing.T) {
	// текстовый статус reactive-штрафа
	tests := []struct {
		threshold int
		want      string
	}{
		{threshold: 0, want: "off"},
		{threshold: -1, want: "off"},
		{threshold: 2, want: "on"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := RBStealStatus(tt.threshold)
			if got != tt.want {
				t.Fatalf("RBStealStatus(%d) = %q, want %q", tt.threshold, got, tt.want)
			}
		})
	}
}

func TestIsActive(t *testing.T) {
	// активность чата по включённым правилам
	tests := []struct {
		name                string
		trackedBot          string
		communityBanEnabled bool
		hasBotDeleteRules   bool
		want                bool
	}{
		{name: "tracked bot", trackedBot: "spambot", want: true},
		{name: "community ban", communityBanEnabled: true, want: true},
		{name: "bot delete rules", hasBotDeleteRules: true, want: true},
		{name: "inactive", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsActive(tt.trackedBot, tt.communityBanEnabled, tt.hasBotDeleteRules)
			if got != tt.want {
				t.Fatalf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}
