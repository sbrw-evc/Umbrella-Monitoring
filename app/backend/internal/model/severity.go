package model

const (
	SeverityCritical = "critical"
	SeverityError    = "error"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// Severity is a level of the alert scale. Rank orders the levels (higher is more severe,
// unknown ones rank 0); Tone is the colour the interface shows the level in.
type Severity struct {
	Name string `json:"name"`
	Rank int    `json:"rank"`
	Tone string `json:"tone"`
}

// Severities is the alert scale, most severe first: the engine, the wallboards, the catalog
// map and the interface all derive their order, counters and colours from it.
var Severities = []Severity{
	{Name: SeverityCritical, Rank: 4, Tone: "critical"},
	{Name: SeverityError, Rank: 3, Tone: "error"},
	{Name: SeverityWarning, Rank: 2, Tone: "warn"},
	{Name: SeverityInfo, Rank: 1, Tone: "info"},
}

// SeverityNames lists the names of Severities, most severe first.
func SeverityNames() []string {
	out := make([]string, 0, len(Severities))
	for _, s := range Severities {
		out = append(out, s.Name)
	}
	return out
}

// SeverityRank orders severities; unknown ones rank 0.
func SeverityRank(name string) int {
	for _, s := range Severities {
		if s.Name == name {
			return s.Rank
		}
	}
	return 0
}

// ValidSeverity reports whether the name is a level of the scale.
func ValidSeverity(name string) bool { return SeverityRank(name) > 0 }

// SeverityCounts counts alerts or events per severity; it is embedded in the counters of the
// API, whose fields stay critical, error, warning and info.
type SeverityCounts struct {
	Critical int `json:"critical"`
	Error    int `json:"error"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

// Add counts n of a severity; an unknown severity is not counted.
func (c *SeverityCounts) Add(name string, n int) {
	if p := c.slot(name); p != nil {
		*p += n
	}
}

// Get is the count of a severity.
func (c SeverityCounts) Get(name string) int {
	if p := c.slot(name); p != nil {
		return *p
	}
	return 0
}

func (c *SeverityCounts) slot(name string) *int {
	switch name {
	case SeverityCritical:
		return &c.Critical
	case SeverityError:
		return &c.Error
	case SeverityWarning:
		return &c.Warning
	case SeverityInfo:
		return &c.Info
	}
	return nil
}
