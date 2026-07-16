package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	toolTimeout = 60 * time.Second
	maxOutput   = 8000 // tool chiqishi shu belgidan uzun bo'lsa kesiladi (kontekst uchun)
)

// Tool — agent chaqira oladigan bitta qobiliyat.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any // JSON schema (LLM tools parametri)
	Run         func(ctx context.Context, in map[string]any) (string, error)
}

// Registry — mavjud tool'lar to'plami. Docker klientini collector kabi FromEnv oladi.
type Registry struct {
	docker *client.Client
	tools  map[string]*Tool
	order  []string
}

func NewRegistry(mgr *Manager) *Registry {
	cli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	r := &Registry{docker: cli, tools: map[string]*Tool{}}
	r.register(bashTool(mgr))
	r.register(readFileTool())
	r.register(writeFileTool())
	r.register(dockerPsTool(cli))
	r.register(dockerLogsTool(cli))
	r.register(dockerRestartTool(cli))
	return r
}

func (r *Registry) register(t *Tool) {
	r.tools[t.Name] = t
	r.order = append(r.order, t.Name)
}

// Specs LLM'ga beriladigan tool ta'riflarini qaytaradi (nom, tavsif, schema).
func (r *Registry) Specs() []*Tool {
	out := make([]*Tool, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.tools[n])
	}
	return out
}

// guardArg tool kirishidan xavfsizlik darvozasi uchun matn ajratadi.
func guardArg(name string, in map[string]any) string {
	if name == "bash" {
		return str(in["command"])
	}
	return str(in["container"])
}

// Run tool'ni ishga tushiradi (chaqiruvchi allaqachon tasdiqni hal qilgan).
func (r *Registry) Run(ctx context.Context, name string, in map[string]any) (string, error) {
	t := r.tools[name]
	if t == nil {
		return "", fmt.Errorf("noma'lum tool: %s", name)
	}
	cctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()
	out, err := t.Run(cctx, in)
	return clip(out), err
}

var reSudo = regexp.MustCompile(`\bsudo\b`)

func bashTool(mgr *Manager) *Tool {
	return &Tool{
		Name:        "bash",
		Description: "Bash buyrug'ini bajaradi (sh -c). stdout+stderr qaytaradi. Sudo paroli sozlangan bo'lsa sudo buyruqlar parolsiz ishlaydi. Xavfli buyruqlar tasdiq so'raydi.",
		Schema:      obj(props{"command": strProp("Bajariladigan bash buyrug'i")}, "command"),
		Run: func(ctx context.Context, in map[string]any) (string, error) {
			command := str(in["command"])
			env := os.Environ()

			// Sudo paroli sozlangan bo'lsa: askpass yordamchisi orqali sudo'ni
			// interaktivsiz (parolsiz) ishlatamiz. `sudo` -> `sudo -A` ga o'giriladi.
			if pw := mgr.SudoPassword(); pw != "" && reSudo.MatchString(command) {
				askpass, cleanup, err := writeAskpass(pw)
				if err == nil {
					defer cleanup()
					env = append(env, "SUDO_ASKPASS="+askpass)
					command = reSudo.ReplaceAllString(command, "sudo -A")
				}
			}

			cmd := exec.CommandContext(ctx, "sh", "-c", command)
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			s := string(out)
			if err != nil {
				return s, fmt.Errorf("%v", err)
			}
			if s == "" {
				s = "(chiqish yo'q)"
			}
			return s, nil
		},
	}
}

