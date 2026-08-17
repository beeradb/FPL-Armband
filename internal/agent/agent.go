package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// Agent runs a Claude conversation over the FPL toolset. Each command issues a
// single Ask; history is kept because the tool loop inside one Ask is itself a
// conversation, not because anything asks a follow-up.
type Agent struct {
	client  anthropic.Client
	toolbox *Toolbox
	tools   []anthropic.BetaTool
	system  string
	cfg     config.Config

	history []anthropic.BetaMessageParam

	// LastUsage reports the token spend of the most recent Ask.
	LastUsage Usage
}

// New builds an agent. The Anthropic client resolves credentials from the
// environment (ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN, or an `ant auth login`
// profile), so no key is passed here.
func New(cfg config.Config, configPath string, client *fpl.Client, engine *analysis.Engine, onCall func(name, summary string)) (*Agent, error) {
	tb := &Toolbox{Client: client, Engine: engine, Cfg: cfg, ConfigPath: configPath, OnCall: onCall}
	tools, err := tb.Tools()
	if err != nil {
		return nil, fmt.Errorf("building tools: %w", err)
	}
	return &Agent{
		client:  anthropic.NewClient(),
		toolbox: tb,
		tools:   tools,
		system:  SystemPrompt(cfg, engine.Boot),
		cfg:     cfg,
	}, nil
}

