package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	discordGatewayURL = "wss://gateway.discord.gg/?v=10&encoding=json"

	gatewayOpDispatch       = 0
	gatewayOpHeartbeat      = 1
	gatewayOpIdentify       = 2
	gatewayOpResume         = 6
	gatewayOpReconnect      = 7
	gatewayOpInvalidSession = 9
	gatewayOpHello          = 10
	gatewayOpHeartbeatACK   = 11

	defaultReconnectDelay = time.Second
	maxReconnectDelay     = 30 * time.Second
	gatewayWriteTimeout   = 10 * time.Second
)

var (
	errGatewayReconnect      = errors.New("discord gateway requested reconnect")
	errGatewayInvalidSession = errors.New("discord gateway invalid session")
)

// Gateway abstracts the Discord gateway transport for the connector service.
type Gateway interface {
	Start(context.Context) error
	Close() error
}

type gatewayMessageHandler interface {
	HandleAcceptedMessage(ctx context.Context, msg AcceptedMessage) error
}

// GatewayConfig captures static config passed into a gateway implementation.
type GatewayConfig struct {
	Global        GlobalConfig
	Binding       WorkspaceBinding
	WorkspaceRoot string
	State         *StateStore
	Bridge        gatewayMessageHandler
	ReplyPoster   discordReplyPoster
	HTTPClient    *http.Client
	Logger        *slog.Logger
}

