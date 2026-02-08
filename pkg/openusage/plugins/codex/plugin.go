package codex

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
)

const (
	authFileName    = "auth.json"
	clientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	refreshURL      = "https://auth.openai.com/oauth/token"
	usageURL        = "https://chatgpt.com/backend-api/wham/usage"
	refreshAge      = 8 * 24 * time.Hour
	sessionPeriodMs = int64((5 * time.Hour) / time.Millisecond)
	weeklyPeriodMs  = int64((7 * 24 * time.Hour) / time.Millisecond)
)

var configAuthPaths = []string{"~/.config/codex", "~/.codex"}

type Plugin struct{}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string {
	return "codex"
}

func (p *Plugin) Query(ctx context.Context, env *pluginruntime.Env) (openusage.QueryResult, error) {
	auth, authPath, ok := p.loadAuth()
	if !ok {
		return openusage.QueryResult{}, fmt.Errorf("Not logged in. Run `codex` to authenticate.")
	}

	tokens, hasTokens := pluginruntime.GetMap(auth, "tokens")
	accessToken, hasAccess := pluginruntime.GetString(tokens, "access_token")
	if hasTokens && hasAccess && strings.TrimSpace(accessToken) != "" {
		nowMs := time.Now().UnixMilli()
		accountID, _ := pluginruntime.GetString(tokens, "account_id")

		if p.needsRefresh(auth, nowMs) {
			if refreshed, err := p.refreshToken(ctx, env, auth, authPath); err != nil {
				return openusage.QueryResult{}, err
			} else if refreshed != "" {
				accessToken = refreshed
			}
		}

		didRefresh := false
		resp, err := pluginruntime.RetryOnceOnAuth(ctx,
			func(token string) (pluginruntime.HTTPResponse, error) {
				useToken := accessToken
				if token != "" {
					useToken = token
				}
				response, reqErr := p.fetchUsage(ctx, useToken, accountID)
				if reqErr != nil {
					if didRefresh {
						return pluginruntime.HTTPResponse{}, fmt.Errorf("Usage request failed after refresh. Try again.")
					}
					return pluginruntime.HTTPResponse{}, fmt.Errorf("Usage request failed. Check your connection.")
				}
				return response, nil
			},
			func() (string, error) {
				didRefresh = true
				return p.refreshToken(ctx, env, auth, authPath)
			},
		)
		if err != nil {
			return openusage.QueryResult{}, err
		}

		if pluginruntime.IsAuthStatus(resp.Status) {
			return openusage.QueryResult{}, fmt.Errorf("Token expired. Run `codex` to log in again.")
		}
		if resp.Status < 200 || resp.Status >= 300 {
			return openusage.QueryResult{}, fmt.Errorf("Usage request failed (HTTP %d). Try again later.", resp.Status)
		}

		data, ok := pluginruntime.TryParseJSONMap(resp.Body)
		if !ok {
			return openusage.QueryResult{}, fmt.Errorf("Usage response invalid. Try again later.")
		}

		nowSec := float64(time.Now().Unix())
		lines := make([]openusage.MetricLine, 0)

		rateLimit, _ := pluginruntime.GetMap(data, "rate_limit")
		primaryWindow, _ := pluginruntime.GetMap(rateLimit, "primary_window")
		secondaryWindow, _ := pluginruntime.GetMap(rateLimit, "secondary_window")

		var reviewWindow map[string]any
		if cr, ok := pluginruntime.GetMap(data, "code_review_rate_limit"); ok {
			reviewWindow, _ = pluginruntime.GetMap(cr, "primary_window")
		}

		if used, ok := pluginruntime.Number(resp.Headers["x-codex-primary-used-percent"]); ok {
			lines = append(lines, buildWindowLine("Session", used, getResetsAtISO(nowSec, primaryWindow), sessionPeriodMs))
		}
		if used, ok := pluginruntime.Number(resp.Headers["x-codex-secondary-used-percent"]); ok {
			lines = append(lines, buildWindowLine("Weekly", used, getResetsAtISO(nowSec, secondaryWindow), weeklyPeriodMs))
		}

		if len(lines) == 0 && rateLimit != nil {
			if used, ok := pluginruntime.GetNumber(primaryWindow, "used_percent"); ok {
				lines = append(lines, buildWindowLine("Session", used, getResetsAtISO(nowSec, primaryWindow), sessionPeriodMs))
			}
			if used, ok := pluginruntime.GetNumber(secondaryWindow, "used_percent"); ok {
				lines = append(lines, buildWindowLine("Weekly", used, getResetsAtISO(nowSec, secondaryWindow), weeklyPeriodMs))
			}
		}

		if reviewWindow != nil {
			if used, ok := pluginruntime.GetNumber(reviewWindow, "used_percent"); ok {
				lines = append(lines, buildWindowLine("Reviews", used, getResetsAtISO(nowSec, reviewWindow), weeklyPeriodMs))
			}
		}

		creditsRemaining, hasCredits := pluginruntime.Number(resp.Headers["x-codex-credits-balance"])
		if !hasCredits {
			if credits, ok := pluginruntime.GetMap(data, "credits"); ok {
				creditsRemaining, hasCredits = pluginruntime.GetNumber(credits, "balance")
			}
		}
		if hasCredits {
			limit := 1000.0
			used := openusage.Clamp(limit-creditsRemaining, 0, limit)
			lines = append(lines, openusage.NewProgressLine("Credits", used, limit, openusage.CountFormat("credits"), openusage.ProgressLineOptions{}))
		}

		plan := ""
		if planType, ok := pluginruntime.GetString(data, "plan_type"); ok {
			plan = pluginruntime.PlanLabel(planType)
		}

		if len(lines) == 0 {
			lines = append(lines, openusage.NewBadgeLine("Status", "No usage data", openusage.TextLineOptions{Color: "#a3a3a3"}))
		}

		return openusage.QueryResult{Plan: plan, Lines: lines}, nil
	}

	if key, ok := pluginruntime.GetString(auth, "OPENAI_API_KEY"); ok && strings.TrimSpace(key) != "" {
		return openusage.QueryResult{}, fmt.Errorf("Usage not available for API key.")
	}

	return openusage.QueryResult{}, fmt.Errorf("Not logged in. Run `codex` to authenticate.")
}

