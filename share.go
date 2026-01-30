package main

import (
	"net"
)

var config Config

var Whitelist []string
var WhitelistIPs []uint32
var WhitelistCIDRs []*net.IPNet
var RiskIPDataInstance *RiskIPData

// 全局通知映射
var NotificationMap = make(map[uint32][]NotificationItem)
