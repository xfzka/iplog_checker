package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/imroc/req/v3"
	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/bark"
	"github.com/nikoksr/notify/service/discord"
	"github.com/nikoksr/notify/service/http"
	"github.com/nikoksr/notify/service/pushbullet"
	"github.com/nikoksr/notify/service/pushover"
	"github.com/nikoksr/notify/service/rocketchat"
	"github.com/nikoksr/notify/service/slack"
	"github.com/nikoksr/notify/service/telegram"
	"github.com/nikoksr/notify/service/webpush"
	"github.com/nikoksr/notify/service/wechat"
	"github.com/sirupsen/logrus"
)

// AddNotificationItem 添加通知项 (线程安全)
func AddNotificationItem(ip uint32, finfo ListInfo, linfo ListInfo) {
	NotificationMapMutex.Lock()
	defer NotificationMapMutex.Unlock()
	NotificationMap[ip] = append(NotificationMap[ip], NewNotificationItem(ip, len(NotificationMap[ip])+1, finfo, linfo))
}

// AddPendingNotification 添加待发送通知到队列 (线程安全)
func AddPendingNotification(notif Notification, message, title string, data TemplateData) {
	PendingNotificationsMutex.Lock()
	defer PendingNotificationsMutex.Unlock()
	PendingNotifications = append(PendingNotifications, PendingNotification{
		Notif:   notif,
		Message: message,
		Title:   title,
		Data:    data,
	})
}

// CheckAndNotify 检查是否达到阈值并将通知加入队列 (异步发送)
// 该函数检查所有待处理的 IP，对于满足条件的通知加入 PendingNotifications 队列
// 通知由独立的 goroutine 定时检查并发送
func CheckAndNotify(info ListInfo, isOnce bool) {
	NotificationMapMutex.Lock()
	defer NotificationMapMutex.Unlock()

	for ip, items := range NotificationMap {
		if len(items) == 0 {
			continue
		}
		// 获取最新项
		latest := items[len(items)-1]
		ipStr := Uint32ToIPv4(ip).String()
		timeStr := time.Unix(latest.Timestamp, 0).Format("2006-01-02 15:04:05")
		data := NewTemplateData(ipStr, latest.Count, latest.SourceListInfo, latest.SourceLogInfo, latest.Timestamp, timeStr)

		// 对于每个通知配置，独立判断其触发条件：
		// - 命中次数 >= notif.Threshold
		// - 日志文件等级 (latest.SourceLogInfo.Level) >= notif.LogLevel
		// - IP 风险等级 (latest.SourceListInfo.Level) >= notif.RiskLevel
		sentAny := false
		for _, notif := range config.Notifications.Services {
			if latest.Count < notif.Threshold {
				continue
			}

			// 检查 LogLevel 与 RiskLevel
			if latest.SourceLogInfo.Level < notif.LogLevel {
				logrus.Debugf("Skip notification [%s] for IP %s: log_level %d < required %d",
					notif.Service, ipStr, latest.SourceLogInfo.Level, notif.LogLevel)
				continue
			}
			if latest.SourceListInfo.Level < notif.RiskLevel {
				logrus.Debugf("Skip notification [%s] for IP %s: risk_level %d < required %d",
					notif.Service, ipStr, latest.SourceListInfo.Level, notif.RiskLevel)
				continue
			}

			// 解析模板
			tmpl, err := template.New("payload").Parse(notif.PayloadTemplate)
			if err != nil {
				logrus.Errorf("Failed to parse template: %v", err)
				continue
			}
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, data); err != nil {
				logrus.Errorf("Failed to execute template: %v", err)
				continue
			}
			message := buf.String()

			// 获取标题
			title := notif.PayloadTitle
			if title == "" {
				title = "Risk IP Alert"
			}

			// 将通知加入待发送队列
			AddPendingNotification(notif, message, title, data)
			sentAny = true
			logrus.Debugf("Queued notification [%s] for IP %s, log_level: %d, risk_level: %d, count: %d",
				notif.Service, ipStr, latest.SourceLogInfo.Level, latest.SourceListInfo.Level, latest.Count)
		}

		if sentAny {
			logrus.Infof("Notification queued for IP %s from %s, list_level: %d, log_level: %d, count: %d",
				ipStr, info.Name, latest.SourceListInfo.Level, latest.SourceLogInfo.Level, latest.Count)
			if !isOnce {
				// tail 模式下，通知后清理该 IP
				delete(NotificationMap, ip)
			}
		}
	}
	// once 模式下，读取完后清理所有
	if isOnce {
		for ip := range NotificationMap {
			delete(NotificationMap, ip)
		}
	}
}

