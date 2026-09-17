package markdown

import (
	"testing"

	"github.com/gotd/td/tg"
)

func parse(t *testing.T, in string) (string, []tg.MessageEntityClass) {
	t.Helper()
	text, entities, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", in, err)
	}
	return text, entities
}

func TestPlainTextPassesThrough(t *testing.T) {
	text, entities := parse(t, "just a message")

	if text != "just a message" {
		t.Errorf("text = %q, want %q", text, "just a message")
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0", len(entities))
	}
}

func TestBold(t *testing.T) {
	text, entities := parse(t, "a **bold** word")

	if text != "a bold word" {
		t.Errorf("text = %q, want %q", text, "a bold word")
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1", len(entities))
	}
	e, ok := entities[0].(*tg.MessageEntityBold)
	if !ok {
		t.Fatalf("entity type = %T, want *tg.MessageEntityBold", entities[0])
	}
	if e.Offset != 2 || e.Length != 4 {
		t.Errorf("offset/length = %d/%d, want 2/4", e.Offset, e.Length)
	}
}

func TestItalicStrikeSpoiler(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{"__it__", &tg.MessageEntityItalic{}},
		{"~~st~~", &tg.MessageEntityStrike{}},
		{"||sp||", &tg.MessageEntitySpoiler{}},
	}

	for _, tc := range cases {
		text, entities := parse(t, tc.in)
		if len(text) != 2 {
			t.Errorf("Parse(%q) text = %q, want 2 characters", tc.in, text)
		}
		if len(entities) != 1 {
			t.Fatalf("Parse(%q) got %d entities, want 1", tc.in, len(entities))
		}
		if got, want := typeName(entities[0]), typeName(tc.want.(tg.MessageEntityClass)); got != want {
			t.Errorf("Parse(%q) entity = %s, want %s", tc.in, got, want)
		}
	}
}

func TestInlineCode(t *testing.T) {
	text, entities := parse(t, "run `go test` now")

	if text != "run go test now" {
		t.Errorf("text = %q, want %q", text, "run go test now")
	}
	e, ok := entities[0].(*tg.MessageEntityCode)
	if !ok {
		t.Fatalf("entity type = %T, want *tg.MessageEntityCode", entities[0])
	}
	if e.Offset != 4 || e.Length != 7 {
		t.Errorf("offset/length = %d/%d, want 4/7", e.Offset, e.Length)
	}
}

func TestCodeDoesNotParseNestedMarkup(t *testing.T) {
	// A merchant id like a_b_c must survive verbatim inside code.
	text, entities := parse(t, "`a **b** c`")

	if text != "a **b** c" {
		t.Errorf("text = %q, want %q", text, "a **b** c")
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1 (code only)", len(entities))
	}
	if _, ok := entities[0].(*tg.MessageEntityCode); !ok {
		t.Errorf("entity type = %T, want *tg.MessageEntityCode", entities[0])
	}
}

func TestFencedPreWithLanguage(t *testing.T) {
	text, entities := parse(t, "```go\nfmt.Println()\n```")

	if text != "fmt.Println()" {
		t.Errorf("text = %q, want %q", text, "fmt.Println()")
	}
	e, ok := entities[0].(*tg.MessageEntityPre)
	if !ok {
		t.Fatalf("entity type = %T, want *tg.MessageEntityPre", entities[0])
	}
	if e.Language != "go" {
		t.Errorf("language = %q, want %q", e.Language, "go")
	}
	if e.Offset != 0 || e.Length != 13 {
		t.Errorf("offset/length = %d/%d, want 0/13", e.Offset, e.Length)
	}
}

func TestFencedPreWithoutLanguage(t *testing.T) {
	text, entities := parse(t, "```\nplain\n```")

	if text != "plain" {
		t.Errorf("text = %q, want %q", text, "plain")
	}
	e := entities[0].(*tg.MessageEntityPre)
	if e.Language != "" {
		t.Errorf("language = %q, want empty", e.Language)
	}
}

