package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/imroc/req/v3"
	"github.com/sirupsen/logrus"
)

// NewIPData 创建新的IPData
func NewIPData() *IPData {
	return &IPData{
		ips:   make(map[string][]uint32),
		cidrs: make(map[string][]*IPNet),
	}
}

// Set 设置IP列表
func (r *IPData) Set(name string, ips []uint32, cidrs []*IPNet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ips[name] = ips
	r.cidrs[name] = cidrs
}

// GetAllIPs 获取所有单个IP
func (r *IPData) GetAllIPs() []uint32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []uint32
	for _, ips := range r.ips {
		all = append(all, ips...)
	}
	return all
}

// GetTotalCount 获取总数
func (r *IPData) GetTotalCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := 0
	for _, ips := range r.ips {
		total += len(ips)
	}
	for _, cidrs := range r.cidrs {
		total += len(cidrs)
	}
	return total
}

// Contains 检查IP是否在列表中（支持单IP和CIDR匹配）
func (r *IPData) Contains(ip uint32) (bool, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 检查单个IP
	for name, ips := range r.ips {
		for _, listIP := range ips {
			if ip == listIP {
				return true, name
			}
		}
	}

	// 检查CIDR
	for name, cidrs := range r.cidrs {
		for _, cidr := range cidrs {
			if ip >= cidr.Start && ip <= cidr.End {
				return true, name
			}
		}
	}

	return false, ""
}

// LoadIPList 加载IP列表（通用函数，用于 safe_list 和 risk_list）
func LoadIPList(lists []IPList, data *IPData, listType string) {
	client := req.C()

	for _, list := range lists {
		if len(list.IPs) > 0 {
			// 从手动配置的IP列表加载
			ips, cidrs := parseIPStrings(list.IPs)
			data.Set(list.Name, ips, cidrs)
			logrus.Infof("Loaded %d IPs and %d CIDRs from manual list [%s] %s", len(ips), len(cidrs), listType, list.Name)
		} else if list.File != "" {
			// 从文件加载
			go func(list IPList) {
				for {
					err := loadFromFile(list, data, listType)
					if err != nil {
						logrus.Errorf("Failed to load from file %s: %v", list.File, err)
					}
					if list.UpdateIntervalParsed > 0 {
						// Debug: 输出下一次更新等待时间
						var source string
						if list.Name != "" {
							source = list.Name
						} else {
							source = list.File
						}
						logrus.Debugf("Next update for %s (%s) after %s", source, listType, list.UpdateIntervalParsed.String())
						time.Sleep(list.UpdateIntervalParsed)
					} else {
						return // 没有设置更新间隔，只加载一次
					}
				}
			}(list)
		} else if list.URL != "" {
			// 从 URL 下载
			go func(list IPList) {
				for {
					err := downloadAndParse(client, list, data, listType)
					if err != nil {
						logrus.Errorf("Failed to download %s: %v", list.URL, err)
					}
					if list.UpdateIntervalParsed > 0 {
						// Debug: 输出下一次更新等待时间 (简化 source 构建)
						logrus.Debugf("Next update for %s (%s) after %s", list.Name+": "+list.URL, listType, list.UpdateIntervalParsed.String())
						time.Sleep(list.UpdateIntervalParsed)
					} else {
						return // 没有设置更新间隔，只加载一次
					}
				}
			}(list)
		} else {
			logrus.Warnf("IP list [%s] %s has no source (file/url/ips), skipping", listType, list.Name)
		}
	}
}

// parseIPStrings 解析IP字符串列表（支持单IP和CIDR）
func parseIPStrings(ipStrings []string) ([]uint32, []*IPNet) {
	var ips []uint32
	var cidrs []*IPNet

	for _, entry := range ipStrings {
		entry = strings.TrimSpace(entry)
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}

		if strings.Contains(entry, "/") {
			// CIDR格式
			cidr := parseCIDR(entry)
			if cidr != nil {
				cidrs = append(cidrs, cidr)
			}
		} else {
			// 单个IP
			ip, err := IPv4ToUint32(entry)
			if err != nil {
				logrus.Warnf("Invalid IP: %s, skipping", entry)
				continue
			}
			ips = append(ips, ip)
		}
	}

	return ips, cidrs
}

// parseCIDR 解析CIDR格式并返回IPNet
func parseCIDR(cidrStr string) *IPNet {
	_, network, err := net.ParseCIDR(cidrStr)
	if err != nil {
		logrus.Warnf("Invalid CIDR: %s, skipping", cidrStr)
		return nil
	}

	// 计算起始和结束IP
	startIP := ipToUint32(network.IP)
	mask := network.Mask
	ones, bits := mask.Size()
	hostBits := uint(bits - ones)
	endIP := startIP | (uint32(1<<hostBits) - 1)

	return &IPNet{
		Start: startIP,
		End:   endIP,
	}
}

