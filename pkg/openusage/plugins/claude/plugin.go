package claude

import (
	"context"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
)

const (
	credentialFile = "~/.claude/.credentials.json"
	keychainKey    = "Claude Code-credentials"
	usageURL       = "https://api.anthropic.com/api/oauth/usage"
	refreshURL     = "https://platform.claude.com/v1/oauth/token"
	clientID       = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	scopes         = "user:profile user:inference user:sessions:claude_code user:mcp_servers"
	refreshBuffer  = int64((5 * time.Minute) / time.Millisecond)
)

var hexPattern = regexp.MustCompile(`^[0-9a-fA-F]+$`)

type Plugin struct{}

type credentials struct {
	OAuth    map[string]any
	Source   string
	FullData map[string]any
}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string {
	return "claude"
}

func (p *Plugin) Query(ctx context.Context, env *pluginruntime.Env) (openusage.QueryResult, error) {
	creds := p.loadCredentials(env)
	if creds == nil {
		return openusage.QueryResult{}, fmt.Errorf("Not logged in. Run `claude` to authenticate.")
	}

	accessToken, ok := pluginruntime.GetString(creds.OAuth, "accessToken")
	if !ok || strings.TrimSpace(accessToken) == "" {
		return openusage.QueryResult{}, fmt.Errorf("Not logged in. Run `claude` to authenticate.")
	}

	nowMs := time.Now().UnixMilli()
	if p.needsRefresh(creds.OAuth, nowMs) {
		if refreshed, err := p.refreshToken(ctx, env, creds); err != nil {
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
			response, reqErr := p.fetchUsage(ctx, useToken)
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
			return p.refreshToken(ctx, env, creds)
		},
	)
	if err != nil {
		return openusage.QueryResult{}, err
	}

	if pluginruntime.IsAuthStatus(resp.Status) {
		return openusage.QueryResult{}, fmt.Errorf("Token expired. Run `claude` to log in again.")
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return openusage.QueryResult{}, fmt.Errorf("Usage request failed (HTTP %d). Try again later.", resp.Status)
	}

	data, ok := pluginruntime.TryParseJSONMap(resp.Body)
	if !ok {
		return openusage.QueryResult{}, fmt.Errorf("Usage response invalid. Try again later.")
	}

	plan := ""
	if subscriptionType, ok := pluginruntime.GetString(creds.OAuth, "subscriptionType"); ok {
		plan = pluginruntime.PlanLabel(subscriptionType)
	}

	lines := make([]openusage.MetricLine, 0)

	if fiveHour, ok := pluginruntime.GetMap(data, "five_hour"); ok {
		if utilization, ok := pluginruntime.GetNumber(fiveHour, "utilization"); ok {
			opts := openusage.ProgressLineOptions{PeriodDurationMs: int64((5 * time.Hour) / time.Millisecond)}
			if resets := pluginruntime.ToISO(fiveHour["resets_at"]); resets != "" {
				opts.ResetsAt = resets
			}
			lines = append(lines, openusage.NewProgressLine("Session", utilization, 100, openusage.PercentFormat(), opts))
		}
	}

	if sevenDay, ok := pluginruntime.GetMap(data, "seven_day"); ok {
		if utilization, ok := pluginruntime.GetNumber(sevenDay, "utilization"); ok {
			opts := openusage.ProgressLineOptions{PeriodDurationMs: int64((7 * 24 * time.Hour) / time.Millisecond)}
			if resets := pluginruntime.ToISO(sevenDay["resets_at"]); resets != "" {
				opts.ResetsAt = resets
			}
			lines = append(lines, openusage.NewProgressLine("Weekly", utilization, 100, openusage.PercentFormat(), opts))
		}
	}

	if sonnet, ok := pluginruntime.GetMap(data, "seven_day_sonnet"); ok {
		if utilization, ok := pluginruntime.GetNumber(sonnet, "utilization"); ok {
			opts := openusage.ProgressLineOptions{PeriodDurationMs: int64((7 * 24 * time.Hour) / time.Millisecond)}
			if resets := pluginruntime.ToISO(sonnet["resets_at"]); resets != "" {
				opts.ResetsAt = resets
			}
			lines = append(lines, openusage.NewProgressLine("Sonnet", utilization, 100, openusage.PercentFormat(), opts))
		}
	}

	if extraUsage, ok := pluginruntime.GetMap(data, "extra_usage"); ok {
		if enabled, ok := pluginruntime.GetBool(extraUsage, "is_enabled"); ok && enabled {
			used, hasUsed := pluginruntime.GetNumber(extraUsage, "used_credits")
			limit, hasLimit := pluginruntime.GetNumber(extraUsage, "monthly_limit")
			if hasUsed && hasLimit && limit > 0 {
				lines = append(lines, openusage.NewProgressLine("Extra usage", pluginruntime.Dollars(used), pluginruntime.Dollars(limit), openusage.DollarsFormat(), openusage.ProgressLineOptions{}))
			} else if hasUsed && used > 0 {
				lines = append(lines, openusage.NewTextLine("Extra usage", "$"+fmt.Sprint(pluginruntime.Dollars(used)), openusage.TextLineOptions{}))
			}
		}
	}

	if len(lines) == 0 {
		lines = append(lines, openusage.NewBadgeLine("Status", "No usage data", openusage.TextLineOptions{Color: "#a3a3a3"}))
	}

	return openusage.QueryResult{Plan: plan, Lines: lines}, nil
}

