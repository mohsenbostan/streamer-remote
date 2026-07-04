// Package config loads and validates the application's YAML configuration,
// creating a documented default file on first run.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Twitch struct {
	Channel  string `yaml:"channel"`
	ClientID string `yaml:"clientId"`
}

// Validate checks the fields required to actually connect to Twitch. It is
// skipped in local-only test mode, where no Twitch connection is made.
func (t Twitch) Validate() error {
	if strings.TrimSpace(t.Channel) == "" {
		return fmt.Errorf("twitch.channel is required")
	}
	if strings.TrimSpace(t.ClientID) == "" {
		return fmt.Errorf("twitch.clientId is required (register a free app at https://dev.twitch.tv/console/apps)")
	}
	return nil
}

// Blacklist holds streamer-added restrictions on top of the hardcoded base
// denylist in the commands package (which can never be relaxed via config).
type Blacklist struct {
	DeniedKeys   []string   `yaml:"deniedKeys"`
	DeniedCombos [][]string `yaml:"deniedCombos"`
}

type Config struct {
	Twitch Twitch `yaml:"twitch"`

	Prefix      string `yaml:"prefix"`
	ModOnlyMode bool   `yaml:"modOnlyMode"`

	GlobalCooldownMs  int `yaml:"globalCooldownMs"`
	PerUserCooldownMs int `yaml:"perUserCooldownMs"`

	MaxComboSize int `yaml:"maxComboSize"`
	TapHoldMs    int `yaml:"tapHoldMs"`
	MaxHoldMs    int `yaml:"maxHoldMs"`
	MaxMoveStep  int `yaml:"maxMoveStep"`

	Blacklist Blacklist `yaml:"blacklist"`

	LogDebug bool `yaml:"logDebug"`
}

const defaultConfigTemplate = `# streamer-remote configuration.
# After editing, restart the app to apply changes.

twitch:
  channel: ""            # your Twitch channel name, lowercase, no '#'
  clientId: ""            # Client ID of a free app you register at https://dev.twitch.tv/console/apps
                          # (Category: "Chat Bot", Client Type: "Public", OAuth Redirect URL: "http://localhost")
                          # the app will ask you to log in once and remembers you after that

prefix: "rc!"              # viewers trigger commands with e.g. rc!w or rc!w+shift
                          # kept unusual on purpose so it won't collide with Nightbot/StreamElements/Moobot commands

modOnlyMode: false        # when true, only moderators/broadcaster can send commands

globalCooldownMs: 150     # minimum time between any two accepted commands
perUserCooldownMs: 1500   # minimum time between commands from the same viewer

maxComboSize: 3           # max number of keys/buttons chained with '+' in one command
tapHoldMs: 40             # how long a tapped key/button is held down, in ms
maxHoldMs: 3000           # upper bound for any explicit hold duration, in ms
maxMoveStep: 300          # upper bound for a single mouse-move command, in pixels

blacklist:
  deniedKeys: []          # extra keys to block, beyond the built-in unsafe ones
  deniedCombos: []        # extra key combos to block, e.g. [["ctrl", "w"]]

logDebug: false           # verbose logging for troubleshooting
`

// Load reads the config at path, creating a default file if none exists.
// When a default file is created, it returns ErrDefaultCreated so the
// caller can prompt the operator to fill it in before continuing.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if writeErr := os.WriteFile(path, []byte(defaultConfigTemplate), 0o600); writeErr != nil {
			return nil, fmt.Errorf("config: create default at %s: %w", path, writeErr)
		}
		return nil, ErrDefaultCreated
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: invalid %s: %w", path, err)
	}
	return cfg, nil
}

// ErrDefaultCreated signals that no config existed and a template was
// written; it is not a failure, just a "come back after editing" signal.
var ErrDefaultCreated = fmt.Errorf("config: default file created, edit it and restart")

// UpdateTwitchFields sets twitch.channel and twitch.clientId in place,
// preserving the rest of the file (comments included) via YAML's node
// tree rather than text matching. That matters for backward compatibility:
// a config.yaml from an older release may have these keys missing,
// reordered, or differently indented, and a naive line-matching rewrite
// would silently fail to persist anything, leaving setup stuck in a loop.
// Missing keys are added rather than requiring an exact match.
func UpdateTwitchFields(path, channel, clientID string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: read %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config: %s is not a valid YAML mapping", path)
	}
	root := doc.Content[0]

	twitch := yamlMapValue(root, "twitch")
	if twitch == nil {
		twitch = &yaml.Node{Kind: yaml.MappingNode}
		root.Content = append(root.Content, yamlScalar("twitch"), twitch)
	}
	yamlSetMapString(twitch, "channel", channel)
	yamlSetMapString(twitch, "clientId", clientID)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("config: encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}

// yamlMapValue returns the value node for key in a YAML mapping node, or
// nil if absent.
func yamlMapValue(mapNode *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			return mapNode.Content[i+1]
		}
	}
	return nil
}

// yamlSetMapString sets key to a string value in a YAML mapping node,
// adding the key if it isn't already present.
func yamlSetMapString(mapNode *yaml.Node, key, value string) {
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			mapNode.Content[i+1].SetString(value)
			return
		}
	}
	valNode := &yaml.Node{}
	valNode.SetString(value)
	mapNode.Content = append(mapNode.Content, yamlScalar(key), valNode)
}

func yamlScalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Prefix) == "" {
		return fmt.Errorf("prefix must not be empty")
	}
	if c.GlobalCooldownMs < 0 || c.PerUserCooldownMs < 0 {
		return fmt.Errorf("cooldowns must not be negative")
	}
	if c.MaxComboSize < 1 || c.MaxComboSize > 6 {
		return fmt.Errorf("maxComboSize must be between 1 and 6")
	}
	if c.TapHoldMs <= 0 || c.TapHoldMs > 2000 {
		return fmt.Errorf("tapHoldMs must be between 1 and 2000")
	}
	if c.MaxHoldMs <= 0 || c.MaxHoldMs > 10000 {
		return fmt.Errorf("maxHoldMs must be between 1 and 10000")
	}
	if c.MaxMoveStep <= 0 || c.MaxMoveStep > 2000 {
		return fmt.Errorf("maxMoveStep must be between 1 and 2000")
	}
	return nil
}
