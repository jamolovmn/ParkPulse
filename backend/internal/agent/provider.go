package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
)

// Models provayderdagi mavjud modellar ro'yxatini oladi (UI'da avto tanlash uchun).
func (m *Manager) Models(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	p, key, _, base := m.resolved()
	m.mu.RUnlock()

	url := base + "/models"
	headers := map[string]string{}
	if p == ProviderAnthropic {
		url = base + "/v1/models"
		headers["x-api-key"] = key
		headers["anthropic-version"] = "2023-06-01"
	} else if key != "" {
		headers["Authorization"] = "Bearer " + key
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var r struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(r.Data))
	for _, d := range r.Data {
		if d.ID != "" {
			ids = append(ids, d.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// provider.go — LLM provayder abstraksiyasi. Suhbat neytral `turn` ro'yxatida
// saqlanadi; har provayder uni o'z simiga (OpenAI chat/completions yoki Anthropic
// messages) o'giradi. tool_use ikkala tomonda ham qo'llab-quvvatlanadi.

const maxTokens = 2048

// turn — provayderdan mustaqil suhbat qadami.
type turn struct {
	role       string // "user" | "assistant" | "tool"
	text       string
	toolCalls  []toolCall // assistant qadamida
	toolCallID string     // tool natijasi qadamida
}

type toolCall struct {
	id   string
	name string
	args map[string]any
}

// complete bitta LLM chaqiruvi: tizim promti + tarix + tool'lar -> matn va tool chaqiruvlari.
func (m *Manager) complete(ctx context.Context, sys string, hist []turn, tools []*Tool) (string, []toolCall, error) {
	m.mu.RLock()
	p, key, model, base := m.resolved()
	m.mu.RUnlock()
	if p == ProviderAnthropic {
		return m.callAnthropic(ctx, key, model, base, sys, hist, tools)
	}
	return m.callOpenAI(ctx, key, model, base, sys, hist, tools)
}

// --- OpenAI-mos (OpenAI / OpenRouter / local) ---

func (m *Manager) callOpenAI(ctx context.Context, key, model, base, sys string, hist []turn, tools []*Tool) (string, []toolCall, error) {
	msgs := []map[string]any{{"role": "system", "content": sys}}
	for _, t := range hist {
		switch t.role {
		case "assistant":
			mm := map[string]any{"role": "assistant", "content": t.text}
			if len(t.toolCalls) > 0 {
				var tc []map[string]any
				for _, c := range t.toolCalls {
					args, _ := json.Marshal(c.args)
					tc = append(tc, map[string]any{
						"id": c.id, "type": "function",
						"function": map[string]any{"name": c.name, "arguments": string(args)},
					})
				}
				mm["tool_calls"] = tc
			}
			msgs = append(msgs, mm)
		case "tool":
			msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": t.toolCallID, "content": t.text})
		default:
			msgs = append(msgs, map[string]any{"role": "user", "content": t.text})
		}
	}

	var toolDefs []map[string]any
	for _, t := range tools {
		toolDefs = append(toolDefs, map[string]any{
			"type":     "function",
			"function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Schema},
		})
	}

	body := map[string]any{"model": model, "max_tokens": maxTokens, "messages": msgs}
	if len(toolDefs) > 0 {
		body["tools"] = toolDefs
		body["tool_choice"] = "auto"
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := m.post(ctx, base+"/chat/completions", openAIHeaders(key), body, &resp); err != nil {
		return "", nil, err
	}
	if resp.Error != nil {
		return "", nil, fmt.Errorf("provayder: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return "", nil, fmt.Errorf("javob bo'sh")
	}
	msg := resp.Choices[0].Message
	var calls []toolCall
	for _, c := range msg.ToolCalls {
		var args map[string]any
		json.Unmarshal([]byte(c.Function.Arguments), &args)
		calls = append(calls, toolCall{id: c.ID, name: c.Function.Name, args: args})
	}
	return msg.Content, calls, nil
}

func openAIHeaders(key string) map[string]string {
	h := map[string]string{"Content-Type": "application/json"}
	if key != "" {
		h["Authorization"] = "Bearer " + key
	}
	return h
}

// --- Anthropic (Messages API) ---

func (m *Manager) callAnthropic(ctx context.Context, key, model, base, sys string, hist []turn, tools []*Tool) (string, []toolCall, error) {
	var msgs []map[string]any
	var pendingResults []map[string]any // ketma-ket tool natijalarini bitta user xabariga yig'amiz

	flush := func() {
		if len(pendingResults) > 0 {
			msgs = append(msgs, map[string]any{"role": "user", "content": pendingResults})
			pendingResults = nil
		}
	}
	for _, t := range hist {
		switch t.role {
		case "assistant":
			flush()
			var content []map[string]any
			if t.text != "" {
				content = append(content, map[string]any{"type": "text", "text": t.text})
			}
			for _, c := range t.toolCalls {
				content = append(content, map[string]any{"type": "tool_use", "id": c.id, "name": c.name, "input": c.args})
			}
			msgs = append(msgs, map[string]any{"role": "assistant", "content": content})
		case "tool":
			pendingResults = append(pendingResults, map[string]any{
				"type": "tool_result", "tool_use_id": t.toolCallID, "content": t.text,
			})
		default:
			flush()
			msgs = append(msgs, map[string]any{"role": "user", "content": t.text})
		}
	}
	flush()

	var toolDefs []map[string]any
	for _, t := range tools {
		toolDefs = append(toolDefs, map[string]any{"name": t.Name, "description": t.Description, "input_schema": t.Schema})
	}

	body := map[string]any{"model": model, "max_tokens": maxTokens, "system": sys, "messages": msgs}
	if len(toolDefs) > 0 {
		body["tools"] = toolDefs
	}

	var resp struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	headers := map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01", "Content-Type": "application/json"}
	if err := m.post(ctx, base+"/v1/messages", headers, body, &resp); err != nil {
		return "", nil, err
	}
	if resp.Error != nil {
		return "", nil, fmt.Errorf("provayder: %s", resp.Error.Message)
	}
	var text string
	var calls []toolCall
	for _, c := range resp.Content {
		switch c.Type {
		case "text":
			text += c.Text
		case "tool_use":
			var args map[string]any
			json.Unmarshal(c.Input, &args)
			calls = append(calls, toolCall{id: c.ID, name: c.Name, args: args})
		}
	}
	return text, calls, nil
}

// post JSON so'rov yuboradi va javobni out'ga o'qiydi.
func (m *Manager) post(ctx context.Context, url string, headers map[string]string, body any, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("kalit rad etildi (%d)", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
