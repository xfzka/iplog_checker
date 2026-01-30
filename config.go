package main

import (
	"fmt"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// Config 表示根配置结构
type Config struct {
	Logging          Logging `yaml:"logging"`            // 日志配置
	AutoReloadConfig bool    `yaml:"auto_reload_config"` // 配置自动重载 (watch 形式)

	RiskIPLists   []RiskIPList   `yaml:"risk_ip_lists"` // 风险 IP 列表配置
	WhitelistIPs  []string       `yaml:"whitelist_ips"` // 白名单 IP (支持单个 IP 或 CIDR 范围)
	IPLogFiles    []IPLogFile    `yaml:"ip_log_files"`  // 监控的 IP 日志文件
	Notifications []Notification `yaml:"notifications"` // 通知配置
}

// Logging 日志配置
type Logging struct {
	Level string `yaml:"level"` // 日志级别: debug, info, warn, error
	To    string `yaml:"to"`    // 日志文件路径
}

// RiskIPList 风险 IP 列表配置
type RiskIPList struct {
	Name           string            `yaml:"name"`                     // 风险 IP 列表名称 (用于日志输出和标记 IP 来源)
	URL            string            `yaml:"url"`                      // 风险 IP 列表的 URL
	UpdateInterval time.Duration     `yaml:"update_interval"`          // 更新间隔 (支持 h/m/s, 默认 24h)
	Format         string            `yaml:"format"`                   // 格式: text, csv, json
	Timeout        time.Duration     `yaml:"timeout,omitempty"`        // 请求超时 (默认 30s)
	RetryCount     int               `yaml:"retry_count,omitempty"`    // 重试次数 (默认 3)
	CSVColumn      string            `yaml:"csv_column,omitempty"`     // CSV 列名 (仅 csv 格式)
	JSONPath       string            `yaml:"json_path,omitempty"`      // JSON 路径 (仅 json 格式)
	CustomHeaders  map[string]string `yaml:"custom_headers,omitempty"` // 自定义请求头
}

// IPLogFile IP 日志文件配置
type IPLogFile struct {
	Path         string        `yaml:"path"`                    // 日志文件路径
	ReadMode     string        `yaml:"read_mode"`               // 读取模式: watch (持续监控), once (一次性)
	ReadInterval time.Duration `yaml:"read_interval,omitempty"` // 一次性读取间隔 (仅 once 模式, 默认 24h)
}

// Notification 通知配置
type Notification struct {
	Method     string      `yaml:"method"`                // 通知方法: curl, email, slack, webhook
	Threshold  int         `yaml:"threshold"`             // 预警阈值 (命中次数)
	CurlConfig *CurlConfig `yaml:"curl_config,omitempty"` // curl 配置 (仅 method 为 curl 时使用)
	// Add other notification methods here if needed
}

// CurlConfig curl 通知方法配置
type CurlConfig struct {
	URL             string            `yaml:"url"`                   // 通知端点 URL
	Headers         map[string]string `yaml:"headers,omitempty"`     // 请求头
	PayloadTemplate string            `yaml:"payload_template"`      // 负载模板 (使用 Go 模板语法)
	Timeout         time.Duration     `yaml:"timeout,omitempty"`     // 请求超时 (默认 10s)
	RetryCount      int               `yaml:"retry_count,omitempty"` // 重试次数 (默认 2)
}

// watchConfigFile 监控配置文件变更并自动重载
func watchConfigFile(configPath string, config *Config) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("Failed to create file watcher: %v", err)
		return
	}
	defer watcher.Close()

	err = watcher.Add(configPath)
	if err != nil {
		logrus.Errorf("Failed to watch config file: %v", err)
		return
	}

	logrus.Info("Started watching config file for changes")

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				logrus.Info("Config file changed, reloading...")
				err := reloadConfig(configPath, config)
				if err != nil {
					logrus.Errorf("Failed to reload config: %v", err)
				} else {
					logrus.Info("Config reloaded successfully")
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logrus.Errorf("File watcher error: %v", err)
		}
	}
}

// reloadConfig 重新加载配置文件
func reloadConfig(configPath string, config *Config) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}

	var newConfig Config
	err = yaml.Unmarshal(data, &newConfig)
	if err != nil {
		return fmt.Errorf("error parsing YAML: %v", err)
	}

	// 重新初始化日志（如果配置变更）
	err = initLogger(&newConfig.Logging)
	if err != nil {
		return fmt.Errorf("error reinitializing logger: %v", err)
	}

	// 更新配置
	*config = newConfig

	// 重新设置白名单
	Whitelist = config.WhitelistIPs

	// 注意：风险IP下载goroutines可能需要重新启动，但这里简化处理
	// 在实际应用中，可能需要停止旧的goroutines并启动新的

	return nil
}
