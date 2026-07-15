// Command pulse — ParkPulse AI agentiga terminal interfeysi (claude kabi).
//
// Serverning /api/agent/stream WS endpointiga ulanadi — Web UI bilan AYNAN bitta
// agent sessiyasi. Bir joyda boshlangan ish ikkinchisida ham ko'rinadi.
//
//	pulse-cli                      # ws://localhost:8888 ga ulanadi
//	pulse-cli ws://host:8888       # boshqa manzil
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// resolveToken parol yoqilgan bo'lsa tokenni oladi (PULSE_PASSWORD env yoki so'rov).
func resolveToken(httpBase string) (string, error) {
	resp, err := http.Get(httpBase + "/api/agent/config")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var st struct {
		Auth bool `json:"auth"`
	}
	json.NewDecoder(resp.Body).Decode(&st)
	if !st.Auth {
		return "", nil // parol yo'q
	}
	pw := os.Getenv("PULSE_PASSWORD")
	if pw == "" {
		fmt.Print("Agent paroli: ")
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		pw = strings.TrimSpace(line)
	}
	body, _ := json.Marshal(map[string]string{"password": pw})
	lr, err := http.Post(httpBase+"/api/agent/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer lr.Body.Close()
	if lr.StatusCode != http.StatusOK {
		return "", fmt.Errorf("parol noto'g'ri")
	}
	var d struct {
		Token string `json:"token"`
	}
	json.NewDecoder(lr.Body).Decode(&d)
	return d.Token, nil
}

func httpFromWS(u string) string {
	h := strings.Replace(u, "wss://", "https://", 1)
	h = strings.Replace(h, "ws://", "http://", 1)
	return strings.TrimSuffix(h, "/api/agent/stream")
}

type event struct {
	Type    string `json:"type"`
	State   string `json:"state"`
	Text    string `json:"text"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Input   string `json:"input"`
	Output  string `json:"output"`
	Exit    int    `json:"exit"`
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

func main() {
	url := "ws://localhost:8888/api/agent/stream"
	if len(os.Args) > 1 {
		url = strings.TrimSuffix(os.Args[1], "/")
		if !strings.Contains(url, "/api/agent/stream") {
			url += "/api/agent/stream"
		}
	}

	// Parol yoqilgan bo'lsa token olamiz va WS manziliga qo'shamiz.
	if tok, err := resolveToken(httpFromWS(url)); err != nil {
		fmt.Fprintf(os.Stderr, "kirish xatosi: %v\n", err)
		os.Exit(1)
	} else if tok != "" {
		url += "?token=" + tok
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ulanib bo'lmadi (%s): %v\n", url, err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("ParkPulse agent — savol yozing (chiqish: Ctrl+D). Xavfli buyruqlar Y/N so'raydi.")

	var mu sync.Mutex
	pending := "" // kutilayotgan tasdiq id'si

	go func() {
		for {
			var ev event
			if err := conn.ReadJSON(&ev); err != nil {
				fmt.Println("\n[ulanish uzildi]")
				os.Exit(0)
			}
			switch ev.Type {
			case "assistant":
				fmt.Printf("\n\033[36m%s\033[0m\n", ev.Text)
			case "log":
				fmt.Printf("\033[90m· %s\033[0m\n", ev.Text)
			case "tool":
				if ev.State == "running" {
					fmt.Printf("\033[90m$ %s %s\033[0m\n", ev.Name, ev.Input)
				} else if ev.Output != "" {
					col := "\033[90m"
					if ev.Exit != 0 {
						col = "\033[31m"
					}
					fmt.Printf("%s%s\033[0m\n", col, strings.TrimRight(ev.Output, "\n"))
				}
			case "confirm":
				mu.Lock()
				pending = ev.ID
				mu.Unlock()
				fmt.Printf("\n\033[33m⚠️  Xavfli: %s\n   (%s)\n   Bajarilsinmi? [y/N]: \033[0m", ev.Command, ev.Reason)
			case "status":
				if ev.State == "error" && ev.Text != "" {
					fmt.Printf("\033[31m[xato] %s\033[0m\n", ev.Text)
				}
			}
		}
	}()

	sc := bufio.NewScanner(os.Stdin)
	fmt.Print("\n> ")
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		mu.Lock()
		id := pending
		mu.Unlock()
		if id != "" { // tasdiq javobi
			approve := line == "y" || line == "Y" || line == "yes" || line == "ha"
			conn.WriteJSON(map[string]any{"type": "decision", "id": id, "approve": approve})
			mu.Lock()
			pending = ""
			mu.Unlock()
		} else if line != "" {
			conn.WriteJSON(map[string]any{"type": "chat", "text": line})
		}
		fmt.Print("\n> ")
	}
}
