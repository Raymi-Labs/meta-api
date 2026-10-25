package common

// ReasoningAdaptationTrace contains only bounded metadata, never request text.
type ReasoningAdaptationTrace struct {
	ChannelID       int    `json:"channel_id"`
	Template        string `json:"template"`
	Protocol        string `json:"protocol"`
	Source          string `json:"source"`
	Original        string `json:"original"`
	Mapped          string `json:"mapped"`
	Final           string `json:"final"`
	FinalPresent    bool   `json:"final_present"`
	OverrideChanged bool   `json:"override_changed"`
}
