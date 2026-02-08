package cursor

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/pluginruntime"
)

const (
	stateDBPath      = "~/Library/Application Support/Cursor/User/globalStorage/state.vscdb"
	baseURL          = "https://api2.cursor.sh"
	usageURL         = baseURL + "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	planURL          = baseURL + "/aiserver.v1.DashboardService/GetPlanInfo"
	refreshURL       = baseURL + "/oauth/token"
	creditsURL       = baseURL + "/aiserver.v1.DashboardService/GetCreditGrantsBalance"
	clientID         = "KbZUR41cY7W6zRSdpSUJ7I7mLYBKOCmB"
	refreshBufferMs  = int64((5 * time.Minute) / time.Millisecond)
	defaultBillingMs = int64((30 * 24 * time.Hour) / time.Millisecond)
)

type Plugin struct{}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string {
	return "cursor"
}

func (p *Plugin) Query(ctx context.Context, env *pluginruntime.Env) (openusage.QueryResult, error) {
	accessToken := p.readStateValue("cursorAuth/accessToken")
	refreshToken := p.readStateValue("cursorAuth/refreshToken")

	if accessToken == "" && refreshToken == "" {
		return openusage.QueryResult{}, fmt.Errorf("not logged in; sign in via cursor app")
	}

	nowMs := time.Now().UnixMilli()
	if p.needsRefresh(accessToken, nowMs) {
		refreshed, err := p.refreshToken(ctx, env, refreshToken)
		if err != nil {
			if accessToken == "" {
				return openusage.QueryResult{}, err
			}
		} else if refreshed != "" {
			accessToken = refreshed
		} else if accessToken == "" {
			return openusage.QueryResult{}, fmt.Errorf("not logged in; sign in via cursor app")
		}
	}

	didRefresh := false
	usageResp, err := pluginruntime.RetryOnceOnAuth(ctx,
		func(token string) (pluginruntime.HTTPResponse, error) {
			useToken := accessToken
			if token != "" {
				useToken = token
			}
			resp, reqErr := p.connectPost(ctx, usageURL, useToken)
			if reqErr != nil {
				if didRefresh {
					return pluginruntime.HTTPResponse{}, fmt.Errorf("usage request failed after refresh, try again")
				}
				return pluginruntime.HTTPResponse{}, fmt.Errorf("usage request failed, check your connection")
			}
			return resp, nil
		},
		func() (string, error) {
			didRefresh = true
			refreshed, refreshErr := p.refreshToken(ctx, env, refreshToken)
			if refreshErr != nil {
				return "", refreshErr
			}
			if refreshed != "" {
				accessToken = refreshed
			}
			return refreshed, nil
		},
	)
	if err != nil {
		return openusage.QueryResult{}, err
	}

	if pluginruntime.IsAuthStatus(usageResp.Status) {
		return openusage.QueryResult{}, fmt.Errorf("token expired; sign in via cursor app")
	}
	if usageResp.Status < 200 || usageResp.Status >= 300 {
		return openusage.QueryResult{}, fmt.Errorf("usage request failed (HTTP %d), try again later", usageResp.Status)
	}

	usage, ok := pluginruntime.TryParseJSONMap(usageResp.Body)
	if !ok {
		return openusage.QueryResult{}, fmt.Errorf("usage response invalid, try again later")
	}

	enabled, okEnabled := pluginruntime.GetBool(usage, "enabled")
	planUsage, hasPlanUsage := pluginruntime.GetMap(usage, "planUsage")
	if !okEnabled || !enabled || !hasPlanUsage {
		return openusage.QueryResult{}, fmt.Errorf("no active cursor subscription")
	}

	planName := ""
	if planResp, err := p.connectPost(ctx, planURL, accessToken); err == nil && planResp.Status >= 200 && planResp.Status < 300 {
		if planData, ok := pluginruntime.TryParseJSONMap(planResp.Body); ok {
			if planInfo, ok := pluginruntime.GetMap(planData, "planInfo"); ok {
				planName, _ = pluginruntime.GetString(planInfo, "planName")
			}
		}
	}

	var creditGrants map[string]any
	if creditsResp, err := p.connectPost(ctx, creditsURL, accessToken); err == nil && creditsResp.Status >= 200 && creditsResp.Status < 300 {
		creditGrants, _ = pluginruntime.TryParseJSONMap(creditsResp.Body)
	}

	plan := pluginruntime.PlanLabel(planName)

	limit, hasLimit := pluginruntime.GetNumber(planUsage, "limit")
	if !hasLimit {
		return openusage.QueryResult{}, fmt.Errorf("plan usage limit missing from API response")
	}

	planUsed, hasPlanUsed := pluginruntime.GetNumber(planUsage, "totalSpend")
	if !hasPlanUsed {
		remaining, ok := pluginruntime.GetNumber(planUsage, "remaining")
		if !ok {
			remaining = 0
		}
		planUsed = limit - remaining
	}

	billingPeriodMs := defaultBillingMs
	cycleStart, hasCycleStart := pluginruntime.GetNumber(usage, "billingCycleStart")
	cycleEnd, hasCycleEnd := pluginruntime.GetNumber(usage, "billingCycleEnd")
	if hasCycleStart && hasCycleEnd {
		delta := int64(cycleEnd - cycleStart)
		if delta > 0 {
			billingPeriodMs = delta
		}
	}

	lines := make([]openusage.MetricLine, 0)

	if creditGrants != nil {
		if hasCreditGrants, ok := pluginruntime.GetBool(creditGrants, "hasCreditGrants"); ok && hasCreditGrants {
			total, okTotal := pluginruntime.GetNumber(creditGrants, "totalCents")
			used, okUsed := pluginruntime.GetNumber(creditGrants, "usedCents")
			if okTotal && okUsed && total > 0 && !math.IsNaN(total) && !math.IsNaN(used) {
				lines = append(lines, openusage.NewProgressLine("Credits", pluginruntime.Dollars(used), pluginruntime.Dollars(total), openusage.DollarsFormat(), openusage.ProgressLineOptions{}))
			}
		}
	}

	planOpts := openusage.ProgressLineOptions{PeriodDurationMs: billingPeriodMs}
	if resetsAt := pluginruntime.ToISO(usage["billingCycleEnd"]); resetsAt != "" {
		planOpts.ResetsAt = resetsAt
	}
	lines = append(lines, openusage.NewProgressLine("Plan usage", pluginruntime.Dollars(planUsed), pluginruntime.Dollars(limit), openusage.DollarsFormat(), planOpts))

	if bonusSpend, ok := pluginruntime.GetNumber(planUsage, "bonusSpend"); ok && bonusSpend > 0 {
		lines = append(lines, openusage.NewTextLine("Bonus spend", "$"+fmt.Sprint(pluginruntime.Dollars(bonusSpend)), openusage.TextLineOptions{}))
	}

	if spendLimit, ok := pluginruntime.GetMap(usage, "spendLimitUsage"); ok {
		limit, okLimit := pluginruntime.GetNumber(spendLimit, "individualLimit")
		if !okLimit {
			limit, okLimit = pluginruntime.GetNumber(spendLimit, "pooledLimit")
		}
		remaining, okRemaining := pluginruntime.GetNumber(spendLimit, "individualRemaining")
		if !okRemaining {
			remaining, _ = pluginruntime.GetNumber(spendLimit, "pooledRemaining")
		}
		if okLimit && limit > 0 {
			used := limit - remaining
			lines = append(lines, openusage.NewProgressLine("On-demand", pluginruntime.Dollars(used), pluginruntime.Dollars(limit), openusage.DollarsFormat(), openusage.ProgressLineOptions{}))
		}
	}

	return openusage.QueryResult{Plan: plan, Lines: lines}, nil
}

