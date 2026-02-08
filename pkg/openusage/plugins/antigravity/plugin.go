package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
)

const lsService = "exa.language_server_pb.LanguageServerService"

type Plugin struct{}

type modelUsage struct {
	Label             string
	RemainingFraction float64
	ResetTime         string
	SortKey           string
}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string {
	return "antigravity"
}

func (p *Plugin) Query(ctx context.Context, _ *pluginruntime.Env) (openusage.QueryResult, error) {
	discovery, err := pluginruntime.DiscoverLS(pluginruntime.LSDiscoverOptions{
		ProcessName: "language_server",
		Markers:     []string{"antigravity"},
		CSRFFlag:    "--csrf_token",
		PortFlag:    "--extension_server_port",
	})
	if err != nil || discovery == nil {
		return openusage.QueryResult{}, fmt.Errorf("Start Antigravity and try again.")
	}

	port, scheme, ok := p.findWorkingPort(ctx, discovery)
	if !ok {
		return openusage.QueryResult{}, fmt.Errorf("Start Antigravity and try again.")
	}

	metadata := map[string]any{
		"ideName":       "antigravity",
		"extensionName": "antigravity",
		"ideVersion":    "unknown",
		"locale":        "en",
	}

	data := p.callLS(ctx, port, scheme, discovery.CSRF, "GetUserStatus", map[string]any{"metadata": metadata})
	hasUserStatus := false
	if data != nil {
		if _, ok := pluginruntime.GetMap(data, "userStatus"); ok {
			hasUserStatus = true
		}
	}
	if !hasUserStatus {
		data = p.callLS(ctx, port, scheme, discovery.CSRF, "GetCommandModelConfigs", map[string]any{"metadata": metadata})
	}

	var configs []any
	plan := ""

	if hasUserStatus {
		userStatus, ok := pluginruntime.GetMap(data, "userStatus")
		if ok {
			if planStatus, ok := pluginruntime.GetMap(userStatus, "planStatus"); ok {
				if planInfo, ok := pluginruntime.GetMap(planStatus, "planInfo"); ok {
					plan, _ = pluginruntime.GetString(planInfo, "planName")
				}
			}
			if cascade, ok := pluginruntime.GetMap(userStatus, "cascadeModelConfigData"); ok {
				configs, _ = pluginruntime.Array(cascade["clientModelConfigs"])
			}
		}
	} else if data != nil {
		configs, _ = pluginruntime.Array(data["clientModelConfigs"])
	}

	if len(configs) == 0 {
		return openusage.QueryResult{}, fmt.Errorf("No data from language server.")
	}

	deduped := make(map[string]modelUsage)
	for _, entry := range configs {
		cfg, ok := pluginruntime.Map(entry)
		if !ok {
			continue
		}
		quota, ok := pluginruntime.GetMap(cfg, "quotaInfo")
		if !ok {
			continue
		}
		remaining, ok := pluginruntime.GetNumber(quota, "remainingFraction")
		if !ok {
			continue
		}
		label, ok := pluginruntime.GetString(cfg, "label")
		if !ok {
			continue
		}
		normalized := normalizeLabel(label)
		resetTime, _ := pluginruntime.GetString(quota, "resetTime")

		existing, exists := deduped[normalized]
		if !exists || remaining < existing.RemainingFraction {
			deduped[normalized] = modelUsage{
				Label:             normalized,
				RemainingFraction: remaining,
				ResetTime:         resetTime,
				SortKey:           modelSortKey(normalized),
			}
		}
	}

	models := make([]modelUsage, 0, len(deduped))
	for _, model := range deduped {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].SortKey < models[j].SortKey
	})

	lines := make([]openusage.MetricLine, 0, len(models))
	for _, model := range models {
		lines = append(lines, modelLine(model.Label, model.RemainingFraction, model.ResetTime))
	}

	if len(lines) == 0 {
		return openusage.QueryResult{}, fmt.Errorf("No usage data available.")
	}

	return openusage.QueryResult{Plan: plan, Lines: lines}, nil
}

func (p *Plugin) findWorkingPort(ctx context.Context, discovery *pluginruntime.LSDiscoverResult) (int, string, bool) {
	for _, port := range discovery.Ports {
		if p.probePort(ctx, "https", port, discovery.CSRF) {
			return port, "https", true
		}
		if p.probePort(ctx, "http", port, discovery.CSRF) {
			return port, "http", true
		}
	}
	if discovery.ExtensionPort != nil {
		return *discovery.ExtensionPort, "http", true
	}
	return 0, "", false
}

func (p *Plugin) probePort(ctx context.Context, scheme string, port int, csrf string) bool {
	body, _ := json.Marshal(map[string]any{
		"context": map[string]any{
			"properties": map[string]string{
				"devMode":          "false",
				"extensionVersion": "unknown",
				"ide":              "antigravity",
				"ideVersion":       "unknown",
				"os":               "macos",
			},
		},
	})

	_, err := pluginruntime.CallLS(ctx, scheme, port, csrf, lsService+"/GetUnleashData", "POST", string(body), 5*time.Second)
	return err == nil
}

func (p *Plugin) callLS(ctx context.Context, port int, scheme, csrf, method string, body any) map[string]any {
	payload, _ := json.Marshal(body)
	resp, err := pluginruntime.CallLS(ctx, scheme, port, csrf, lsService+"/"+method, "POST", string(payload), 10*time.Second)
	if err != nil {
		return nil
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return nil
	}
	data, _ := pluginruntime.TryParseJSONMap(resp.Body)
	return data
}

func normalizeLabel(label string) string {
	trimmed := strings.TrimSpace(label)
	for {
		start := strings.LastIndex(trimmed, "(")
		end := strings.LastIndex(trimmed, ")")
		if start < 0 || end <= start || end != len(trimmed)-1 {
			break
		}
		trimmed = strings.TrimSpace(trimmed[:start])
	}
	return trimmed
}

func modelSortKey(label string) string {
	lower := strings.ToLower(label)
	switch {
	case strings.Contains(lower, "gemini") && strings.Contains(lower, "pro"):
		return "0a_" + label
	case strings.Contains(lower, "gemini"):
		return "0b_" + label
	case strings.Contains(lower, "claude") && strings.Contains(lower, "opus"):
		return "1a_" + label
	case strings.Contains(lower, "claude"):
		return "1b_" + label
	default:
		return "2_" + label
	}
}

func modelLine(label string, remainingFraction float64, resetTime string) openusage.MetricLine {
	clamped := openusage.Clamp(remainingFraction, 0, 1)
	used := openusage.RoundTo((1-clamped)*100, 2)
	opts := openusage.ProgressLineOptions{PeriodDurationMs: int64((5 * time.Hour) / time.Millisecond)}
	if resetTime != "" {
		opts.ResetsAt = resetTime
	}
	return openusage.NewProgressLine(label, used, 100, openusage.PercentFormat(), opts)
}
