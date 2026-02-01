package main

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// StartNotificationWorker 启动独立的通知发送工作器
// 每 500ms 检查一次是否有待发送的通知，并并行发送所有通知
func StartNotificationWorker() {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			processAndSendNotifications()
		}
	}()
	logrus.Info("Notification worker started (checking every 500ms)")
}

// processAndSendNotifications 处理并并行发送所有待发送的通知
func processAndSendNotifications() {
	// 获取并清空待发送队列
	PendingNotificationsMutex.Lock()
	if len(PendingNotifications) == 0 {
		PendingNotificationsMutex.Unlock()
		return
	}
	// 复制队列并清空
	toSend := make([]PendingNotification, len(PendingNotifications))
	copy(toSend, PendingNotifications)
	PendingNotifications = PendingNotifications[:0]
	PendingNotificationsMutex.Unlock()

	logrus.Debugf("Processing %d pending notifications", len(toSend))

	// 使用 WaitGroup 等待所有通知发送完成
	var wg sync.WaitGroup
	for _, pn := range toSend {
		wg.Add(1)
		go func(pn PendingNotification) {
			defer wg.Done()
			sendNotificationWithLogging(pn.Notif, pn.Message, pn.Title, pn.Data)
		}(pn)
	}
	wg.Wait()

	logrus.Debugf("Finished sending %d notifications", len(toSend))
}
