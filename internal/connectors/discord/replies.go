package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	acknowledgementReplyText = "Received. Processing."
	submitFailedReplyText    = "Sesame 没有接受这条请求。请检查本地 daemon / workspace 配置。"
	runtimeFailedReplyText   = "Sesame 已接受请求，但执行没有生成最终回复。请在本地 console 查看错误详情。"
	timeoutReplyText         = "Sesame 已接受请求，但在限定时间内没有产生最终回复。请在本地 console 查看任务状态。"
	discordAPIBaseURL        = "https://discord.com/api/v10"
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

type discordRESTPoster struct {
	token   string
	client  *http.Client
	baseURL string
}

func newDiscordRESTPoster(token string, client *http.Client, baseURL string) *discordRESTPoster {
	if client == nil {
		client = http.DefaultClient
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = discordAPIBaseURL
	}
	return &discordRESTPoster{
		token:   strings.TrimSpace(token),
		client:  client,
		baseURL: baseURL,
	}
}

func (p *discordRESTPoster) PostMessage(ctx context.Context, channelID string, msg discordOutboundMessage) error {
	if p == nil {
		return errors.New("discord REST poster is not configured")
	}
	if strings.TrimSpace(p.token) == "" {
		return errors.New("discord bot token is empty")
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return errors.New("discord channel id is required")
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/channels/%s/messages", p.baseURL, url.PathEscape(channelID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+p.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("discord post message failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
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
