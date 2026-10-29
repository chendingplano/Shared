package llm

import (
	"context"
	"strings"
)

// Request gate call kinds.
const (
	RequestGateCallChat      = "chat"
	RequestGateCallEmbedding = "embedding"
)

// RequestGateCall identifies the provider request a RequestGate is asked to
// admit. Usage carries the identity the request's own usage event would
// record (account, profile, model, prompt, call reason/location), so a gate
// can log and record a hold against the call it delays.
type RequestGateCall struct {
	Kind  string
	Usage UsageCaptureInput
}

// RequestGate decides when a provider request may be sent.
type RequestGate interface {
	// Wait blocks until a request may be sent. It returns context.Cause(ctx)
	// if ctx ends first.
	Wait(ctx context.Context, call RequestGateCall) error
	// Open reports, without blocking, whether a request may be sent now.
	Open(ctx context.Context) bool
}

type requestGateKey struct{}

// WithRequestGate installs gate for every provider request made with ctx by
// the clients in this package. A nil gate leaves ctx unchanged.
func WithRequestGate(ctx context.Context, gate RequestGate) context.Context {
	if gate == nil {
		return ctx
	}
	return context.WithValue(ctx, requestGateKey{}, gate)
}

func requestGateFromContext(ctx context.Context) RequestGate {
	gate, _ := ctx.Value(requestGateKey{}).(RequestGate)
	return gate
}

// waitForRequestGate blocks until ctx's gate (if any) admits call.
func waitForRequestGate(ctx context.Context, call RequestGateCall) error {
	if gate := requestGateFromContext(ctx); gate != nil {
		return gate.Wait(ctx, call)
	}
	return nil
}

// requestGateOpen reports whether ctx's gate (if any) admits a request now.
func requestGateOpen(ctx context.Context) bool {
	if gate := requestGateFromContext(ctx); gate != nil {
		return gate.Open(ctx)
	}
	return true
}

// chatRequestGateCall describes req, a chat request, for the request gate
// with the identity captureUsageRecord records for it.
func chatRequestGateCall(req Request, provider ProviderID, baseURL, apiKey, profileName string) RequestGateCall {
	userID := strings.TrimSpace(req.UserID)
	if userID == "" && req.Capture != nil {
		userID = strings.TrimSpace(req.Capture.UserID)
	}
	return RequestGateCall{Kind: RequestGateCallChat, Usage: UsageCaptureInput{
		UserID:      userID,
		AccountID:   captureAccountID(req),
		ProfileID:   captureProfileID(req),
		ProfileName: profileName,
		Provider:    provider,
		BaseURL:     baseURL,
		APIKey:      apiKey,
		ModelName:   req.Model,
		PromptName:  req.PromptName,
		RecordID:    req.RecordID,
		RunID:       req.RunID,
		CallReason:  req.CallReason,
		CallLoc:     req.CallLoc,
		Metadata:    req.Metadata,
	}}
}
