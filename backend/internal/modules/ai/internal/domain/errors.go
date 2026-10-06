package domain

import (
	"levelup/internal/modules/ai/contracts"
	"levelup/internal/shared/errs"
)

var (
	ErrNotConfigured = errs.WithCode(
		errs.New(errs.Unavailable, "AI drafting is not configured"), contracts.CodeNotConfigured)
	// ErrQuotaExceeded is rendered as 429 by transport: errs has no
	// rate-limit kind, so its kind (Unavailable) only matters off HTTP.
	ErrQuotaExceeded = errs.WithCode(
		errs.New(errs.Unavailable, "daily AI request limit reached"), contracts.CodeQuotaExceeded)
	ErrUnknownKind = errs.WithCode(
		errs.New(errs.Invalid, "kind must be one of badge, level, mission, reward, rule, segment"), "invalid_kind")
	ErrPromptRequired = errs.WithCode(errs.New(errs.Invalid, "prompt is required"), "invalid_prompt")
	ErrPromptTooLong  = errs.WithCode(
		errs.New(errs.Invalid, "prompt must be at most 2000 characters"), "invalid_prompt")
	ErrBadCount = errs.WithCode(errs.New(errs.Invalid, "count must be between 1 and 5"), "invalid_count")
	ErrRefused  = errs.WithCode(
		errs.New(errs.Invalid, "the AI declined this request; rephrase the prompt"), contracts.CodeRefused)
	ErrTruncated = errs.WithCode(
		errs.New(errs.Invalid, "the AI response was cut off; ask for fewer drafts or a shorter prompt"),
		contracts.CodeOutputTruncated)
	ErrBadOutput = errs.WithCode(
		errs.New(errs.Unavailable, "the AI returned an unreadable response; try again"), contracts.CodeBadOutput)
)
