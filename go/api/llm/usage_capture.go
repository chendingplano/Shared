package llm

import (
	"context"
	"time"
)

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
	AccountID         string
	ProfileID         string
	ProfileName       string
	Provider          ProviderID
	BaseURL           string
	APIKey            string
	ModelName         string
	PromptName        string
	RequestStartedAt  time.Time
	RequestFinishedAt time.Time
	InputTokens       int
	OutputTokens      int
	InputBodyRef      string
	OutputBodyRef     string
	ErrorMessage      string
	ProviderRequestID string
	InputBody         []byte
	OutputBody        []byte
	RecordID          int64
	CallReason        string
	CallLoc           string
}

type UsageCaptureRecord struct {
	AccountID         string
	ProfileID         string
	ProfileName       string
	Provider          ProviderID
	BaseURL           string
	APIKey            string
	ModelName         string
	PromptName        string
	RequestStartedAt  time.Time
	RequestFinishedAt time.Time
	InputTokens       int
	OutputTokens      int
	TotalTokens       int
	InputBodyRef      string
	OutputBodyRef     string
	ErrorMessage      string
	ProviderRequestID string
	InputBody         []byte
	OutputBody        []byte
	RecordID          int64
	CallReason        string
	CallLoc           string
}

func NewUsageCaptureRecord(in UsageCaptureInput) UsageCaptureRecord {
	promptName := EnsurePromptName(in.PromptName, in.CallReason, in.CallLoc, in.ModelName)
	return UsageCaptureRecord{
		AccountID:         in.AccountID,
		ProfileID:         in.ProfileID,
		ProfileName:       in.ProfileName,
		Provider:          in.Provider,
		BaseURL:           in.BaseURL,
		APIKey:            in.APIKey,
		ModelName:         in.ModelName,
		PromptName:        promptName,
		RequestStartedAt:  in.RequestStartedAt,
		RequestFinishedAt: in.RequestFinishedAt,
		InputTokens:       in.InputTokens,
		OutputTokens:      in.OutputTokens,
		TotalTokens:       in.InputTokens + in.OutputTokens,
		InputBodyRef:      in.InputBodyRef,
		OutputBodyRef:     in.OutputBodyRef,
		ErrorMessage:      in.ErrorMessage,
		ProviderRequestID: in.ProviderRequestID,
		InputBody:         in.InputBody,
		OutputBody:        in.OutputBody,
		RecordID:          in.RecordID,
		CallReason:        in.CallReason,
		CallLoc:           in.CallLoc,
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
	sink := DefaultUsageCaptureSink
	if req.Capture != nil && req.Capture.Sink != nil {
		sink = req.Capture.Sink
	}
	if sink == nil {
		return
	}
	_ = sink.Capture(ctx, NewUsageCaptureRecord(in))
}
