package pluginruntime

import (
	"fmt"
	"os/exec"
	"strings"
)

func SQLiteQuery(dbPath, sql string) (string, error) {
	if hasDotCommand(sql) {
		return "", fmt.Errorf("sqlite3 dot-commands are not allowed")
	}

	expanded := ExpandPath(dbPath)
	encoded := strings.NewReplacer(
		"%", "%25",
		" ", "%20",
		"#", "%23",
		"?", "%3F",
	).Replace(expanded)
	uri := fmt.Sprintf("file:%s?immutable=1", encoded)

	cmd := exec.Command("sqlite3", "-readonly", "-json", uri, sql)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("sqlite3 error: %s", msg)
	}

	return string(output), nil
}

func SQLiteExec(dbPath, sql string) error {
	if hasDotCommand(sql) {
		return fmt.Errorf("sqlite3 dot-commands are not allowed")
	}

	expanded := ExpandPath(dbPath)
	cmd := exec.Command("sqlite3", expanded, sql)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("sqlite3 error: %s", msg)
	}

	return nil
}

func hasDotCommand(sql string) bool {
	for _, line := range strings.Split(sql, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), ".") {
			return true
		}
	}
	return false
}
