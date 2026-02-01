package main

import (
	"fmt"
	"time"

	"github.com/creasty/defaults"
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
	RetryCount int            `yaml:"retry_count,omitempty" default:"5"` // 共享重试次数 (默认 5)
	Services   []Notification `yaml:"services"`                          // 通知服务列表
}

// IPList IP 列表配置 (用于 safe_list 和 risk_list)
type IPList struct {
	Name                 string            `yaml:"name"`                                   // 列表名称 (用于日志输出和标记 IP 来源) - 必填
	URL                  string            `yaml:"url,omitempty"`                          // URL 来源 - file/url/ips 三选一
	File                 string            `yaml:"file,omitempty"`                         // 本地文件来源 - file/url/ips 三选一
	IPs                  []string          `yaml:"ips,omitempty"`                          // 手动 IP 列表 - file/url/ips 三选一
	UpdateInterval       string            `yaml:"update_interval,omitempty" default:"2h"` // 更新间隔 (仅 file/url, 支持 h/m/s/d, 默认 2h)
	Format               string            `yaml:"format,omitempty" default:"text"`        // 格式: text, csv, json (仅 file/url, 默认 text)
	Timeout              string            `yaml:"timeout,omitempty" default:"30s"`        // 请求超时 (仅 url, 支持 h/m/s, 默认 30s)
	RetryCount           int               `yaml:"retry_count,omitempty" default:"3"`      // 重试次数 (仅 url, 默认 3)
	CSVColumn            string            `yaml:"csv_column,omitempty"`                   // CSV 列名 (仅 csv 格式)
	JSONPath             string            `yaml:"json_path,omitempty"`                    // JSON 路径 (仅 json 格式)
	CustomHeaders        map[string]string `yaml:"custom_headers,omitempty"`               // 自定义请求头 (仅 url)
	Level                int               `yaml:"level,omitempty" default:"1"`            // 列表等级 (仅 risk_list, 默认 1)
	UpdateIntervalParsed time.Duration     // 解析后的更新间隔
	TimeoutParsed        time.Duration     // 解析后的超时
}

// TargetLog 目标日志文件配置
type TargetLog struct {
	Name               string        `yaml:"name"`                                       // 文件名称 (用于通知)
	Path               string        `yaml:"path"`                                       // 日志文件路径
	ReadMode           string        `yaml:"read_mode" default:"once"`                   // 读取模式: tail (持续监控), once (一次性) (默认 once)
	ReadInterval       string        `yaml:"read_interval,omitempty" default:"2h"`       // 一次性读取间隔 (仅 once 模式, 支持 h/m/s/d, 默认 2h)
	CleanAfterRead     bool          `yaml:"clean_after_read,omitempty" default:"false"` // 读取后清空 (仅 once 模式, 默认 false)
	Level              int           `yaml:"level,omitempty" default:"1"`                // 日志文件等级 (用于标记 IP 来源, 默认 1)
	ReadIntervalParsed time.Duration // 解析后的读取间隔
}

// Notification 通知配置
type Notification struct {
	Service         string         `yaml:"service"`                          // 通知服务: slack, discord, email, webhook
	Threshold       int            `yaml:"threshold" default:"5"`            // 预警阈值 (命中次数) (默认 5)
	PayloadTemplate string         `yaml:"payload_template"`                 // 消息模板 (使用 Go 模板语法)
	PayloadTitle    string         `yaml:"payload_title,omitempty"`          // 消息标题 (可选)
	Config          map[string]any `yaml:"config,omitempty"`                 // 服务配置 (如 webhook_url, token 等)
	LogLevel        int            `yaml:"log_level,omitempty" default:"1"`  // 通知等级 (仅通知高于等于该等级的日志文件, 默认 1)
	RiskLevel       int            `yaml:"risk_level,omitempty" default:"1"` // 风险等级 (仅通知高于等于该等级的风险 IP, 默认 1)
}

// initAppConfig 初始化应用配置（日志、IP列表等）
func initAppConfig(config *Config) error {
	// 应用默认值到所有配置
	if err := defaults.Set(config); err != nil {
		return fmt.Errorf("failed to set defaults: %v", err)
	}

	// 设置 safe_list 并解析时间字符串
	for i := range config.SafeList {
		// 白名单的 Level 始终为 0（风险等级为 0）
		config.SafeList[i].Level = 0
		if err := initIPListConfig(&config.SafeList[i]); err != nil {
			return fmt.Errorf("invalid safe_list config for %s: %v", config.SafeList[i].Name, err)
		}
	}

	// 设置 risk_list 并解析时间字符串
	for i := range config.RiskList {
		if err := initIPListConfig(&config.RiskList[i]); err != nil {
			return fmt.Errorf("invalid risk_list config for %s: %v", config.RiskList[i].Name, err)
		}
	}

	// 解析 TargetLog 的时间字符串
	for i := range config.TargetLogs {
		dur, err := ParseDuration(config.TargetLogs[i].ReadInterval)
		if err != nil {
			return fmt.Errorf("invalid read_interval for %s: %v", config.TargetLogs[i].Name, err)
		}
		config.TargetLogs[i].ReadIntervalParsed = dur
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
	sourceCount := BoolToInt(list.File != "") + BoolToInt(list.URL != "") + BoolToInt(len(list.IPs) > 0)
	switch sourceCount {
	case 0:
		return fmt.Errorf("must specify one of: file, url, or ips")
	case 1:
		// 正确：恰好指定了一个来源
	default:
		return fmt.Errorf("can only specify one of: file, url, or ips (not multiple)")
	}

	// 解析 file/url 来源的时间字符串
	if list.File != "" || list.URL != "" {
		dur, err := ParseDuration(list.UpdateInterval)
		if err != nil {
			return fmt.Errorf("invalid update_interval: %v", err)
		}
		list.UpdateIntervalParsed = dur
	}

	// 解析 url 来源的超时
	if list.URL != "" {
		dur, err := ParseDuration(list.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout: %v", err)
		}
		list.TimeoutParsed = dur
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
