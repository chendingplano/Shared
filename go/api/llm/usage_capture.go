package llm

import (
	"context"
	"time"
)

type UsageCaptureSink interface {
	Capture(ctx context.Context, record UsageCaptureRecord) error
}

type RequestCapture struct {
	AccountID     int64
	ProfileID     int64
	InputBodyRef  string
	OutputBodyRef string
	Sink          UsageCaptureSink
}

type UsageCaptureInput struct {
	AccountID         int64
	ProfileID         int64
	Provider          ProviderID
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
}

type UsageCaptureRecord struct {
	AccountID         int64
	ProfileID         int64
	Provider          ProviderID
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
}

func NewUsageCaptureRecord(in UsageCaptureInput) UsageCaptureRecord {
	return UsageCaptureRecord{
		AccountID:         in.AccountID,
		ProfileID:         in.ProfileID,
		Provider:          in.Provider,
		ModelName:         in.ModelName,
		PromptName:        in.PromptName,
		RequestStartedAt:  in.RequestStartedAt,
		RequestFinishedAt: in.RequestFinishedAt,
		InputTokens:       in.InputTokens,
		OutputTokens:      in.OutputTokens,
		TotalTokens:       in.InputTokens + in.OutputTokens,
		InputBodyRef:      in.InputBodyRef,
		OutputBodyRef:     in.OutputBodyRef,
		ErrorMessage:      in.ErrorMessage,
		ProviderRequestID: in.ProviderRequestID,
	}
}

func captureUsageRecord(ctx context.Context, req Request, in UsageCaptureInput) {
	if req.Capture == nil || req.Capture.Sink == nil {
		return
	}
	_ = req.Capture.Sink.Capture(ctx, NewUsageCaptureRecord(in))
}
