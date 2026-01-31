package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	nethttp "net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
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

// ParseDuration 解析时间字符串，如 "30d" -> 30*24*time.Hour
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	var multiplier time.Duration = 1
	switch strings.ToLower(s[len(s)-1:]) {
	case "d":
		multiplier = 24 * time.Hour
		s = s[:len(s)-1]
	case "h":
		multiplier = time.Hour
		s = s[:len(s)-1]
	case "m":
		multiplier = time.Minute
		s = s[:len(s)-1]
	case "s":
		multiplier = time.Second
		s = s[:len(s)-1]
	default:
		return 0, fmt.Errorf("invalid duration unit")
	}
	d, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(d) * multiplier, nil
}

// IPv4ToUint32 将IPv4地址字符串转换为uint32
func IPv4ToUint32(ip string) (uint32, error) {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return 0, fmt.Errorf("invalid IPv4 address: %s", ip)
	}
	var result uint32
	for i, part := range parts {
		num, err := strconv.Atoi(part)
		if err != nil || num < 0 || num > 255 {
			return 0, fmt.Errorf("invalid IPv4 address: %s", ip)
		}
		result |= uint32(num) << ((3 - i) * 8)
	}
	return result, nil
}

// Uint32ToIPv4 将uint32转换为net.IP
func Uint32ToIPv4(ip uint32) net.IP {
	return net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}

// IsIPInSafeList 检查 IP 是否在安全列表中
func IsIPInSafeList(ip uint32) bool {
	if SafeListData == nil {
		return false
	}
	found, _ := SafeListData.Contains(ip)
	return found
}

// AddNotificationItem 添加通知项
func AddNotificationItem(ip uint32, source string) {
	NotificationMap[ip] = append(NotificationMap[ip], NotificationItem{
		IP:        ip,
		Count:     len(NotificationMap[ip]) + 1,
		Source:    source,
		Timestamp: time.Now().Unix(),
	})
}

