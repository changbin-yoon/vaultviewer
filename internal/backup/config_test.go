package backup

import "testing"

func TestConfigEnabled(t *testing.T) {
	full := Config{Endpoint: "minio:9000", Bucket: "b", AccessKey: "a", SecretKey: "s"}
	if !full.Enabled() {
		t.Errorf("expected Enabled() with all fields set")
	}
	if (Config{}).Enabled() {
		t.Errorf("expected Enabled() false for zero value")
	}
	missingKey := full
	missingKey.SecretKey = ""
	if missingKey.Enabled() {
		t.Errorf("expected Enabled() false when SecretKey is missing")
	}
}

func TestLoadConfigFromEnvDisabledByDefault(t *testing.T) {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	if cfg.Enabled() {
		t.Errorf("expected backup disabled when no ACCESSLENS_S3_* env vars are set")
	}
	if cfg.Interval.Minutes() != 30 {
		t.Errorf("Interval = %v, want 30m default", cfg.Interval)
	}
}

func TestLoadConfigFromEnvInvalidInterval(t *testing.T) {
	t.Setenv("ACCESSLENS_BACKUP_INTERVAL_MINUTES", "not-a-number")
	if _, err := LoadConfigFromEnv(); err == nil {
		t.Fatalf("expected error for non-numeric ACCESSLENS_BACKUP_INTERVAL_MINUTES")
	}
}

func TestLoadConfigFromEnvZeroInterval(t *testing.T) {
	t.Setenv("ACCESSLENS_BACKUP_INTERVAL_MINUTES", "0")
	if _, err := LoadConfigFromEnv(); err == nil {
		t.Fatalf("expected error for zero ACCESSLENS_BACKUP_INTERVAL_MINUTES")
	}
}

func TestObjectKey(t *testing.T) {
	cases := []struct {
		prefix, date, rel, want string
	}{
		{"", "2026-08-22", "notes/a.md", "2026-08-22/notes/a.md"},
		{"vaultviewer", "2026-08-22", "notes/a.md", "vaultviewer/2026-08-22/notes/a.md"},
		{"/vaultviewer/", "2026-08-22", "a.md", "vaultviewer/2026-08-22/a.md"},
		{"vaultviewer", "2026-08-22", "", "vaultviewer/2026-08-22"},
	}
	for _, c := range cases {
		if got := objectKey(c.prefix, c.date, c.rel); got != c.want {
			t.Errorf("objectKey(%q,%q,%q) = %q, want %q", c.prefix, c.date, c.rel, got, c.want)
		}
	}
}
