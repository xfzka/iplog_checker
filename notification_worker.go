package main

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// NotificationResult 通知发送结果
type NotificationResult struct {
	Notification PendingNotification
	Success      bool
	Error        error
}

// StartNotificationWorker 启动独立的通知发送工作器
// 每 1 秒检查一次是否有待发送的通知，不等待上一次发送完毕
func StartNotificationWorker() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// 不阻塞，每次检测都在新的 goroutine 中处理
			go processAndSendNotifications()
		}
	}()
	logrus.Info("Notification worker started (checking every 1s)")
}

// processAndSendNotifications 处理并发送所有待发送的通知
// 按 IP 分组处理，只要有一个通知发送成功，就认为该 IP 的本次通知成功
func processAndSendNotifications() {
	// 从队列中取出所有待发送通知
	toSend := TakeAllPendingNotifications()
	if len(toSend) == 0 {
		return
	}

	logrus.Debugf("Processing %d pending notifications", len(toSend))

	// 按 IP 分组
	ipGroups := make(map[string][]PendingNotification)
	for _, pn := range toSend {
		ip := pn.Data.IP
		ipGroups[ip] = append(ipGroups[ip], pn)
	}

	// 获取最大重试次数
	configMutex.RLock()
	maxRetry := config.Notifications.RetryCount
	configMutex.RUnlock()
	if maxRetry <= 0 {
		maxRetry = 5 // 默认 5 次
	}

	// 并行处理每个 IP 分组
	var wg sync.WaitGroup
	for ip, group := range ipGroups {
		wg.Add(1)
		go func(ip string, notifications []PendingNotification) {
			defer wg.Done()
			processIPNotificationGroup(ip, notifications, maxRetry)
		}(ip, group)
	}
	wg.Wait()

	logrus.Debugf("Finished processing notifications for %d IPs", len(ipGroups))
}

// processIPNotificationGroup 处理单个 IP 的通知组
// 只要有一个成功就认为成功，失败的不再重试
// 如果全部失败，失败的放回队尾
func processIPNotificationGroup(ip string, notifications []PendingNotification, maxRetry int) {
	if len(notifications) == 0 {
		return
	}

	// 并行发送所有通知
	results := make(chan NotificationResult, len(notifications))
	var wg sync.WaitGroup

	for _, pn := range notifications {
		wg.Add(1)
		go func(pn PendingNotification) {
			defer wg.Done()
			err := sendNotification(pn.Notif, pn.Message, pn.Title)
			results <- NotificationResult{
				Notification: pn,
				Success:      err == nil,
				Error:        err,
			}
		}(pn)
	}

	// 等待所有发送完成并关闭 channel
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集结果
	var successCount int
	var failedNotifications []PendingNotification

	for result := range results {
		if result.Success {
			successCount++
			logrus.Infof("Successfully sent notification [%s] for IP %s (count: %d, list_level: %d, log_level: %d)",
				result.Notification.Notif.Service, ip,
				result.Notification.Data.Count,
				result.Notification.Data.SourceListInfo.Level,
				result.Notification.Data.SourceLogInfo.Level)
		} else {
			// 记录失败
			pn := result.Notification
			pn.RetryCount++

			if successCount > 0 {
				// 已有成功的，失败的以 warn 级别记录但不重试
				logrus.Warnf("Failed to send notification [%s] for IP %s (retry %d/%d): %v (skipped due to other success)",
					pn.Notif.Service, ip, pn.RetryCount, maxRetry, result.Error)
			} else if pn.RetryCount >= maxRetry {
				// 重试耗尽，以 error 级别记录
				logrus.Errorf("Failed to send notification [%s] for IP %s after %d retries: %v",
					pn.Notif.Service, ip, pn.RetryCount, result.Error)
			} else {
				// 记录失败，稍后放回队列
				logrus.Warnf("Failed to send notification [%s] for IP %s (retry %d/%d): %v",
					pn.Notif.Service, ip, pn.RetryCount, maxRetry, result.Error)
				failedNotifications = append(failedNotifications, pn)
			}
		}
	}

	// 如果有成功的，不再重试失败的
	if successCount > 0 {
		logrus.Infof("Notification for IP %s completed: %d success, %d failed (not retrying due to success)",
			ip, successCount, len(notifications)-successCount)
		return
	}

	// 全部失败，将未超过重试次数的放回队尾
	if len(failedNotifications) > 0 {
		logrus.Warnf("All notifications failed for IP %s, re-queuing %d notifications for retry",
			ip, len(failedNotifications))
		AddPendingNotificationsToEnd(failedNotifications)
	}
}
