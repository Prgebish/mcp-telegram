package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestRenderText_EmptyParseModeIsVerbatim(t *testing.T) {
	// Existing callers pass no parse_mode and must keep the old behaviour,
	// including text that happens to contain markdown characters.
	in := "cost is 5 ** 2 and _x_ and `y`"
	text, entities, err := renderText(in, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != in {
		t.Errorf("text = %q, want it unchanged", text)
	}
	if entities != nil {
		t.Errorf("got %d entities, want none", len(entities))
	}
}

func TestRenderText_Markdown(t *testing.T) {
	text, entities, err := renderText("a **b**", parseModeMarkdown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "a b" {
		t.Errorf("text = %q, want %q", text, "a b")
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1", len(entities))
	}
}

func TestRenderText_UnsupportedParseMode(t *testing.T) {
	_, _, err := renderText("x", "html")
	if err == nil {
		t.Fatal("expected an error for an unsupported parse_mode")
	}
	if !strings.Contains(err.Error(), "html") {
		t.Errorf("error = %q, should name the rejected mode", err)
	}
}

func TestHandleDraft_MarkdownSetsEntities(t *testing.T) {
	var gotMessage string
	var gotEntities []tg.MessageEntityClass

	api := mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		if req, ok := input.(*tg.MessagesSaveDraftRequest); ok {
			gotMessage = req.Message
			gotEntities, _ = req.GetEntities()
		}
		if v, ok := output.(*tg.BoolBox); ok {
			v.Bool = &tg.BoolTrue{}
		}
		return nil
	})

	deps := newTestDeps(newAllowedResolver(), api, config.PermDraft)
	result := handleDraft(context.Background(), deps, draftInput{
		Chat:      "@testchat",
		Text:      "a **bold** word",
		ParseMode: "markdown",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	if gotMessage != "a bold word" {
		t.Errorf("message = %q, want %q (markup stripped)", gotMessage, "a bold word")
	}
	if len(gotEntities) != 1 {
		t.Fatalf("got %d entities, want 1", len(gotEntities))
	}
	e, ok := gotEntities[0].(*tg.MessageEntityBold)
	if !ok {
		t.Fatalf("entity type = %T, want *tg.MessageEntityBold", gotEntities[0])
	}
	if e.Offset != 2 || e.Length != 4 {
		t.Errorf("offset/length = %d/%d, want 2/4", e.Offset, e.Length)
	}
}

func TestHandleDraft_NoParseModeSendsNoEntities(t *testing.T) {
	var gotMessage string
	var hadEntities bool

	api := mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		if req, ok := input.(*tg.MessagesSaveDraftRequest); ok {
			gotMessage = req.Message
			_, hadEntities = req.GetEntities()
		}
		if v, ok := output.(*tg.BoolBox); ok {
			v.Bool = &tg.BoolTrue{}
		}
		return nil
	})

	deps := newTestDeps(newAllowedResolver(), api, config.PermDraft)
	result := handleDraft(context.Background(), deps, draftInput{
		Chat: "@testchat",
		Text: "a **bold** word",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	if gotMessage != "a **bold** word" {
		t.Errorf("message = %q, want the asterisks preserved", gotMessage)
	}
	if hadEntities {
		t.Error("entities were set although no parse_mode was requested")
	}
}

func TestHandleDraft_InvalidParseMode(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermDraft)
	result := handleDraft(context.Background(), deps, draftInput{
		Chat:      "@testchat",
		Text:      "hi",
		ParseMode: "html",
	})
	if !result.IsError {
		t.Fatal("expected an error for an unsupported parse_mode")
	}
	if text := resultText(result); !strings.Contains(text, "parse_mode") {
		t.Errorf("error = %q, want it to mention parse_mode", text)
	}
}

func TestHandleSend_MarkdownSetsEntities(t *testing.T) {
	var gotMessage string
	var gotEntities []tg.MessageEntityClass

	api := mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		if req, ok := input.(*tg.MessagesSendMessageRequest); ok {
			gotMessage = req.Message
			gotEntities, _ = req.GetEntities()
		}
		if v, ok := output.(*tg.UpdatesBox); ok {
			v.Updates = &tg.Updates{}
		}
		return nil
	})

	deps := newTestDeps(newAllowedResolver(), api, config.PermSend)
	result := handleSend(context.Background(), deps, sendInput{
		Chat:      "@testchat",
		Text:      "run `go test`",
		ParseMode: "markdown",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	if gotMessage != "run go test" {
		t.Errorf("message = %q, want %q", gotMessage, "run go test")
	}
	if len(gotEntities) != 1 {
		t.Fatalf("got %d entities, want 1", len(gotEntities))
	}
	if _, ok := gotEntities[0].(*tg.MessageEntityCode); !ok {
		t.Errorf("entity type = %T, want *tg.MessageEntityCode", gotEntities[0])
	}
}

func TestHandleSend_InvalidParseMode(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermSend)
	result := handleSend(context.Background(), deps, sendInput{
		Chat:      "@testchat",
		Text:      "hi",
		ParseMode: "markdownv2",
	})
	if !result.IsError {
		t.Fatal("expected an error for an unsupported parse_mode")
	}
}
