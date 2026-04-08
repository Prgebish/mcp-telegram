package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestHandleDraft_ResolveError(t *testing.T) {
	deps := newTestDeps(newErrorResolver("peer not found"), nil, config.PermDraft)
	result := handleDraft(context.Background(), deps, draftInput{Chat: "@unknown", Text: "hi"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "peer not found") {
		t.Errorf("error = %q, want 'peer not found'", text)
	}
}

func TestHandleDraft_ACLDenied(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), nil, config.PermRead) // no draft perm
	result := handleDraft(context.Background(), deps, draftInput{Chat: "@testchat", Text: "hi"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "access denied") {
		t.Errorf("error = %q, want 'access denied'", text)
	}
}

func TestHandleDraft_APIError(t *testing.T) {
	deps := newTestDeps(newAllowedResolver(), errorTgClient("rpc error"), config.PermDraft)
	result := handleDraft(context.Background(), deps, draftInput{Chat: "@testchat", Text: "hi"})
	if !result.IsError {
		t.Fatal("expected error")
	}
	if text := resultText(result); !strings.Contains(text, "rpc error") {
		t.Errorf("error = %q, want 'rpc error'", text)
	}
}

func TestHandleDraft_Success(t *testing.T) {
	api := mockTgClient(func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		if req, ok := input.(*tg.MessagesSaveDraftRequest); ok {
			if req.Message != "hello world" {
				t.Errorf("draft message = %q, want %q", req.Message, "hello world")
			}
		}
		if v, ok := output.(*tg.BoolBox); ok {
			v.Bool = &tg.BoolTrue{}
		}
		return nil
	})
	deps := newTestDeps(newAllowedResolver(), api, config.PermDraft)
	result := handleDraft(context.Background(), deps, draftInput{Chat: "@testchat", Text: "hello world"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	if text := resultText(result); !strings.Contains(text, "Draft saved in @testchat") {
		t.Errorf("result = %q, want 'Draft saved in @testchat'", text)
	}
}