// setupNotificationService 根据通知类型设置对应的服务
func setupNotificationService(notif Notification) (notify.Notifier, error) {
	switch strings.ToLower(notif.Service) {
	case "slack":
		return setupSlackService(notif)
	case "discord":
		return setupDiscordService(notif)
	case "webhook":
		return setupWebhookService(notif)
	case "bark":
		return setupBarkService(notif)
	case "telegram":
		return setupTelegramService(notif)
	case "pushover":
		return setupPushoverService(notif)
	case "pushbullet":
		return setupPushbulletService(notif)
	case "rocketchat":
		return setupRocketchatService(notif)
	case "wechat":
		return setupWechatService(notif)
	case "webpush":
		return setupWebpushService(notif)
	default:
		return nil, fmt.Errorf("unsupported notification service: %s", notif.Service)
	}
}

// setupSlackService 设置 Slack 服务
func setupSlackService(notif Notification) (notify.Notifier, error) {
	token, ok := notif.Config["token"].(string)
	if !ok {
		return nil, fmt.Errorf("Slack token not configured or not string")
	}

	channel := ""
	if ch, ok := notif.Config["channel"].(string); ok {
		channel = ch
	}
	logrus.Debugf("Setup Slack with token=%s, channel=%s", token, channel)
	slackSvc := slack.New(token)
	if channel != "" {
		slackSvc.AddReceivers(channel)
	}
	return slackSvc, nil
}

// setupDiscordService 设置 Discord 服务
func setupDiscordService(notif Notification) (notify.Notifier, error) {
	token, ok := notif.Config["token"].(string)
	if !ok {
		return nil, fmt.Errorf("Discord token not configured or not string")
	}
	masked := token
	channel := ""
	if ch, ok := notif.Config["channel"].(string); ok {
		channel = ch
	}
	logrus.Debugf("Setup Discord with token=%s, channel=%s", masked, channel)
	discordSvc := discord.New()
	discordSvc.AuthenticateWithBotToken(token)
	if channel != "" {
		discordSvc.AddReceivers(channel)
	}
	return discordSvc, nil
}

// setupWebhookService 设置 Webhook 服务
func setupWebhookService(notif Notification) (notify.Notifier, error) {
	webhookURL, ok := notif.Config["url"].(string)
	if !ok {
		return nil, fmt.Errorf("Webhook url not configured or not string")
	}
	httpSvc := http.New()

	// 构建 webhook header - 必须初始化，否则会 panic
	webhookHeader := nethttp.Header{}
	if headers, ok := notif.Config["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			if strVal, ok := v.(string); ok {
				webhookHeader.Set(k, strVal)
			}
		}
	}

	// 获取 method，默认为 POST
	method := "POST"
	if m, ok := notif.Config["method"].(string); ok {
		method = m
	}

	// 获取 content_type，默认为 application/json
	contentType := "application/json"
	if ct, ok := notif.Config["content_type"].(string); ok {
		contentType = ct
	}

	// 打印 debug 信息（不展示完整 header 值以免泄露）
	logrus.Debugf("Setup Webhook with url=%s method=%s content_type=%s headers=%v", webhookURL, method, contentType, webhookHeader)

	webhook := &http.Webhook{
		URL:         webhookURL,
		Method:      method,
		Header:      webhookHeader,
		ContentType: contentType,
		BuildPayload: func(subject, message string) any {
			// 尝试解析 message 为 JSON，如果失败则返回原始字符串
			var jsonPayload map[string]interface{}
			if err := json.Unmarshal([]byte(message), &jsonPayload); err == nil {
				return jsonPayload
			}
			// 如果 message 不是 JSON，包装为标准格式
			return map[string]interface{}{
				"title":   subject,
				"message": message,
			}
		},
	}
	httpSvc.AddReceivers(webhook)
	return httpSvc, nil
}

// setupBarkService 设置 Bark 服务
func setupBarkService(notif Notification) (notify.Notifier, error) {
	key, ok := notif.Config["key"].(string)
	if !ok {
		return nil, fmt.Errorf("Bark key not configured or not string")
	}
	serverURL, ok := notif.Config["server_url"].(string)
	if ok {
		logrus.Debugf("Setup Bark with custom server_url: %s , key: %s", serverURL, key)
		barkSvc := bark.NewWithServers(key, serverURL)
		return barkSvc, nil
	} else {
		logrus.Debugf("Setup Bark with default server_url: %s, key: %s", bark.DefaultServerURL, key)
		barkSvc := bark.New(key)
		return barkSvc, nil
	}
}

