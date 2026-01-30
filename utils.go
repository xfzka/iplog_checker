package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseDuration 解析时间字符串，如 "30d" -> 30*24*time.Hour
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	var multiplier time.Duration = 1
	switch strings.ToLower(s[len(s)-1:]) {
	case "d":
		multiplier = 24 * time.Hour
		s = s[:len(s)-1]
	case "h":
		multiplier = time.Hour
		s = s[:len(s)-1]
	case "m":
		multiplier = time.Minute
		s = s[:len(s)-1]
	case "s":
		multiplier = time.Second
		s = s[:len(s)-1]
	default:
		return 0, fmt.Errorf("invalid duration unit")
	}
	d, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(d) * multiplier, nil
}
