// Package markdown converts a Markdown-formatted string into the plain text
// and message entities that Telegram expects.
//
// Telegram does not accept markup inline: a message carries plain text plus a
// list of entities that point into it by offset and length. Those offsets are
// counted in UTF-16 code units, not bytes and not runes, which is the detail
// most implementations get wrong for emoji and other astral-plane characters.
//
// Supported syntax:
//
//	**bold**        __italic__      ~~strikethrough~~
//	||spoiler||     `code`          ```lang\npre\n```
//	[text](https://example.com)
//
// A backslash escapes the character that follows it, so \** is a literal "**".
// Unterminated markers are treated as literal text rather than an error, which
// matches how Telegram clients behave while a message is being typed.
package markdown

import (
	"strings"
	"unicode/utf16"

	"github.com/gotd/td/tg"
)

// utf16Len returns the number of UTF-16 code units a rune occupies.
// Runes outside the Basic Multilingual Plane need a surrogate pair.
func utf16Len(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	return 1
}

// marker describes an inline delimiter pair.
type marker struct {
	token string
	kind  string
}

// Longest tokens first so "**" is matched before "*" would be.
var markers = []marker{
	{"**", "bold"},
	{"__", "italic"},
	{"~~", "strike"},
	{"||", "spoiler"},
}

// openEntity is a delimiter that has been opened but not yet closed.
type openEntity struct {
	kind   string
	offset int // UTF-16 offset into the plain text
	// textStart is the byte offset in the output builder, used to discard
	// the entity if it turns out to be unterminated.
	tokenLen int
}

// Parse converts markdown into Telegram plain text plus entities.
//
// It never fails on malformed input: anything it cannot interpret is kept as
// literal text. The error return exists for future strictness and is always
// nil today.
func Parse(input string) (string, []tg.MessageEntityClass, error) {
	var out strings.Builder
	var entities []tg.MessageEntityClass
	var stack []openEntity

	offset := 0 // UTF-16 offset into out
	runes := []rune(input)

	writeRunes := func(rs []rune) {
		for _, r := range rs {
			out.WriteRune(r)
			offset += utf16Len(r)
		}
	}

	for i := 0; i < len(runes); {
		r := runes[i]

		// Backslash escape: emit the next rune literally.
		if r == '\\' && i+1 < len(runes) {
			writeRunes(runes[i+1 : i+2])
			i += 2
			continue
		}

		// Fenced pre block. Must be checked before inline code.
		if strings.HasPrefix(string(runes[i:]), "```") {
			if consumed, ok := parseFence(runes[i:], &out, &entities, &offset); ok {
				i += consumed
				continue
			}
		}

		// Inline code. No nested formatting inside.
		if r == '`' {
			if consumed, ok := parseCode(runes[i:], &out, &entities, &offset); ok {
				i += consumed
				continue
			}
		}

		// Inline link.
		if r == '[' {
			if consumed, ok := parseLink(runes[i:], &out, &entities, &offset); ok {
				i += consumed
				continue
			}
		}

		// Paired delimiters.
		if m, ok := matchMarker(runes[i:]); ok {
			tokenRunes := []rune(m.token)
			if idx := findOpen(stack, m.kind); idx >= 0 {
				// Closing delimiter: pop it and everything opened after it,
				// since crossing pairs cannot be represented as entities.
				open := stack[idx]
				stack = stack[:idx]
				if e := buildEntity(open.kind, open.offset, offset-open.offset); e != nil {
					entities = append(entities, e)
				}
			} else {
				stack = append(stack, openEntity{
					kind:     m.kind,
					offset:   offset,
					tokenLen: len(tokenRunes),
				})
			}
			i += len(tokenRunes)
			continue
		}

		writeRunes(runes[i : i+1])
		i++
	}

	// Unterminated delimiters: their opening tokens were swallowed, so put
	// them back as literal text at the position they occupied.
	if len(stack) > 0 {
		text := out.String()
		for idx := len(stack) - 1; idx >= 0; idx-- {
			open := stack[idx]
			token := tokenFor(open.kind)
			byteIdx := byteIndexForUTF16Offset(text, open.offset)
			text = text[:byteIdx] + token + text[byteIdx:]
		}
		// Reinserting literal tokens shifts every entity after them, so the
		// simplest correct answer is to re-parse with escapes in place.
		return text, shiftEntities(entities, stack, text), nil
	}

	return out.String(), entities, nil
}

func matchMarker(runes []rune) (marker, bool) {
	s := string(runes)
	for _, m := range markers {
		if strings.HasPrefix(s, m.token) {
			return m, true
		}
	}
	return marker{}, false
}

func findOpen(stack []openEntity, kind string) int {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].kind == kind {
			return i
		}
	}
	return -1
}

func tokenFor(kind string) string {
	for _, m := range markers {
		if m.kind == kind {
			return m.token
		}
	}
	return ""
}