// setupTelegramService 设置 Telegram 服务
func setupTelegramService(notif Notification) (notify.Notifier, error) {
	token, ok := notif.Config["token"].(string)
	if !ok {
		return nil, fmt.Errorf("Telegram token not configured or not string")
	}
	masked := token
	chatIDStr := ""
	if c, ok := notif.Config["chat_id"].(string); ok {
		chatIDStr = c
	}
	logrus.Debugf("Setup Telegram with token=%s chat_id=%s", masked, chatIDStr)
	telegramSvc, terr := telegram.New(token)
	if terr != nil {
		return nil, fmt.Errorf("failed to create Telegram service: %v", terr)
	}
	if chatIDStr != "" {
		if chatID, perr := strconv.ParseInt(chatIDStr, 10, 64); perr == nil {
			telegramSvc.AddReceivers(chatID)
		}
	}
	return telegramSvc, nil
}

// setupPushoverService 设置 Pushover 服务
func setupPushoverService(notif Notification) (notify.Notifier, error) {
	token, ok := notif.Config["token"].(string)
	if !ok {
		return nil, fmt.Errorf("Pushover token not configured or not string")
	}
	masked := token
	userKey := ""
	if uk, ok := notif.Config["user_key"].(string); ok {
		userKey = uk
	}
	logrus.Debugf("Setup Pushover with token=%s user_key=%s", masked, userKey)
	pushoverSvc := pushover.New(token)
	if userKey != "" {
		pushoverSvc.AddReceivers(userKey)
	}
	return pushoverSvc, nil
}

// setupPushbulletService 设置 Pushbullet 服务
func setupPushbulletService(notif Notification) (notify.Notifier, error) {
	token, ok := notif.Config["token"].(string)
	if !ok {
		return nil, fmt.Errorf("Pushbullet token not configured or not string")
	}
	masked := token
	deviceID := ""
	if did, ok := notif.Config["device_id"].(string); ok {
		deviceID = did
	}
	logrus.Debugf("Setup Pushbullet with token=%s device_id=%s", masked, deviceID)
	pushbulletSvc := pushbullet.New(token)
	if deviceID != "" {
		pushbulletSvc.AddReceivers(deviceID)
	}
	return pushbulletSvc, nil
}

// setupRocketchatService 设置 RocketChat 服务
func setupRocketchatService(notif Notification) (notify.Notifier, error) {
	rocketURL, ok := notif.Config["url"].(string)
	if !ok {
		return nil, fmt.Errorf("RocketChat url not configured or not string")
	}
	scheme, ok := notif.Config["scheme"].(string)
	if !ok {
		scheme = "https"
	}
	userID, ok := notif.Config["user_id"].(string)
	if !ok {
		return nil, fmt.Errorf("RocketChat user_id not configured or not string")
	}
	token, ok := notif.Config["token"].(string)
	if !ok {
		return nil, fmt.Errorf("RocketChat token not configured or not string")
	}
	masked := token
	channel := ""
	if ch, ok := notif.Config["channel"].(string); ok {
		channel = ch
	}
	logrus.Debugf("Setup RocketChat with url=%s scheme=%s user_id=%s token=%s channel=%s", rocketURL, scheme, userID, masked, channel)
	rocketchatSvc, rerr := rocketchat.New(rocketURL, scheme, userID, token)
	if rerr != nil {
		return nil, fmt.Errorf("failed to create RocketChat service: %v", rerr)
	}
	if channel != "" {
		rocketchatSvc.AddReceivers(channel)
	}
	return rocketchatSvc, nil
}

// setupWechatService 设置 WeChat 服务
func setupWechatService(notif Notification) (notify.Notifier, error) {
	appID, ok := notif.Config["app_id"].(string)
	if !ok {
		return nil, fmt.Errorf("WeChat app_id not configured or not string")
	}
	appSecret, ok := notif.Config["app_secret"].(string)
	if !ok {
		return nil, fmt.Errorf("WeChat app_secret not configured or not string")
	}
	masked := appSecret
	openID := ""
	if oid, ok := notif.Config["open_id"].(string); ok {
		openID = oid
	}
	logrus.Debugf("Setup WeChat with app_id=%s app_secret=%s open_id=%s", appID, masked, openID)
	wechatSvc := wechat.New(&wechat.Config{
		AppID:     appID,
		AppSecret: appSecret,
	})
	if openID != "" {
		wechatSvc.AddReceivers(openID)
	}
	return wechatSvc, nil
}

