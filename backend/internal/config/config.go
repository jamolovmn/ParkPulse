// Package config ixtiyoriy YAML konfig faylini o'qiydi va uni env
// o'zgaruvchilariga yoyadi. Shu tufayli qolgan kod avvalgidek env orqali
// ishlaydi — konfig faqat qulay, git-friendly muqobil.
//
// Ustuvorlik: aniq berilgan env HAR DOIM ustun. Konfig fayl faqat env
// bo'sh bo'lganda qiymat beradi. Ya'ni `docker run -e ...` konfigni bekor qiladi.
//
// Fayl yo'li: CONFIG_FILE env, bo'lmasa ./parkpulse.yaml, so'ng
// /etc/parkpulse/config.yaml. Fayl bo'lmasa — jimgina o'tkazib yuboriladi.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Device struct {
	Name string `yaml:"name"`
	IP   string `yaml:"ip"`
}

type Config struct {
	TargetContainers []string       `yaml:"target_containers"`
	ListenAddr       string         `yaml:"listen_addr"`
	StaticDir        string         `yaml:"static_dir"`
	SpeedtestMin     *int           `yaml:"speedtest_min"`
	ScanSubnet       []string       `yaml:"scan_subnet"`
	Devices          []Device       `yaml:"devices"`
	Analyzer         map[string]int `yaml:"analyzer"` // match_window_sec, autopay_sec, ...
	RelayOpenRe      string         `yaml:"relay_open_re"`
	RelayRemoteRe    string         `yaml:"relay_remote_re"`
}

func candidatePaths() []string {
	if p := os.Getenv("CONFIG_FILE"); p != "" {
		return []string{p}
	}
	return []string{"parkpulse.yaml", "/etc/parkpulse/config.yaml"}
}

// Load konfig faylni topib o'qiydi va env'ni to'ldiradi. Topilgan fayl yo'lini
// (yoki bo'shligini) va xatoni qaytaradi. Fayl yo'qligi xato emas.
func Load() (path string, err error) {
	var data []byte
	for _, p := range candidatePaths() {
		b, e := os.ReadFile(p)
		if e == nil {
			data, path = b, p
			break
		}
		if !os.IsNotExist(e) && os.Getenv("CONFIG_FILE") != "" {
			return p, fmt.Errorf("konfig o'qib bo'lmadi (%s): %w", p, e)
		}
	}
	if data == nil {
		return "", nil // konfigsiz ishlash — normal
	}

	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return path, fmt.Errorf("konfig YAML xato (%s): %w", path, err)
	}
	c.apply()
	return path, nil
}

// apply konfig qiymatlarini env'ga yoyadi (faqat env bo'sh bo'lsa).
func (c *Config) apply() {
	setIfEmpty("TARGET_CONTAINER", strings.Join(trimAll(c.TargetContainers), ","))
	setIfEmpty("LISTEN_ADDR", c.ListenAddr)
	setIfEmpty("STATIC_DIR", c.StaticDir)
	setIfEmpty("SCAN_SUBNET", strings.Join(trimAll(c.ScanSubnet), ","))
	setIfEmpty("RELAY_OPEN_RE", c.RelayOpenRe)
	setIfEmpty("RELAY_REMOTE_RE", c.RelayRemoteRe)
	if c.SpeedtestMin != nil {
		setIfEmpty("SPEEDTEST_MIN", strconv.Itoa(*c.SpeedtestMin))
	}

	// devices: [{name,ip}] -> "name=ip,name=ip"
	var pairs []string
	for _, d := range c.Devices {
		ip := strings.TrimSpace(d.IP)
		if ip == "" {
			continue
		}
		if name := strings.TrimSpace(d.Name); name != "" {
			pairs = append(pairs, name+"="+ip)
		} else {
			pairs = append(pairs, ip)
		}
	}
	setIfEmpty("DEVICES", strings.Join(pairs, ","))

	// analyzer: {match_window_sec: 180} -> MATCH_WINDOW_SEC=180
	for k, v := range c.Analyzer {
		setIfEmpty(strings.ToUpper(k), strconv.Itoa(v))
	}
}

func setIfEmpty(key, val string) {
	if val != "" && os.Getenv(key) == "" {
		os.Setenv(key, val)
	}
}

func trimAll(s []string) []string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