// ipToUint32 将net.IP转换为uint32
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// downloadAndParse 下载并解析IP列表
func downloadAndParse(client *req.Client, list IPList, data *IPData, listType string) error {
	c := client
	if list.TimeoutParsed > 0 {
		c = client.SetTimeout(list.TimeoutParsed)
	}

	req := c.R().SetHeaders(list.CustomHeaders)
	if list.RetryCount > 0 {
		req.SetRetryCount(list.RetryCount)
	}

	resp, err := req.Get(list.URL)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body := resp.String()

	ips, cidrs, err := parseIPsFromContent(list.Format, body, list.CSVColumn, list.JSONPath)
	if err != nil {
		return err
	}

	data.Set(list.Name, ips, cidrs)
	source := list.Name
	if source == "" {
		source = list.URL
	}
	logrus.Infof("Downloaded %d IPs and %d CIDRs from [%s] %s, %s", len(ips), len(cidrs), listType, source, list.URL)
	if config.Logging.Level == "debug" {
		logrus.Debugf("Top 10 IP from %s:", source)
		for i, ip := range ips {
			if i >= 10 {
				break
			}
			logrus.Debugf(" - %s", Uint32ToIPv4(ip).String())
		}
	}
	return nil
}

// loadFromFile 从文件加载IP列表
func loadFromFile(list IPList, data *IPData, listType string) error {
	body, err := os.ReadFile(list.File)
	if err != nil {
		return err
	}

	ips, cidrs, err := parseIPsFromContent(list.Format, string(body), list.CSVColumn, list.JSONPath)
	if err != nil {
		return err
	}

	data.Set(list.Name, ips, cidrs)
	source := list.Name
	if source == "" {
		source = list.File
	}
	logrus.Infof("Loaded %d IPs and %d CIDRs from file [%s] %s", len(ips), len(cidrs), listType, source)
	return nil
}

// parseText 解析文本格式，每行一个IP（支持CIDR）
func parseText(body string) ([]uint32, []*IPNet, error) {
	var ips []uint32
	var cidrs []*IPNet

	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.Contains(line, "/") {
			// CIDR格式
			cidr := parseCIDR(line)
			if cidr != nil {
				cidrs = append(cidrs, cidr)
			}
		} else {
			ip, err := IPv4ToUint32(line)
			if err != nil {
				logrus.Warnf("Invalid IP: %s, skipping", line)
				continue
			}
			ips = append(ips, ip)
		}
	}
	return ips, cidrs, scanner.Err()
}

// parseIPsFromContent 根据格式解析IP列表（支持CIDR）
func parseIPsFromContent(format, body, csvColumn, jsonPath string) ([]uint32, []*IPNet, error) {
	switch strings.ToLower(format) {
	case "text", "":
		return parseText(body)
	case "csv":
		ips, err := parseCSV(body, csvColumn)
		return ips, nil, err
	case "json":
		ips, err := parseJSON(body, jsonPath)
		return ips, nil, err
	default:
		return nil, nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// parseCSV 解析CSV格式
func parseCSV(body, column string) ([]uint32, error) {
	reader := csv.NewReader(strings.NewReader(body))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}

	// 找到列索引
	headers := records[0]
	colIndex := -1
	for i, h := range headers {
		if h == column {
			colIndex = i
			break
		}
	}
	if colIndex == -1 {
		return nil, fmt.Errorf("column %s not found", column)
	}

	var ips []uint32
	for _, record := range records[1:] {
		if colIndex < len(record) {
			ipStr := strings.TrimSpace(record[colIndex])
			if ipStr != "" {
				ip, err := IPv4ToUint32(ipStr)
				if err != nil {
					logrus.Warnf("Invalid IP: %s, skipping", ipStr)
					continue
				}
				ips = append(ips, ip)
			}
		}
	}
	return ips, nil
}

// parseJSON 解析JSON格式
func parseJSON(body, path string) ([]uint32, error) {
	var data interface{}
	err := json.Unmarshal([]byte(body), &data)
	if err != nil {
		return nil, err
	}

	// 简单实现：假设path是顶级key
	if m, ok := data.(map[string]interface{}); ok {
		if val, exists := m[path]; exists {
			if arr, ok := val.([]interface{}); ok {
				var ips []uint32
				for _, item := range arr {
					if str, ok := item.(string); ok {
						ip, err := IPv4ToUint32(str)
						if err != nil {
							logrus.Warnf("Invalid IP: %s, skipping", str)
							continue
						}
						ips = append(ips, ip)
					}
				}
				return ips, nil
			}
		}
	}
	return nil, fmt.Errorf("path %s not found or not an array", path)
}
