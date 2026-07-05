package commands

import "strings"

// Permission ranks a chatter's privilege level, derived from their Twitch
// badges. Higher values can do everything lower values can.
type Permission int

const (
	Everyone Permission = iota
	Subscriber
	VIP
	Moderator
	Broadcaster
)

// PermissionFromBadges derives the highest permission level implied by a
// chatter's IRC badge tags (as parsed from the `badges=` tag).
func PermissionFromBadges(badges map[string]bool) Permission {
	switch {
	case badges["broadcaster"]:
		return Broadcaster
	case badges["moderator"]:
		return Moderator
	case badges["vip"]:
		return VIP
	case badges["subscriber"] || badges["founder"]:
		return Subscriber
	default:
		return Everyone
	}
}

// ParsePermission parses a permission name (used by the local test
// console to simulate chatters of different rank).
func ParsePermission(s string) (Permission, bool) {
	switch strings.ToLower(s) {
	case "everyone":
		return Everyone, true
	case "subscriber":
		return Subscriber, true
	case "vip":
		return VIP, true
	case "moderator", "mod":
		return Moderator, true
	case "broadcaster":
		return Broadcaster, true
	default:
		return Everyone, false
	}
}

func (p Permission) String() string {
	switch p {
	case Everyone:
		return "everyone"
	case Subscriber:
		return "subscriber"
	case VIP:
		return "vip"
	case Moderator:
		return "moderator"
	case Broadcaster:
		return "broadcaster"
	default:
		return "unknown"
	}
}
