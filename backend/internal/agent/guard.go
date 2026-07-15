package agent

import "regexp"

// guard.go — inson-tsikldagi xavfsizlik darvozasi.
//
// Siyosat ("ko'proq avtomatik"): o'qish va oddiy yozish (config tahriri)
// avtomatik bajariladi; faqat DESTRUKTIV amallar (o'chirish, DB drop, konteyner
// to'xtatish/o'ldirish, disk formatlash...) tasdiq so'raydi. Hech qachon
// destruktiv buyruq avtomatik bajarilmaydi.

// Har biri tanasida bitta destruktiv niyat + inson o'qiy oladigan sabab.
var destructivePatterns = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`(?i)\brm\s+(-[a-z]*\s+)*`), "fayl/papka o'chirish (rm)"},
	{regexp.MustCompile(`(?i)\brmdir\b`), "papka o'chirish (rmdir)"},
	{regexp.MustCompile(`(?i)\bdd\b`), "disk yozish (dd)"},
	{regexp.MustCompile(`(?i)\bmkfs\b`), "disk formatlash (mkfs)"},
	{regexp.MustCompile(`(?i)\b(shutdown|reboot|poweroff|halt)\b`), "tizimni o'chirish/qayta yuklash"},
	{regexp.MustCompile(`(?i)\b(kill|pkill|killall)\b`), "jarayonni o'ldirish (kill)"},
	{regexp.MustCompile(`(?i)\btruncate\b`), "faylni bo'shatish (truncate)"},
	{regexp.MustCompile(`(?i)\bdocker\b[^\n]*\b(rm|rmi|kill|stop|prune|down)\b`), "konteyner/ image o'chirish yoki to'xtatish"},
	{regexp.MustCompile(`(?i)\bdrop\s+(database|table|schema)\b`), "ma'lumotlar bazasini drop qilish"},
	{regexp.MustCompile(`(?i)\bdelete\s+from\b`), "SQL DELETE"},
	{regexp.MustCompile(`(?i)\bgit\b[^\n]*\b(reset\s+--hard|clean\s+-[a-z]*f|push\s+--force|push\s+-f)\b`), "git tarixini yo'qotish"},
	{regexp.MustCompile(`(?i)\bchmod\s+-R\b|\bchown\s+-R\b`), "rekursiv ruxsat o'zgartirish"},
	{regexp.MustCompile(`(?i):\s*>\s*/|>\s*/dev/sd`), "muhim faylni bo'shatish/qurilmaga yozish"},
}

// classifyBash bash buyrug'i destruktivligini aniqlaydi.
func classifyBash(cmd string) (bool, string) {
	for _, p := range destructivePatterns {
		if p.re.MatchString(cmd) {
			return true, p.reason
		}
	}
	return false, ""
}

// destructive — tool + kirish bo'yicha tasdiq kerakmi.
//
//	bash            → buyruq matnini tekshiradi
//	docker_stop/kill/restart/remove → doim ha (konteyner ishini uzadi)
//	read/write_file, docker_logs, docker_ps → yo'q (yozish "oddiy" hisoblanadi)
func destructive(tool, rawInput string) (bool, string) {
	switch tool {
	case "bash":
		return classifyBash(rawInput)
	case "docker_stop", "docker_kill", "docker_remove", "docker_restart":
		return true, "konteyner ishini uzadi"
	default:
		return false, ""
	}
}
