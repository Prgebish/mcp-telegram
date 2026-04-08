package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestHandleMe_APIError(t *testing.T) {
	deps := &Deps{API: errorTgClient("connection failed")}
	result := handleMe(context.Background(), deps)
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if text := resultText(result); !strings.Contains(text, "connection failed") {
		t.Errorf("error text = %q, want to contain 'connection failed'", text)
	}
}

func TestHandleMe_EmptyUsers(t *testing.T) {
	api := mockTgClient(func(_ context.Context, _ bin.Encoder, output bin.Decoder) error {
		v := output.(*tg.UserClassVector)
		v.Elems = nil
		return nil
	})
	deps := &Deps{API: api}
	result := handleMe(context.Background(), deps)
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if text := resultText(result); !strings.Contains(text, "no user info") {
		t.Errorf("error text = %q, want 'no user info'", text)
	}
}

func TestHandleMe_EmptyUser(t *testing.T) {
	api := mockTgClient(func(_ context.Context, _ bin.Encoder, output bin.Decoder) error {
		v := output.(*tg.UserClassVector)
		v.Elems = []tg.UserClass{&tg.UserEmpty{ID: 1}}
		return nil
	})
	deps := &Deps{API: api}
	result := handleMe(context.Background(), deps)
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if text := resultText(result); !strings.Contains(text, "empty user info") {
		t.Errorf("error text = %q, want 'empty user info'", text)
	}
}

func TestHandleMe_FullUser(t *testing.T) {
	api := mockTgClient(func(_ context.Context, _ bin.Encoder, output bin.Decoder) error {
		v := output.(*tg.UserClassVector)
		v.Elems = []tg.UserClass{&tg.User{
			ID:        12345,
			FirstName: "Alice",
			LastName:  "Smith",
			Username:  "alice",
		}}
		return nil
	})
	deps := &Deps{API: api}
	result := handleMe(context.Background(), deps)
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	for _, want := range []string{"ID: 12345", "First name: Alice", "Last name: Smith", "Username: @alice"} {
		if !strings.Contains(text, want) {
			t.Errorf("result = %q, want to contain %q", text, want)
		}
	}
}

func TestHandleMe_MinimalUser(t *testing.T) {
	api := mockTgClient(func(_ context.Context, _ bin.Encoder, output bin.Decoder) error {
		v := output.(*tg.UserClassVector)
		v.Elems = []tg.UserClass{&tg.User{
			ID:        99,
			FirstName: "Bob",
		}}
		return nil
	})
	deps := &Deps{API: api}
	result := handleMe(context.Background(), deps)
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "ID: 99") || !strings.Contains(text, "First name: Bob") {
		t.Errorf("missing expected fields in %q", text)
	}
	if strings.Contains(text, "Last name") {
		t.Error("should not include Last name for user without last name")
	}
	if strings.Contains(text, "Username") {
		t.Error("should not include Username for user without username")
	}
}
