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
	Channel  string `yaml:"channel" json:"channel"`
	ClientID string `yaml:"clientId" json:"clientId"`
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
	DeniedKeys   []string   `yaml:"deniedKeys" json:"deniedKeys"`
	DeniedCombos [][]string `yaml:"deniedCombos" json:"deniedCombos"`
}

// RewardAction gates one action (same syntax as a chat combo body, e.g.
// "lwin" or "alt+f4") behind a Twitch Channel Points reward: viewers can
// only trigger it by redeeming RewardTitle, never by typing the command in
// chat. RewardID is filled in automatically once the app creates the
// reward on Twitch; streamers manage these through the app's menu rather
// than hand-editing this section.
type RewardAction struct {
	Action      string `yaml:"action" json:"action"`
	RewardTitle string `yaml:"rewardTitle" json:"rewardTitle"`
	Cost        int    `yaml:"cost" json:"cost"`
	RewardID    string `yaml:"rewardId" json:"rewardId"`
}

// RewardProfile is a named, saved set of reward actions the streamer can
// switch to as a group. Rewards here are templates (no RewardID): IDs only
// exist for whichever set is actually live on Twitch at a given moment.
type RewardProfile struct {
	Name    string         `yaml:"name" json:"name"`
	Color   string         `yaml:"color,omitempty" json:"color,omitempty"`
	Rewards []RewardAction `yaml:"rewards" json:"rewards"`
}

type Config struct {
	Twitch Twitch `yaml:"twitch" json:"twitch"`

	Prefix      string `yaml:"prefix" json:"prefix"`
	ModOnlyMode bool   `yaml:"modOnlyMode" json:"modOnlyMode"`

	TextToSpeechEnabled bool `yaml:"textToSpeechEnabled" json:"textToSpeechEnabled"`

	GlobalCooldownMs  int `yaml:"globalCooldownMs" json:"globalCooldownMs"`
	PerUserCooldownMs int `yaml:"perUserCooldownMs" json:"perUserCooldownMs"`

	MaxComboSize     int `yaml:"maxComboSize" json:"maxComboSize"`
	MaxSequenceSteps int `yaml:"maxSequenceSteps" json:"maxSequenceSteps"`
	TapHoldMs        int `yaml:"tapHoldMs" json:"tapHoldMs"`
	MaxHoldMs        int `yaml:"maxHoldMs" json:"maxHoldMs"`
	MaxMoveStep      int `yaml:"maxMoveStep" json:"maxMoveStep"`

	Blacklist     Blacklist      `yaml:"blacklist" json:"blacklist"`
	RewardActions []RewardAction `yaml:"rewardActions" json:"rewardActions"`

	// RewardProfiles are saved sets of reward actions the streamer can
	// switch between; RewardActions above is always whichever set is
	// currently live on Twitch. ActiveRewardProfile names the profile that
	// produced it, or "" if RewardActions was built by hand instead.
	RewardProfiles      []RewardProfile `yaml:"rewardProfiles" json:"rewardProfiles"`
	ActiveRewardProfile string          `yaml:"activeRewardProfile" json:"activeRewardProfile"`

	LogDebug bool `yaml:"logDebug" json:"logDebug"`
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

textToSpeechEnabled: true # when true, chat messages starting with rc-say: are spoken aloud

globalCooldownMs: 150     # minimum time between any two accepted commands
perUserCooldownMs: 1500   # minimum time between commands from the same viewer

maxComboSize: 3           # max number of keys/buttons chained with '+' in one command
maxSequenceSteps: 6       # max comma-separated steps in one command, e.g. alt+f10,wait:800,enter
tapHoldMs: 40             # how long a tapped key/button is held down, in ms
maxHoldMs: 3000           # upper bound for any explicit hold or wait duration, in ms
maxMoveStep: 300          # upper bound for a single mouse-move command's distance on either axis, in pixels

blacklist:
  deniedKeys: []          # extra keys to block, beyond the built-in unsafe ones
  deniedCombos: []        # extra key combos to block, e.g. [["ctrl", "w"]]

rewardActions: []         # actions only redeemable via Channel Points, never by typing in chat
                          # managed from the dashboard's Rewards tab, not by hand

rewardProfiles: []        # saved sets of reward actions to switch between as a group
                          # managed from the dashboard's Rewards tab, not by hand
activeRewardProfile: ""   # name of the rewardProfiles entry currently live on Twitch, if any

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
	textToSpeechConfigured := yamlHasTopLevelKey(data, "textToSpeechEnabled")
	cfg.applyDefaults()
	if !textToSpeechConfigured {
		cfg.TextToSpeechEnabled = true
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: invalid %s: %w", path, err)
	}
	return cfg, nil
}

