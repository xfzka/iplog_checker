package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/imroc/req/v3"
	"github.com/sirupsen/logrus"
)

var SafeListData *ListGroup // 全局安全 IP 数据实例 (白名单)
var RiskListData *ListGroup // 全局风险 IP 数据实例

// LoadIPList 加载IP列表（通用函数，用于 safe_list 和 risk_list）
// wg: 可选的WaitGroup，用于等待初始加载完成（仅首次加载时使用）
func LoadIPList(lists []IPList, data *ListGroup, listType string, wg *sync.WaitGroup) {
	client := req.C()

	for _, list := range lists {
		if len(list.IPs) > 0 {
			ips, cidrs := parseLines(list.IPs)
			data.DelList(list.Name)
			data.AddList(NewNetListInfo(list.Name, list.Level), ips, cidrs)
			logrus.Infof("Loaded %d IPs and %d CIDRs from manual list [%s] %s", len(ips), len(cidrs), listType, list.Name)
			// 手动 IP 列表是同步加载的，不需要 WaitGroup
		} else if list.File != "" {
			// 从文件加载
			if wg != nil {
				wg.Add(1)
			}
			go func(list IPList) {
				// 首次加载
				err := loadFromFile(list, data, listType)
				if err != nil {
					logrus.Errorf("Failed to load from file %s: %v", list.File, err)
				}
				// 首次加载完成，通知 WaitGroup
				if wg != nil {
					wg.Done()
				}
				// 如果需要周期性更新，继续循环
				if list.UpdateIntervalParsed > 0 {
					for {
						var source string
						if list.Name != "" {
							source = list.Name
						} else {
							source = list.File
						}
						logrus.Debugf("Next update for %s (%s) after %s", source, listType, list.UpdateIntervalParsed.String())
						time.Sleep(list.UpdateIntervalParsed)
						err := loadFromFile(list, data, listType)
						if err != nil {
							logrus.Errorf("Failed to load from file %s: %v", list.File, err)
						}
					}
				}
			}(list)
		} else if list.URL != "" {
			// 从 URL 下载
			if wg != nil {
				wg.Add(1)
			}
			go func(list IPList) {
				// 首次加载
				err := downloadAndParse(client, list, data, listType)
				if err != nil {
					logrus.Errorf("Failed to download %s: %v", list.URL, err)
				}
				// 首次加载完成，通知 WaitGroup
				if wg != nil {
					wg.Done()
				}
				// 如果需要周期性更新，继续循环
				if list.UpdateIntervalParsed > 0 {
					for {
						logrus.Debugf("Next update for %s (%s) after %s", list.Name+": "+list.URL, listType, list.UpdateIntervalParsed.String())
						time.Sleep(list.UpdateIntervalParsed)
						err := downloadAndParse(client, list, data, listType)
						if err != nil {
							logrus.Errorf("Failed to download %s: %v", list.URL, err)
						}
					}
				}
			}(list)
		} else {
			logrus.Warnf("IP list [%s] %s has no source (file/url/ips), skipping", listType, list.Name)
		}
	}
}