// setupWebpushService 设置 WebPush 服务
func setupWebpushService(notif Notification) (notify.Notifier, error) {
	vapidPublicKey, ok := notif.Config["vapid_public_key"].(string)
	if !ok {
		return nil, fmt.Errorf("WebPush vapid_public_key not configured or not string")
	}
	vapidPrivateKey, ok := notif.Config["vapid_private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("WebPush vapid_private_key not configured or not string")
	}
	logrus.Debugf("Setup WebPush with public_key=%s private_key=%s", vapidPublicKey, vapidPrivateKey)
	webpushSvc := webpush.New(vapidPublicKey, vapidPrivateKey)
	return webpushSvc, nil
}

// sendCurlNotification 发送 curl 通知 (特殊处理，不使用 notify 库)
func sendCurlNotification(notif Notification, message, title string, retryCount int) error {
	urlStr, ok := notif.Config["url"].(string)
	if !ok {
		return fmt.Errorf("Curl url not configured or not string")
	}
	method := "GET"
	if m, ok := notif.Config["method"].(string); ok {
		method = strings.ToUpper(m)
	}
	headers := map[string]string{}
	if h, ok := notif.Config["headers"].(map[string]interface{}); ok {
		for k, v := range h {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}

	c := req.C()
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		c.EnableDebugLog()
	}
	r := c.R().SetHeaders(headers)
	var resp *req.Response
	var err error

	if method == "POST" {
		r = r.SetBodyString(message)
		logrus.Debugf("Curl Request (summary): method=%s url=%s headers=%v body=%s", method, urlStr, headers, message)
		resp, err = r.Post(urlStr)
	} else {
		// 将 title 与 message URL 编码并拼接到 URL 上
		curlTitle := title
		if curlTitle == "" {
			curlTitle = "Risk IP Alert"
		}
		u, perr := url.Parse(urlStr)
		if perr != nil {
			err = perr
		} else {
			q := u.Query()
			q.Set("title", curlTitle)
			q.Set("message", message)
			u.RawQuery = q.Encode()
			logrus.Debugf("Curl Request (summary): method=%s url=%s headers=%v", method, u.String(), headers)
			switch method {
			case "GET":
				resp, err = r.Get(u.String())
			case "PUT":
				resp, err = r.Put(u.String())
			case "DELETE":
				resp, err = r.Delete(u.String())
			case "PATCH":
				resp, err = r.Patch(u.String())
			case "HEAD":
				resp, err = r.Head(u.String())
			case "OPTIONS":
				resp, err = r.Options(u.String())
			default:
				resp, err = r.Get(u.String())
			}
		}
	}

	if err == nil && resp != nil && resp.IsSuccessState() {
		return nil
	}

	if err == nil && resp != nil {
		err = fmt.Errorf("curl request failed, status: %s", resp.Status)
	} else if err == nil {
		err = fmt.Errorf("curl request failed: unknown error")
	}

	return err
}

// sendNotification 发送通知, 返回错误信息
func sendNotification(notif Notification, message, title string) error {
	retryCount := config.Notifications.RetryCount
	if retryCount == 0 {
		retryCount = 5 // 默认 5
	}

	// 特殊处理 curl 服务
	if strings.ToLower(notif.Service) == "curl" {
		var err error
		for i := 0; i <= retryCount; i++ {
			err = sendCurlNotification(notif, message, title, retryCount)
			if err == nil {
				return nil
			}
			if i < retryCount {
				logrus.Warnf("Curl notification failed, retrying (%d/%d): %v", i+1, retryCount, err)
				wait := time.Second * time.Duration(i+1)
				logrus.Debugf("Next curl notification retry after %s", wait.String())
				time.Sleep(wait)
			}
		}
		return fmt.Errorf("failed to send curl notification after %d retries: %v", retryCount, err)
	}

	// 其他服务使用 notify 库
	var err error
	for i := 0; i <= retryCount; i++ {
		service, err := setupNotificationService(notif)
		if err != nil {
			return err
		}

		ntf := notify.New()
		ntf.UseServices(service)
		err = ntf.Send(context.Background(), title, message)
		logrus.Debugf("Use service: %s, send Title: %s, message: %s", service, title, message)
		if err == nil {
			return nil
		}
		if i < retryCount {
			logrus.Warnf("Notification failed, retrying (%d/%d): %v", i+1, retryCount, err)
			wait := time.Second * time.Duration(i+1)
			logrus.Debugf("Next notification retry after %s", wait.String())
			time.Sleep(wait) // 简单退避
		}
	}
	return fmt.Errorf("failed to send notification after %d retries: %v", retryCount, err)
}
