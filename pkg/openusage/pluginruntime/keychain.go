package pluginruntime

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

var acctPattern = regexp.MustCompile(`"acct"<blob>="([^"]+)"`)

func ReadKeychainGenericPassword(service string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("keychain API is only supported on macOS")
	}

	output, err := exec.Command("security", "find-generic-password", "-s", service, "-w").CombinedOutput()
	if err != nil {
		line := strings.TrimSpace(firstLine(string(output)))
		if line == "" {
			line = err.Error()
		}
		return "", fmt.Errorf("keychain item not found: %s", line)
	}

	return strings.TrimSpace(string(output)), nil
}

func WriteKeychainGenericPassword(service, value string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("keychain API is only supported on macOS")
	}

	account := ""
	findOut, err := exec.Command("security", "find-generic-password", "-s", service).CombinedOutput()
	if err == nil {
		if matches := acctPattern.FindStringSubmatch(string(findOut)); len(matches) == 2 {
			account = matches[1]
		}
	}

	args := []string{"add-generic-password", "-s", service}
	if account != "" {
		args = append(args, "-a", account)
	}
	args = append(args, "-w", value, "-U")

	output, err := exec.Command("security", args...).CombinedOutput()
	if err != nil {
		line := strings.TrimSpace(firstLine(string(output)))
		if line == "" {
			line = err.Error()
		}
		return fmt.Errorf("keychain write failed: %s", line)
	}

	return nil
}

func DeleteKeychainGenericPassword(service string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("keychain API is only supported on macOS")
	}

	output, err := exec.Command("security", "delete-generic-password", "-s", service).CombinedOutput()
	if err != nil {
		line := strings.TrimSpace(firstLine(string(output)))
		if line == "" {
			line = err.Error()
		}
		return fmt.Errorf("keychain delete failed: %s", line)
	}
	return nil
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
