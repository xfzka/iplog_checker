package main

import (
	"sync"
	"time"
)

// NotificationItem 通知项结构体
type NotificationItem struct {
	IP             uint32
	Count          int
	SourceListInfo ListInfo // 来源列表信息
	SourceLogInfo  ListInfo // 来源日志信息
	Timestamp      int64    // 时间戳
}

// NewNotificationItem 创建新的通知项
func NewNotificationItem(ip uint32, count int, finfo ListInfo, linfo ListInfo) NotificationItem {
	return NotificationItem{
		IP:             ip,
		Count:          count,
		SourceListInfo: finfo,
		SourceLogInfo:  linfo,
		Timestamp:      time.Now().Unix(),
	}
}

// TemplateData 用于通知模板的数据结构
type TemplateData struct {
	IP             string
	Count          int
	SourceListInfo ListInfo
	SourceLogInfo  ListInfo
	Timestamp      int64
	Time           string
}

// NewTemplateData 创建新的模板数据
func NewTemplateData(ip string, count int, finfo ListInfo, linfo ListInfo, timestamp int64, timeStr string) TemplateData {
	return TemplateData{
		IP:             ip,
		Count:          count,
		SourceListInfo: finfo,
		SourceLogInfo:  linfo,
		Timestamp:      timestamp,
		Time:           timeStr,
	}
}

// PendingNotification 待发送的通知
type PendingNotification struct {
	Notif      Notification // 通知配置
	Message    string       // 通知消息
	Title      string       // 通知标题
	Data       TemplateData // 模板数据
	RetryCount int          // 已重试次数
}

// 通知映射：IP -> 通知项列表
var NotificationMap = make(map[uint32][]NotificationItem)
var NotificationMapMutex sync.Mutex

// 待发送通知队列
var PendingNotifications []PendingNotification
var PendingNotificationsMutex sync.Mutex

// TakeAllPendingNotifications 取出所有待发送通知 (线程安全)
// 取出后队列将被清空
func TakeAllPendingNotifications() []PendingNotification {
	PendingNotificationsMutex.Lock()
	defer PendingNotificationsMutex.Unlock()
	if len(PendingNotifications) == 0 {
		return nil
	}
	taken := make([]PendingNotification, len(PendingNotifications))
	copy(taken, PendingNotifications)
	PendingNotifications = PendingNotifications[:0]
	return taken
}

// AddPendingNotificationsToEnd 将通知项添加到队尾 (线程安全)
func AddPendingNotificationsToEnd(items []PendingNotification) {
	if len(items) == 0 {
		return
	}
	PendingNotificationsMutex.Lock()
	defer PendingNotificationsMutex.Unlock()
	PendingNotifications = append(PendingNotifications, items...)
}
