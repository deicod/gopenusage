package pluginruntime

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var numericPattern = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
var datetimeNoTZPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(\.\d+)?$`)
var datetimeWithTZPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)
var tzNoColonPattern = regexp.MustCompile(`[+-]\d{4}$`)

func PlanLabel(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}

	runes := []rune(text)
	for i := range runes {
		if unicode.IsLower(runes[i]) && (i == 0 || unicode.IsSpace(runes[i-1])) {
			runes[i] = unicode.ToUpper(runes[i])
		}
	}
	return string(runes)
}

func Dollars(cents float64) float64 {
	return math.Round(cents) / 100
}

func ParseDateMs(value any) (int64, bool) {
	switch v := value.(type) {
	case time.Time:
		return v.UnixMilli(), true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case float32:
		if !isFinite(float64(v)) {
			return 0, false
		}
		return int64(v), true
	case float64:
		if !isFinite(v) {
			return 0, false
		}
		return int64(v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, true
		}
		f, err := v.Float64()
		if err != nil || !isFinite(f) {
			return 0, false
		}
		return int64(f), true
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false
		}
		if t, ok := parseTimeString(s); ok {
			return t.UnixMilli(), true
		}
		if n, err := strconv.ParseFloat(s, 64); err == nil && isFinite(n) {
			return int64(n), true
		}
	}
	return 0, false
}

func ToISO(value any) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return ""
		}

		if strings.Contains(s, " ") && strings.HasPrefix(s, "20") {
			// YYYY-MM-DD HH:MM:SS -> YYYY-MM-DDTHH:MM:SS
			s = strings.Replace(s, " ", "T", 1)
		}
		if strings.HasSuffix(s, " UTC") {
			s = strings.TrimSuffix(s, " UTC") + "Z"
		}

		if numericPattern.MatchString(s) {
			n, err := strconv.ParseFloat(s, 64)
			if err != nil || !isFinite(n) {
				return ""
			}
			return numberToISO(n)
		}

		if tzNoColonPattern.MatchString(s) {
			s = s[:len(s)-2] + ":" + s[len(s)-2:]
		}

		if m := datetimeWithTZPattern.FindStringSubmatch(s); len(m) == 4 {
			head := m[1]
			frac := normalizeFraction(m[2])
			tz := m[3]
			s = head + frac + tz
		} else if m := datetimeNoTZPattern.FindStringSubmatch(s); len(m) == 3 {
			head := m[1]
			frac := normalizeFraction(m[2])
			s = head + frac + "Z"
		}

		if t, ok := parseTimeString(s); ok {
			return formatISO(t)
		}
		return ""
	case int:
		return numberToISO(float64(v))
	case int32:
		return numberToISO(float64(v))
	case int64:
		return numberToISO(float64(v))
	case float32:
		return numberToISO(float64(v))
	case float64:
		return numberToISO(v)
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return ""
		}
		return numberToISO(f)
	case time.Time:
		return formatISO(v)
	}

	return ""
}

func NeedsRefreshByExpiry(nowMs, expiresAtMs, bufferMs int64, hasExpiry bool) bool {
	if !hasExpiry {
		return true
	}
	return nowMs+bufferMs >= expiresAtMs
}

func TryParseJSONMap(text string) (map[string]any, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, false
	}
	var out map[string]any
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, false
	}
	return out, true
}

func TryParseJSONArray(text string) ([]any, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, false
	}
	var out []any
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, false
	}
	return out, true
}

func JSONMarshalIndent(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func JSONMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func DecodeBase64(value string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return string(decoded), nil
	}
	decoded, err = base64.RawStdEncoding.DecodeString(value)
	if err == nil {
		return string(decoded), nil
	}
	decoded, err = base64.URLEncoding.DecodeString(value)
	if err == nil {
		return string(decoded), nil
	}
	decoded, err = base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		return string(decoded), nil
	}
	return "", err
}

func DecodeJWTPayload(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	decoded, err := DecodeBase64(parts[1])
	if err != nil {
		return nil, false
	}
	payload, ok := TryParseJSONMap(decoded)
	if !ok {
		return nil, false
	}
	return payload, true
}

func Number(value any) (float64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case float64:
		if !isFinite(v) {
			return 0, false
		}
		return v, true
	case float32:
		if !isFinite(float64(v)) {
			return 0, false
		}
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil || !isFinite(f) {
			return 0, false
		}
		return f, true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || !isFinite(n) {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func Int(value any) (int64, bool) {
	n, ok := Number(value)
	if !ok {
		return 0, false
	}
	return int64(n), true
}

func String(value any) (string, bool) {
	if value == nil {
		return "", false
	}
	s, ok := value.(string)
	if !ok {
		return "", false
	}
	return s, true
}

func Bool(value any) (bool, bool) {
	if value == nil {
		return false, false
	}
	b, ok := value.(bool)
	return b, ok
}

func Map(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	m, ok := value.(map[string]any)
	return m, ok
}

func Array(value any) ([]any, bool) {
	if value == nil {
		return nil, false
	}
	a, ok := value.([]any)
	return a, ok
}

func GetMap(m map[string]any, key string) (map[string]any, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	return Map(v)
}

func GetString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	return String(v)
}

func GetNumber(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	return Number(v)
}

func GetBool(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	return Bool(v)
}

func parseTimeString(s string) (time.Time, bool) {
	layouts := []struct {
		layout    string
		assumeUTC bool
	}{
		{layout: time.RFC3339Nano, assumeUTC: false},
		{layout: "2006-01-02T15:04:05.999999999Z07:00", assumeUTC: false},
		{layout: "2006-01-02T15:04:05.999999999", assumeUTC: true},
		{layout: "2006-01-02T15:04:05", assumeUTC: true},
		{layout: "2006-01-02", assumeUTC: true},
	}

	for _, item := range layouts {
		t, err := time.Parse(item.layout, s)
		if err == nil {
			if item.assumeUTC {
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
			}
			return t, true
		}
	}

	return time.Time{}, false
}

func normalizeFraction(frac string) string {
	if frac == "" {
		return ""
	}
	digits := strings.TrimPrefix(frac, ".")
	if len(digits) > 3 {
		digits = digits[:3]
	}
	for len(digits) < 3 {
		digits += "0"
	}
	return "." + digits
}

func numberToISO(n float64) string {
	if !isFinite(n) {
		return ""
	}
	ms := n
	if math.Abs(n) < 1e10 {
		ms = n * 1000
	}
	t := time.UnixMilli(int64(ms))
	return formatISO(t)
}

func formatISO(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func AsErrorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