func buildWindowLine(label string, used float64, resetsAt string, periodMs int64) openusage.MetricLine {
	opts := openusage.ProgressLineOptions{PeriodDurationMs: periodMs}
	if resetsAt != "" {
		opts.ResetsAt = resetsAt
	}
	return openusage.NewProgressLine(label, used, 100, openusage.PercentFormat(), opts)
}

func getResetsAtISO(nowSec float64, window map[string]any) string {
	if window == nil {
		return ""
	}
	if resetAt, ok := pluginruntime.GetNumber(window, "reset_at"); ok {
		return pluginruntime.ToISO(resetAt)
	}
	if after, ok := pluginruntime.GetNumber(window, "reset_after_seconds"); ok {
		return pluginruntime.ToISO(nowSec + after)
	}
	return ""
}

func (p *Plugin) loadAuth() (map[string]any, string, bool) {
	authPath := p.resolveAuthPath()
	if authPath == "" || !pluginruntime.FileExists(authPath) {
		return nil, "", false
	}
	text, err := pluginruntime.ReadText(authPath)
	if err != nil {
		return nil, authPath, false
	}
	auth, ok := pluginruntime.TryParseJSONMap(text)
	if !ok {
		return nil, authPath, false
	}
	return auth, authPath, true
}

func (p *Plugin) resolveAuthPath() string {
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		return filepath.Join(pluginruntime.ExpandPath(codexHome), authFileName)
	}

	for _, basePath := range configAuthPaths {
		authPath := filepath.Join(pluginruntime.ExpandPath(basePath), authFileName)
		if pluginruntime.FileExists(authPath) {
			return authPath
		}
	}

	return ""
}

