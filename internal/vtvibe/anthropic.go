package vtvibe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

// Claude through the Anthropic Messages API with the official Go SDK
// (unxed/f4#1842, docs/VTVIBE.md § 6.3.1). Every other preset speaks the
// OpenAI chat-completions dialect; Claude is reached natively instead of
// through a compatibility shim.

// KindAnthropic marks a Config whose BaseURL speaks the Messages API.
const KindAnthropic = "anthropic"

// anthropicMaxTokens is the answer cap of one non-streaming request: large
// enough for whole files, small enough to stay under the HTTP timeouts.
const anthropicMaxTokens = 16000

// anthropicFallbackModels are the models for which a request asks the API to
// re-serve a refused answer with a fallback model ("fallbacks": "default").
var anthropicFallbackModels = map[string]bool{
	"claude-fable-5-1":  true,
	"claude-opus-5-5":   true,
	"claude-opus-5":     true,
	"claude-sonnet-5-5": true,
}

func (c Config) anthropicClient() anthropic.Client {
	opts := []option.RequestOption{option.WithAPIKey(c.APIKey), option.WithHTTPClient(httpClient)}
	if c.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(c.BaseURL))
	}
	return anthropic.NewClient(opts...)
}

// anthropicParams turns the chat-completions shaped history into a Messages
// API request: system messages become the system prompt, the rest alternate
// between user and assistant.
func (c Config) anthropicParams(msgs []Message) anthropic.BetaMessageNewParams {
	params := anthropic.BetaMessageNewParams{Model: anthropic.Model(c.Model), MaxTokens: anthropicMaxTokens}
	for _, m := range msgs {
		switch m.Role {
		case "system":
			params.System = append(params.System, anthropic.BetaTextBlockParam{Text: m.Content})
		case "assistant", "model":
			params.Messages = append(params.Messages, anthropic.BetaMessageParam{
				Role:    anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(m.Content)},
			})
		default:
			params.Messages = append(params.Messages, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(m.Content)))
		}
	}
	if anthropicFallbackModels[c.Model] {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()}
	}
	return params
}

func (c Config) chatAnthropic(ctx context.Context, msgs []Message) (string, Usage, error) {
	if c.APIKey == "" && !isLocal(c.BaseURL) {
		return "", Usage{}, ErrNoKey
	}
	client := c.anthropicClient()
	reply, err := client.Beta.Messages.New(ctx, c.anthropicParams(msgs))
	if err != nil {
		return "", Usage{}, err
	}
	usage := Usage{In: int(reply.Usage.InputTokens), Out: int(reply.Usage.OutputTokens)}
	if reply.StopReason == anthropic.BetaStopReasonRefusal {
		why := strings.TrimSpace(reply.StopDetails.Explanation)
		if why == "" {
			why = string(reply.StopDetails.Category)
		}
		return "", usage, fmt.Errorf("the model declined to answer: %s", why)
	}
	var sb strings.Builder
	for _, block := range reply.Content {
		if text, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			sb.WriteString(text.Text)
		}
	}
	if strings.TrimSpace(sb.String()) == "" {
		return "", usage, errors.New("the model returned an empty answer")
	}
	return sb.String(), usage, nil
}

func (c Config) modelsAnthropic(ctx context.Context) ([]string, error) {
	if c.APIKey == "" && !isLocal(c.BaseURL) {
		return nil, ErrNoKey
	}
	client := c.anthropicClient()
	pager := client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	var out []string
	for pager.Next() {
		out = append(out, pager.Current().ID)
	}
	return out, pager.Err()
}
