package process

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const userHZ = 100

var pageSize = int64(os.Getpagesize())

type procStat struct {
	RSSBytes   int64
	UtimeTicks int64
	StimeTicks int64
}

func readProcStat(pid int) (procStat, error) {
	var ps procStat

	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return ps, err
	}

	// /proc/<pid>/stat has the comm field (field 2) in parens which may
	// contain spaces. Find the last ')' to skip it reliably.
	closeParen := strings.LastIndexByte(string(data), ')')
	if closeParen < 0 || closeParen+1 >= len(data) {
		return ps, fmt.Errorf("malformed %s", statPath)
	}
	fields := strings.Fields(string(data)[closeParen+1:])
	// fields[0] = state (field 3), so utime = fields[11] (field 14),
	// stime = fields[12] (field 15).
	if len(fields) < 13 {
		return ps, fmt.Errorf("too few fields in %s", statPath)
	}
	ps.UtimeTicks, err = strconv.ParseInt(fields[11], 10, 64)
	if err != nil {
		return ps, fmt.Errorf("parse utime: %w", err)
	}
	ps.StimeTicks, err = strconv.ParseInt(fields[12], 10, 64)
	if err != nil {
		return ps, fmt.Errorf("parse stime: %w", err)
	}

	statmPath := fmt.Sprintf("/proc/%d/statm", pid)
	data, err = os.ReadFile(statmPath)
	if err != nil {
		return ps, err
	}
	statmFields := strings.Fields(string(data))
	if len(statmFields) < 2 {
		return ps, fmt.Errorf("too few fields in %s", statmPath)
	}
	rssPages, err := strconv.ParseInt(statmFields[1], 10, 64)
	if err != nil {
		return ps, fmt.Errorf("parse rss: %w", err)
	}
	ps.RSSBytes = rssPages * pageSize

	return ps, nil
}
