package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
)

const (
	keychainService = "OpenUsage-copilot"
	ghKeychain      = "gh:github.com"
	usageURL        = "https://api.github.com/copilot_internal/user"
)

type Plugin struct{}

type credential struct {
	Token  string
	Source string
}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string {
	return "copilot"
}

func (p *Plugin) Query(ctx context.Context, env *pluginruntime.Env) (openusage.QueryResult, error) {
	cred := p.loadToken(env)
	if cred == nil {
		return openusage.QueryResult{}, fmt.Errorf("Not logged in. Run `gh auth login` first.")
	}

	token := cred.Token
	source := cred.Source

	resp, err := p.fetchUsage(ctx, token)
	if err != nil {
		return openusage.QueryResult{}, fmt.Errorf("Usage request failed. Check your connection.")
	}

	if pluginruntime.IsAuthStatus(resp.Status) {
		if source == "keychain" {
			env.Logger.Printf("cached token invalid, trying fallback sources")
			p.clearCachedToken(env)
			fallback := p.loadTokenFromGhCLI(env)
			if fallback != nil {
				resp, err = p.fetchUsage(ctx, fallback.Token)
				if err != nil {
					return openusage.QueryResult{}, fmt.Errorf("Usage request failed. Check your connection.")
				}
				if resp.Status >= 200 && resp.Status < 300 {
					p.saveToken(env, fallback.Token)
					token = fallback.Token
					source = fallback.Source
				}
			}
		}
		if pluginruntime.IsAuthStatus(resp.Status) {
			return openusage.QueryResult{}, fmt.Errorf("Token invalid. Run `gh auth login` to re-authenticate.")
		}
	}

	if resp.Status < 200 || resp.Status >= 300 {
		return openusage.QueryResult{}, fmt.Errorf("Usage request failed (HTTP %d). Try again later.", resp.Status)
	}

	if source == "gh-cli" {
		p.saveToken(env, token)
	}

	data, ok := pluginruntime.TryParseJSONMap(resp.Body)
	if !ok {
		return openusage.QueryResult{}, fmt.Errorf("Usage response invalid. Try again later.")
	}

	plan := ""
	if rawPlan, ok := pluginruntime.GetString(data, "copilot_plan"); ok {
		plan = pluginruntime.PlanLabel(rawPlan)
	}

	lines := make([]openusage.MetricLine, 0)

	if snapshots, ok := pluginruntime.GetMap(data, "quota_snapshots"); ok {
		if premium, ok := pluginruntime.GetMap(snapshots, "premium_interactions"); ok {
			if line, ok := makeProgressLine("Premium", premium, data["quota_reset_date"]); ok {
				lines = append(lines, line)
			}
		}
		if chat, ok := pluginruntime.GetMap(snapshots, "chat"); ok {
			if line, ok := makeProgressLine("Chat", chat, data["quota_reset_date"]); ok {
				lines = append(lines, line)
			}
		}
	}

	if lq, ok := pluginruntime.GetMap(data, "limited_user_quotas"); ok {
		if mq, ok := pluginruntime.GetMap(data, "monthly_quotas"); ok {
			if line, ok := makeLimitedProgressLine("Chat", lq["chat"], mq["chat"], data["limited_user_reset_date"]); ok {
				lines = append(lines, line)
			}
			if line, ok := makeLimitedProgressLine("Completions", lq["completions"], mq["completions"], data["limited_user_reset_date"]); ok {
				lines = append(lines, line)
			}
		}
	}

	if len(lines) == 0 {
		lines = append(lines, openusage.NewBadgeLine("Status", "No usage data", openusage.TextLineOptions{Color: "#a3a3a3"}))
	}

	return openusage.QueryResult{Plan: plan, Lines: lines}, nil
}

func makeProgressLine(label string, snapshot map[string]any, resetDate any) (openusage.MetricLine, bool) {
	remaining, ok := pluginruntime.GetNumber(snapshot, "percent_remaining")
	if !ok {
		return openusage.MetricLine{}, false
	}
	used := openusage.Clamp(100-remaining, 0, 100)
	opts := openusage.ProgressLineOptions{PeriodDurationMs: int64((30 * 24 * time.Hour) / time.Millisecond)}
	if iso := pluginruntime.ToISO(resetDate); iso != "" {
		opts.ResetsAt = iso
	}
	return openusage.NewProgressLine(label, used, 100, openusage.PercentFormat(), opts), true
}

