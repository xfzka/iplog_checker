package main

import (
	"fmt"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

var config Config                         // 全局配置实例
var ConfigFilePath string = "config.yaml" // 默认配置文件路径

// Config 表示根配置结构
type Config struct {
	Logging Logging `yaml:"logging"` // 日志配置

	SafeList      []IPList      `yaml:"safe_list"`     // 安全 IP 列表配置 (白名单)
	RiskList      []IPList      `yaml:"risk_list"`     // 风险 IP 列表配置
	TargetLogs    []TargetLog   `yaml:"target_logs"`   // 监控的目标日志文件
	Notifications Notifications `yaml:"notifications"` // 通知配置
}

// Notifications 通知配置包装
type Notifications struct {
	Timeout    time.Duration  `yaml:"timeout,omitempty"`     // 共享请求超时 (默认 10s)
	RetryCount int            `yaml:"retry_count,omitempty"` // 共享重试次数 (默认 5)
	Services   []Notification `yaml:"services"`              // 通知服务列表
}

// IPList IP 列表配置 (用于 safe_list 和 risk_list)
type IPList struct {
	Name                 string            `yaml:"name"`                      // 列表名称 (用于日志输出和标记 IP 来源) - 必填
	URL                  string            `yaml:"url,omitempty"`             // URL 来源 - file/url/ips 三选一
	File                 string            `yaml:"file,omitempty"`            // 本地文件来源 - file/url/ips 三选一
	IPs                  []string          `yaml:"ips,omitempty"`             // 手动 IP 列表 - file/url/ips 三选一
	UpdateInterval       string            `yaml:"update_interval,omitempty"` // 更新间隔 (仅 file/url, 支持 h/m/s/d, 默认 2h)
	UpdateIntervalParsed time.Duration     // 解析后的更新间隔
	Format               string            `yaml:"format,omitempty"`  // 格式: text, csv, json (仅 file/url, 默认 text)
	Timeout              string            `yaml:"timeout,omitempty"` // 请求超时 (仅 url, 支持 h/m/s, 默认 30s)
	TimeoutParsed        time.Duration     // 解析后的超时
	RetryCount           int               `yaml:"retry_count,omitempty"`    // 重试次数 (仅 url, 默认 3)
	CSVColumn            string            `yaml:"csv_column,omitempty"`     // CSV 列名 (仅 csv 格式)
	JSONPath             string            `yaml:"json_path,omitempty"`      // JSON 路径 (仅 json 格式)
	CustomHeaders        map[string]string `yaml:"custom_headers,omitempty"` // 自定义请求头 (仅 url)
	Level                int               `yaml:"level,omitempty"`          // 列表等级 (仅 risk_list, 默认 1)
}

// TargetLog 目标日志文件配置
type TargetLog struct {
	Name               string        `yaml:"name"`                    // 文件名称 (用于通知)
	Path               string        `yaml:"path"`                    // 日志文件路径
	ReadMode           string        `yaml:"read_mode"`               // 读取模式: tail (持续监控), once (一次性) (默认 once)
	ReadInterval       string        `yaml:"read_interval,omitempty"` // 一次性读取间隔 (仅 once 模式, 支持 h/m/s/d, 默认 2h)
	ReadIntervalParsed time.Duration // 解析后的读取间隔
	CleanAfterRead     bool          `yaml:"clean_after_read,omitempty"` // 读取后清空 (仅 once 模式, 默认 false)
	Level              int           `yaml:"level,omitempty"`            // 日志文件等级 (用于标记 IP 来源, 默认 1)
}

// Notification 通知配置
type Notification struct {
	Service         string                 `yaml:"service"`                 // 通知服务: slack, discord, email, webhook
	Threshold       int                    `yaml:"threshold"`               // 预警阈值 (命中次数) (默认 5)
	PayloadTemplate string                 `yaml:"payload_template"`        // 消息模板 (使用 Go 模板语法)
	PayloadTitle    string                 `yaml:"payload_title,omitempty"` // 消息标题 (可选)
	Config          map[string]interface{} `yaml:"config,omitempty"`        // 服务配置 (如 webhook_url, token 等)
	LogLevel        int                    `yaml:"log_level,omitempty"`     // 通知等级 (仅通知高于等于该等级的日志文件, 默认 1)
	RiskLevel       int                    `yaml:"risk_level,omitempty"`    // 风险等级 (仅通知高于等于该等级的风险 IP, 默认 1)
}

// initAppConfig 初始化应用配置（日志、IP列表等）
func initAppConfig(config *Config) error {
	// 设置 safe_list 默认值并解析时间字符串
	for i := range config.SafeList {
		// 白名单的 Level 始终为 0（风险等级为 0）
		if config.SafeList[i].Level != 0 {
			config.SafeList[i].Level = 0
		}
		if err := initIPListConfig(&config.SafeList[i]); err != nil {
			return fmt.Errorf("invalid safe_list config for %s: %v", config.SafeList[i].Name, err)
		}
	}

	// 设置 risk_list 默认值并解析时间字符串
	for i := range config.RiskList {
		if err := initIPListConfig(&config.RiskList[i]); err != nil {
			return fmt.Errorf("invalid risk_list config for %s: %v", config.RiskList[i].Name, err)
		}
		// 风险列表的 Level 默认值为 1
		if config.RiskList[i].Level == 0 {
			config.RiskList[i].Level = 1
		}
	}

	for i := range config.TargetLogs {
		if config.TargetLogs[i].ReadMode == "" {
			config.TargetLogs[i].ReadMode = "once"
		}
		if config.TargetLogs[i].ReadInterval == "" {
			config.TargetLogs[i].ReadInterval = "2h"
		}
		dur, err := ParseDuration(config.TargetLogs[i].ReadInterval)
		if err != nil {
			return fmt.Errorf("invalid read_interval for %s: %v", config.TargetLogs[i].Name, err)
		}
		config.TargetLogs[i].ReadIntervalParsed = dur
		// TargetLog.Level 默认值为 1
		if config.TargetLogs[i].Level == 0 {
			config.TargetLogs[i].Level = 1
		}
	}
	for i := range config.Notifications.Services {
		if config.Notifications.Services[i].Threshold == 0 {
			config.Notifications.Services[i].Threshold = 5
		}
		// Notification 的 LogLevel 与 RiskLevel 默认值均为 1
		if config.Notifications.Services[i].LogLevel == 0 {
			config.Notifications.Services[i].LogLevel = 1
		}
		if config.Notifications.Services[i].RiskLevel == 0 {
			config.Notifications.Services[i].RiskLevel = 1
		}
	}

	// 初始化日志
	err := initLogger(&config.Logging)
	if err != nil {
		return fmt.Errorf("error initializing logger: %v", err)
	}
	return nil
}

// initIPListConfig 初始化 IPList 配置项
func initIPListConfig(list *IPList) error {
	// 验证来源：file, url, ips 三选一且必选一
	sourceCount := 0
	if list.File != "" {
		sourceCount++
	}
	if list.URL != "" {
		sourceCount++
	}
	if len(list.IPs) > 0 {
		sourceCount++
	}

	if sourceCount == 0 {
		return fmt.Errorf("must specify one of: file, url, or ips")
	}
	if sourceCount > 1 {
		return fmt.Errorf("can only specify one of: file, url, or ips (not multiple)")
	}

	// 仅对 file/url 来源设置默认值
	if list.File != "" || list.URL != "" {
		if list.Format == "" {
			list.Format = "text"
		}
		if list.UpdateInterval == "" {
			list.UpdateInterval = "2h"
		}
		dur, err := ParseDuration(list.UpdateInterval)
		if err != nil {
			return fmt.Errorf("invalid update_interval: %v", err)
		}
		list.UpdateIntervalParsed = dur
	}

	// 仅对 url 来源设置超时和重试
	if list.URL != "" {
		if list.Timeout == "" {
			list.Timeout = "30s"
		}
		dur, err := ParseDuration(list.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout: %v", err)
		}
		list.TimeoutParsed = dur

		if list.RetryCount == 0 {
			list.RetryCount = 3
		}
	}

	return nil
}

// watchConfigFile 监控配置文件变更并自动重载
func watchConfigFile() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("Failed to create file watcher: %v", err)
		return
	}
	defer watcher.Close()

	err = watcher.Add(ConfigFilePath)
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
				err := initAPP()
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
