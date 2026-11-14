package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// The Pi executor runs the pi CLI (@earendil-works/pi-coding-agent) as a reviewer. pi has no
// --json-schema equivalent, so the answer rides the prompt as a JSON output contract — codex's
// trick — and is extracted from the assistant text pi accumulated over its turns.
//
// Liveness needs no third channel: pi's --mode json stream writes token deltas and tool events to
// stdout as they happen, so the tee's touch and the sink's activity are the same bytes.

// piRetryPatterns mark transient provider failures worth one retry.
var piRetryPatterns = []string{
	"api error: 529", "api error: 502", "api error: 503", "api error: 504",
	"overloaded", "connection reset", "temporary failure",
}

// piLimitPatterns mark quota exhaustion the caller must wait out.
var piLimitPatterns = []string{
	"rate limit exceeded", "rate limit reached", "429 too many requests",
	"usage limit reached", "quota exceeded", "monthly limit exceeded",
}

// Pi runs the pi CLI. Its stdout is a JSONL event stream; the parser below decodes only what the
// pipeline acts on and skips every other line, so a pi release that adds event shapes degrades
// nothing.
type Pi struct {
	proc
}

// NewPi builds a pi executor. A nil Opts.Clock is filled with the production clock, because the
// composition root assembles Opts from flags that carry no clock at all.
func NewPi(runner CommandRunner, opts Opts) *Pi {
	return &Pi{proc: newProc("pi", runner, opts)}
}

// PiOutputContract is pi's substitute for claude's --json-schema, appended to every prompt Run
// dispatches. The wording matches CodexOutputContract on purpose: one contract, two prose-first
// executors, so a prompt archived for either reads the same.
func PiOutputContract(schema json.RawMessage) string {
	if len(schema) == 0 {
		return ""
	}
	return "\n\nReturn ONLY a JSON object matching the schema below. No prose before or after it.\n\nSchema:\n" +
		string(schema) + "\n"
}

// Run executes one request. A non-zero exit or an idle timeout comes back on the Result rather
// than as an error, and output holding no JSON degrades the source instead of failing the run.
func (p *Pi) Run(ctx context.Context, req Request, sink EventSink) (Result, error) {
	req.Prompt += PiOutputContract(req.Schema)
	acc := &piAccumulator{}

	spec := runSpec{
		argv:       p.args(req),
		sink:       sink,
		parse:      func(ctx context.Context, r io.Reader) Result { return p.parseStream(ctx, r, acc, sink) },
		stderrLine: acc.stderrLine,
	}

	res, err := p.run(ctx, req, spec)
	res.RequestedModel = req.Model
	res.ActualModel = acc.model
	res.Tokens = acc.tokens
	if err != nil {
		return res, err
	}
	if out, exErr := firstJSONObject(acc.text.String()); exErr == nil {
		res.StructuredOutput = out
	}
	return p.classify(res, acc.diag, sink)
}

// args builds the invocation. pi reads the prompt from stdin when --print carries no positional
// message, which is how proc.start delivers it. Effort maps straight onto pi's --thinking levels:
// the vocabularies coincide on low, medium, high, xhigh and max.
func (p *Pi) args(req Request) []string {
	argv := []string{"--mode", "json", "--print"}
	if req.Model != "" {
		argv = append(argv, "--model", req.Model)
	}
	if req.Effort != "" {
		argv = append(argv, "--thinking", req.Effort)
	}
	return argv
}

