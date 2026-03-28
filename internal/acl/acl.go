package acl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chestnykh/mcp-telegram/internal/config"
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

type compiledRule struct {
	matcher func(PeerIdentity) bool
	perms   map[config.Permission]bool
}

type Checker struct {
	rules []compiledRule
}

func NewChecker(cfg config.ACLConfig) (*Checker, error) {
	rules := make([]compiledRule, 0, len(cfg.Chats))
	for i, chat := range cfg.Chats {
		matcher, err := compileMatcher(chat.Match)
		if err != nil {
			return nil, fmt.Errorf("acl.chats[%d]: %w", i, err)
		}
		perms := make(map[config.Permission]bool, len(chat.Permissions))
		for _, p := range chat.Permissions {
			perms[p] = true
		}
		rules = append(rules, compiledRule{matcher: matcher, perms: perms})
	}
	return &Checker{rules: rules}, nil
}

func (c *Checker) Allowed(peer PeerIdentity, perm config.Permission) bool {
	for _, rule := range c.rules {
		if rule.matcher(peer) {
			return rule.perms[perm]
		}
	}
	return false
}

func (c *Checker) MatchesAny(peer PeerIdentity) bool {
	for _, rule := range c.rules {
		if rule.matcher(peer) {
			return true
		}
	}
	return false
}

func compileMatcher(match string) (func(PeerIdentity) bool, error) {
	switch {
	case strings.HasPrefix(match, "@"):
		username := strings.ToLower(match[1:])
		return func(p PeerIdentity) bool {
			return strings.EqualFold(p.Username, username)
		}, nil

	case strings.HasPrefix(match, "+"):
		phone := match
		return func(p PeerIdentity) bool {
			return p.Phone == phone
		}, nil

	case strings.HasPrefix(match, "user:"):
		id, err := strconv.ParseInt(match[5:], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user ID in %q: %w", match, err)
		}
		return func(p PeerIdentity) bool {
			return p.Kind == KindUser && p.ID == id
		}, nil

	case strings.HasPrefix(match, "chat:"):
		id, err := strconv.ParseInt(match[5:], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid chat ID in %q: %w", match, err)
		}
		return func(p PeerIdentity) bool {
			return p.Kind == KindChat && p.ID == id
		}, nil

	case strings.HasPrefix(match, "channel:"):
		id, err := strconv.ParseInt(match[8:], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid channel ID in %q: %w", match, err)
		}
		return func(p PeerIdentity) bool {
			return p.Kind == KindChannel && p.ID == id
		}, nil

	default:
		return nil, fmt.Errorf("unknown match format %q", match)
	}
}
