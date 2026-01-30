package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var Whitelist []string
var WhitelistIPs []uint32
var WhitelistCIDRs []*net.IPNet
var RiskIPDataInstance *RiskIPData

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
func Uint32ToIPv4(ip uint32) net.IP {
	return net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}

// InitWhitelist 初始化白名单，将字符串列表转换为预处理的uint32和CIDR
func InitWhitelist(whitelist []string) {
	WhitelistIPs = nil
	WhitelistCIDRs = nil
	for _, entry := range whitelist {
		if strings.Contains(entry, "/") {
			_, network, err := net.ParseCIDR(entry)
			if err != nil {
				logrus.Warnf("Invalid CIDR in whitelist: %s", entry)
				continue // invalid, skip
			}
			WhitelistCIDRs = append(WhitelistCIDRs, network)
		} else {
			ipUint, err := IPv4ToUint32(entry)
			if err != nil {
				logrus.Warnf("Invalid IP in whitelist: %s", entry)
				continue
			}
			WhitelistIPs = append(WhitelistIPs, ipUint)
		}
	}
	logrus.Infof("Initialized whitelist with %d IP/CIDR", len(WhitelistIPs))
}

// IsIPInWhitelist 检查 IP 是否在白名单内
func IsIPInWhitelist(ip uint32) bool {
	// 检查单个 IP
	for _, wip := range WhitelistIPs {
		if ip == wip {
			return true
		}
	}
	// 检查 CIDR
	ipNet := Uint32ToIPv4(ip)
	for _, network := range WhitelistCIDRs {
		if network.Contains(ipNet) {
			return true
		}
	}
	return false
}

// IsSensitiveIP 检测IP是否敏感
func IsSensitiveIP(ip uint32) (bool, string) {
	// 2. 判断是否在白名单中
	if IsIPInWhitelist(ip) {
		return false, ""
	}
	// 3. 判断是否在风险IP列表中
	if RiskIPDataInstance == nil {
		return false, ""
	}
	RiskIPDataInstance.mu.RLock()
	defer RiskIPDataInstance.mu.RUnlock()
	for name, ips := range RiskIPDataInstance.data {
		for _, riskIP := range ips {
			if ip == riskIP {
				return true, name
			}
		}
	}
	return false, ""
}
