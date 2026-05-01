package config

import "testing"

func TestLoadRejectsInvalidMaxMessageAge(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "zero", value: "0"},
		{name: "negative", value: "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TELEGRAM_TOKEN", "token")
			t.Setenv("WEBHOOK_URL", "https://example.com/webhook")
			t.Setenv("MAX_MESSAGE_AGE_SEC", tt.value)

			_, err := Load()
			if err == nil {
				t.Fatal("ожидалась ошибка для некорректного MAX_MESSAGE_AGE_SEC")
			}
		})
	}
}

func TestLoadAcceptsPositiveMaxMessageAge(t *testing.T) {
	t.Setenv("TELEGRAM_TOKEN", "token")
	t.Setenv("WEBHOOK_URL", "https://example.com/webhook")
	t.Setenv("MAX_MESSAGE_AGE_SEC", "60")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MaxMessageAgeSec != 60 {
		t.Fatalf("MaxMessageAgeSec = %d, want 60", cfg.MaxMessageAgeSec)
	}
}