type gatewayEnvelope struct {
	Op int             `json:"op"`
	S  *int64          `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
	D  json.RawMessage `json:"d,omitempty"`
}

type gatewayCommand struct {
	Op int `json:"op"`
	D  any `json:"d"`
}

type gatewayHello struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

type gatewayIdentify struct {
	Token      string                    `json:"token"`
	Intents    int                       `json:"intents"`
	Properties gatewayIdentifyProperties `json:"properties"`
}

type gatewayIdentifyProperties struct {
	OS      string `json:"$os"`
	Browser string `json:"$browser"`
	Device  string `json:"$device"`
}

type gatewayResume struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Seq       int64  `json:"seq"`
}

type gatewayReady struct {
	SessionID        string `json:"session_id"`
	ResumeGatewayURL string `json:"resume_gateway_url"`
	User             struct {
		ID string `json:"id"`
	} `json:"user"`
}

type discordGateway struct {
	cfg         GatewayConfig
	token       string
	intents     int
	logger      *slog.Logger
	client      *http.Client
	state       *StateStore
	bridge      gatewayMessageHandler
	replyPoster discordReplyPoster
	workspace   string

	mu               sync.Mutex
	started          bool
	closed           bool
	cancel           context.CancelFunc
	done             chan struct{}
	conn             *websocket.Conn
	sessionID        string
	resumeGatewayURL string
	botUserID        string
	lastSeq          int64
	hasSeq           bool
	inFlight         map[string]struct{}
}

// NewGateway creates a Discord gateway client bound to ingress/bridge/replies.
func NewGateway(cfg GatewayConfig) (Gateway, error) {
	token, err := resolveBotToken(cfg.Global)
	if err != nil {
		return nil, err
	}
	intents, err := resolveGatewayIntents(cfg.Global)
	if err != nil {
		return nil, err
	}
	if cfg.State == nil {
		return nil, errors.New("discord gateway state store is not configured")
	}
	if cfg.Bridge == nil {
		return nil, errors.New("discord gateway bridge is not configured")
	}
	if cfg.ReplyPoster == nil {
		return nil, errors.New("discord gateway reply poster is not configured")
	}
	workspace := strings.TrimSpace(cfg.WorkspaceRoot)
	if workspace == "" {
		return nil, errors.New("discord gateway workspace root is required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &discordGateway{
		cfg:         cfg,
		token:       token,
		intents:     intents,
		logger:      logger,
		client:      httpClient,
		state:       cfg.State,
		bridge:      cfg.Bridge,
		replyPoster: cfg.ReplyPoster,
		workspace:   workspace,
		inFlight:    make(map[string]struct{}),
	}, nil
}

func (g *discordGateway) Start(ctx context.Context) error {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return errors.New("discord gateway is closed")
	}
	if g.started {
		g.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	g.cancel = cancel
	g.done = make(chan struct{})
	g.started = true
	g.mu.Unlock()

	go g.run(runCtx)
	return nil
}

func (g *discordGateway) Close() error {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return nil
	}
	g.closed = true
	g.started = false
	cancel := g.cancel
	done := g.done
	conn := g.conn
	g.conn = nil
	g.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "shutdown")
	}
	if done != nil {
		<-done
	}
	return nil
}

func (g *discordGateway) run(ctx context.Context) {
	defer func() {
		g.mu.Lock()
		if g.done != nil {
			close(g.done)
			g.done = nil
		}
		g.mu.Unlock()
	}()

	delay := defaultReconnectDelay
	for {
		err := g.runSession(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			return
		}
		if errors.Is(err, errGatewayInvalidSession) {
			g.logger.Warn("discord gateway session invalidated; reconnecting with identify")
		} else {
			g.logger.Warn("discord gateway disconnected; reconnecting", "error", err)
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay *= 2
		if delay > maxReconnectDelay {
			delay = maxReconnectDelay
		}
	}
}

func (g *discordGateway) runSession(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, g.nextGatewayURL(), &websocket.DialOptions{
		HTTPClient: g.client,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close(websocket.StatusGoingAway, "reconnect")
	}()
	g.setConn(conn)
	defer g.setConn(nil)

	var helloEnvelope gatewayEnvelope
	if err := wsjson.Read(ctx, conn, &helloEnvelope); err != nil {
		return err
	}
	if helloEnvelope.Op != gatewayOpHello {
		return fmt.Errorf("discord gateway expected HELLO op=%d, got %d", gatewayOpHello, helloEnvelope.Op)
	}

	var hello gatewayHello
	if err := json.Unmarshal(helloEnvelope.D, &hello); err != nil {
		return err
	}
	heartbeatInterval := time.Duration(hello.HeartbeatInterval) * time.Millisecond
	if heartbeatInterval <= 0 {
		return errors.New("discord gateway heartbeat interval is invalid")
	}

	resume, err := g.tryResumeOrIdentify(ctx, conn)
	if err != nil {
		return err
	}
	if resume {
		g.logger.Info("discord gateway sent RESUME")
	}

	events := make(chan gatewayEnvelope, 32)
	readErrs := make(chan error, 1)
	go g.readLoop(ctx, conn, events, readErrs)

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErrs:
			return err
		case env := <-events:
			if env.S != nil {
				g.setSequence(*env.S)
			}
			switch env.Op {
			case gatewayOpDispatch:
				if err := g.handleDispatch(ctx, env); err != nil {
					return err
				}
			case gatewayOpHeartbeat:
				if err := g.sendHeartbeat(ctx, conn); err != nil {
					return err
				}
			case gatewayOpReconnect:
				return errGatewayReconnect
			case gatewayOpInvalidSession:
				canResume := false
				_ = json.Unmarshal(env.D, &canResume)
				if !canResume {
					g.clearResumeState()
				}
				return errGatewayInvalidSession
			case gatewayOpHeartbeatACK:
				continue
			}
		case <-heartbeat.C:
			if err := g.sendHeartbeat(ctx, conn); err != nil {
				return err
			}
		}
	}
}

func (g *discordGateway) readLoop(ctx context.Context, conn *websocket.Conn, events chan<- gatewayEnvelope, errs chan<- error) {
	for {
		var env gatewayEnvelope
		if err := wsjson.Read(ctx, conn, &env); err != nil {
			select {
			case errs <- err:
			default:
			}
			return
		}
		select {
		case events <- env:
		case <-ctx.Done():
			return
		}
	}
}

func (g *discordGateway) tryResumeOrIdentify(ctx context.Context, conn *websocket.Conn) (bool, error) {
	resumeGatewayURL, sessionID, seq, hasSeq := g.resumeSnapshot()
	if strings.TrimSpace(resumeGatewayURL) != "" && strings.TrimSpace(sessionID) != "" && hasSeq {
		if err := g.sendGatewayCommand(ctx, conn, gatewayOpResume, gatewayResume{
			Token:     g.token,
			SessionID: sessionID,
			Seq:       seq,
		}); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, g.sendGatewayCommand(ctx, conn, gatewayOpIdentify, gatewayIdentify{
		Token:   g.token,
		Intents: g.intents,
		Properties: gatewayIdentifyProperties{
			OS:      runtime.GOOS,
			Browser: "sesame",
			Device:  "sesame",
		},
	})
}

func (g *discordGateway) sendHeartbeat(ctx context.Context, conn *websocket.Conn) error {
	seq, hasSeq := g.sequenceSnapshot()
	var payload any
	if hasSeq {
		payload = seq
	}
	return g.sendGatewayCommand(ctx, conn, gatewayOpHeartbeat, payload)
}

func (g *discordGateway) sendGatewayCommand(ctx context.Context, conn *websocket.Conn, op int, payload any) error {
	writeCtx, cancel := context.WithTimeout(ctx, gatewayWriteTimeout)
	defer cancel()
	return wsjson.Write(writeCtx, conn, gatewayCommand{Op: op, D: payload})
}

func (g *discordGateway) handleDispatch(ctx context.Context, env gatewayEnvelope) error {
	switch env.T {
	case "READY":
		var ready gatewayReady
		if err := json.Unmarshal(env.D, &ready); err != nil {
			return err
		}
		g.setReadyState(ready.SessionID, ready.ResumeGatewayURL, ready.User.ID)
		g.logger.Info("discord gateway ready", "bot_user_id", ready.User.ID)
	case "RESUMED":
		g.logger.Info("discord gateway resumed")
	case "MESSAGE_CREATE":
		var msg GatewayMessage
		if err := json.Unmarshal(env.D, &msg); err != nil {
			g.logger.Warn("discord message decode failed", "error", err)
			return nil
		}
		go func() {
			if err := g.handleMessageCreate(context.WithoutCancel(ctx), msg); err != nil {
				g.logger.Error("discord message handling failed", "discord_message_id", msg.ID, "error", err)
			}
		}()
	}
	return nil
}

func (g *discordGateway) handleMessageCreate(ctx context.Context, msg GatewayMessage) error {
	if g.state == nil {
		return errors.New("discord gateway state store is not configured")
	}
	if g.bridge == nil {
		return errors.New("discord gateway bridge is not configured")
	}
	if g.replyPoster == nil {
		return errors.New("discord gateway reply poster is not configured")
	}

	_, duplicate, err := g.state.GetDiscordIngress(ctx, msg.ID)
	if err != nil {
		return err
	}
	if g.isInFlight(msg.ID) {
		duplicate = true
	}

	decision := processMessageForIngress(msg, g.cfg.Binding, ingressOptions{
		BotUserID: g.botID(),
		Duplicate: duplicate,
	})

	switch decision.Action {
	case ingressActionIgnore:
		if g.cfg.Global.LogIgnoredMessages {
			g.logger.Info("discord message ignored",
				"discord_message_id", msg.ID,
				"guild_id", msg.GuildID,
				"channel_id", msg.ChannelID,
				"author_id", msg.Author.ID,
				"reason", decision.Reason,
			)
		}
		return nil
	case ingressActionRejectWithReply:
		return g.replyPoster.PostMessage(ctx, msg.ChannelID, buildOutboundMessage(decision.ReplyText, msg.ID))
	case ingressActionAccept:
		if !g.markInFlight(msg.ID) {
			return nil
		}
		defer g.clearInFlight(msg.ID)
		return g.bridge.HandleAcceptedMessage(ctx, AcceptedMessage{
			DiscordMessageID: msg.ID,
			GuildID:          msg.GuildID,
			ChannelID:        msg.ChannelID,
			AuthorID:         msg.Author.ID,
			WorkspaceRoot:    g.workspace,
			CleanedText:      decision.CleanedText,
		})
	default:
		return nil
	}
}

func (g *discordGateway) nextGatewayURL() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	resume := strings.TrimSpace(g.resumeGatewayURL)
	if resume == "" {
		return discordGatewayURL
	}
	if strings.Contains(resume, "?") {
		return resume + "&v=10&encoding=json"
	}
	return resume + "?v=10&encoding=json"
}

func (g *discordGateway) setConn(conn *websocket.Conn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.conn = conn
}

func (g *discordGateway) setReadyState(sessionID, resumeURL, botUserID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sessionID = strings.TrimSpace(sessionID)
	g.resumeGatewayURL = strings.TrimSpace(resumeURL)
	g.botUserID = strings.TrimSpace(botUserID)
}

func (g *discordGateway) clearResumeState() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sessionID = ""
	g.resumeGatewayURL = ""
	g.botUserID = ""
	g.lastSeq = 0
	g.hasSeq = false
}

func (g *discordGateway) setSequence(seq int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastSeq = seq
	g.hasSeq = true
}

func (g *discordGateway) sequenceSnapshot() (int64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lastSeq, g.hasSeq
}

func (g *discordGateway) resumeSnapshot() (resumeURL, sessionID string, seq int64, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.resumeGatewayURL, g.sessionID, g.lastSeq, g.hasSeq
}

func (g *discordGateway) botID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.botUserID
}

func (g *discordGateway) isInFlight(messageID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight == nil {
		return false
	}
	_, ok := g.inFlight[strings.TrimSpace(messageID)]
	return ok
}

func (g *discordGateway) markInFlight(messageID string) bool {
	id := strings.TrimSpace(messageID)
	if id == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight == nil {
		g.inFlight = make(map[string]struct{})
	}
	if _, exists := g.inFlight[id]; exists {
		return false
	}
	g.inFlight[id] = struct{}{}
	return true
}

func (g *discordGateway) clearInFlight(messageID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight == nil {
		return
	}
	delete(g.inFlight, strings.TrimSpace(messageID))
}

func resolveBotToken(global GlobalConfig) (string, error) {
	if token := strings.TrimSpace(global.BotToken); token != "" {
		return token, nil
	}
	name := strings.TrimSpace(global.BotTokenEnv)
	if name == "" {
		return "", errors.New("discord bot_token or bot_token_env is required")
	}
	token := strings.TrimSpace(os.Getenv(name))
	if token == "" {
		return "", fmt.Errorf("discord bot token is empty: %s", name)
	}
	return token, nil
}

func resolveGatewayIntents(global GlobalConfig) (int, error) {
	names := append([]string(nil), global.GatewayIntents...)
	if len(names) == 0 {
		names = []string{"guilds", "guild_messages"}
	}

	intents := 0
	for _, name := range names {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "guilds":
			intents |= 1 << 0
		case "guild_messages":
			intents |= 1 << 9
		case "message_content":
			intents |= 1 << 15
		case "":
		default:
			return 0, fmt.Errorf("unsupported discord gateway intent: %s", name)
		}
	}
	if global.MessageContentIntent {
		intents |= 1 << 15
	}
	if intents == 0 {
		return 0, errors.New("discord gateway intents are empty")
	}
	return intents, nil
}

var _ Gateway = (*discordGateway)(nil)
