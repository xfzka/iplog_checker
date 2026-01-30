package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/imroc/req/v3"
	"github.com/sirupsen/logrus"
)

// RiskIPData 存储下载的风险IP数据
type RiskIPData struct {
	mu   sync.RWMutex
	data map[string][]string // key: URL, value: IP list
}

// NewRiskIPData 创建新的RiskIPData
func NewRiskIPData() *RiskIPData {
	return &RiskIPData{
		data: make(map[string][]string),
	}
}

// Set 设置IP列表
func (r *RiskIPData) Set(url string, ips []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[url] = ips
}

// GetAll 获取所有IP
func (r *RiskIPData) GetAll() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []string
	for _, ips := range r.data {
		all = append(all, ips...)
	}
	return all
}

// GetTotalLines 获取总行数
func (r *RiskIPData) GetTotalLines() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := 0
	for _, ips := range r.data {
		total += len(ips)
	}
	return total
}

// DownloadRiskIPs 下载风险IP列表
func DownloadRiskIPs(config *Config, data *RiskIPData) {
	client := req.C()

	for _, list := range config.RiskIPLists {
		go func(list RiskIPList) {
			for {
				err := downloadAndParse(client, list, data)
				if err != nil {
					logrus.Errorf("Failed to download %s: %v", list.URL, err)
				}
				time.Sleep(list.UpdateInterval)
			}
		}(list)
	}
}

// downloadAndParse 下载并解析IP列表
func downloadAndParse(client *req.Client, list RiskIPList, data *RiskIPData) error {
	c := client
	if list.Timeout > 0 {
		c = client.SetTimeout(list.Timeout)
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

	var ips []string
	switch strings.ToLower(list.Format) {
	case "text":
		ips, err = parseText(body)
	case "csv":
		ips, err = parseCSV(body, list.CSVColumn)
	case "json":
		ips, err = parseJSON(body, list.JSONPath)
	default:
		return fmt.Errorf("unsupported format: %s", list.Format)
	}

	if err != nil {
		return err
	}

	data.Set(list.URL, ips)
	logrus.Infof("Downloaded %d IPs from %s", len(ips), list.URL)
	return nil
}

// parseText 解析文本格式，每行一个IP
func parseText(body string) ([]string, error) {
	var ips []string
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			ips = append(ips, line)
		}
	}
	return ips, scanner.Err()
}

// parseCSV 解析CSV格式
func parseCSV(body, column string) ([]string, error) {
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

	var ips []string
	for _, record := range records[1:] {
		if colIndex < len(record) {
			ip := strings.TrimSpace(record[colIndex])
			if ip != "" {
				ips = append(ips, ip)
			}
		}
	}
	return ips, nil
}

// parseJSON 解析JSON格式
func parseJSON(body, path string) ([]string, error) {
	var data interface{}
	err := json.Unmarshal([]byte(body), &data)
	if err != nil {
		return nil, err
	}

	// 简单实现：假设path是顶级key
	if m, ok := data.(map[string]interface{}); ok {
		if val, exists := m[path]; exists {
			if arr, ok := val.([]interface{}); ok {
				var ips []string
				for _, item := range arr {
					if str, ok := item.(string); ok {
						ips = append(ips, str)
					}
				}
				return ips, nil
			}
		}
	}
	return nil, fmt.Errorf("path %s not found or not an array", path)
}
