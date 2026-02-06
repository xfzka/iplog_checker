package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// NotificationsSentCount 全局通知发送成功计数器
var NotificationsSentCount uint64

// IncrementNotificationsSent 原子增加通知发送计数
func IncrementNotificationsSent() {
	atomic.AddUint64(&NotificationsSentCount, 1)
}

// GetNotificationsSent 原子获取通知发送计数
func GetNotificationsSent() uint64 {
	return atomic.LoadUint64(&NotificationsSentCount)
}

// NotifyResponse /notify 端点的响应结构
type NotifyResponse struct {
	Status  string `json:"status"`  // "success" 或 "failure"
	Message string `json:"message"` // 详细消息
}

// StatusResponse /status 端点的响应结构
type StatusResponse struct {
	SafeListCount     int            `json:"safe_list_count"`    // 安全列表条目总数
	RiskListCount     int            `json:"risk_list_count"`    // 风险列表条目总数
	RiskListStatus    map[string]int `json:"risk_list_status"`   // 每个风险列表的条目数
	NotificationsSent uint64         `json:"notifications_sent"` // 已发送通知总数
	ConfigInJSON      interface{}    `json:"config_in_json"`     // 当前配置的 JSON 对象
}

// StartAPIServer 启动 API 服务器
func StartAPIServer(addr string) error {
	mux := http.NewServeMux()

	// 注册路由
	mux.HandleFunc("/notify", handleNotify)
	mux.HandleFunc("/status", handleStatus)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	logrus.Infof("Starting API server on %s", addr)
	return server.ListenAndServe()
}

// handleNotify 处理 /notify 端点
// 查询参数：
//   - service: 指定要测试的通知服务名称（可选）
//
// 返回 JSON 数组，包含每个服务的测试结果
func handleNotify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 获取 service 参数
	serviceName := r.URL.Query().Get("service")

	// 获取配置的通知服务
	configMutex.RLock()
	allServices := make([]Notification, len(config.Notifications.Services))
	copy(allServices, config.Notifications.Services)
	configMutex.RUnlock()

	// 筛选要测试的服务
	var servicesToTest []Notification
	if serviceName != "" {
		// 测试指定服务
		found := false
		for _, svc := range allServices {
			if svc.Service == serviceName {
				servicesToTest = append(servicesToTest, svc)
				found = true
				break
			}
		}
		if !found {
			responses := []NotifyResponse{
				{
					Status:  "failure",
					Message: fmt.Sprintf("Service '%s' not found in configuration", serviceName),
				},
			}
			json.NewEncoder(w).Encode(responses)
			return
		}
	} else {
		// 测试所有服务
		servicesToTest = allServices
	}

	// 如果没有配置的服务
	if len(servicesToTest) == 0 {
		responses := []NotifyResponse{
			{
				Status:  "failure",
				Message: "No notification services configured",
			},
		}
		json.NewEncoder(w).Encode(responses)
		return
	}

	// 测试每个服务
	responses := make([]NotifyResponse, 0, len(servicesToTest))

	for _, svc := range servicesToTest {
		// 构造测试消息
		testMessage := fmt.Sprintf("Test notification from iplog_checker at %s", time.Now().Format("2006-01-02 15:04:05"))
		testTitle := "Test Notification"

		// 发送测试通知
		err := sendNotification(svc, testMessage, testTitle)

		if err != nil {
			responses = append(responses, NotifyResponse{
				Status:  "failure",
				Message: fmt.Sprintf("Failed to send notification to [%s]: %v", svc.Service, err),
			})
			logrus.Warnf("API test notification to %s failed: %v", svc.Service, err)
		} else {
			responses = append(responses, NotifyResponse{
				Status:  "success",
				Message: fmt.Sprintf("Notification sent successfully to [%s]", svc.Service),
			})
			logrus.Infof("API test notification to %s sent successfully", svc.Service)
		}
	}

	// 返回 JSON 响应
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		logrus.Errorf("Failed to encode notify response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleStatus 处理 /status 端点
// 返回系统当前状态的详细信息
func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 统计安全列表条目数
	safeListCount := 0
	if SafeListData != nil {
		for _, netList := range SafeListData.AllList {
			// 统计单个 IP 数量
			safeListCount += len(netList.ips)
			// 统计 CIDR 数量（遍历 trie 树）
			safeListCount += countCIDRNodes(netList.cidrRoot)
		}
	}

	// 统计风险列表条目数和详细状态
	riskListCount := 0
	riskListStatus := make(map[string]int)
	if RiskListData != nil {
		for info, netList := range RiskListData.AllList {
			// 统计单个 IP 数量
			ipCount := len(netList.ips)
			// 统计 CIDR 数量
			cidrCount := countCIDRNodes(netList.cidrRoot)
			totalCount := ipCount + cidrCount

			riskListCount += totalCount
			riskListStatus[info.Name] = totalCount
		}
	}

	// 获取当前配置
	configMutex.RLock()
	configCopy := config
	configMutex.RUnlock()

	// 构造响应
	response := StatusResponse{
		SafeListCount:     safeListCount,
		RiskListCount:     riskListCount,
		RiskListStatus:    riskListStatus,
		NotificationsSent: GetNotificationsSent(),
		ConfigInJSON:      configCopy,
	}

	// 返回 JSON 响应
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logrus.Errorf("Failed to encode status response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// countCIDRNodes 递归统计 CIDR trie 树中的节点数（即 CIDR 条目数）
func countCIDRNodes(node *CIDRNode) int {
	if node == nil {
		return 0
	}

	count := 0
	if node.end {
		count = 1
	}

	count += countCIDRNodes(node.children[0])
	count += countCIDRNodes(node.children[1])

	return count
}
