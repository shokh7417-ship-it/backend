package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Gateway is the single point every model call flows through — no client ever
// talks to OpenAI directly. It absorbs the unpredictability of one shared API
// account: per-user quotas, a concurrency cap, response caching, retry with
// fallback, and cost metering. (See §11 "One API, many users" in the plan.)
type Gateway struct {
	cfg  Config
	http *http.Client

	sem chan struct{} // concurrency cap

	mu       sync.Mutex
	dayKey   string
	dailyUse map[string]int // userID -> calls today

	cacheMu sync.RWMutex
	cache   map[string]string

	meterMu    sync.Mutex
	tokensIn   int
	tokensOut  int
	costUSD    float64
	callsTotal int
	callsCache int
}

func newGateway(cfg Config) *Gateway {
	conc := cfg.MaxConcurrency
	if conc < 1 {
		conc = 1
	}
	return &Gateway{
		cfg:      cfg,
		http:     &http.Client{Timeout: 60 * time.Second},
		sem:      make(chan struct{}, conc),
		dailyUse: map[string]int{},
		cache:    map[string]string{},
	}
}

// Enabled reports whether a real API key is configured. When false the pipeline
// runs in mock mode and never calls this gateway's network path.
func (g *Gateway) Enabled() bool { return g.cfg.OpenAIKey != "" }

// rough per-1K-token USD rates, used only for the demo cost meter.
func modelRates(model string) (in, out float64) {
	switch model {
	case "gpt-4o":
		return 0.0025, 0.01
	default: // gpt-4o-mini and unknowns
		return 0.00015, 0.0006
	}
}

type oaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaReq struct {
	Model          string          `json:"model"`
	Messages       []oaMsg         `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *oaRespFormat   `json:"response_format,omitempty"`
}

type oaRespFormat struct {
	Type string `json:"type"`
}

type oaResp struct {
	Choices []struct {
		Message oaMsg `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat runs one completion through all the gateway protections and returns the
// assistant text. cacheKey (when non-empty) enables caching for repeatable calls.
func (g *Gateway) Chat(ctx context.Context, userID, model, system, user string, jsonMode bool, cacheKey string) (string, error) {
	if !g.Enabled() {
		return "", errors.New("gateway disabled (no OPENAI_API_KEY)")
	}

	// 1) cache — the most-repeated calls (skill overviews, question templates)
	//    become zero-cost, zero-latency hits.
	if cacheKey != "" {
		g.cacheMu.RLock()
		if v, ok := g.cache[cacheKey]; ok {
			g.cacheMu.RUnlock()
			g.meterMu.Lock()
			g.callsCache++
			g.meterMu.Unlock()
			return v, nil
		}
		g.cacheMu.RUnlock()
	}

	// 2) per-user daily quota — one heavy user can never starve the rest.
	if err := g.checkQuota(userID); err != nil {
		return "", err
	}

	// 3) hard monthly spend cap.
	g.meterMu.Lock()
	over := g.cfg.MonthlyUSDCap > 0 && g.costUSD >= g.cfg.MonthlyUSDCap
	g.meterMu.Unlock()
	if over {
		return "", errors.New("monthly AI spend cap reached")
	}

	// 4) concurrency cap (backpressure).
	select {
	case g.sem <- struct{}{}:
		defer func() { <-g.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}

	// 5) call with retry + fallback to the fast model on repeated failure.
	models := []string{model}
	if model != g.cfg.ModelFast {
		models = append(models, g.cfg.ModelFast)
	}
	var lastErr error
	for _, m := range models {
		for attempt := 0; attempt < 3; attempt++ {
			out, retryable, err := g.callOnce(ctx, m, system, user, jsonMode)
			if err == nil {
				if cacheKey != "" {
					g.cacheMu.Lock()
					g.cache[cacheKey] = out
					g.cacheMu.Unlock()
				}
				return out, nil
			}
			lastErr = err
			if !retryable {
				break
			}
			select {
			case <-time.After(time.Duration(400*(attempt+1)) * time.Millisecond):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	return "", fmt.Errorf("ai call failed: %w", lastErr)
}

func (g *Gateway) checkQuota(userID string) error {
	if g.cfg.DailyCallsPerUser <= 0 {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	key := time.Now().Format("2006-01-02")
	if g.dayKey != key {
		g.dayKey = key
		g.dailyUse = map[string]int{}
	}
	if g.dailyUse[userID] >= g.cfg.DailyCallsPerUser {
		return fmt.Errorf("daily AI quota reached (%d calls)", g.cfg.DailyCallsPerUser)
	}
	g.dailyUse[userID]++
	return nil
}

func (g *Gateway) callOnce(ctx context.Context, model, system, user string, jsonMode bool) (string, bool, error) {
	reqBody := oaReq{
		Model:       model,
		Temperature: 0.4,
		Messages: []oaMsg{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}
	if jsonMode {
		reqBody.ResponseFormat = &oaRespFormat{Type: "json_object"}
	}
	buf, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.OpenAIBase+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.cfg.OpenAIKey)

	resp, err := g.http.Do(req)
	if err != nil {
		return "", true, err // network error — retryable
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", true, fmt.Errorf("openai status %d: %s", resp.StatusCode, string(body))
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("openai status %d: %s", resp.StatusCode, string(body))
	}

	var parsed oaResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", false, err
	}
	if parsed.Error != nil {
		return "", false, errors.New(parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", true, errors.New("no choices returned")
	}

	g.meter(model, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens)
	return parsed.Choices[0].Message.Content, false, nil
}

func (g *Gateway) meter(model string, in, out int) {
	inRate, outRate := modelRates(model)
	g.meterMu.Lock()
	defer g.meterMu.Unlock()
	g.tokensIn += in
	g.tokensOut += out
	g.costUSD += float64(in)/1000*inRate + float64(out)/1000*outRate
	g.callsTotal++
}

// MeterSnapshot is the read model behind GET /api/meter.
type MeterSnapshot struct {
	Enabled     bool    `json:"enabled"`
	CallsTotal  int     `json:"callsTotal"`
	CallsCached int     `json:"callsCached"`
	TokensIn    int     `json:"tokensIn"`
	TokensOut   int     `json:"tokensOut"`
	CostUSD     float64 `json:"costUsd"`
	MonthlyCap  float64 `json:"monthlyCapUsd"`
}

func (g *Gateway) Snapshot() MeterSnapshot {
	g.meterMu.Lock()
	defer g.meterMu.Unlock()
	return MeterSnapshot{
		Enabled:     g.Enabled(),
		CallsTotal:  g.callsTotal,
		CallsCached: g.callsCache,
		TokensIn:    g.tokensIn,
		TokensOut:   g.tokensOut,
		CostUSD:     g.costUSD,
		MonthlyCap:  g.cfg.MonthlyUSDCap,
	}
}
