package agent

import "testing"

func TestDestructiveClassifier(t *testing.T) {
	cases := []struct {
		tool, arg string
		want      bool
	}{
		// Xavfsiz — avtomatik bajariladi
		{"bash", "ls -la /app", false},
		{"bash", "cat parkpulse.yaml", false},
		{"bash", "grep -r enter p24.json", false},
		{"bash", "docker ps", false},
		{"read_file", "/etc/p24.json", false},
		{"write_file", "/app/p24.json", false}, // config tahriri "oddiy" = avto
		{"docker_logs", "p24gui", false},
		{"docker_ps", "", false},

		// Destruktiv — tasdiq so'raydi
		{"bash", "rm -rf /var/data", true},
		{"bash", "rm old.log", true},
		{"bash", "docker rm p24gui", true},
		{"bash", "docker stop p24gui", true},
		{"bash", "DROP TABLE sessions", true},
		{"bash", "mkfs.ext4 /dev/sdb", true},
		{"bash", "kill -9 1234", true},
		{"bash", "shutdown now", true},
		{"bash", "git reset --hard HEAD~3", true},
		{"docker_restart", "p24gui", true},
		{"docker_stop", "p24gui", true},
	}
	for _, c := range cases {
		got, reason := destructive(c.tool, c.arg)
		if got != c.want {
			t.Errorf("destructive(%q, %q) = %v (%q), kutildi %v", c.tool, c.arg, got, reason, c.want)
		}
		if got && reason == "" {
			t.Errorf("destructive(%q, %q): sabab bo'sh bo'lmasligi kerak", c.tool, c.arg)
		}
	}
}
