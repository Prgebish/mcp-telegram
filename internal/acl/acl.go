package acl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Prgebish/mcp-telegram/internal/config"
)

type PeerKind int

const (
	KindUser PeerKind = iota
	KindChat
	KindChannel
)

type PeerIdentity struct {
	Kind     PeerKind
	ID       int64
	Username string // without @
	Phone    string // with + prefix
}

// TypedRef returns the user:ID, chat:ID or channel:ID reference of the peer,
// or "" when the identity has no ID.
func (p PeerIdentity) TypedRef() string {
	if p.ID == 0 {
		return ""
	}
	switch p.Kind {
	case KindUser:
		return fmt.Sprintf("user:%d", p.ID)
	case KindChat:
		return fmt.Sprintf("chat:%d", p.ID)
	case KindChannel:
		return fmt.Sprintf("channel:%d", p.ID)
	default:
		return ""
	}
}

// Ref returns @username when the peer has one, otherwise TypedRef.
func (p PeerIdentity) Ref() string {
	if p.Username != "" {
		return "@" + p.Username
	}
	return p.TypedRef()
}

// RefForm is the form of a chat reference or ACL matcher: typed ID,
// @username or +phone.
type RefForm int

const (
	RefID RefForm = iota
	RefUsername
	RefPhone
)

// Ref is a chat reference or ACL matcher in canonical form. For RefID it
// holds Kind and ID, for RefUsername the lowercase Username, for RefPhone the
// Phone as + followed by digits only.
type Ref struct {
	Form RefForm
	PeerIdentity
}

// String returns the canonical reference string.
func (r Ref) String() string {
	switch r.Form {
	case RefUsername:
		return "@" + r.Username
	case RefPhone:
		return r.Phone
	default:
		return r.TypedRef()
	}
}

// ParseRef parses a chat reference: @username, +phone, user:ID, chat:ID or
// channel:ID. It is the single grammar shared by ACL matchers, the pre-check
// in MayAllow and the peer resolver, so all three read a reference the same
// way.
func ParseRef(s string) (Ref, error) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "@"):
		name := s[1:]
		if !isPlainUsername(name) {
			return Ref{}, fmt.Errorf("invalid username in %q: only letters, digits and _ are allowed", s)
		}
		return Ref{Form: RefUsername, PeerIdentity: PeerIdentity{Username: strings.ToLower(name)}}, nil

	case strings.HasPrefix(s, "+"):
		phone := normalizePhone(s)
		if phone == "" {
			return Ref{}, fmt.Errorf("invalid phone %q: no digits", s)
		}
		return Ref{Form: RefPhone, PeerIdentity: PeerIdentity{Phone: phone}}, nil
	}
	for _, t := range []struct {
		prefix string
		kind   PeerKind
	}{{"user:", KindUser}, {"chat:", KindChat}, {"channel:", KindChannel}} {
		if strings.HasPrefix(s, t.prefix) {
			id, err := strconv.ParseInt(s[len(t.prefix):], 10, 64)
			if err != nil {
				return Ref{}, fmt.Errorf("invalid %sID in %q: %w", t.prefix, s, err)
			}
			return Ref{Form: RefID, PeerIdentity: PeerIdentity{Kind: t.kind, ID: id}}, nil
		}
	}
	return Ref{}, fmt.Errorf("unknown peer reference format %q: use @username, +phone, user:ID, chat:ID, or channel:ID", s)
}

func isPlainUsername(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

// normalizePhone reduces a phone to + followed by its digits, as the
// resolver does, or "" when it has no digits.
func normalizePhone(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "+" + b.String()
}

type compiledRule struct {
	ref   Ref
	perms map[config.Permission]bool
}

type Checker struct {
	rules []compiledRule
}

func NewChecker(cfg config.ACLConfig) (*Checker, error) {
	rules := make([]compiledRule, 0, len(cfg.Chats))
	for i, chat := range cfg.Chats {
		ref, err := ParseRef(chat.Match)
		if err != nil {
			return nil, fmt.Errorf("acl.chats[%d]: %w", i, err)
		}
		perms := make(map[config.Permission]bool, len(chat.Permissions))
		for _, p := range chat.Permissions {
			perms[p] = true
		}
		rules = append(rules, compiledRule{ref: ref, perms: perms})
	}
	return &Checker{rules: rules}, nil
}

// Allowed checks if peer has the given permission.
// Permissions are merged across all matching rules — if any matching rule
// grants the permission, it is allowed. This avoids shadowing when the same
// peer is referenced by multiple matchers (@username, +phone, user:ID).
func (c *Checker) Allowed(peer PeerIdentity, perm config.Permission) bool {
	for _, rule := range c.rules {
		if rule.ref.matches(peer) && rule.perms[perm] {
			return true
		}
	}
	return false
}

func (c *Checker) MatchesAny(peer PeerIdentity) bool {
	for _, rule := range c.rules {
		if rule.ref.matches(peer) {
			return true
		}
	}
	return false
}

// MayAllow reports whether a chat reference could be granted perm, judging
// by the reference string alone — no Telegram lookup. A false result is
// definitive: no rule can match, so the caller must deny before resolving
// the peer over the network. A true result still requires Allowed on the
// resolved identity.
//
// Only typed IDs and phones are compared with rules of the same form: a
// user:ID or +phone reference resolves to exactly that peer. Everything else
// is decided after resolution: rules of another form (e.g. @username rules
// for a user:ID reference) need the resolved identity, and an @username
// reference may name a secondary or collectible username of a peer whose
// main username is in the ACL.
func (c *Checker) MayAllow(ref string, perm config.Permission) bool {
	r, err := ParseRef(ref)
	if err != nil {
		// Invalid reference: the resolver rejects it without a lookup.
		return true
	}
	for _, rule := range c.rules {
		if !rule.perms[perm] {
			continue
		}
		if r.Form == RefUsername || rule.ref.Form != r.Form || rule.ref.matches(r.PeerIdentity) {
			return true
		}
	}
	return false
}

// matches reports whether the matcher r matches peer.
func (r Ref) matches(p PeerIdentity) bool {
	switch r.Form {
	case RefUsername:
		return strings.EqualFold(p.Username, r.Username)
	case RefPhone:
		return normalizePhone(p.Phone) == r.Phone
	default:
		return p.Kind == r.Kind && p.ID == r.ID
	}
}
