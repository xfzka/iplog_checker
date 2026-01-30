package main

var config Config                                         // 全局配置实例
var ConfigFilePath string = "config.yaml"                 // 默认配置文件路径
var SafeListData *IPData                                  // 全局安全 IP 数据实例 (白名单)
var RiskListData *IPData                                  // 全局风险 IP 数据实例
var NotificationMap = make(map[uint32][]NotificationItem) // 全局通知映射