// Ask sends a prompt, runs the tool loop to completion, and returns the reply.
func (a *Agent) Ask(ctx context.Context, prompt string) (string, error) {
	a.history = append(a.history, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(prompt)))

	params := anthropic.BetaToolRunnerParams{
		BetaMessageNewParams: anthropic.BetaMessageNewParams{
			Model:     anthropic.Model(a.cfg.Model),
			MaxTokens: 16000,
			System: []anthropic.BetaTextBlockParam{{
				Text: a.system,
				// Cache the system prompt and tool definitions: they are
				// byte-identical across every turn and every run.
				CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
			}},
			OutputConfig: anthropic.BetaOutputConfigParam{
				Effort: anthropic.BetaOutputConfigEffort(a.cfg.Effort),
			},
			// Second breakpoint on the conversation tail. Tool results are bulky
			// and the runner replays the whole history every iteration, so each
			// round trip reads what the previous one wrote.
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
			// Clear stale tool results once the conversation grows. In a long
			// tool-calling loop the full JSON of every earlier search is
			// replayed on every request; after the model has read it once it is
			// dead weight. Keeping the most recent few preserves working
			// context while dropping the bulk.
			ContextManagement: anthropic.BetaContextManagementConfigParam{
				Edits: []anthropic.BetaContextManagementConfigEditUnionParam{{
					OfClearToolUses20250919: &anthropic.BetaClearToolUses20250919EditParam{
						Keep: anthropic.BetaToolUsesKeepParam{Value: 4},
					},
				}},
			},
			Betas:    []anthropic.AnthropicBeta{"context-management-2025-06-27"},
			Messages: a.history,
		},
		MaxIterations: a.cfg.MaxIterations,
	}

	runner := a.client.Beta.Messages.NewToolRunner(a.tools, params)

	// The runner overwrites Params.Tools with the local tools, so server-side
	// tools have to be appended afterwards. Web search is what lets the agent
	// read actual team news and transfer rumours — FPL's own news field is
	// terse and lags press conferences by days.
	//
	// Deliberately the 20250305 tool, not the newer 20260209. The newer one does
	// dynamic filtering, which it implements by running code execution
	// server-side, so its responses carry code_execution_tool_result blocks. The
	// SDK's BetaMessage.ToParam() drops the discriminator inside those blocks'
	// content, and the tool runner replays every assistant turn through ToParam
	// on the next iteration, so the following request is rejected:
	//
	//	400 code_execution_tool_result.content.RequestCodeExecutionToolResultError.type:
	//	    Input should be 'code_execution_tool_result_error'
	//
	// That makes any search followed by another turn fail — which is every search
	// in an agentic loop. The 20250305 tool emits only server_tool_use and
	// web_search_tool_result, both of which round-trip cleanly. Verified against
	// the live API; re-test before upgrading.
	runner.Params.Tools = append(runner.Params.Tools, anthropic.BetaToolUnionParam{
		OfWebSearchTool20250305: &anthropic.BetaWebSearchTool20250305Param{
			MaxUses: anthropic.Int(8),
		},
	})

	// Iterate rather than RunToCompletion so per-request usage can be summed:
	// the API reports usage per call, and an agentic loop makes one call per
	// iteration, so the run total is the sum, not the final message.
	a.LastUsage = Usage{Model: a.cfg.Model}
	var msg *anthropic.BetaMessage
	for m, err := range runner.All(ctx) {
		if err != nil {
			return "", err
		}
		a.LastUsage.Add(m)
		msg = m
	}
	if msg == nil {
		return "", fmt.Errorf("the model returned no messages")
	}

	// A server-side tool loop can hit its iteration cap and return
	// stop_reason "pause_turn". The runner does not resume automatically —
	// it only continues after a local tool produces a result — so a paused
	// turn would otherwise surface as a silently truncated answer.
	for restarts := 0; msg.StopReason == anthropic.BetaStopReasonPauseTurn && restarts < 4; restarts++ {
		resumed := a.client.Beta.Messages.NewToolRunner(a.tools, anthropic.BetaToolRunnerParams{
			BetaMessageNewParams: params.BetaMessageNewParams,
			MaxIterations:        a.cfg.MaxIterations,
		})
		resumed.Params.Messages = append(runner.Params.Messages, msg.ToParam())
		resumed.Params.Tools = runner.Params.Tools
		for m, err := range resumed.All(ctx) {
			if err != nil {
				return "", err
			}
			a.LastUsage.Add(m)
			msg = m
		}
		runner = resumed
	}

	// The runner accumulates the whole exchange — assistant turns, tool_use and
	// tool_result blocks, and the final message — onto Params.Messages. Adopt it
	// wholesale so follow-up questions keep their context and hit the prompt cache.
	a.history = runner.Params.Messages

	if msg.StopReason == anthropic.BetaStopReasonRefusal {
		return "", fmt.Errorf("the model declined this request (%s)", msg.StopReason)
	}

	var out strings.Builder
	titleByURL := map[string]string{}
	var urls []string
	for _, block := range msg.Content {
		b, ok := block.AsAny().(anthropic.BetaTextBlock)
		if !ok {
			continue
		}
		out.WriteString(b.Text)
		for _, c := range b.Citations {
			if c.URL == "" {
				continue
			}
			if _, dup := titleByURL[c.URL]; dup {
				continue
			}
			title := strings.TrimSpace(c.Title)
			if title == "" {
				title = c.URL
			}
			titleByURL[c.URL] = title
			urls = append(urls, c.URL)
		}
	}

	// When web search is in play the model wraps sourced claims in <cite
	// index="..."> tags. Those indices refer to internal search-result ordering
	// and mean nothing to a reader, so they were landing verbatim in the
	// terminal and the Markdown report. Strip the tags, keep the prose, and put
	// the real URLs — which the citation metadata carries — at the end.
	text := strings.TrimSpace(citeTag.ReplaceAllString(out.String(), ""))
	if text == "" {
		return "", fmt.Errorf("the model returned no text (stop reason: %s)", msg.StopReason)
	}
	if len(urls) > 0 {
		var b strings.Builder
		b.WriteString(text)
		b.WriteString("\n\n## Sources\n\n")
		for i, u := range urls {
			fmt.Fprintf(&b, "%d. [%s](%s)\n", i+1, titleByURL[u], u)
		}
		text = b.String()
	}
	return text, nil
}

// citeTag matches the <cite index="9-5"> / </cite> markup the model emits around
// claims backed by a web search.
var citeTag = regexp.MustCompile(`(?i)</?cite\b[^>]*>`)

// Reset clears conversation history, keeping tools and system prompt.
func (a *Agent) Reset() { a.history = nil }