// parseFence handles ```lang\ncode\n``` and returns how many runes it consumed.
func parseFence(runes []rune, out *strings.Builder, entities *[]tg.MessageEntityClass, offset *int) (int, bool) {
	rest := string(runes[3:])
	end := strings.Index(rest, "```")
	if end < 0 {
		return 0, false
	}

	body := rest[:end]
	lang := ""

	// An optional language tag occupies the remainder of the opening line.
	if nl := strings.IndexByte(body, '\n'); nl >= 0 {
		candidate := strings.TrimSpace(body[:nl])
		if candidate != "" && !strings.ContainsAny(candidate, " \t") {
			lang = candidate
			body = body[nl+1:]
		}
	}
	body = strings.Trim(body, "\n")

	start := *offset
	for _, r := range body {
		out.WriteRune(r)
		*offset += utf16Len(r)
	}

	*entities = append(*entities, &tg.MessageEntityPre{
		Offset:   start,
		Length:   *offset - start,
		Language: lang,
	})

	// 3 opening backticks + the body as originally written + 3 closing.
	return 3 + len([]rune(rest[:end])) + 3, true
}

// parseCode handles `inline code`.
func parseCode(runes []rune, out *strings.Builder, entities *[]tg.MessageEntityClass, offset *int) (int, bool) {
	rest := runes[1:]
	end := -1
	for i, r := range rest {
		if r == '`' {
			end = i
			break
		}
	}
	if end <= 0 {
		return 0, false
	}

	start := *offset
	for _, r := range rest[:end] {
		out.WriteRune(r)
		*offset += utf16Len(r)
	}

	*entities = append(*entities, &tg.MessageEntityCode{
		Offset: start,
		Length: *offset - start,
	})

	return 1 + end + 1, true
}

// parseLink handles [label](url).
func parseLink(runes []rune, out *strings.Builder, entities *[]tg.MessageEntityClass, offset *int) (int, bool) {
	s := string(runes)
	closeBracket := strings.IndexByte(s, ']')
	if closeBracket < 0 || closeBracket+1 >= len(s) || s[closeBracket+1] != '(' {
		return 0, false
	}
	closeParen := strings.IndexByte(s[closeBracket:], ')')
	if closeParen < 0 {
		return 0, false
	}
	closeParen += closeBracket

	label := s[1:closeBracket]
	url := s[closeBracket+2 : closeParen]
	if label == "" || url == "" {
		return 0, false
	}

	start := *offset
	for _, r := range label {
		out.WriteRune(r)
		*offset += utf16Len(r)
	}

	*entities = append(*entities, &tg.MessageEntityTextURL{
		Offset: start,
		Length: *offset - start,
		URL:    url,
	})

	return len([]rune(s[:closeParen+1])), true
}

func buildEntity(kind string, offset, length int) tg.MessageEntityClass {
	if length <= 0 {
		return nil
	}
	switch kind {
	case "bold":
		return &tg.MessageEntityBold{Offset: offset, Length: length}
	case "italic":
		return &tg.MessageEntityItalic{Offset: offset, Length: length}
	case "strike":
		return &tg.MessageEntityStrike{Offset: offset, Length: length}
	case "spoiler":
		return &tg.MessageEntitySpoiler{Offset: offset, Length: length}
	}
	return nil
}

// byteIndexForUTF16Offset maps a UTF-16 offset back to a byte index in s.
func byteIndexForUTF16Offset(s string, target int) int {
	units := 0
	for i, r := range s {
		if units >= target {
			return i
		}
		units += utf16Len(r)
	}
	return len(s)
}

// shiftEntities moves entities that sit after a reinserted literal token.
func shiftEntities(entities []tg.MessageEntityClass, reinserted []openEntity, _ string) []tg.MessageEntityClass {
	for _, open := range reinserted {
		shift := len(utf16.Encode([]rune(tokenFor(open.kind))))
		for _, e := range entities {
			shiftEntity(e, open.offset, shift)
		}
	}
	return entities
}

func shiftEntity(e tg.MessageEntityClass, from, by int) {
	switch v := e.(type) {
	case *tg.MessageEntityBold:
		if v.Offset >= from {
			v.Offset += by
		}
	case *tg.MessageEntityItalic:
		if v.Offset >= from {
			v.Offset += by
		}
	case *tg.MessageEntityStrike:
		if v.Offset >= from {
			v.Offset += by
		}
	case *tg.MessageEntitySpoiler:
		if v.Offset >= from {
			v.Offset += by
		}
	case *tg.MessageEntityCode:
		if v.Offset >= from {
			v.Offset += by
		}
	case *tg.MessageEntityPre:
		if v.Offset >= from {
			v.Offset += by
		}
	case *tg.MessageEntityTextURL:
		if v.Offset >= from {
			v.Offset += by
		}
	}
}
