package commands

import "streamer-remote/internal/config"

// keyAliasGroups lets a denylist entry of e.g. "alt" also catch "lalt"/"ralt".
// There is no built-in denylist: by default every key/button is permitted,
// and it's entirely up to the streamer to opt into restrictions via config.
var keyAliasGroups = map[string][]string{
	"alt":   {"alt", "lalt", "ralt"},
	"ctrl":  {"ctrl", "lctrl", "rctrl"},
	"shift": {"shift", "lshift", "rshift"},
	"win":   {"lwin", "rwin"},
}

// canonicalKey maps a specific key name to the generic group a denylist
// rule would refer to (e.g. "lalt" -> "alt"), or returns name unchanged.
func canonicalKey(name string) string {
	for group, variants := range keyAliasGroups {
		for _, v := range variants {
			if v == name {
				return group
			}
		}
	}
	return name
}

type blacklist struct {
	deniedKeys   map[string]bool
	deniedCombos [][]string
}

func buildBlacklist(cfg *config.Config) *blacklist {
	denied := map[string]bool{}
	for _, k := range cfg.Blacklist.DeniedKeys {
		denied[k] = true
	}
	return &blacklist{
		deniedKeys:   denied,
		deniedCombos: cfg.Blacklist.DeniedCombos,
	}
}

// Check inspects the key names involved in a parsed combo and returns a
// human-readable reason if it must be rejected, or "" if it's allowed.
func (b *blacklist) Check(actions []Action) string {
	present := map[string]bool{}
	for _, a := range actions {
		if a.Kind != KindKey {
			continue
		}
		present[a.Name] = true
		present[canonicalKey(a.Name)] = true
	}

	for key := range present {
		if b.deniedKeys[key] {
			return "key '" + key + "' is not allowed"
		}
	}

	for _, combo := range b.deniedCombos {
		allPresent := true
		for _, k := range combo {
			if !present[k] {
				allPresent = false
				break
			}
		}
		if allPresent {
			return "combo is not allowed"
		}
	}

	return ""
}
