package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

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

// InitWhitelist 初始化白名单，将字符串列表转换为预处理的uint32和CIDR
func InitWhitelist(whitelist []string) {
	WhitelistIPs = nil
	WhitelistCIDRs = nil
	for _, entry := range whitelist {
		if strings.Contains(entry, "/") {
			_, network, err := net.ParseCIDR(entry)
			if err != nil {
				logrus.Warnf("Invalid CIDR in whitelist: %s", entry)
				continue // invalid, skip
			}
			WhitelistCIDRs = append(WhitelistCIDRs, network)
		} else {
			ipUint, err := IPv4ToUint32(entry)
			if err != nil {
				logrus.Warnf("Invalid IP in whitelist: %s", entry)
				continue
			}
			WhitelistIPs = append(WhitelistIPs, ipUint)
		}
	}
	logrus.Infof("Initialized whitelist with %d IP/CIDR", len(WhitelistIPs)+len(WhitelistCIDRs))
}

// IsIPInWhitelist 检查 IP 是否在白名单内
func IsIPInWhitelist(ip uint32) bool {
	// 检查单个 IP
	for _, wip := range WhitelistIPs {
		if ip == wip {
			return true
		}
	}
	// 检查 CIDR
	ipNet := Uint32ToIPv4(ip)
	for _, network := range WhitelistCIDRs {
		if network.Contains(ipNet) {
			return true
		}
	}
	return false
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

			logrus.Infof("Notification triggered for IP %s from %s: %v", ipStr, source, items)
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
	// 2. 判断是否在白名单中
	if IsIPInWhitelist(ip) {
		return false, ""
	}
	// 3. 判断是否在风险IP列表中
	if RiskIPDataInstance == nil {
		return false, ""
	}
	RiskIPDataInstance.mu.RLock()
	defer RiskIPDataInstance.mu.RUnlock()
	for name, ips := range RiskIPDataInstance.data {
		for _, riskIP := range ips {
			if ip == riskIP {
				return true, name
			}
		}
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
			slackSvc := slack.New(token)
			if channel, ok := notif.Config["channel"].(string); ok {
				slackSvc.AddReceivers(channel)
			}
			service = slackSvc
		case "discord":
			token, ok := notif.Config["token"].(string)
			if !ok {
				logrus.Errorf("Discord token not configured or not string")
				return
			}
			discordSvc := discord.New()
			discordSvc.AuthenticateWithBotToken(token)
			if channel, ok := notif.Config["channel"].(string); ok {
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
			webhook := &http.Webhook{
				URL:         url,
				Method:      "POST",
				ContentType: "application/json",
				BuildPayload: func(subject, message string) any {
					return message // 直接使用 message 作为 payload
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
			barkSvc := bark.New(key)
			service = barkSvc
		case "telegram":
			token, ok := notif.Config["token"].(string)
			if !ok {
				logrus.Errorf("Telegram token not configured or not string")
				return
			}
			telegramSvc, err := telegram.New(token)
			if err != nil {
				logrus.Errorf("Failed to create Telegram service: %v", err)
				return
			}
			if chatIDStr, ok := notif.Config["chat_id"].(string); ok {
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
			pushoverSvc := pushover.New(token)
			if userKey, ok := notif.Config["user_key"].(string); ok {
				pushoverSvc.AddReceivers(userKey)
			}
			service = pushoverSvc
		case "pushbullet":
			token, ok := notif.Config["token"].(string)
			if !ok {
				logrus.Errorf("Pushbullet token not configured or not string")
				return
			}
			pushbulletSvc := pushbullet.New(token)
			if deviceID, ok := notif.Config["device_id"].(string); ok {
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
			rocketchatSvc, err := rocketchat.New(url, scheme, userID, token)
			if err != nil {
				logrus.Errorf("Failed to create RocketChat service: %v", err)
				return
			}
			if channel, ok := notif.Config["channel"].(string); ok {
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
			wechatSvc := wechat.New(&wechat.Config{
				AppID:     appID,
				AppSecret: appSecret,
			})
			if openID, ok := notif.Config["open_id"].(string); ok {
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
			webpushSvc := webpush.New(vapidPublicKey, vapidPrivateKey)
			// For webpush, subscription is complex, assume it's in config as string or something, but for simplicity, skip adding receivers here
			service = webpushSvc
		default:
			logrus.Errorf("Unsupported notification service: %s", notif.Service)
			return
		}

		ntf := notify.New()
		ntf.UseServices(service)
		err = ntf.Send(context.Background(), "Risk IP Alert", message)
		if err == nil {
			return
		}
		if i < retryCount {
			logrus.Warnf("Notification failed, retrying (%d/%d): %v", i+1, retryCount, err)
			time.Sleep(time.Second * time.Duration(i+1)) // 简单退避
		}
	}
	logrus.Errorf("Failed to send notification after %d retries: %v", retryCount, err)
}