func makeLimitedProgressLine(label string, remainingValue, totalValue, resetDate any) (openusage.MetricLine, bool) {
	remaining, ok := pluginruntime.Number(remainingValue)
	if !ok {
		return openusage.MetricLine{}, false
	}
	total, ok := pluginruntime.Number(totalValue)
	if !ok || total <= 0 {
		return openusage.MetricLine{}, false
	}
	used := total - remaining
	usedPercent := openusage.Clamp(float64(int((used/total)*100+0.5)), 0, 100)
	opts := openusage.ProgressLineOptions{PeriodDurationMs: int64((30 * 24 * time.Hour) / time.Millisecond)}
	if iso := pluginruntime.ToISO(resetDate); iso != "" {
		opts.ResetsAt = iso
	}
	return openusage.NewProgressLine(label, usedPercent, 100, openusage.PercentFormat(), opts), true
}

func (p *Plugin) statePath(env *pluginruntime.Env) string {
	return filepath.Join(env.PluginDataDir, "auth.json")
}

func (p *Plugin) loadToken(env *pluginruntime.Env) *credential {
	if c := p.loadTokenFromKeychain(env); c != nil {
		return c
	}
	if c := p.loadTokenFromGhCLI(env); c != nil {
		return c
	}
	return p.loadTokenFromState(env)
}

func (p *Plugin) loadTokenFromKeychain(env *pluginruntime.Env) *credential {
	_ = env
	raw, err := pluginruntime.ReadKeychainGenericPassword(keychainService)
	if err != nil {
		return nil
	}
	data, ok := pluginruntime.TryParseJSONMap(raw)
	if !ok {
		return nil
	}
	token, ok := pluginruntime.GetString(data, "token")
	if !ok || strings.TrimSpace(token) == "" {
		return nil
	}
	return &credential{Token: token, Source: "keychain"}
}

func (p *Plugin) loadTokenFromGhCLI(env *pluginruntime.Env) *credential {
	_ = env
	if raw, err := pluginruntime.ReadKeychainGenericPassword(ghKeychain); err == nil {
		if token := normalizeGhToken(raw); token != "" {
			return &credential{Token: token, Source: "gh-cli"}
		}
	}

	// Cross-platform fallback: ask gh directly for the token.
	// This is needed on systems where gh uses encrypted storage
	// that cannot be read through the macOS keychain path.
	for _, args := range [][]string{
		{"auth", "token", "--hostname", "github.com"},
		{"auth", "token", "-h", "github.com"},
		{"auth", "token"},
	} {
		out, err := exec.Command("gh", args...).Output()
		if err != nil {
			continue
		}
		if token := normalizeGhToken(string(out)); token != "" {
			return &credential{Token: token, Source: "gh-cli"}
		}
	}

	// Final fallback for CI/headless setups.
	for _, envName := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(envName)); token != "" {
			return &credential{Token: token, Source: "env"}
		}
	}

	return nil
}

func normalizeGhToken(raw string) string {
	token := strings.TrimSpace(raw)
	if strings.HasPrefix(token, "go-keyring-base64:") {
		decoded, err := pluginruntime.DecodeBase64(strings.TrimPrefix(token, "go-keyring-base64:"))
		if err == nil {
			token = decoded
		}
	}
	return strings.TrimSpace(token)
}

func (p *Plugin) loadTokenFromState(env *pluginruntime.Env) *credential {
	text, err := pluginruntime.ReadText(p.statePath(env))
	if err != nil {
		return nil
	}
	data, ok := pluginruntime.TryParseJSONMap(text)
	if !ok {
		return nil
	}
	token, ok := pluginruntime.GetString(data, "token")
	if !ok || strings.TrimSpace(token) == "" {
		return nil
	}
	return &credential{Token: token, Source: "state"}
}

func (p *Plugin) saveToken(env *pluginruntime.Env, token string) {
	_ = pluginruntime.WriteKeychainGenericPassword(keychainService, fmt.Sprintf(`{"token":%q}`, token))
	_ = p.writeState(env, map[string]any{"token": token})
}

func (p *Plugin) clearCachedToken(env *pluginruntime.Env) {
	_ = pluginruntime.DeleteKeychainGenericPassword(keychainService)
	_ = p.writeState(env, nil)
}

func (p *Plugin) writeState(env *pluginruntime.Env, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return pluginruntime.WriteText(p.statePath(env), string(data))
}

func (p *Plugin) fetchUsage(ctx context.Context, token string) (pluginruntime.HTTPResponse, error) {
	return pluginruntime.DoHTTPRequest(ctx, pluginruntime.HTTPRequest{
		Method: "GET",
		URL:    usageURL,
		Headers: map[string]string{
			"Authorization":         "token " + token,
			"Accept":                "application/json",
			"Editor-Version":        "vscode/1.96.2",
			"Editor-Plugin-Version": "copilot-chat/0.26.7",
			"User-Agent":            "GitHubCopilotChat/0.26.7",
			"X-Github-Api-Version":  "2025-04-01",
		},
		Timeout: 10 * time.Second,
	})
}
