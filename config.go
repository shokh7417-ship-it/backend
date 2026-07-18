package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Config holds every runtime setting, sourced from environment variables
// (which .env populates on startup — see loadDotenv).
type Config struct {
	OpenAIKey  string
	ModelFast  string
	ModelSmart string
	OpenAIBase string

	Port       string
	CORSOrigin string

	DailyCallsPerUser int
	MaxConcurrency    int
	MonthlyUSDCap     float64

	DataDir     string
	FrontendDir string

	RolloverIntervalMin int
}

// loadDotenv reads KEY=VALUE lines from path into the process environment.
// Real environment variables always win, so it never clobbers an explicit setting.
func loadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

func loadConfig() Config {
	loadDotenv(".env")
	return Config{
		OpenAIKey:           os.Getenv("OPENAI_API_KEY"),
		ModelFast:           getenv("OPENAI_MODEL_FAST", "gpt-4o-mini"),
		ModelSmart:          getenv("OPENAI_MODEL_SMART", "gpt-4o"),
		OpenAIBase:          getenv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		Port:                getenv("PORT", "8080"),
		CORSOrigin:          getenv("CORS_ALLOW_ORIGIN", "*"),
		DailyCallsPerUser:   getenvInt("AI_DAILY_CALLS_PER_USER", 40),
		MaxConcurrency:      getenvInt("AI_MAX_CONCURRENCY", 4),
		MonthlyUSDCap:       getenvFloat("AI_MONTHLY_USD_CAP", 50),
		DataDir:             getenv("DATA_DIR", "./data"),
		FrontendDir:         getenv("FRONTEND_DIR", "../frontend"),
		RolloverIntervalMin: getenvInt("ROLLOVER_INTERVAL_MINUTES", 1440),
	}
}