// ipToUint32 将 netip.Addr 转换为 uint32 (大端序逻辑适配 Trie)
func ipToUint32(addr netip.Addr) uint32 {
	b := addr.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// downloadAndParse 下载并解析IP列表
func downloadAndParse(client *req.Client, list IPList, data *ListGroup, listType string) error {
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

	data.DelList(list.Name)
	data.AddList(NewNetListInfo(list.Name, list.Level), ips, cidrs)
	logrus.Infof("Downloaded %d IPs and %d CIDRs from [%s] %s, %s", len(ips), len(cidrs), listType, list.Name, list.URL)
	configMutex.RLock()
	isDebug := config.Logging.Level == "debug"
	configMutex.RUnlock()
	if isDebug {
		logrus.Debugf("Top 10 IP from %s:", list.Name)
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
func loadFromFile(list IPList, data *ListGroup, listType string) error {
	body, err := os.ReadFile(list.File)
	if err != nil {
		return err
	}

	ips, cidrs, err := parseIPsFromContent(list.Format, string(body), list.CSVColumn, list.JSONPath)
	if err != nil {
		return err
	}

	data.DelList(list.Name)
	data.AddList(NewNetListInfo(list.Name, list.Level), ips, cidrs)
	logrus.Infof("Loaded %d IPs and %d CIDRs from file [%s] %s", len(ips), len(cidrs), listType, list.Name)
	return nil
}

// parseIPsFromContent 根据格式解析IP列表（支持CIDR）
func parseIPsFromContent(format, body, csvColumn, jsonPath string) ([]uint32, []netip.Prefix, error) {
	switch strings.ToLower(format) {
	case "text", "": // 默认文本格式
		return parseText(body)
	case "csv":
		ips, cidrs, err := parseCSV(body, csvColumn)
		return ips, cidrs, err
	case "json":
		ips, cidrs, err := parseJSON(body, jsonPath)
		return ips, cidrs, err
	default:
		return nil, nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// parseText 解析文本格式，每行一个IP（支持CIDR）
func parseText(body string) ([]uint32, []netip.Prefix, error) {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}

	ips, cidrs := parseLines(lines)
	return ips, cidrs, scanner.Err()
}

// parseCSV 解析CSV格式
func parseCSV(body, column string) ([]uint32, []netip.Prefix, error) {
	reader := csv.NewReader(strings.NewReader(body))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}

	if len(records) == 0 {
		return nil, nil, nil
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
		return nil, nil, fmt.Errorf("column %s not found", column)
	}

	var lines []string
	for _, record := range records[1:] {
		if colIndex < len(record) {
			ipdata := strings.TrimSpace(record[colIndex])
			if ipdata != "" {
				lines = append(lines, ipdata)
			}
		}
	}

	ips, cidrs := parseLines(lines)
	return ips, cidrs, nil
}

// parseJSON 解析JSON格式
func parseJSON(body, path string) ([]uint32, []netip.Prefix, error) {
	var data interface{}
	err := json.Unmarshal([]byte(body), &data)
	if err != nil {
		return nil, nil, err
	}

	// 简单实现：假设path是顶级key
	if m, ok := data.(map[string]interface{}); ok {
		if val, exists := m[path]; exists {
			if arr, ok := val.([]interface{}); ok {
				var lines []string
				for _, item := range arr {
					if str, ok := item.(string); ok {
						lines = append(lines, str)
					}
				}
				ips, cidrs := parseLines(lines)
				return ips, cidrs, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("path %s not found or not an array", path)
}

// parseLines 解析多行文本，返回IP和CIDR列表
func parseLines(text []string) ([]uint32, []netip.Prefix) {
	var ips []uint32
	var cidrs []netip.Prefix
	for _, line := range text {
		if strings.Contains(line, "/") {
			// CIDR格式
			// 暂时不处理CIDR
			perfix, err := netip.ParsePrefix(line)
			if err != nil {
				logrus.Warnf("Invalid CIDR: %s, skipping", line)
				continue
			}
			cidrs = append(cidrs, perfix)
		} else {
			ip, err := IPv4ToUint32(line)
			if err != nil {
				logrus.Warnf("Invalid IP: %s, skipping", line)
				continue
			}
			ips = append(ips, ip)
		}
	}
	return ips, cidrs
}

// IsIPInSafeList 检查 IP 是否在安全列表中
func IsIPInSafeList(ip uint32) bool {
	if SafeListData == nil {
		return false
	}
	found, _ := SafeListData.Contains(ip)
	return found
}

// IsSensitiveIP 检测IP是否敏感
func IsSensitiveIP(ip uint32) (bool, ListInfo) {
	// 判断是否在安全列表中（白名单）
	if IsIPInSafeList(ip) {
		return false, ListInfo{}
	}
	// 判断是否在风险IP列表中
	if RiskListData == nil {
		return false, ListInfo{}
	}
	found, info := RiskListData.Contains(ip)
	if found {
		return true, info
	}
	return false, ListInfo{}
}
