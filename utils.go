package main

import (
	"fmt"
	"net/netip"
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

// IPv4ToUint32 将IPv4地址字符串转换为uint32
func IPv4ToUint32(ip string) (uint32, error) {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return 0, fmt.Errorf("invalid IPv4 address: %s", ip)
	}
	var result uint32
	for i, part := range parts {
		num, err := strconv.Atoi(part)
		if err != nil || num < 0 || num > 255 {
			return 0, fmt.Errorf("invalid IPv4 address: %s", ip)
		}
		result |= uint32(num) << ((3 - i) * 8)
	}
	return result, nil
}

// Uint32ToIPv4 将uint32转换为net.IP
func Uint32ToIPv4(ip uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(ip >> 24), byte(ip >> 16), byte(ip >> 8), byte(ip)})
}
