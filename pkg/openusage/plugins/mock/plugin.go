package mock

import (
	"context"
	"time"

	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
)

type Plugin struct{}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string {
	return "mock"
}

func (p *Plugin) Query(_ context.Context, _ *pluginruntime.Env) (openusage.QueryResult, error) {
	fifteenDays := 15 * 24 * time.Hour
	thirtyDaysMs := int64((30 * 24 * time.Hour) / time.Millisecond)
	resetsAt := time.Now().Add(fifteenDays).UTC().Format("2006-01-02T15:04:05.000Z")
	pastReset := time.Now().Add(-time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")

	lines := []openusage.MetricLine{
		openusage.NewProgressLine("Ahead pace", 30, 100, openusage.PercentFormat(), openusage.ProgressLineOptions{ResetsAt: resetsAt, PeriodDurationMs: thirtyDaysMs}),
		openusage.NewProgressLine("On Track pace", 45, 100, openusage.PercentFormat(), openusage.ProgressLineOptions{ResetsAt: resetsAt, PeriodDurationMs: thirtyDaysMs}),
		openusage.NewProgressLine("Behind pace", 65, 100, openusage.PercentFormat(), openusage.ProgressLineOptions{ResetsAt: resetsAt, PeriodDurationMs: thirtyDaysMs}),
		openusage.NewProgressLine("Empty bar", 0, 500, openusage.DollarsFormat(), openusage.ProgressLineOptions{}),
		openusage.NewProgressLine("Exactly full", 1000, 1000, openusage.CountFormat("tokens"), openusage.ProgressLineOptions{}),
		openusage.NewProgressLine("Over limit!", 1337, 1000, openusage.CountFormat("requests"), openusage.ProgressLineOptions{}),
		openusage.NewProgressLine("Huge numbers", 8429301, 10000000, openusage.CountFormat("tokens"), openusage.ProgressLineOptions{}),
		openusage.NewProgressLine("Tiny sliver", 1, 10000, openusage.PercentFormat(), openusage.ProgressLineOptions{}),
		openusage.NewProgressLine("Almost full", 9999, 10000, openusage.PercentFormat(), openusage.ProgressLineOptions{}),
		openusage.NewProgressLine("Expired reset", 42, 100, openusage.PercentFormat(), openusage.ProgressLineOptions{ResetsAt: pastReset, PeriodDurationMs: thirtyDaysMs}),
		openusage.NewTextLine("Status", "Active", openusage.TextLineOptions{}),
		openusage.NewTextLine("Very long value", "This is an extremely long value string that should test text overflow and wrapping behavior in the card layout", openusage.TextLineOptions{}),
		openusage.NewTextLine("", "Empty label", openusage.TextLineOptions{}),
		openusage.NewBadgeLine("Tier", "Enterprise", openusage.TextLineOptions{Color: "#8B5CF6"}),
		openusage.NewBadgeLine("Alert", "Rate limited", openusage.TextLineOptions{Color: "#ef4444"}),
		openusage.NewBadgeLine("Region", "us-east-1", openusage.TextLineOptions{}),
	}

	return openusage.QueryResult{Plan: "stress-test", Lines: lines}, nil
}
