package openusage

import "math"

const (
	LineTypeText     = "text"
	LineTypeProgress = "progress"
	LineTypeBadge    = "badge"
)

const (
	FormatKindPercent = "percent"
	FormatKindDollars = "dollars"
	FormatKindCount   = "count"
)

type ProgressFormat struct {
	Kind   string `json:"kind"`
	Suffix string `json:"suffix,omitempty"`
}

type MetricLine struct {
	Type             string          `json:"type"`
	Label            string          `json:"label"`
	Value            *string         `json:"value,omitempty"`
	Text             *string         `json:"text,omitempty"`
	Used             *float64        `json:"used,omitempty"`
	Limit            *float64        `json:"limit,omitempty"`
	Format           *ProgressFormat `json:"format,omitempty"`
	ResetsAt         *string         `json:"resetsAt,omitempty"`
	PeriodDurationMs *int64          `json:"periodDurationMs,omitempty"`
	Color            *string         `json:"color,omitempty"`
	Subtitle         *string         `json:"subtitle,omitempty"`
}

type PluginOutput struct {
	ProviderID  string       `json:"providerId"`
	DisplayName string       `json:"displayName"`
	Plan        string       `json:"plan,omitempty"`
	Lines       []MetricLine `json:"lines"`
	IconURL     string       `json:"iconUrl,omitempty"`
	Error       string       `json:"error,omitempty"`
}

type QueryResult struct {
	Plan  string
	Lines []MetricLine
}

type ProgressLineOptions struct {
	ResetsAt         string
	PeriodDurationMs int64
	Color            string
	Subtitle         string
}

type TextLineOptions struct {
	Color    string
	Subtitle string
}

func PercentFormat() ProgressFormat {
	return ProgressFormat{Kind: FormatKindPercent}
}

func DollarsFormat() ProgressFormat {
	return ProgressFormat{Kind: FormatKindDollars}
}

func CountFormat(suffix string) ProgressFormat {
	return ProgressFormat{Kind: FormatKindCount, Suffix: suffix}
}

func NewTextLine(label, value string, opts TextLineOptions) MetricLine {
	line := MetricLine{
		Type:  LineTypeText,
		Label: label,
		Value: Ptr(value),
	}
	if opts.Color != "" {
		line.Color = Ptr(opts.Color)
	}
	if opts.Subtitle != "" {
		line.Subtitle = Ptr(opts.Subtitle)
	}
	return line
}

func NewBadgeLine(label, text string, opts TextLineOptions) MetricLine {
	line := MetricLine{
		Type:  LineTypeBadge,
		Label: label,
		Text:  Ptr(text),
	}
	if opts.Color != "" {
		line.Color = Ptr(opts.Color)
	}
	if opts.Subtitle != "" {
		line.Subtitle = Ptr(opts.Subtitle)
	}
	return line
}

func NewProgressLine(label string, used, limit float64, format ProgressFormat, opts ProgressLineOptions) MetricLine {
	line := MetricLine{
		Type:   LineTypeProgress,
		Label:  label,
		Used:   Ptr(used),
		Limit:  Ptr(limit),
		Format: &format,
	}
	if opts.ResetsAt != "" {
		line.ResetsAt = Ptr(opts.ResetsAt)
	}
	if opts.PeriodDurationMs > 0 {
		line.PeriodDurationMs = Ptr(opts.PeriodDurationMs)
	}
	if opts.Color != "" {
		line.Color = Ptr(opts.Color)
	}
	if opts.Subtitle != "" {
		line.Subtitle = Ptr(opts.Subtitle)
	}
	return line
}

func ErrorLines(message string) []MetricLine {
	return []MetricLine{NewBadgeLine("Error", message, TextLineOptions{Color: "#ef4444"})}
}

func Clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func RoundTo(value float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Round(value*factor) / factor
}

func Ptr[T any](v T) *T {
	return &v
}