// applyDefaults fills in fields that are absent from an older config.yaml
// (and so unmarshal to their zero value) but aren't allowed to be zero.
// Without this, every field added to Config after its first release would
// need every existing install's config.yaml hand-edited before the app
// could start again — exactly the kind of upgrade breakage this guards
// against. Fields where zero is a legitimate value (the cooldowns) are
// deliberately left alone.
func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Prefix) == "" {
		c.Prefix = "rc!"
	}
	if c.MaxComboSize == 0 {
		c.MaxComboSize = 3
	}
	if c.MaxSequenceSteps == 0 {
		c.MaxSequenceSteps = 6
	}
	if c.TapHoldMs == 0 {
		c.TapHoldMs = 40
	}
	if c.MaxHoldMs == 0 {
		c.MaxHoldMs = 3000
	}
	if c.MaxMoveStep == 0 {
		c.MaxMoveStep = 300
	}
}

// Save validates and writes the full config back to path. Used by the
// dashboard's Settings tab, which edits the in-memory Config wholesale;
// unlike UpdateTwitchFields/AddRewardAction/RemoveRewardAction, this does
// not preserve comments, since once a streamer is using the dashboard to
// manage settings there's no hand-written YAML left to protect.
func (c *Config) Save(path string) error {
	if err := c.validate(); err != nil {
		return fmt.Errorf("config: invalid settings: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
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
	doc, root, err := loadYAMLDoc(path)
	if err != nil {
		return err
	}

	twitch := yamlMapValue(root, "twitch")
	if twitch == nil {
		twitch = &yaml.Node{Kind: yaml.MappingNode}
		root.Content = append(root.Content, yamlScalar("twitch"), twitch)
	}
	yamlSetMapString(twitch, "channel", channel)
	yamlSetMapString(twitch, "clientId", clientID)

	return saveYAMLDoc(path, doc)
}

// AddRewardAction appends a new entry to the rewardActions list, creating
// the list if it doesn't exist yet. Used once the app has created the
// corresponding reward on Twitch and knows its RewardID.
func AddRewardAction(path string, ra RewardAction) error {
	doc, root, err := loadYAMLDoc(path)
	if err != nil {
		return err
	}

	seq := yamlMapValue(root, "rewardActions")
	if seq == nil || seq.Kind != yaml.SequenceNode {
		seq = &yaml.Node{Kind: yaml.SequenceNode}
		root.Content = append(root.Content, yamlScalar("rewardActions"), seq)
	}

	item := &yaml.Node{Kind: yaml.MappingNode}
	yamlSetMapString(item, "action", ra.Action)
	yamlSetMapString(item, "rewardTitle", ra.RewardTitle)
	yamlSetMapInt(item, "cost", ra.Cost)
	yamlSetMapString(item, "rewardId", ra.RewardID)
	seq.Content = append(seq.Content, item)

	return saveYAMLDoc(path, doc)
}

// RemoveRewardAction deletes the rewardActions entry with the given
// RewardID, if present. No-op if it's already gone.
func RemoveRewardAction(path, rewardID string) error {
	doc, root, err := loadYAMLDoc(path)
	if err != nil {
		return err
	}

	seq := yamlMapValue(root, "rewardActions")
	if seq == nil {
		return nil
	}
	kept := seq.Content[:0]
	for _, item := range seq.Content {
		if item.Kind == yaml.MappingNode {
			if v := yamlMapValue(item, "rewardId"); v != nil && v.Value == rewardID {
				continue
			}
		}
		kept = append(kept, item)
	}
	seq.Content = kept

	return saveYAMLDoc(path, doc)
}

// ClearRewardActions empties the rewardActions list, keeping the rest of
// the file intact. Used when switching reward profiles: the previously
// live rewards are torn down on Twitch first, then this clears their
// config entries before the newly selected profile's rewards are added.
func ClearRewardActions(path string) error {
	doc, root, err := loadYAMLDoc(path)
	if err != nil {
		return err
	}
	if seq := yamlMapValue(root, "rewardActions"); seq != nil {
		seq.Content = nil
	}
	return saveYAMLDoc(path, doc)
}

// SaveRewardProfile adds or, if a profile with the same Name already
// exists, replaces it in rewardProfiles.
func SaveRewardProfile(path string, profile RewardProfile) error {
	doc, root, err := loadYAMLDoc(path)
	if err != nil {
		return err
	}

	seq := yamlMapValue(root, "rewardProfiles")
	if seq == nil || seq.Kind != yaml.SequenceNode {
		seq = &yaml.Node{Kind: yaml.SequenceNode}
		root.Content = append(root.Content, yamlScalar("rewardProfiles"), seq)
	}

	item := &yaml.Node{}
	if err := item.Encode(profile); err != nil {
		return fmt.Errorf("config: encode reward profile: %w", err)
	}
	for i, existing := range seq.Content {
		if v := yamlMapValue(existing, "name"); v != nil && v.Value == profile.Name {
			seq.Content[i] = item
			return saveYAMLDoc(path, doc)
		}
	}
	seq.Content = append(seq.Content, item)

	return saveYAMLDoc(path, doc)
}

// DeleteRewardProfile removes the rewardProfiles entry with the given
// name, if present, and clears activeRewardProfile if it pointed at it.
// No-op if the name is already gone.
func DeleteRewardProfile(path, name string) error {
	doc, root, err := loadYAMLDoc(path)
	if err != nil {
		return err
	}

	if seq := yamlMapValue(root, "rewardProfiles"); seq != nil {
		kept := seq.Content[:0]
		for _, item := range seq.Content {
			if item.Kind == yaml.MappingNode {
				if v := yamlMapValue(item, "name"); v != nil && v.Value == name {
					continue
				}
			}
			kept = append(kept, item)
		}
		seq.Content = kept
	}
	if active := yamlMapValue(root, "activeRewardProfile"); active != nil && active.Value == name {
		yamlSetMapString(root, "activeRewardProfile", "")
	}

	return saveYAMLDoc(path, doc)
}

// SetActiveRewardProfile records which rewardProfiles entry is currently
// live on Twitch, purely for display; it does not itself change any
// rewards.
func SetActiveRewardProfile(path, name string) error {
	doc, root, err := loadYAMLDoc(path)
	if err != nil {
		return err
	}
	yamlSetMapString(root, "activeRewardProfile", name)
	return saveYAMLDoc(path, doc)
}

func loadYAMLDoc(path string) (doc yaml.Node, root *yaml.Node, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return doc, nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return doc, nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return doc, nil, fmt.Errorf("config: %s is not a valid YAML mapping", path)
	}
	return doc, doc.Content[0], nil
}

func saveYAMLDoc(path string, doc yaml.Node) error {
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

// yamlSetMapInt sets key to an integer value in a YAML mapping node,
// adding the key if it isn't already present.
func yamlSetMapInt(mapNode *yaml.Node, key string, value int) {
	valNode := &yaml.Node{}
	_ = valNode.Encode(value)
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			mapNode.Content[i+1] = valNode
			return
		}
	}
	mapNode.Content = append(mapNode.Content, yamlScalar(key), valNode)
}

func yamlScalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
}

func yamlHasTopLevelKey(data []byte, key string) bool {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return false
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return false
	}
	return yamlMapValue(root, key) != nil
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Prefix) == "" {
		return fmt.Errorf("prefix must not be empty")
	}
	if c.GlobalCooldownMs < 0 || c.PerUserCooldownMs < 0 {
		return fmt.Errorf("cooldowns must not be negative")
	}
	if c.MaxComboSize < 1 || c.MaxComboSize > 20 {
		return fmt.Errorf("maxComboSize must be between 1 and 20")
	}
	if c.MaxSequenceSteps < 1 || c.MaxSequenceSteps > 20 {
		return fmt.Errorf("maxSequenceSteps must be between 1 and 20")
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