func TestLink(t *testing.T) {
	text, entities := parse(t, "see [docs](https://example.com) please")

	if text != "see docs please" {
		t.Errorf("text = %q, want %q", text, "see docs please")
	}
	e, ok := entities[0].(*tg.MessageEntityTextURL)
	if !ok {
		t.Fatalf("entity type = %T, want *tg.MessageEntityTextURL", entities[0])
	}
	if e.URL != "https://example.com" {
		t.Errorf("url = %q, want %q", e.URL, "https://example.com")
	}
	if e.Offset != 4 || e.Length != 4 {
		t.Errorf("offset/length = %d/%d, want 4/4", e.Offset, e.Length)
	}
}

func TestBackslashEscape(t *testing.T) {
	text, entities := parse(t, `\**not bold\**`)

	if text != "**not bold**" {
		t.Errorf("text = %q, want %q", text, "**not bold**")
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0", len(entities))
	}
}

// Telegram counts entity offsets in UTF-16 code units. An emoji outside the
// BMP occupies two, so a naive rune or byte count puts the entity in the
// wrong place for everything that follows it.
func TestOffsetsAreUTF16CodeUnits(t *testing.T) {
	text, entities := parse(t, "🎉 **party**")

	if text != "🎉 party" {
		t.Errorf("text = %q, want %q", text, "🎉 party")
	}
	e := entities[0].(*tg.MessageEntityBold)
	// 🎉 is a surrogate pair (2 units) plus a space (1) = offset 3.
	if e.Offset != 3 {
		t.Errorf("offset = %d, want 3 (surrogate pair counts as two units)", e.Offset)
	}
	if e.Length != 5 {
		t.Errorf("length = %d, want 5", e.Length)
	}
}

func TestOffsetsWithEmojiInsideEntity(t *testing.T) {
	_, entities := parse(t, "**a🎉b**")

	e := entities[0].(*tg.MessageEntityBold)
	if e.Offset != 0 {
		t.Errorf("offset = %d, want 0", e.Offset)
	}
	// a(1) + surrogate pair(2) + b(1) = 4
	if e.Length != 4 {
		t.Errorf("length = %d, want 4", e.Length)
	}
}

func TestGermanUmlautsCountAsOneUnit(t *testing.T) {
	_, entities := parse(t, "Händler **Süd**")

	e := entities[0].(*tg.MessageEntityBold)
	// "Händler " is 8 BMP characters regardless of its 9-byte UTF-8 encoding.
	if e.Offset != 8 {
		t.Errorf("offset = %d, want 8 (bytes must not leak into offsets)", e.Offset)
	}
}

func TestUnterminatedMarkerStaysLiteral(t *testing.T) {
	text, entities := parse(t, "a ** dangling")

	if text != "a ** dangling" {
		t.Errorf("text = %q, want %q", text, "a ** dangling")
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0", len(entities))
	}
}

func TestUnterminatedCodeStaysLiteral(t *testing.T) {
	text, entities := parse(t, "a ` dangling")

	if text != "a ` dangling" {
		t.Errorf("text = %q, want %q", text, "a ` dangling")
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0", len(entities))
	}
}

func TestMultipleEntities(t *testing.T) {
	text, entities := parse(t, "**one** and `two` and __three__")

	if text != "one and two and three" {
		t.Errorf("text = %q, want %q", text, "one and two and three")
	}
	if len(entities) != 3 {
		t.Fatalf("got %d entities, want 3", len(entities))
	}
}

func TestNestedBoldItalic(t *testing.T) {
	text, entities := parse(t, "**bold __both__**")

	if text != "bold both" {
		t.Errorf("text = %q, want %q", text, "bold both")
	}
	if len(entities) != 2 {
		t.Fatalf("got %d entities, want 2", len(entities))
	}
}

func TestEmptyInput(t *testing.T) {
	text, entities := parse(t, "")

	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0", len(entities))
	}
}

func TestEmptyEntityIsDropped(t *testing.T) {
	text, entities := parse(t, "****")

	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0 (zero-length entities are invalid)", len(entities))
	}
}

func typeName(e tg.MessageEntityClass) string {
	switch e.(type) {
	case *tg.MessageEntityBold:
		return "bold"
	case *tg.MessageEntityItalic:
		return "italic"
	case *tg.MessageEntityStrike:
		return "strike"
	case *tg.MessageEntitySpoiler:
		return "spoiler"
	case *tg.MessageEntityCode:
		return "code"
	case *tg.MessageEntityPre:
		return "pre"
	case *tg.MessageEntityTextURL:
		return "texturl"
	}
	return "unknown"
}
