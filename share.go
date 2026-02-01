package main

import "sync"

var config Config                                         // 全局配置实例
var ConfigFilePath string = "config.yaml"                 // 默认配置文件路径
var SafeListData *ListGroup                               // 全局安全 IP 数据实例 (白名单)
var RiskListData *ListGroup                               // 全局风险 IP 数据实例
var NotificationMap = make(map[uint32][]NotificationItem) // 全局通知映射
var NotificationMapMutex sync.Mutex                       // 通知映射锁

// PendingNotification 待发送的通知
type PendingNotification struct {
	Notif   Notification // 通知配置
	Message string       // 通知消息
	Title   string       // 通知标题
	Data    TemplateData // 模板数据
}

var PendingNotifications []PendingNotification // 待发送通知队列
var PendingNotificationsMutex sync.Mutex       // 待发送通知队列锁

// Version will be set at build time via -ldflags "-X main.Version=..."
// If not set during build, it defaults to "dev".
var Version string = "dev"
