package main

var config Config                                         // 全局配置实例
var ConfigFilePath string = "config.yaml"                 // 默认配置文件路径
var SafeListData *ListGroup                               // 全局安全 IP 数据实例 (白名单)
var RiskListData *ListGroup                               // 全局风险 IP 数据实例
var NotificationMap = make(map[uint32][]NotificationItem) // 全局通知映射

// Version will be set at build time via -ldflags "-X main.Version=..."
// If not set during build, it defaults to "dev".
var Version string = "dev"
