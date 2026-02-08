package windsurf

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
)

const lsService = "exa.language_server_pb.LanguageServerService"

type variant struct {
	Marker  string
	IdeName string
	StateDB string
}

var variants = []variant{
	{
		Marker:  "windsurf",
		IdeName: "windsurf",
		StateDB: "~/Library/Application Support/Windsurf/User/globalStorage/state.vscdb",
	},
	{
		Marker:  "windsurf-next",
		IdeName: "windsurf-next",
		StateDB: "~/Library/Application Support/Windsurf - Next/User/globalStorage/state.vscdb",
	},
}

type Plugin struct{}

type variantResult struct {
	Plan  string
	Lines []openusage.MetricLine
}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string {
	return "windsurf"
}

func (p *Plugin) Query(ctx context.Context, _ *pluginruntime.Env) (openusage.QueryResult, error) {
	for _, v := range variants {
		result := p.probeVariant(ctx, v)
		if result != nil {
			return openusage.QueryResult{Plan: result.Plan, Lines: result.Lines}, nil
		}
	}

	return openusage.QueryResult{}, fmt.Errorf("Start Windsurf and try again.")
}

func (p *Plugin) probeVariant(ctx context.Context, v variant) *variantResult {
	discovery, err := pluginruntime.DiscoverLS(pluginruntime.LSDiscoverOptions{
		ProcessName: "language_server",
		Markers:     []string{v.Marker},
		CSRFFlag:    "--csrf_token",
		PortFlag:    "--extension_server_port",
		ExtraFlags:  []string{"--windsurf_version"},
	})
	if err != nil || discovery == nil {
		return nil
	}

	port, scheme, ok := p.findWorkingPort(ctx, discovery, v.IdeName)
	if !ok {
		return nil
	}

	apiKey := p.loadAPIKey(v)
	if apiKey == "" {
		return nil
	}

	version := "unknown"
	if value, ok := discovery.Extra["windsurf_version"]; ok && strings.TrimSpace(value) != "" {
		version = value
	}

	metadata := map[string]any{
		"apiKey":           apiKey,
		"ideName":          v.IdeName,
		"ideVersion":       version,
		"extensionName":    v.IdeName,
		"extensionVersion": version,
		"locale":           "en",
	}

	data := p.callLS(ctx, port, scheme, discovery.CSRF, "GetUserStatus", map[string]any{"metadata": metadata})
	if data == nil {
		return nil
	}
	userStatus, ok := pluginruntime.GetMap(data, "userStatus")
	if !ok {
		return nil
	}
	planStatus, _ := pluginruntime.GetMap(userStatus, "planStatus")
	planInfo, _ := pluginruntime.GetMap(planStatus, "planInfo")
	plan, _ := pluginruntime.GetString(planInfo, "planName")

	planStart, _ := pluginruntime.GetString(planStatus, "planStart")
	planEnd, _ := pluginruntime.GetString(planStatus, "planEnd")
	periodMs := int64(0)
	if startMs, ok := pluginruntime.ParseDateMs(planStart); ok {
		if endMs, ok := pluginruntime.ParseDateMs(planEnd); ok && endMs > startMs {
			periodMs = endMs - startMs
		}
	}

	lines := make([]openusage.MetricLine, 0)

	promptTotal, hasPromptTotal := pluginruntime.GetNumber(planStatus, "availablePromptCredits")
	promptUsed, _ := pluginruntime.GetNumber(planStatus, "usedPromptCredits")
	if hasPromptTotal && promptTotal > 0 {
		if line, ok := creditLine("Prompt credits", promptUsed/100, promptTotal/100, planEnd, periodMs); ok {
			lines = append(lines, line)
		}
	}

	flexTotal, hasFlexTotal := pluginruntime.GetNumber(planStatus, "availableFlexCredits")
	flexUsed, _ := pluginruntime.GetNumber(planStatus, "usedFlexCredits")
	if hasFlexTotal && flexTotal > 0 {
		if line, ok := creditLine("Flex credits", flexUsed/100, flexTotal/100, planEnd, periodMs); ok {
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		lines = append(lines, openusage.NewBadgeLine("Credits", "Unlimited", openusage.TextLineOptions{}))
	}

	return &variantResult{Plan: plan, Lines: lines}
}

func (p *Plugin) loadAPIKey(v variant) string {
	rowsJSON, err := pluginruntime.SQLiteQuery(v.StateDB, "SELECT value FROM ItemTable WHERE key = 'windsurfAuthStatus' LIMIT 1")
	if err != nil {
		return ""
	}
	rows, ok := pluginruntime.TryParseJSONArray(rowsJSON)
	if !ok || len(rows) == 0 {
		return ""
	}
	row, ok := pluginruntime.Map(rows[0])
	if !ok {
		return ""
	}
	value, ok := pluginruntime.GetString(row, "value")
	if !ok {
		return ""
	}
	auth, ok := pluginruntime.TryParseJSONMap(value)
	if !ok {
		return ""
	}
	apiKey, _ := pluginruntime.GetString(auth, "apiKey")
	return apiKey
}

func (p *Plugin) findWorkingPort(ctx context.Context, discovery *pluginruntime.LSDiscoverResult, ideName string) (int, string, bool) {
	for _, port := range discovery.Ports {
		if p.probePort(ctx, "https", port, discovery.CSRF, ideName) {
			return port, "https", true
		}
		if p.probePort(ctx, "http", port, discovery.CSRF, ideName) {
			return port, "http", true
		}
	}
	if discovery.ExtensionPort != nil {
		return *discovery.ExtensionPort, "http", true
	}
	return 0, "", false
}

func (p *Plugin) probePort(ctx context.Context, scheme string, port int, csrf, ideName string) bool {
	body, _ := json.Marshal(map[string]any{
		"context": map[string]any{
			"properties": map[string]string{
				"devMode":          "false",
				"extensionVersion": "unknown",
				"ide":              ideName,
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

func creditLine(label string, used, total float64, resetsAt string, periodMs int64) (openusage.MetricLine, bool) {
	if total <= 0 {
		return openusage.MetricLine{}, false
	}
	if used < 0 {
		used = 0
	}
	opts := openusage.ProgressLineOptions{}
	if resetsAt != "" {
		opts.ResetsAt = resetsAt
	}
	if periodMs > 0 {
		opts.PeriodDurationMs = periodMs
	}
	return openusage.NewProgressLine(label, used, total, openusage.CountFormat("credits"), opts), true
}
