package llm

import (
	"context"
	"log/slog"
	"time"
)

var captureLogger = slog.Default()

type UsageCaptureSink interface {
	Capture(ctx context.Context, record UsageCaptureRecord) error
}

var DefaultUsageCaptureSink UsageCaptureSink

type RequestCapture struct {
	AccountID     string
	ProfileID     string
	InputBodyRef  string
	OutputBodyRef string
	Sink          UsageCaptureSink
}

type UsageCaptureInput struct {
	AccountID             string
	ProfileID             string
	ProfileName           string
	Provider              ProviderID
	BaseURL               string
	APIKey                string
	ModelName             string
	PromptName            string
	RequestStartedAt      time.Time
	RequestFinishedAt     time.Time
	InputTokens           int
	OutputTokens          int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
	InputBodyRef          string
	OutputBodyRef         string
	ErrorMessage          string
	ProviderRequestID     string
	InputBody             []byte
	OutputBody            []byte
	RecordID              int64
	CallReason            string
	CallLoc               string
	Metadata              map[string]any
}

type UsageCaptureRecord struct {
	AccountID             string
	ProfileID             string
	ProfileName           string
	Provider              ProviderID
	BaseURL               string
	APIKey                string
	ModelName             string
	PromptName            string
	RequestStartedAt      time.Time
	RequestFinishedAt     time.Time
	InputTokens           int
	OutputTokens          int
	TotalTokens           int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
	InputBodyRef          string
	OutputBodyRef         string
	ErrorMessage          string
	ProviderRequestID     string
	InputBody             []byte
	OutputBody            []byte
	RecordID              int64
	CallReason            string
	CallLoc               string
	Metadata              map[string]any
}

func NewUsageCaptureRecord(in UsageCaptureInput) UsageCaptureRecord {
	promptName := EnsurePromptName(in.PromptName, in.CallReason, in.CallLoc, in.ModelName)
	return UsageCaptureRecord{
		AccountID:             in.AccountID,
		ProfileID:             in.ProfileID,
		ProfileName:           in.ProfileName,
		Provider:              in.Provider,
		BaseURL:               in.BaseURL,
		APIKey:                in.APIKey,
		ModelName:             in.ModelName,
		PromptName:            promptName,
		RequestStartedAt:      in.RequestStartedAt,
		RequestFinishedAt:     in.RequestFinishedAt,
		InputTokens:           in.InputTokens,
		OutputTokens:          in.OutputTokens,
		TotalTokens:           in.InputTokens + in.OutputTokens,
		PromptCacheHitTokens:  in.PromptCacheHitTokens,
		PromptCacheMissTokens: in.PromptCacheMissTokens,
		InputBodyRef:          in.InputBodyRef,
		OutputBodyRef:         in.OutputBodyRef,
		ErrorMessage:          in.ErrorMessage,
		ProviderRequestID:     in.ProviderRequestID,
		InputBody:             in.InputBody,
		OutputBody:            in.OutputBody,
		RecordID:              in.RecordID,
		CallReason:            in.CallReason,
		CallLoc:               in.CallLoc,
		Metadata:              in.Metadata,
	}
}

func EnsurePromptName(promptName, callReason, callLoc, modelName string) string {
	if promptName != "" {
		return promptName
	}
	if callLoc != "" {
		return "missing_prompt_name@" + callLoc
	}
	if callReason != "" {
		return "missing_prompt_name@" + callReason
	}
	if modelName != "" {
		return "missing_prompt_name@" + modelName
	}
	return "missing_prompt_name"
}

func captureUsageRecord(ctx context.Context, req Request, in UsageCaptureInput) {
	if in.RecordID == 0 {
		in.RecordID = req.RecordID
	}
	if in.CallReason == "" {
		in.CallReason = req.CallReason
	}
	if in.CallLoc == "" {
		in.CallLoc = req.CallLoc
	}
	if in.PromptName == "" {
		in.PromptName = req.PromptName
	}
	if in.Metadata == nil {
		in.Metadata = req.Metadata
	}
	if in.CallReason == "" || in.CallLoc == "" {
		captureLogger.WarnContext(ctx, "llm usage event missing mandatory call_reason/call_loc",
			"provider", string(in.Provider), "model", in.ModelName,
			"call_reason", in.CallReason, "call_loc", in.CallLoc)
	}
	sink := DefaultUsageCaptureSink
	if req.Capture != nil && req.Capture.Sink != nil {
		sink = req.Capture.Sink
	}
	if sink == nil {
		return
	}
	if err := sink.Capture(ctx, NewUsageCaptureRecord(in)); err != nil {
		captureLogger.ErrorContext(ctx, "llm usage capture failed", "error", err,
			"provider", string(in.Provider), "model", in.ModelName, "call_loc", in.CallLoc)
	}
}