func (p *Plugin) readStateValue(key string) string {
	sql := fmt.Sprintf("SELECT value FROM ItemTable WHERE key = '%s' LIMIT 1;", key)
	jsonRows, err := pluginruntime.SQLiteQuery(stateDBPath, sql)
	if err != nil {
		return ""
	}
	rows, ok := pluginruntime.TryParseJSONArray(jsonRows)
	if !ok || len(rows) == 0 {
		return ""
	}
	row, ok := pluginruntime.Map(rows[0])
	if !ok {
		return ""
	}
	value, _ := pluginruntime.GetString(row, "value")
	return value
}

func (p *Plugin) writeStateValue(key, value string) bool {
	escaped := strings.ReplaceAll(value, "'", "''")
	sql := fmt.Sprintf("INSERT OR REPLACE INTO ItemTable (key, value) VALUES ('%s', '%s');", key, escaped)
	return pluginruntime.SQLiteExec(stateDBPath, sql) == nil
}

func (p *Plugin) tokenExpiration(token string) (int64, bool) {
	payload, ok := pluginruntime.DecodeJWTPayload(token)
	if !ok {
		return 0, false
	}
	exp, ok := pluginruntime.GetNumber(payload, "exp")
	if !ok {
		return 0, false
	}
	return int64(exp * 1000), true
}

func (p *Plugin) needsRefresh(accessToken string, nowMs int64) bool {
	if strings.TrimSpace(accessToken) == "" {
		return true
	}
	expiresAt, ok := p.tokenExpiration(accessToken)
	return pluginruntime.NeedsRefreshByExpiry(nowMs, expiresAt, refreshBufferMs, ok)
}

func (p *Plugin) refreshToken(ctx context.Context, env *pluginruntime.Env, refreshToken string) (string, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return "", nil
	}

	body := `{"grant_type":"refresh_token","client_id":"` + clientID + `","refresh_token":"` + refreshToken + `"}`
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
		if data, ok := pluginruntime.TryParseJSONMap(resp.Body); ok {
			if shouldLogout, ok := pluginruntime.GetBool(data, "shouldLogout"); ok && shouldLogout {
				return "", fmt.Errorf("session expired; sign in via cursor app")
			}
		}
		return "", fmt.Errorf("token expired; sign in via cursor app")
	}

	if resp.Status < 200 || resp.Status >= 300 {
		return "", nil
	}

	data, ok := pluginruntime.TryParseJSONMap(resp.Body)
	if !ok {
		return "", nil
	}
	if shouldLogout, ok := pluginruntime.GetBool(data, "shouldLogout"); ok && shouldLogout {
		return "", fmt.Errorf("session expired; sign in via cursor app")
	}
	newAccessToken, ok := pluginruntime.GetString(data, "access_token")
	if !ok || strings.TrimSpace(newAccessToken) == "" {
		return "", nil
	}

	p.writeStateValue("cursorAuth/accessToken", newAccessToken)
	return newAccessToken, nil
}

func (p *Plugin) connectPost(ctx context.Context, endpoint, token string) (pluginruntime.HTTPResponse, error) {
	return pluginruntime.DoHTTPRequest(ctx, pluginruntime.HTTPRequest{
		Method: "POST",
		URL:    endpoint,
		Headers: map[string]string{
			"Authorization":            "Bearer " + token,
			"Content-Type":             "application/json",
			"Connect-Protocol-Version": "1",
		},
		BodyText: "{}",
		Timeout:  10 * time.Second,
	})
}
