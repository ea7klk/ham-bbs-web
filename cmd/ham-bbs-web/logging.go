package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func (s *server) logBBSAction(callsign, action, format string, args ...any) {
	detail := ""
	if format != "" {
		detail = " " + fmt.Sprintf(format, args...)
	}
	appendLogFile(s.cfg.bbsLog, fmt.Sprintf("%s web user=%s action=%s%s\n", now(), callsign, action, detail))
}

func appendLogFile(path, text string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(text)
}