func (p *Plugin) loadCredentials(_ *pluginruntime.Env) *credentials {
	if pluginruntime.FileExists(credentialFile) {
		if text, err := pluginruntime.ReadText(credentialFile); err == nil {
			if parsed, ok := parseCredentialJSON(text); ok {
				if oauth, ok := pluginruntime.GetMap(parsed, "claudeAiOauth"); ok {
					if accessToken, ok := pluginruntime.GetString(oauth, "accessToken"); ok && strings.TrimSpace(accessToken) != "" {
						return &credentials{OAuth: oauth, Source: "file", FullData: parsed}
					}
				}
			}
		}
	}

	if keychainValue, err := pluginruntime.ReadKeychainGenericPassword(keychainKey); err == nil {
		if parsed, ok := parseCredentialJSON(keychainValue); ok {
			if oauth, ok := pluginruntime.GetMap(parsed, "claudeAiOauth"); ok {
				if accessToken, ok := pluginruntime.GetString(oauth, "accessToken"); ok && strings.TrimSpace(accessToken) != "" {
					return &credentials{OAuth: oauth, Source: "keychain", FullData: parsed}
				}
			}
		}
	}

	return nil
}

func parseCredentialJSON(text string) (map[string]any, bool) {
	if parsed, ok := pluginruntime.TryParseJSONMap(text); ok {
		return parsed, true
	}

	hexText := strings.TrimSpace(text)
	if strings.HasPrefix(hexText, "0x") || strings.HasPrefix(hexText, "0X") {
		hexText = hexText[2:]
	}
	if hexText == "" || len(hexText)%2 != 0 || !hexPattern.MatchString(hexText) {
		return nil, false
	}

	decoded, err := hex.DecodeString(hexText)
	if err != nil {
		return nil, false
	}

	return pluginruntime.TryParseJSONMap(string(decoded))
}

func (p *Plugin) saveCredentials(creds *credentials) {
	creds.FullData["claudeAiOauth"] = creds.OAuth
	data, err := pluginruntime.JSONMarshal(creds.FullData)
	if err != nil {
		return
	}
	text := string(data)

	switch creds.Source {
	case "file":
		_ = pluginruntime.WriteText(credentialFile, text)
	case "keychain":
		_ = pluginruntime.WriteKeychainGenericPassword(keychainKey, text)
	}
}

func (p *Plugin) needsRefresh(oauth map[string]any, nowMs int64) bool {
	expiresAt, ok := pluginruntime.GetNumber(oauth, "expiresAt")
	if !ok {
		return true
	}
	return pluginruntime.NeedsRefreshByExpiry(nowMs, int64(expiresAt), refreshBuffer, true)
}

func (p *Plugin) refreshToken(ctx context.Context, env *pluginruntime.Env, creds *credentials) (string, error) {
	refreshToken, ok := pluginruntime.GetString(creds.OAuth, "refreshToken")
	if !ok || strings.TrimSpace(refreshToken) == "" {
		return "", nil
	}

	body := `{"grant_type":"refresh_token","refresh_token":"` + refreshToken + `","client_id":"` + clientID + `","scope":"` + scopes + `"}`
	resp, err := pluginruntime.DoHTTPRequest(ctx, pluginruntime.HTTPRequest{
		Method:   "POST",
		URL:      refreshURL,
		Headers:  map[string]string{"Content-Type": "application/json"},
		BodyText: body,
		Timeout:  15 * time.Second,
	})
	if err != nil {
		env.Logger.Printf("refresh exception: %v", err)
		return "", nil
	}

	if resp.Status == 400 || resp.Status == 401 {
		errorCode := ""
		if payload, ok := pluginruntime.TryParseJSONMap(resp.Body); ok {
			if value, ok := pluginruntime.GetString(payload, "error"); ok {
				errorCode = value
			}
			if errorCode == "" {
				if value, ok := pluginruntime.GetString(payload, "error_description"); ok {
					errorCode = value
				}
			}
		}
		if errorCode == "invalid_grant" {
			return "", fmt.Errorf("Session expired. Run `claude` to log in again.")
		}
		return "", fmt.Errorf("Token expired. Run `claude` to log in again.")
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

	creds.OAuth["accessToken"] = newAccessToken
	if newRefresh, ok := pluginruntime.GetString(payload, "refresh_token"); ok {
		creds.OAuth["refreshToken"] = newRefresh
	}
	if expiresIn, ok := pluginruntime.GetNumber(payload, "expires_in"); ok {
		creds.OAuth["expiresAt"] = float64(time.Now().UnixMilli()) + expiresIn*1000
	}
	p.saveCredentials(creds)
	return newAccessToken, nil
}

func (p *Plugin) fetchUsage(ctx context.Context, accessToken string) (pluginruntime.HTTPResponse, error) {
	return pluginruntime.DoHTTPRequest(ctx, pluginruntime.HTTPRequest{
		Method: "GET",
		URL:    usageURL,
		Headers: map[string]string{
			"Authorization":  "Bearer " + strings.TrimSpace(accessToken),
			"Accept":         "application/json",
			"Content-Type":   "application/json",
			"anthropic-beta": "oauth-2025-04-20",
			"User-Agent":     "OpenUsage",
		},
		Timeout: 10 * time.Second,
	})
}
