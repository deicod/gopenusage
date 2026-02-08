package pluginruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type LSDiscoverOptions struct {
	ProcessName string
	Markers     []string
	CSRFFlag    string
	PortFlag    string
	ExtraFlags  []string
}

type LSDiscoverResult struct {
	PID           int               `json:"pid"`
	CSRF          string            `json:"csrf"`
	Ports         []int             `json:"ports"`
	Extra         map[string]string `json:"extra,omitempty"`
	ExtensionPort *int              `json:"extensionPort,omitempty"`
}

func DiscoverLS(opts LSDiscoverOptions) (*LSDiscoverResult, error) {
	if opts.ProcessName == "" {
		return nil, fmt.Errorf("process name is required")
	}
	if len(opts.Markers) == 0 {
		return nil, fmt.Errorf("at least one marker is required")
	}
	if opts.CSRFFlag == "" {
		return nil, fmt.Errorf("csrf flag is required")
	}

	psOutput, err := exec.Command("/bin/ps", "-ax", "-o", "pid=,command=").Output()
	if err != nil {
		return nil, nil
	}

	processNameLower := strings.ToLower(opts.ProcessName)
	markersLower := make([]string, len(opts.Markers))
	for i, marker := range opts.Markers {
		markersLower[i] = strings.ToLower(marker)
	}

	var foundPID int
	var foundCommand string

	for _, line := range strings.Split(string(psOutput), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		parts := strings.Fields(trimmed)
		if len(parts) < 2 {
			continue
		}

		pidValue, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		command := strings.TrimSpace(strings.TrimPrefix(trimmed, parts[0]))
		commandLower := strings.ToLower(command)

		if !strings.Contains(commandLower, processNameLower) {
			continue
		}

		ideName := strings.ToLower(extractFlag(command, "--ide_name"))
		appDataDir := strings.ToLower(extractFlag(command, "--app_data_dir"))

		hasMarker := false
		for _, marker := range markersLower {
			switch {
			case ideName != "":
				hasMarker = ideName == marker
			case appDataDir != "":
				hasMarker = appDataDir == marker
			default:
				hasMarker = strings.Contains(commandLower, "/"+marker+"/")
			}
			if hasMarker {
				break
			}
		}
		if !hasMarker {
			continue
		}

		foundPID = pidValue
		foundCommand = command
		break
	}

	if foundPID == 0 {
		return nil, nil
	}

	csrf := extractFlag(foundCommand, opts.CSRFFlag)
	if csrf == "" {
		return nil, nil
	}

	var extensionPort *int
	if opts.PortFlag != "" {
		if raw := extractFlag(foundCommand, opts.PortFlag); raw != "" {
			if port, err := strconv.Atoi(raw); err == nil {
				extensionPort = &port
			}
		}
	}

	extra := make(map[string]string, len(opts.ExtraFlags))
	for _, flag := range opts.ExtraFlags {
		if value := extractFlag(foundCommand, flag); value != "" {
			key := strings.TrimLeft(flag, "-")
			extra[key] = value
		}
	}

	ports := parsePortsFromLsof(foundPID)
	if len(ports) == 0 && extensionPort == nil {
		return nil, nil
	}

	return &LSDiscoverResult{
		PID:           foundPID,
		CSRF:          csrf,
		Ports:         ports,
		Extra:         extra,
		ExtensionPort: extensionPort,
	}, nil
}

func CallLS(ctx context.Context, scheme string, port int, csrf, service, method string, body string, timeout time.Duration) (HTTPResponse, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if scheme == "" {
		scheme = "http"
	}
	return DoHTTPRequest(ctx, HTTPRequest{
		Method: method,
		URL:    fmt.Sprintf("%s://127.0.0.1:%d/%s", scheme, port, service),
		Headers: map[string]string{
			"Content-Type":             "application/json",
			"Connect-Protocol-Version": "1",
			"x-codeium-csrf-token":     csrf,
		},
		BodyText:             body,
		Timeout:              timeout,
		DangerouslyIgnoreTLS: scheme == "https",
	})
}

func extractFlag(command, flag string) string {
	parts := strings.Fields(command)
	flagEq := flag + "="
	for i, part := range parts {
		if part == flag {
			if i+1 < len(parts) {
				return parts[i+1]
			}
		}
		if strings.HasPrefix(part, flagEq) {
			return strings.TrimPrefix(part, flagEq)
		}
	}
	return ""
}

func parsePortsFromLsof(pid int) []int {
	lsofPath := ""
	for _, path := range []string{"/usr/sbin/lsof", "/usr/bin/lsof", "lsof"} {
		if path == "lsof" {
			lsofPath = path
			break
		}
		if _, err := os.Stat(path); err == nil {
			lsofPath = path
			break
		}
	}
	if lsofPath == "" {
		return nil
	}

	output, err := exec.Command(lsofPath, "-nP", "-iTCP", "-sTCP:LISTEN", "-a", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return nil
	}

	set := make(map[int]struct{})
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.Contains(line, "LISTEN") {
			continue
		}
		tokens := strings.Fields(line)
		for i := len(tokens) - 1; i >= 0; i-- {
			token := tokens[i]
			colon := strings.LastIndex(token, ":")
			if colon < 0 {
				continue
			}
			port, err := strconv.Atoi(token[colon+1:])
			if err != nil {
				continue
			}
			if port > 0 && port < 65536 {
				set[port] = struct{}{}
				break
			}
		}
	}

	ports := make([]int, 0, len(set))
	for port := range set {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}