// CheckAndNotify 检查是否达到阈值并通知
func CheckAndNotify(threshold int, source string, isOnce bool) {
	for ip, items := range NotificationMap {
		if len(items) >= threshold {
			// 获取最新项
			latest := items[len(items)-1]
			ipStr := Uint32ToIPv4(ip).String()
			timeStr := time.Unix(latest.Timestamp, 0).Format("2006-01-02 15:04:05")
			data := struct {
				IP        string
				Count     int
				Source    string
				Timestamp int64
				Time      string
			}{
				IP:        ipStr,
				Count:     latest.Count,
				Source:    latest.Source,
				Timestamp: latest.Timestamp,
				Time:      timeStr,
			}

			for _, notif := range config.Notifications.Services {
				if latest.Count >= notif.Threshold {
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

					// 发送通知
					sendNotification(notif, message)
				}
			}

			logrus.Infof("Notification triggered for IP %s from %s", ipStr, source)
			if !isOnce {
				// tail 模式下，通知后清理
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

// ExtractIPFromLine 从日志行中提取IP地址
func ExtractIPFromLine(line string) (uint32, error) {
	// 正则表达式匹配IPv4地址
	re := regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	matches := re.FindAllString(line, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("no IP found in line")
	}
	// 取第一个匹配的IP
	return IPv4ToUint32(matches[0])
}

// IsSensitiveIP 检测IP是否敏感
func IsSensitiveIP(ip uint32) (bool, string) {
	// 判断是否在安全列表中（白名单）
	if IsIPInSafeList(ip) {
		return false, ""
	}
	// 判断是否在风险IP列表中
	if RiskListData == nil {
		return false, ""
	}
	found, name := RiskListData.Contains(ip)
	if found {
		return true, name
	}
	return false, ""
}

// sendNotification 发送通知
func sendNotification(notif Notification, message string) {
	var service notify.Notifier

	retryCount := config.Notifications.RetryCount
	if retryCount == 0 {
		retryCount = 5 // 默认 5
	}

	var err error
	for i := 0; i <= retryCount; i++ {
		switch strings.ToLower(notif.Service) {
		case "slack":
			token, ok := notif.Config["token"].(string)
			if !ok {
				logrus.Errorf("Slack token not configured or not string")
				return
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
			service = slackSvc
		case "discord":
			token, ok := notif.Config["token"].(string)
			if !ok {
				logrus.Errorf("Discord token not configured or not string")
				return
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
			service = discordSvc
		case "webhook":
			url, ok := notif.Config["url"].(string)
			if !ok {
				logrus.Errorf("Webhook url not configured or not string")
				return
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
			logrus.Debugf("Setup Webhook with url=%s method=%s content_type=%s headers=%v", url, method, contentType, webhookHeader)

			webhook := &http.Webhook{
				URL:         url,
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
			service = httpSvc
		case "bark":
			key, ok := notif.Config["key"].(string)
			if !ok {
				logrus.Errorf("Bark key not configured or not string")
				return
			}
			server_url, ok := notif.Config["server_url"].(string)
			if ok {
				logrus.Debugf("Setup Bark with custom server_url: %s , key: %s", server_url, key)
				barkSvc := bark.NewWithServers(key, server_url)
				service = barkSvc
			} else {
				logrus.Debugf("Setup Bark with default server_url: %s, key: %s", bark.DefaultServerURL, key)
				barkSvc := bark.New(key)
				service = barkSvc
			}
		case "telegram":
			token, ok := notif.Config["token"].(string)
			if !ok {
				logrus.Errorf("Telegram token not configured or not string")
				return
			}
			masked := token
			chatIDStr := ""
			if c, ok := notif.Config["chat_id"].(string); ok {
				chatIDStr = c
			}
			logrus.Debugf("Setup Telegram with token=%s chat_id=%s", masked, chatIDStr)
			telegramSvc, err := telegram.New(token)
			if err != nil {
				logrus.Errorf("Failed to create Telegram service: %v", err)
				return
			}
			if chatIDStr != "" {
				if chatID, err := strconv.ParseInt(chatIDStr, 10, 64); err == nil {
					telegramSvc.AddReceivers(chatID)
				}
			}
			service = telegramSvc
		case "pushover":
			token, ok := notif.Config["token"].(string)
			if !ok {
				logrus.Errorf("Pushover token not configured or not string")
				return
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
			service = pushoverSvc
		case "pushbullet":
			token, ok := notif.Config["token"].(string)
			if !ok {
				logrus.Errorf("Pushbullet token not configured or not string")
				return
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
			service = pushbulletSvc
		case "rocketchat":
			url, ok := notif.Config["url"].(string)
			if !ok {
				logrus.Errorf("RocketChat url not configured or not string")
				return
			}
			scheme, ok := notif.Config["scheme"].(string)
			if !ok {
				scheme = "https"
			}
			userID, ok := notif.Config["user_id"].(string)
			if !ok {
				logrus.Errorf("RocketChat user_id not configured or not string")
				return
			}
			token, ok := notif.Config["token"].(string)
			if !ok {
				logrus.Errorf("RocketChat token not configured or not string")
				return
			}
			masked := token
			channel := ""
			if ch, ok := notif.Config["channel"].(string); ok {
				channel = ch
			}
			logrus.Debugf("Setup RocketChat with url=%s scheme=%s user_id=%s token=%s channel=%s", url, scheme, userID, masked, channel)
			rocketchatSvc, err := rocketchat.New(url, scheme, userID, token)
			if err != nil {
				logrus.Errorf("Failed to create RocketChat service: %v", err)
				return
			}
			if channel != "" {
				rocketchatSvc.AddReceivers(channel)
			}
			service = rocketchatSvc
		case "wechat":
			appID, ok := notif.Config["app_id"].(string)
			if !ok {
				logrus.Errorf("WeChat app_id not configured or not string")
				return
			}
			appSecret, ok := notif.Config["app_secret"].(string)
			if !ok {
				logrus.Errorf("WeChat app_secret not configured or not string")
				return
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
			service = wechatSvc
		case "webpush":
			vapidPublicKey, ok := notif.Config["vapid_public_key"].(string)
			if !ok {
				logrus.Errorf("WebPush vapid_public_key not configured or not string")
				return
			}
			vapidPrivateKey, ok := notif.Config["vapid_private_key"].(string)
			if !ok {
				logrus.Errorf("WebPush vapid_private_key not configured or not string")
				return
			}
			logrus.Debugf("Setup WebPush with public_key=%s private_key=%s", vapidPublicKey, vapidPrivateKey)
			webpushSvc := webpush.New(vapidPublicKey, vapidPrivateKey)
			// For webpush, subscription is complex, assume it's in config as string or something, but for simplicity, skip adding receivers here
			service = webpushSvc
		case "curl":
			urlStr, ok := notif.Config["url"].(string)
			if !ok {
				logrus.Errorf("Curl url not configured or not string")
				return
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
			r := c.R().SetHeaders(headers)
			var resp *req.Response
			var rerr error
			if method == "POST" {
				r = r.SetBodyString(message)
				// Debug: 输出 POST 请求的 URL、headers 与 body，并尝试输出原始请求信息
				if logrus.IsLevelEnabled(logrus.DebugLevel) {
					httpReq, herr := nethttp.NewRequest(method, urlStr, strings.NewReader(message))
					if herr == nil {
						for k, v := range headers {
							httpReq.Header.Set(k, v)
						}
						if dump, derr := httputil.DumpRequestOut(httpReq, true); derr == nil {
							logrus.Debugf("Curl Raw Request:\n%s", string(dump))
						} else {
							logrus.Debugf("Curl Request: method=%s url=%s headers=%v body=%s (dump err: %v)", method, urlStr, headers, message, derr)
						}
					} else {
						logrus.Debugf("Curl Request: method=%s url=%s headers=%v body=%s (new request err: %v)", method, urlStr, headers, message, herr)
					}
				}
				resp, rerr = r.Post(urlStr)
			} else {
				// 将 title 与 message URL 编码并拼接到 URL 上
				title := notif.PayloadTitle
				if title == "" {
					title = "Risk IP Alert"
				}
				u, perr := url.Parse(urlStr)
				if perr != nil {
					rerr = perr
				} else {
					q := u.Query()
					q.Set("title", title)
					q.Set("message", message)
					u.RawQuery = q.Encode()
					// Debug: 输出最终请求 URL 与 headers，并尝试输出原始请求信息（不包含 body）
					if logrus.IsLevelEnabled(logrus.DebugLevel) {
						logrus.Debugf("Curl Request: method=%s url=%s headers=%v", method, u.String(), headers)
						httpReq, herr := nethttp.NewRequest(method, u.String(), nil)
						if herr == nil {
							for k, v := range headers {
								httpReq.Header.Set(k, v)
							}
							if dump, derr := httputil.DumpRequestOut(httpReq, false); derr == nil {
								logrus.Debugf("Curl Raw Request:\n%s", string(dump))
							} else {
								logrus.Debugf("Curl Raw Request failed to dump: %v", derr)
							}
						} else {
							logrus.Debugf("Curl Request: new request err: %v", herr)
						}
					}
					switch method {
					case "GET":
						resp, rerr = r.Get(u.String())
					case "PUT":
						resp, rerr = r.Put(u.String())
					case "DELETE":
						resp, rerr = r.Delete(u.String())
					case "PATCH":
						resp, rerr = r.Patch(u.String())
					case "HEAD":
						resp, rerr = r.Head(u.String())
					case "OPTIONS":
						resp, rerr = r.Options(u.String())
					default:
						resp, rerr = r.Get(u.String())
					}
				}
			}

			if rerr == nil && resp != nil && resp.IsSuccessState() {
				return
			}
			if rerr != nil {
				err = rerr
			} else if resp != nil {
				err = fmt.Errorf("curl request failed, status: %s", resp.Status)
			} else {
				err = fmt.Errorf("curl request failed: unknown error")
			}

			// 由于 curl 已经直接发送请求，这里处理重试逻辑并跳过下面使用 notify 的步骤
			if strings.ToLower(notif.Service) == "curl" {
				if err == nil {
					return
				}
				if i < retryCount {
					logrus.Warnf("Curl notification failed, retrying (%d/%d): %v", i+1, retryCount, err)
					wait := time.Second * time.Duration(i+1)
					logrus.Debugf("Next curl notification retry after %s", wait.String())
					time.Sleep(wait)
					continue
				}
				logrus.Errorf("Failed to send curl notification after %d retries: %v", retryCount, err)
				return
			}

		default:
			logrus.Errorf("Unsupported notification service: %s", notif.Service)
			return
		}

		title := notif.PayloadTitle
		if title == "" {
			title = "Risk IP Alert"
		}

		ntf := notify.New()
		ntf.UseServices(service)
		err = ntf.Send(context.Background(), title, message)
		logrus.Debugf("Use service: %s, send Title: %s, message: %s", service, title, message)
		if err == nil {
			return
		}
		if i < retryCount {
			logrus.Warnf("Notification failed, retrying (%d/%d): %v", i+1, retryCount, err)
			wait := time.Second * time.Duration(i+1)
			logrus.Debugf("Next notification retry after %s", wait.String())
			time.Sleep(wait) // 简单退避
		}
	}
	logrus.Errorf("Failed to send notification after %d retries: %v", retryCount, err)
}