// piEvent is one line of pi's --mode json stream. Only the fields revmux acts on are decoded.
type piEvent struct {
	Type string `json:"type"`
	// AssistantMessageEvent carries token-level text deltas while a message streams.
	AssistantMessageEvent *struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
	} `json:"assistantMessageEvent"`
	// Message is the finalized message on message_end.
	Message *struct {
		Role    string `json:"role"`
		Model   any    `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage *struct {
			Input  int `json:"input"`
			Output int `json:"output"`
		} `json:"usage"`
	} `json:"message"`
	ToolName string         `json:"toolName"`
	Args     map[string]any `json:"args"`
}

// piAccumulator is one run's per-stream state: the assistant text the answer is extracted from,
// the token and model totals, and the last stderr diagnostic for classify. proc is shared across
// concurrent roster entries, so none of this may live on the executor.
type piAccumulator struct {
	mu     sync.Mutex
	text   strings.Builder
	tokens int
	model  string
	diag   string
	// line buffers token deltas until a newline, so activity is one event per line of prose
	// rather than one per token.
	line strings.Builder
}

// piToolArgKeys are the Args fields worth showing beside a tool's name, most specific first —
// pi's read/grep/find speak path and pattern, bash speaks command.
var piToolArgKeys = []string{"path", "pattern", "command", "query", "url", "description"}

func (a *piAccumulator) stderrLine(l string) {
	l = strings.TrimSpace(l)
	if l == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.diag = l
}

// flush emits the buffered activity line. Called with a.mu held.
func (a *piAccumulator) flush(sink EventSink) {
	s := strings.TrimRight(a.line.String(), "\n")
	a.line.Reset()
	if s == "" {
		return
	}
	if sink != nil {
		sink.Emit(Event{Kind: EventActivity, Text: clampRunes(s)})
	}
}

// parseStream consumes pi's JSONL and returns what it learned. A line that fails to decode is
// skipped: pi's stream carries many event shapes revmux does not read, and a truncated stream
// must degrade to a partial Result rather than fail the run.
func (p *Pi) parseStream(ctx context.Context, r io.Reader, acc *piAccumulator, sink EventSink) Result {
	res := Result{}
	_ = p.readLines(ctx, r, func(line string) {
		var ev piEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return
		}
		acc.mu.Lock()
		defer acc.mu.Unlock()
		switch ev.Type {
		case "message_update":
			if ev.AssistantMessageEvent == nil || ev.AssistantMessageEvent.Type != "text_delta" {
				return
			}
			delta := ev.AssistantMessageEvent.Delta
			if delta == "" {
				return
			}
			a := acc.line
			a.WriteString(delta)
			if strings.ContainsRune(delta, '\n') || a.Len() > 240 {
				acc.flush(sink)
			}
		case "tool_execution_start":
			// a tool call is liveness the stagger gate can latch on even when the agent's
			// first act is reading files rather than saying anything
			note := ev.ToolName
			if arg := piToolArg(ev.Args); arg != "" {
				note += " " + arg
			}
			acc.flush(sink)
			if sink != nil && note != "" {
				sink.Emit(Event{Kind: EventProgress, Text: clampRunes(note)})
			}
		case "message_end":
			if ev.Message == nil || ev.Message.Role != "assistant" {
				return
			}
			for _, block := range ev.Message.Content {
				if block.Type == "text" {
					acc.text.WriteString(block.Text)
				}
			}
			if ev.Message.Usage != nil {
				acc.tokens += ev.Message.Usage.Input + ev.Message.Usage.Output
			}
			if acc.model == "" {
				acc.model = piModelName(ev.Message.Model)
			}
		case "turn_end", "agent_end":
			// a turn boundary closes any buffered line, so a short final answer without a
			// trailing newline still reaches the sink
			acc.flush(sink)
		}
	})
	return res
}

// piToolArg picks the one Args value worth showing beside the tool's name.
func piToolArg(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	for _, key := range piToolArgKeys {
		val, ok := args[key].(string)
		if !ok || strings.TrimSpace(val) == "" {
			continue
		}
		return flattenLines(val)
	}
	return ""
}

// piModelName flattens pi's model field, which may arrive as an id string or a provider/id pair.
func piModelName(v any) string {
	switch m := v.(type) {
	case string:
		return m
	case []any:
		parts := make([]string, 0, len(m))
		for _, p := range m {
			if s, ok := p.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "/")
	default:
		return ""
	}
}

// classify tiers a failed run after codex's semantics: patterns are consulted only on a non-zero
// exit and only against the tail plus the one stderr diagnostic line, and an idle timeout never
// becomes an error, since the caller retries that on its own.
func (p *Pi) classify(res Result, diag string, sink EventSink) (Result, error) {
	if res.ExitCode != 0 {
		text := diag
		if pat := p.match(text, piRetryPatterns); pat != "" && !res.IdleTimedOut {
			return res, fmt.Errorf("pi transient failure: %s", pat)
		}
		if pat := p.match(text, piLimitPatterns); pat != "" {
			res.RateLimited = true
			if sink != nil {
				p.emit(sink, Event{Kind: EventRateLimit, Text: pat})
			}
			return res, fmt.Errorf("pi rate limited: %s", pat)
		}
		if text != "" {
			return res, fmt.Errorf("pi failed: %s", clampRunes(text))
		}
		return res, fmt.Errorf("pi failed: exit %d", res.ExitCode)
	}
	return res, nil
}

// firstJSONObject pulls the answer out of text that may carry prose around it. Shared by the two
// prose-first executors; decoding starts at each brace in turn, and an incomplete tail ends the
// search rather than continuing into it: a nested object inside a truncated answer would otherwise
// come back looking like the whole answer.
func firstJSONObject(raw string) (json.RawMessage, error) {
	for i, ch := range raw {
		if ch != '{' {
			continue
		}
		var out json.RawMessage
		err := json.NewDecoder(strings.NewReader(raw[i:])).Decode(&out)
		if err == nil {
			return out, nil
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
	}
	return nil, errors.New("no JSON object in output")
}
