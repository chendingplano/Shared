package llm

// jev_emulated.go turns any OpenAI-compatible chat model (DeepSeek, OpenAI,
// llama.cpp, vLLM, ...) into a Jev-style decision model.
//
// Jev (https://docs.typesafe.ai/api) evaluates typed questions (noul, choice,
// score) against a state and returns calibrated probabilities instead of text.
// A conventional LLM can approximate this by reading its token probabilities
// (technique from https://allanrbo.blogspot.com/2026/09/a-jev-like-wrapper-for-llms-including.html):
// each question is rendered as a lettered multiple-choice prompt, the model is
// asked for one completion token with logprobs, and the logprobs of the option
// letters are normalized into a distribution over the options.
//
// Questions are evaluated in parallel and in isolation, as Jev does; the state
// leads every prompt so backends with prefix caching reuse it across questions.
// The answers returned in Response.Content use Jev's wire shape, so callers can
// switch between ProviderJevCompatible and ProviderJevEmulated by config alone
// and decode either with ParseJevAnswers.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const (
	jevEmulatedLetters              = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	jevEmulatedDefaultTopLogprobs   = 20
	jevEmulatedDefaultConcurrency   = 4
	jevEmulatedNegligibleMissProb   = 1e-6
	jevEmulatedUnusableLogprobFloor = -9999
)

// JevAnswer is one typed answer in Jev's response shape. Noul answers set
// Noul; Choice answers set Choice, Probabilities and Confidence; Score answers
// set Score, Legend, Probabilities and Confidence.
type JevAnswer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// ParseJevAnswers decodes Response.Content from either Jev client into typed
// answers keyed by question ID.
func ParseJevAnswers(content string) (map[string]JevAnswer, error) {
	var answers map[string]JevAnswer
	if err := json.Unmarshal([]byte(content), &answers); err != nil {
		return nil, fmt.Errorf("jev: decode answers: %w", err)
	}
	return answers, nil
}

// jevOption is one lettered option shown to the model.
type jevOption struct {
	key         string // answer key: choice option, "true"/"false", or level index
	label       string // text shown to the model
	description string
}

// jevEmulatedClient implements Client for ProviderJevEmulated. Extra knobs:
//
//	top_logprobs      alternatives requested per call (default 20; llama.cpp allows more)
//	concurrency       questions evaluated at once (default 4)
//	temperature       sent as-is (default "0"); "omit" leaves it out
//	reasoning_effort  sent as-is when set, e.g. "none" for llama.cpp thinking models
type jevEmulatedClient struct {
	cfg         ProviderConfig
	endpoint    string
	topLogprobs int
	concurrency int
	temperature *float64
	reasoning   string
	logger      ApiTypes.JimoLogger
}

func newJevEmulatedClient(cfg ProviderConfig, logger ApiTypes.JimoLogger) (*jevEmulatedClient, error) {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("20261005-731")
	}
	c := &jevEmulatedClient{
		cfg:         cfg,
		endpoint:    buildChatCompletionsEndpoint(cfg.BaseURL),
		topLogprobs: jevEmulatedDefaultTopLogprobs,
		concurrency: jevEmulatedDefaultConcurrency,
		reasoning:   strings.TrimSpace(cfg.Extra["reasoning_effort"]),
		logger:      logger,
	}
	for name, dst := range map[string]*int{"top_logprobs": &c.topLogprobs, "concurrency": &c.concurrency} {
		if v := strings.TrimSpace(cfg.Extra[name]); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("jev_emulated: Extra[%q] must be a positive integer, got %q", name, v)
			}
			*dst = n
		}
	}
	switch v := strings.TrimSpace(cfg.Extra["temperature"]); v {
	case "omit":
	case "":
		zero := 0.0
		c.temperature = &zero
	default:
		t, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("jev_emulated: Extra[\"temperature\"] must be a number or \"omit\", got %q", v)
		}
		c.temperature = &t
	}
	return c, nil
}

func (c *jevEmulatedClient) Stream(ctx context.Context, req Request, on StreamHandler) error {
	return fmt.Errorf("jev_emulated: streaming is not supported")
}

