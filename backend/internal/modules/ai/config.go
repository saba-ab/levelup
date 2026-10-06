package ai

import "time"

// Config is ai's settings; the composition root embeds it with envPrefix
// "AI_". An empty AnthropicAPIKey disables drafting (503 ai_not_configured)
// while templates and usage keep working.
type Config struct {
	AnthropicAPIKey string `env:"ANTHROPIC_API_KEY"`
	// AnthropicBaseURL is the Messages API origin (tests and proxies).
	AnthropicBaseURL string        `env:"ANTHROPIC_BASE_URL" envDefault:"https://api.anthropic.com"`
	Model            string        `env:"MODEL" envDefault:"claude-sonnet-5-5"`
	MaxTokens        int           `env:"MAX_TOKENS" envDefault:"8192"`
	Effort           string        `env:"EFFORT" envDefault:"low"`
	RefusalFallback  bool          `env:"REFUSAL_FALLBACK" envDefault:"true"`
	Timeout          time.Duration `env:"TIMEOUT" envDefault:"60s"`
	MaxRetries       int           `env:"MAX_RETRIES" envDefault:"1"`
	// DailyLimit caps model requests per tenant per UTC day (0 = unlimited).
	DailyLimit int `env:"DAILY_LIMIT" envDefault:"200"`
}

const (
	minMaxTokens = 1024
	maxMaxTokens = 16000 // non-streaming request: keep well under HTTP timeouts
)

func (c Config) maxTokens() int {
	return min(max(c.MaxTokens, minMaxTokens), maxMaxTokens)
}
