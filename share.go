package main

import (
	"net"
)

var config Config                                         // 全局配置实例
var ConfigFilePath string = "config.yaml"                 // 默认配置文件路径
var WhitelistIPs []uint32                                 // 转换为 uint32 的白名单 IP 列表
var WhitelistCIDRs []*net.IPNet                           // 白名单 CIDR 列表
var RiskIPDataInstance *RiskIPData                        // 全局风险 IP 数据实例
var NotificationMap = make(map[uint32][]NotificationItem) // 全局通知映射
