package discord

import (
	"context"
	"errors"
)

const (
	acknowledgementReplyText = "Received. Processing."
	submitFailedReplyText    = "Sesame 没有接受这条请求。请检查本地 daemon / workspace 配置。"
	runtimeFailedReplyText   = "Sesame 已接受请求，但执行没有生成最终回复。请在本地 console 查看错误详情。"
	timeoutReplyText         = "Sesame 已接受请求，但在限定时间内没有产生最终回复。请在本地 console 查看任务状态。"
)

type genericReplyKind string

const (
	genericReplySubmitFailed genericReplyKind = "submit_failed"
	genericReplyRuntime      genericReplyKind = "runtime_failed"
	genericReplyTimeout      genericReplyKind = "timeout"
)

type discordReplyPoster interface {
	PostMessage(ctx context.Context, channelID string, msg discordOutboundMessage) error
}

func postAcknowledgement(ctx context.Context, poster discordReplyPoster, channelID, replyToMessageID string) error {
	if poster == nil {
		return errors.New("discord reply poster is not configured")
	}
	return poster.PostMessage(ctx, channelID, buildOutboundMessage(acknowledgementReplyText, replyToMessageID))
}

func postFinalReply(ctx context.Context, poster discordReplyPoster, channelID, replyToMessageID, finalText string, cfg WorkspaceBinding) error {
	if poster == nil {
		return errors.New("discord reply poster is not configured")
	}
	return poster.PostMessage(ctx, channelID, buildOutboundMessage(renderFinalReplyText(finalText, cfg), replyToMessageID))
}

func postGenericReply(ctx context.Context, poster discordReplyPoster, channelID, replyToMessageID string, kind genericReplyKind) error {
	if poster == nil {
		return errors.New("discord reply poster is not configured")
	}
	return poster.PostMessage(ctx, channelID, buildOutboundMessage(genericReplyText(kind), replyToMessageID))
}

func genericReplyText(kind genericReplyKind) string {
	switch kind {
	case genericReplySubmitFailed:
		return submitFailedReplyText
	case genericReplyTimeout:
		return timeoutReplyText
	default:
		return runtimeFailedReplyText
	}
}