func (p *Plugin) needsRefresh(auth map[string]any, nowMs int64) bool {
	lastRefresh, ok := auth["last_refresh"]
	if !ok {
		return true
	}
	lastMs, ok := pluginruntime.ParseDateMs(lastRefresh)
	if !ok {
		return true
	}
	return nowMs-lastMs > int64(refreshAge/time.Millisecond)
}

func (p *Plugin) refreshToken(ctx context.Context, env *pluginruntime.Env, auth map[string]any, authPath string) (string, error) {
	tokens, ok := pluginruntime.GetMap(auth, "tokens")
	if !ok {
		return "", nil
	}
	refreshToken, ok := pluginruntime.GetString(tokens, "refresh_token")
	if !ok || strings.TrimSpace(refreshToken) == "" {
		return "", nil
	}

	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("client_id", clientID)
	body.Set("refresh_token", refreshToken)

	resp, err := pluginruntime.DoHTTPRequest(ctx, pluginruntime.HTTPRequest{
		Method:   "POST",
		URL:      refreshURL,
		Headers:  map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		BodyText: body.Encode(),
		Timeout:  15 * time.Second,
	})
	if err != nil {
		env.Logger.Printf("refresh exception: %v", err)
		return "", nil
	}

	if resp.Status == 400 || resp.Status == 401 {
		code := ""
		if payload, ok := pluginruntime.TryParseJSONMap(resp.Body); ok {
			if errValue, ok := payload["error"]; ok {
				if errMap, ok := pluginruntime.Map(errValue); ok {
					code, _ = pluginruntime.GetString(errMap, "code")
				} else if errString, ok := pluginruntime.String(errValue); ok {
					code = errString
				}
			}
			if code == "" {
				code, _ = pluginruntime.GetString(payload, "code")
			}
		}
		switch code {
		case "refresh_token_expired":
			return "", fmt.Errorf("Session expired. Run `codex` to log in again.")
		case "refresh_token_reused":
			return "", fmt.Errorf("Token conflict. Run `codex` to log in again.")
		case "refresh_token_invalidated":
			return "", fmt.Errorf("Token revoked. Run `codex` to log in again.")
		default:
			return "", fmt.Errorf("Token expired. Run `codex` to log in again.")
		}
	}

	if resp.Status < 200 || resp.Status >= 300 {
		return "", nil
	}

	payload, ok := pluginruntime.TryParseJSONMap(resp.Body)
	if !ok {
		return "", nil
	}
	newAccessToken, ok := pluginruntime.GetString(payload, "access_token")
	if !ok || strings.TrimSpace(newAccessToken) == "" {
		return "", nil
	}

	tokens["access_token"] = newAccessToken
	if newRefresh, ok := pluginruntime.GetString(payload, "refresh_token"); ok {
		tokens["refresh_token"] = newRefresh
	}
	if idToken, ok := pluginruntime.GetString(payload, "id_token"); ok {
		tokens["id_token"] = idToken
	}
	auth["tokens"] = tokens
	auth["last_refresh"] = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	serialized, err := pluginruntime.JSONMarshalIndent(auth)
	if err == nil {
		_ = pluginruntime.WriteText(authPath, string(serialized))
	}

	return newAccessToken, nil
}

func (p *Plugin) fetchUsage(ctx context.Context, accessToken, accountID string) (pluginruntime.HTTPResponse, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Accept":        "application/json",
		"User-Agent":    "OpenUsage",
	}
	if strings.TrimSpace(accountID) != "" {
		headers["ChatGPT-Account-Id"] = accountID
	}

	return pluginruntime.DoHTTPRequest(ctx, pluginruntime.HTTPRequest{
		Method:  "GET",
		URL:     usageURL,
		Headers: headers,
		Timeout: 10 * time.Second,
	})
}