// Complete evaluates every question in req.JevQuestions against the state in
// the last user message. Text parts of a multipart message form the state and
// image parts are attached to every question (our extension; Jev is text-only).
func (c *jevEmulatedClient) Complete(ctx context.Context, req Request) (*Response, error) {
	if len(req.JevQuestions) == 0 {
		return nil, fmt.Errorf("jev_emulated: at least one question is required")
	}
	state, images, err := jevEmulatedState(req.Messages)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(req.JevQuestions))
	options := make(map[string][]jevOption, len(req.JevQuestions))
	for id, q := range req.JevQuestions {
		opts, err := jevEmulatedOptions(q)
		if err != nil {
			return nil, fmt.Errorf("jev_emulated: question %q: %w", id, err)
		}
		options[id] = opts
		ids = append(ids, id)
	}
	sort.Strings(ids)

	type result struct {
		answer JevAnswer
		usage  *Usage
		err    error
	}
	results := make([]result, len(ids))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i].err = ctx.Err()
				return
			}
			defer func() { <-sem }()
			q := req.JevQuestions[id]
			probs, usage, err := c.ask(ctx, req, state, images, q.Instructions, options[id])
			if err != nil {
				results[i].err = fmt.Errorf("jev_emulated: question %q: %w", id, err)
				cancel()
				return
			}
			results[i] = result{answer: jevEmulatedAnswer(q.Type, options[id], probs), usage: usage}
		}(i, id)
	}
	wg.Wait()

	answers := make(map[string]JevAnswer, len(ids))
	total := &Usage{}
	var firstErr error
	for i, id := range ids {
		r := results[i]
		if r.err != nil {
			// Report the root cause, not the cancellations it triggered.
			if firstErr == nil || (errors.Is(firstErr, context.Canceled) && !errors.Is(r.err, context.Canceled)) {
				firstErr = r.err
			}
			continue
		}
		answers[id] = r.answer
		if r.usage != nil {
			total.InputTokens += r.usage.InputTokens
			total.OutputTokens += r.usage.OutputTokens
			total.TotalTokens += r.usage.TotalTokens
			total.PromptCacheHitTokens += r.usage.PromptCacheHitTokens
			total.PromptCacheMissTokens += r.usage.PromptCacheMissTokens
			if r.usage.EventID != "" {
				total.EventIDs = append(total.EventIDs, r.usage.EventID)
			}
		}
	}
	if firstErr != nil {
		c.logger.Error("jev_emulated: decision request failed", "model", req.Model, "questions", len(ids), "error", firstErr)
		return nil, firstErr
	}

	content, err := json.Marshal(answers)
	if err != nil {
		return nil, fmt.Errorf("jev_emulated: encode answers: %w", err)
	}
	raw, err := json.Marshal(map[string]any{
		"model":   req.Model,
		"answers": answers,
		"usage":   map[string]int{"input_tokens": total.InputTokens, "output_tokens": total.OutputTokens},
	})
	if err != nil {
		return nil, fmt.Errorf("jev_emulated: encode response: %w", err)
	}
	c.logger.Info("jev_emulated: decision request done", "model", req.Model, "questions", len(ids),
		"input_tokens", total.InputTokens, "output_tokens", total.OutputTokens)
	return &Response{Content: string(content), Raw: raw, Usage: total}, nil
}

// jevEmulatedState extracts the state text and images from the last user message.
func jevEmulatedState(messages []Message) (string, []string, error) {
	i := lastUserMessageIndex(messages)
	if i < 0 {
		return "", nil, fmt.Errorf("jev_emulated: nonempty user state is required")
	}
	msg := messages[i]
	texts := []string{}
	if strings.TrimSpace(msg.Content) != "" {
		texts = append(texts, msg.Content)
	}
	var images []string
	for _, p := range msg.Parts {
		switch p.Type {
		case "text":
			if strings.TrimSpace(p.Text) != "" {
				texts = append(texts, p.Text)
			}
		case "image_url":
			images = append(images, p.ImageURL)
		case "image_b64":
			mime := p.MIME
			if mime == "" {
				mime = "image/jpeg"
			}
			images = append(images, "data:"+mime+";base64,"+p.ImageB64)
		default:
			return "", nil, fmt.Errorf("jev_emulated: unsupported content part type %q", p.Type)
		}
	}
	state := strings.Join(texts, "\n\n")
	if strings.TrimSpace(state) == "" && len(images) == 0 {
		return "", nil, fmt.Errorf("jev_emulated: nonempty user state is required")
	}
	return state, images, nil
}

// jevEmulatedOptions validates a question and lists its options in a stable
// order: choice options sorted by key, noul as yes/no, score levels in order.
func jevEmulatedOptions(q JevQuestion) ([]jevOption, error) {
	if strings.TrimSpace(q.Instructions) == "" {
		return nil, fmt.Errorf("instructions are required")
	}
	var opts []jevOption
	switch q.Type {
	case "noul":
		if q.Criteria != nil {
			return nil, fmt.Errorf("noul question cannot have criteria")
		}
		opts = []jevOption{{key: "true", label: "yes"}, {key: "false", label: "no"}}
	case "choice":
		criteria, ok := q.Criteria.(map[string]string)
		if !ok || len(criteria) < 2 {
			return nil, fmt.Errorf("choice question requires at least 2 options")
		}
		keys := make([]string, 0, len(criteria))
		for k := range criteria {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			opts = append(opts, jevOption{key: k, label: k, description: criteria[k]})
		}
	case "score":
		levels, ok := q.Criteria.([]string)
		if !ok || len(levels) < 2 {
			return nil, fmt.Errorf("score question requires at least 2 ordered levels")
		}
		for i, l := range levels {
			opts = append(opts, jevOption{key: strconv.Itoa(i), label: l})
		}
	default:
		return nil, fmt.Errorf("unsupported question type %q", q.Type)
	}
	if len(opts) > len(jevEmulatedLetters) {
		return nil, fmt.Errorf("at most %d options are supported, got %d", len(jevEmulatedLetters), len(opts))
	}
	return opts, nil
}

