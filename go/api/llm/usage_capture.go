package llm

import (
	"context"
	"log/slog"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

var captureLogger = slog.Default()

type UsageCaptureSink interface {
	// Capture persists one llm_usage_event row and returns its id (empty on
	// failure or when the sink does not persist, e.g. no DB configured).
	Capture(ctx context.Context, record UsageCaptureRecord) (string, error)
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
	RunID                 int64
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
	RunID                 int64
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
		RunID:                 in.RunID,
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

func promptWarningLogFields(in UsageCaptureInput) []any {
	fields := []any{
		"provider", string(in.Provider),
		"model", in.ModelName,
		"call_reason", in.CallReason,
		"call_loc", in.CallLoc,
	}
	if in.Metadata == nil {
		return fields
	}
	if promptRef, ok := in.Metadata["prompt_ref"]; ok {
		fields = append(fields, "prompt_ref", promptRef)
	}
	if promptEnvVar, ok := in.Metadata["prompt_dir_env_var"]; ok {
		fields = append(fields, "prompt_dir_env_var", promptEnvVar)
	}
	if promptDir, ok := in.Metadata["prompt_dir"]; ok {
		fields = append(fields, "prompt_dir", promptDir)
	}
	return fields
}

func captureUsageRecord(
	ctx context.Context,
	req Request,
	in UsageCaptureInput,
	logger ApiTypes.JimoLogger) string {
	if in.RecordID == 0 {
		in.RecordID = req.RecordID
	}
	if in.RunID == 0 {
		in.RunID = req.RunID
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
	if in.PromptName == "" {
		logger.Warn("(MID-20260710-01) llm usage event missing mandatory prompt_name",
			promptWarningLogFields(in)...)
	}
	if in.CallReason == "" || in.CallLoc == "" {
		logger.Warn("(MID-20260708-02) llm usage event missing mandatory call_reason/call_loc",
			"provider", string(in.Provider), 
			"model", in.ModelName,
			"call_reason", in.CallReason, 
			"call_loc", in.CallLoc)
	}
	sink := DefaultUsageCaptureSink
	if req.Capture != nil && req.Capture.Sink != nil {
		sink = req.Capture.Sink
	}
	if sink == nil {
		return ""
	}
	eventID, err := sink.Capture(ctx, NewUsageCaptureRecord(in))
	if err != nil {
		logger.Error("llm usage capture failed",
			"error", err,
			"provider", string(in.Provider),
			"model", in.ModelName,
			"call_loc", in.CallLoc)
	}
	return eventID
}
