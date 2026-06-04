package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func hookLog(statePath, hook, session string, kv ...string) {
	if statePath == "" {
		return
	}
	logPath := filepath.Join(filepath.Dir(statePath), "hooks.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	var b strings.Builder
	fmt.Fprintf(&b, "%s\t%s\tsess=%s", time.Now().UTC().Format(time.RFC3339), hook, shortID(session))
	for i := 0; i+1 < len(kv); i += 2 {
		fmt.Fprintf(&b, "\t%s=%s", kv[i], kv[i+1])
	}
	b.WriteByte('\n')
	f.WriteString(b.String())
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