// jevEmulatedPrompt renders the state-first multiple-choice prompt. The text
// before the images is identical across questions, so it can be prefix-cached.
func jevEmulatedPrompt(state, instructions string, opts []jevOption) (head, tail string) {
	head = "State:\n" + state + "\n\n"
	var b strings.Builder
	b.WriteString("Question: " + instructions + "\nOptions:\n")
	for i, o := range opts {
		b.WriteString("[" + jevEmulatedLetters[i:i+1] + "] " + o.label)
		if o.description != "" {
			b.WriteString(": " + o.description)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nAnswer with the letter of the best option only, without brackets or explanation.")
	return head, b.String()
}

// ask runs one question and returns the probability of each option.
func (c *jevEmulatedClient) ask(ctx context.Context, req Request, state string, images []string, instructions string, opts []jevOption) ([]float64, *Usage, error) {
	head, tail := jevEmulatedPrompt(state, instructions, opts)
	parts := []map[string]any{{"type": "text", "text": head}}
	for _, img := range images {
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": img}})
	}
	parts = append(parts, map[string]any{"type": "text", "text": tail})
	var content any = parts
	if len(images) == 0 {
		content = head + tail // plain string is accepted by every OpenAI-compatible backend
	}

	body := map[string]any{
		"model":        req.Model,
		"messages":     []map[string]any{{"role": "user", "content": content}},
		"max_tokens":   1,
		"logprobs":     true,
		"top_logprobs": c.topLogprobs,
	}
	if c.temperature != nil {
		body["temperature"] = *c.temperature
	}
	if c.reasoning != "" {
		body["reasoning_effort"] = c.reasoning
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("encode request: %w", err)
	}

	startedAt := time.Now().UTC()
	capture := func(raw []byte, usage *Usage, requestID, errMsg string) string {
		return captureUsageRecord(ctx, req, UsageCaptureInput{
			AccountID:             captureAccountID(req),
			ProfileID:             captureProfileID(req),
			Provider:              normalizeOpenAIJSONProvider("", c.cfg.BaseURL),
			BaseURL:               c.cfg.BaseURL,
			APIKey:                c.cfg.APIKey,
			ProfileName:           c.cfg.ProfileName,
			ModelName:             req.Model,
			PromptName:            req.PromptName,
			RecordID:              req.RecordID,
			CallReason:            req.CallReason,
			CallLoc:               req.CallLoc,
			RunID:                 req.RunID,
			RequestStartedAt:      startedAt,
			RequestFinishedAt:     time.Now().UTC(),
			InputTokens:           usageInputTokens(usage),
			OutputTokens:          usageOutputTokens(usage),
			PromptCacheHitTokens:  usagePromptCacheHitTokens(usage),
			PromptCacheMissTokens: usagePromptCacheMissTokens(usage),
			InputBodyRef:          captureInputBodyRef(req),
			OutputBodyRef:         captureOutputBodyRef(req),
			ProviderRequestID:     requestID,
			ErrorMessage:          errMsg,
			InputBody:             payload,
			OutputBody:            raw,
		}, c.logger)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		capture(nil, nil, "", err.Error())
		return nil, nil, &ProviderError{Provider: ProviderJevEmulated, Model: req.Model, Err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(raw)
		if err != nil {
			msg = err.Error()
		}
		capture(raw, nil, "", msg)
		return nil, nil, &ProviderError{Provider: ProviderJevEmulated, Model: req.Model, HTTPStatus: resp.StatusCode, Body: truncate(string(raw), 512), Err: err}
	}

	var parsed struct {
		ID      string `json:"id"`
		Choices []struct {
			Logprobs *struct {
				Content []struct {
					TopLogprobs []struct {
						Token   string  `json:"token"`
						Logprob float64 `json:"logprob"`
					} `json:"top_logprobs"`
				} `json:"content"`
			} `json:"logprobs"`
		} `json:"choices"`
		Usage *oaUsage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		capture(raw, nil, "", err.Error())
		return nil, nil, &ProviderError{Provider: ProviderJevEmulated, Model: req.Model, HTTPStatus: resp.StatusCode, Body: truncate(string(raw), 512), Err: err}
	}
	var usage *Usage
	if parsed.Usage != nil {
		usage = &Usage{
			InputTokens:           parsed.Usage.Prompt,
			OutputTokens:          parsed.Usage.Completion,
			TotalTokens:           parsed.Usage.Total,
			PromptCacheHitTokens:  parsed.Usage.PromptCacheHitTokens,
			PromptCacheMissTokens: parsed.Usage.PromptCacheMissTokens,
		}
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Logprobs == nil || len(parsed.Choices[0].Logprobs.Content) == 0 {
		err := fmt.Errorf("response has no logprobs; the backend or model must support logprobs/top_logprobs")
		capture(raw, usage, parsed.ID, err.Error())
		return nil, nil, err
	}
	candidates := map[string]float64{}
	for _, t := range parsed.Choices[0].Logprobs.Content[0].TopLogprobs {
		candidates[t.Token] = t.Logprob
	}
	probs, err := jevEmulatedNormalize(candidates, len(opts))
	if err != nil {
		capture(raw, usage, parsed.ID, err.Error())
		return nil, nil, err
	}
	if eventID := capture(raw, usage, parsed.ID, ""); eventID != "" {
		if usage == nil {
			usage = &Usage{}
		}
		usage.EventID = eventID
	}
	return probs, usage, nil
}

// jevEmulatedNormalize converts first-token logprobs into a distribution over
// the first n option letters. Tokens such as " A", "a" or "[A" all count for
// letter A. An option absent from the top alternatives can be no likelier
// than the least likely alternative returned, so absent options are treated
// as zero only when that bound on their combined probability is negligible.
func jevEmulatedNormalize(candidates map[string]float64, n int) ([]float64, error) {
	letterLogprob := make([]float64, n)
	found := make([]bool, n)
	cutoff := math.Inf(1)
	for token, lp := range candidates {
		if lp <= jevEmulatedUnusableLogprobFloor {
			continue
		}
		cutoff = math.Min(cutoff, lp)
		t := strings.ToUpper(strings.Trim(token, " \t\n[]().:*\""))
		if len(t) != 1 {
			continue
		}
		i := strings.Index(jevEmulatedLetters[:n], t)
		if i < 0 {
			continue
		}
		if found[i] { // log-sum-exp variants of the same letter
			hi, lo := math.Max(letterLogprob[i], lp), math.Min(letterLogprob[i], lp)
			letterLogprob[i] = hi + math.Log1p(math.Exp(lo-hi))
		} else {
			letterLogprob[i], found[i] = lp, true
		}
	}
	peak := math.Inf(-1)
	missing := 0
	for i := range found {
		if found[i] {
			peak = math.Max(peak, letterLogprob[i])
		} else {
			missing++
		}
	}
	if missing == n {
		return nil, fmt.Errorf("no option letter among the returned token alternatives")
	}
	weights := make([]float64, n)
	total := 0.0
	for i := range found {
		if found[i] {
			weights[i] = math.Exp(letterLogprob[i] - peak)
			total += weights[i]
		}
	}
	if missing > 0 {
		missingWeight := float64(missing) * math.Exp(cutoff-peak)
		if missingWeight/(total+missingWeight) >= jevEmulatedNegligibleMissProb {
			return nil, fmt.Errorf("%d option(s) missing from the top alternatives with non-negligible possible probability; raise top_logprobs", missing)
		}
	}
	for i := range weights {
		weights[i] /= total
	}
	return weights, nil
}

// jevEmulatedAnswer builds a Jev answer, including Jev's confidence formulas
// (https://docs.typesafe.ai/confidence#how-confidence-is-calculated).
func jevEmulatedAnswer(qType string, opts []jevOption, probs []float64) JevAnswer {
	byKey := make(map[string]float64, len(opts))
	best := 0
	for i, o := range opts {
		byKey[o.key] = probs[i]
		if probs[i] > probs[best] {
			best = i
		}
	}
	n := float64(len(opts))
	switch qType {
	case "noul":
		p := byKey["true"]
		return JevAnswer{Type: "noul", Noul: &p}
	case "choice":
		conf := (probs[best] - 1/n) / (1 - 1/n)
		return JevAnswer{Type: "choice", Choice: opts[best].key, Probabilities: byKey, Confidence: &conf}
	default: // score
		score, spread, evenSpread := 0.0, 0.0, 0.0
		legend := make(map[string]string, len(opts))
		for i, o := range opts {
			score += float64(i) * probs[i]
			spread += probs[i] * math.Abs(float64(i-best))
			evenSpread += math.Abs(float64(i) - (n-1)/2)
			legend[o.key] = o.label
		}
		evenSpread /= n
		conf := math.Max(0, 1-spread/evenSpread)
		return JevAnswer{Type: "score", Score: &score, Legend: legend, Probabilities: byKey, Confidence: &conf}
	}
}
