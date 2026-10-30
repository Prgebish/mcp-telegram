package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Prgebish/mcp-telegram/internal/acl"
	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type transcribeInput struct {
	Peer      string `json:"peer" jsonschema:"required,Chat reference: @username, user:ID, chat:ID, or channel:ID"`
	MessageID int    `json:"message_id" jsonschema:"required,ID of a voice message or round video message"`
}

type transcribeOutput struct {
	Peer      string `json:"peer"`
	MessageID int    `json:"message_id"`
	Status    string `json:"status"` // done | pending | failed
	Text      string `json:"text"`
}

const (
	transcribeDone    = "done"
	transcribePending = "pending"
	transcribeFailed  = "failed"
)

// Backoff between messages.transcribeAudio calls while Telegram reports the
// transcription as pending. Variables so tests can shorten them.
var (
	transcribePollInitial = time.Second
	transcribePollMax     = 8 * time.Second
)

// transcribeErrors maps Telegram RPC error types to user-facing messages.
// TRANSCRIPTION_FAILED is not here: it yields status "failed", not an error.
var transcribeErrors = map[string]string{
	"PREMIUM_ACCOUNT_REQUIRED": "Telegram Premium is required to transcribe voice messages",
	"MSG_ID_INVALID":           "invalid message ID",
	"MSG_VOICE_MISSING":        "the message is not a voice message",
	"MSG_VOICE_TOO_LONG":       "the voice message is too long to be transcribed",
	"PEER_ID_INVALID":          "invalid peer",
}

var errMessageNotFound = errors.New("message not found in this chat")

func registerTranscribe(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "tg_transcribe",
		Description: "Transcribe a voice message or round video from a whitelisted chat using Telegram's native transcription (requires Premium). " +
			"Returns JSON {peer, message_id, status, text}; status is done, pending (still processing after the timeout, call again later) or failed.",
		// Not read-only: transcribeAudio starts server-side processing and
		// spends the account's trial or Premium quota.
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: ptrBool(false),
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input transcribeInput) (*mcp.CallToolResult, any, error) {
		return handleTranscribe(ctx, deps, input), nil, nil
	})
}

// handleTranscribe polls messages.transcribeAudio with exponential backoff
// until the transcription is no longer pending or the configured timeout
// expires. Polling keeps the handler self-contained: waiting for
// updateTranscribedAudio would need routing updates from the client's update
// handler to individual tool calls. Every call goes through the client's
// rate-limiting middleware. The transcription text is never logged.
func handleTranscribe(ctx context.Context, deps *Deps, input transcribeInput) *mcp.CallToolResult {
	if input.MessageID <= 0 {
		return toolError("message_id must be a positive integer")
	}

	peer, identity, denied := resolveAllowed(ctx, deps, input.Peer, config.PermRead)
	if denied != nil {
		return denied
	}

	deadline := time.Now().Add(time.Duration(deps.Transcribe.TimeoutSeconds) * time.Second)

	msg, err := fetchMessage(ctx, deps, peer, identity, input.MessageID)
	if err != nil {
		if errors.Is(err, errMessageNotFound) {
			return toolError(fmt.Sprintf("message %d not found in %s", input.MessageID, input.Peer))
		}
		return toolError(fmt.Sprintf("failed to fetch message: %v", err))
	}
	if !isTranscribable(msg) {
		return toolError(fmt.Sprintf("message %d is not a voice message or round video; only those can be transcribed", input.MessageID))
	}

	out := transcribeOutput{Peer: input.Peer, MessageID: input.MessageID}
	req := &tg.MessagesTranscribeAudioRequest{Peer: peer.InputPeer(), MsgID: input.MessageID}
	wait := transcribePollInitial

	for {
		res, err := deps.API.MessagesTranscribeAudio(ctx, req)
		if err != nil {
			if d, ok := tgerr.AsFloodWait(err); ok {
				if time.Now().Add(d).After(deadline) {
					return toolError(fmt.Sprintf("Telegram rate limit (FLOOD_WAIT): retry in %s", d))
				}
				if err := sleepCtx(ctx, d); err != nil {
					return toolError(fmt.Sprintf("transcription cancelled: %v", err))
				}
				continue
			}
			if tgerr.Is(err, "TRANSCRIPTION_FAILED") {
				out.Status = transcribeFailed
				return transcribeResult(out)
			}
			if rpcErr, ok := tgerr.As(err); ok {
				if text, ok := transcribeErrors[rpcErr.Type]; ok {
					return toolError(fmt.Sprintf("transcription failed: %s (%s)", text, rpcErr.Type))
				}
			}
			return toolError(fmt.Sprintf("transcription failed: %v", err))
		}

		out.Text = res.Text
		if !res.Pending {
			out.Status = transcribeDone
			return transcribeResult(out)
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			out.Status = transcribePending
			return transcribeResult(out)
		}
		if err := sleepCtx(ctx, min(wait, remaining)); err != nil {
			return toolError(fmt.Sprintf("transcription cancelled: %v", err))
		}
		wait = min(wait*2, transcribePollMax)
	}
}

// fetchMessage loads a message and verifies it belongs to the given chat.
// Private chats and basic groups share one message ID space per account, so
// messages.getMessages may return a message from another dialog — that must
// not be transcribed on the strength of this chat's ACL entry.
func fetchMessage(ctx context.Context, deps *Deps, peer Peer, identity acl.PeerIdentity, id int) (*tg.Message, error) {
	ids := []tg.InputMessageClass{&tg.InputMessageID{ID: id}}

	var res tg.MessagesMessagesClass
	var err error
	if ch, ok := peer.(ChannelPeer); ok {
		res, err = deps.API.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: ch.InputChannel(),
			ID:      ids,
		})
	} else {
		res, err = deps.API.MessagesGetMessages(ctx, ids)
	}
	if err != nil {
		return nil, err
	}

	modified, ok := res.AsModified()
	if !ok {
		return nil, fmt.Errorf("unexpected response type")
	}
	for _, m := range modified.GetMessages() {
		msg, ok := m.(*tg.Message)
		if ok && msg.ID == id && peerMatches(msg.PeerID, identity) {
			return msg, nil
		}
	}
	return nil, errMessageNotFound
}

func peerMatches(p tg.PeerClass, identity acl.PeerIdentity) bool {
	switch v := p.(type) {
	case *tg.PeerUser:
		return identity.Kind == acl.KindUser && identity.ID == v.UserID
	case *tg.PeerChat:
		return identity.Kind == acl.KindChat && identity.ID == v.ChatID
	case *tg.PeerChannel:
		return identity.Kind == acl.KindChannel && identity.ID == v.ChannelID
	default:
		return false
	}
}

// isTranscribable reports whether msg is a voice message or round video.
func isTranscribable(msg *tg.Message) bool {
	media, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		return false
	}
	if media.Voice || media.Round {
		return true
	}
	doc, ok := media.Document.(*tg.Document)
	if !ok {
		return false
	}
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeAudio:
			if a.Voice {
				return true
			}
		case *tg.DocumentAttributeVideo:
			if a.RoundMessage {
				return true
			}
		}
	}
	return false
}

func transcribeResult(out transcribeOutput) *mcp.CallToolResult {
	data, err := json.Marshal(out)
	if err != nil {
		return toolError(fmt.Sprintf("failed to encode result: %v", err))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
