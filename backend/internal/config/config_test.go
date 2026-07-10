package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesToEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yaml := `
target_containers: [p24gui, gateway]
listen_addr: ":9000"
speedtest_min: 30
scan_subnet: ["192.168.1.0/24"]
devices:
  - name: Kirish kamera
    ip: 192.168.1.64
  - ip: 192.168.1.70
analyzer:
  match_window_sec: 200
  autopay_sec: 45
relay_open_re: "Relay open (exit \\d+)"
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	// Toza muhit
	for _, k := range []string{"TARGET_CONTAINER", "LISTEN_ADDR", "SPEEDTEST_MIN", "SCAN_SUBNET", "DEVICES", "MATCH_WINDOW_SEC", "AUTOPAY_SEC", "RELAY_OPEN_RE"} {
		os.Unsetenv(k)
	}
	t.Setenv("CONFIG_FILE", path)

	got, err := Load()
	if err != nil {
		t.Fatalf("Load xato: %v", err)
	}
	if got != path {
		t.Errorf("path = %q, kutilgan %q", got, path)
	}
	checks := map[string]string{
		"TARGET_CONTAINER": "p24gui,gateway",
		"LISTEN_ADDR":      ":9000",
		"SPEEDTEST_MIN":    "30",
		"SCAN_SUBNET":      "192.168.1.0/24",
		"DEVICES":          "Kirish kamera=192.168.1.64,192.168.1.70",
		"MATCH_WINDOW_SEC": "200",
		"AUTOPAY_SEC":      "45",
		"RELAY_OPEN_RE":    `Relay open (exit \d+)`,
	}
	for k, want := range checks {
		if got := os.Getenv(k); got != want {
			t.Errorf("env %s = %q, kutilgan %q", k, got, want)
		}
	}
}

// Aniq berilgan env konfig faylidan ustun turishi kerak.
func TestEnvOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	os.WriteFile(path, []byte("listen_addr: \":9000\"\n"), 0o644)

	t.Setenv("CONFIG_FILE", path)
	t.Setenv("LISTEN_ADDR", ":7777") // aniq env

	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("LISTEN_ADDR"); got != ":7777" {
		t.Errorf("env ustun turishi kerak edi: %q", got)
	}
}

// Konfig fayl yo'q bo'lsa — xato emas.
func TestLoadNoFile(t *testing.T) {
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "yoq.yaml"))
	if _, err := Load(); err == nil {
		// CONFIG_FILE aniq berilgan-u, fayl yo'q — bu holatda xato qaytishi mumkin,
		// lekin default yo'llar rejimida xato bo'lmasligi kerak. Ikkalasini ham sinaymiz.
		t.Log("CONFIG_FILE ko'rsatilgan, fayl yo'q")
	}
	os.Unsetenv("CONFIG_FILE")
	if _, err := Load(); err != nil {
		t.Errorf("konfigsiz Load xato qaytarmasligi kerak: %v", err)
	}
}
