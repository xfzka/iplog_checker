package main

import (
	"sync"
	"time"
)

// Config 表示根配置结构
type Config struct {
	Logging Logging `yaml:"logging"` // 日志配置

	RiskIPLists   []RiskIPList  `yaml:"risk_ip_lists"` // 风险 IP 列表配置
	WhitelistIPs  []string      `yaml:"whitelist_ips"` // 白名单 IP (支持单个 IP 或 CIDR 范围)
	IPLogFiles    []IPLogFile   `yaml:"ip_log_files"`  // 监控的 IP 日志文件
	Notifications Notifications `yaml:"notifications"` // 通知配置
}

// Notifications 通知配置包装
type Notifications struct {
	Timeout    time.Duration  `yaml:"timeout,omitempty"`     // 共享请求超时 (默认 10s)
	RetryCount int            `yaml:"retry_count,omitempty"` // 共享重试次数 (默认 5)
	Services   []Notification `yaml:"services"`              // 通知服务列表
}

// Logging 日志配置
type Logging struct {
	Level string `yaml:"level"` // 日志级别: debug, info, warn, error
	To    string `yaml:"to"`    // 日志文件路径
}

// RiskIPList 风险 IP 列表配置
type RiskIPList struct {
	Name                 string            `yaml:"name"`            // 风险 IP 列表名称 (用于日志输出和标记 IP 来源) - 必填
	URL                  string            `yaml:"url"`             // 风险 IP 列表的 URL - 与 file 任选其一必填
	File                 string            `yaml:"file"`            // 本地文件路径 - 与 url 任选其一必填
	UpdateInterval       string            `yaml:"update_interval"` // 更新间隔 (支持 h/m/s/d, 默认 2h)
	UpdateIntervalParsed time.Duration     // 解析后的更新间隔
	Format               string            `yaml:"format"`            // 格式: text, csv, json (默认 text)
	Timeout              string            `yaml:"timeout,omitempty"` // 请求超时 (支持 h/m/s, 默认 30s)
	TimeoutParsed        time.Duration     // 解析后的超时
	RetryCount           int               `yaml:"retry_count,omitempty"`    // 重试次数 (默认 3)
	CSVColumn            string            `yaml:"csv_column,omitempty"`     // CSV 列名 (仅 csv 格式)
	JSONPath             string            `yaml:"json_path,omitempty"`      // JSON 路径 (仅 json 格式)
	CustomHeaders        map[string]string `yaml:"custom_headers,omitempty"` // 自定义请求头
}

// IPLogFile IP 日志文件配置
type IPLogFile struct {
	Name               string        `yaml:"name"`                    // 文件名称 (用于通知)
	Path               string        `yaml:"path"`                    // 日志文件路径
	ReadMode           string        `yaml:"read_mode"`               // 读取模式: tail (持续监控), once (一次性) (默认 once)
	ReadInterval       string        `yaml:"read_interval,omitempty"` // 一次性读取间隔 (仅 once 模式, 支持 h/m/s/d, 默认 2h)
	ReadIntervalParsed time.Duration // 解析后的读取间隔
}

// Notification 通知配置
type Notification struct {
	Service         string                 `yaml:"service"`          // 通知服务: slack, discord, email, webhook
	Threshold       int                    `yaml:"threshold"`        // 预警阈值 (命中次数) (默认 5)
	PayloadTemplate string                 `yaml:"payload_template"` // 消息模板 (使用 Go 模板语法)
	Config          map[string]interface{} `yaml:"config,omitempty"` // 服务配置 (如 webhook_url, token 等)
}

// RiskIPData 存储下载的风险IP数据
type RiskIPData struct {
	mu   sync.RWMutex
	data map[string][]uint32 // key: Name, value: IP list as uint32
}

// NotificationItem 通知项结构体
type NotificationItem struct {
	IP        uint32
	Count     int
	Source    string // 来源文件
	Timestamp int64  // 时间戳
}
