package tools

import (
	"testing"
)

func TestToolError(t *testing.T) {
	result := toolError("something went wrong")
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
}

func TestPtrBool(t *testing.T) {
	v := ptrBool(true)
	if v == nil || *v != true {
		t.Error("ptrBool(true) should return pointer to true")
	}
	v = ptrBool(false)
	if v == nil || *v != false {
		t.Error("ptrBool(false) should return pointer to false")
	}
}