// writeAskpass parolni chiqaradigan vaqtinchalik skript yozadi (0700). sudo -A
// shu skriptni chaqirib parolni oladi — parol buyruq satrida ko'rinmaydi.
func writeAskpass(pw string) (string, func(), error) {
	f, err := os.CreateTemp("", "pp-askpass-*.sh")
	if err != nil {
		return "", func() {}, err
	}
	// Parol single-quote ichida; ichidagi ' -> '\'' bo'lib qochiriladi.
	esc := strings.ReplaceAll(pw, "'", `'\''`)
	fmt.Fprintf(f, "#!/bin/sh\nprintf '%%s\\n' '%s'\n", esc)
	f.Close()
	os.Chmod(f.Name(), 0o700)
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

func readFileTool() *Tool {
	return &Tool{
		Name:        "read_file",
		Description: "Faylni o'qiydi va matnini qaytaradi.",
		Schema:      obj(props{"path": strProp("Fayl yo'li")}, "path"),
		Run: func(_ context.Context, in map[string]any) (string, error) {
			b, err := os.ReadFile(str(in["path"]))
			return string(b), err
		},
	}
}

func writeFileTool() *Tool {
	return &Tool{
		Name:        "write_file",
		Description: "Faylga yozadi (mavjud bo'lsa ustidan yozadi). Config tahriri uchun.",
		Schema:      obj(props{"path": strProp("Fayl yo'li"), "content": strProp("Yoziladigan matn")}, "path", "content"),
		Run: func(_ context.Context, in map[string]any) (string, error) {
			if err := os.WriteFile(str(in["path"]), []byte(str(in["content"])), 0o644); err != nil {
				return "", err
			}
			return "yozildi: " + str(in["path"]), nil
		},
	}
}

func dockerPsTool(cli *client.Client) *Tool {
	return &Tool{
		Name:        "docker_ps",
		Description: "Ishlab turgan Docker konteynerlar ro'yxati (nom, image, holat).",
		Schema:      obj(props{}),
		Run: func(ctx context.Context, _ map[string]any) (string, error) {
			list, err := cli.ContainerList(ctx, container.ListOptions{All: true})
			if err != nil {
				return "", err
			}
			var b strings.Builder
			for _, c := range list {
				name := ""
				if len(c.Names) > 0 {
					name = strings.TrimPrefix(c.Names[0], "/")
				}
				fmt.Fprintf(&b, "%-28s %-30s %s\n", name, c.Image, c.State)
			}
			return b.String(), nil
		},
	}
}

func dockerLogsTool(cli *client.Client) *Tool {
	return &Tool{
		Name:        "docker_logs",
		Description: "Konteynerning oxirgi loglarini o'qiydi (tashxis uchun).",
		Schema: obj(props{
			"container": strProp("Konteyner nomi"),
			"tail":      strProp("Nechta oxirgi qator (standart 200)"),
		}, "container"),
		Run: func(ctx context.Context, in map[string]any) (string, error) {
			tail := str(in["tail"])
			if tail == "" {
				tail = "200"
			}
			rc, err := cli.ContainerLogs(ctx, str(in["container"]), container.LogsOptions{
				ShowStdout: true, ShowStderr: true, Tail: tail,
			})
			if err != nil {
				return "", err
			}
			defer rc.Close()
			var out, errb bytes.Buffer
			stdcopy.StdCopy(&out, &errb, rc)
			return out.String() + errb.String(), nil
		},
	}
}

func dockerRestartTool(cli *client.Client) *Tool {
	return &Tool{
		Name:        "docker_restart",
		Description: "Konteynerni qayta ishga tushiradi. DESTRUKTIV — tasdiq so'raydi.",
		Schema:      obj(props{"container": strProp("Konteyner nomi")}, "container"),
		Run: func(ctx context.Context, in map[string]any) (string, error) {
			d := 10 * time.Second
			secs := int(d.Seconds())
			if err := cli.ContainerRestart(ctx, str(in["container"]), container.StopOptions{Timeout: &secs}); err != nil {
				return "", err
			}
			return "qayta ishga tushirildi: " + str(in["container"]), nil
		},
	}
}

// --- kichik yordamchilar ---

type props = map[string]any

func obj(p props, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": p}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

func clip(s string) string {
	if len(s) > maxOutput {
		return s[:maxOutput] + "\n…(kesildi)"
	}
	return s
}
