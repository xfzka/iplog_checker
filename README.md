# IPLog Checker

一个用于监控日志文件中风险 IP 地址的工具。它可以从多种来源（URL、本地文件、手动配置）加载 IP 黑白名单，实时监控或定期扫描日志文件，并在检测到风险 IP 时发送通知。

## 功能特性

- 🔍 **多来源 IP 列表**：支持从 URL、本地文件或直接配置加载 IP 白名单和黑名单
- 📝 **灵活日志监控**：支持 `tail` 模式（实时监控）和 `once` 模式（定时扫描）
- 🔔 **多渠道通知**：支持 10+ 种通知方式，包括 Webhook、Slack、Discord、Telegram 等
- 🔄 **自动更新**：自动定期更新远程 IP 列表
- ⚙️ **热重载**：配置文件修改后自动重新加载

## 快速开始

### 1. 安装

```bash
# 克隆项目
git clone https://github.com/your-username/iplog_checker.git
cd iplog_checker

# 编译
go build -o iplog_checker .
```

### 2. 配置

```bash
# 复制示例配置文件
cp config-example.yaml config.yaml

# 编辑配置文件
vim config.yaml
```

### 3. 运行

```bash
# 使用默认配置文件 (config.yaml)
./iplog_checker

# 或指定配置文件路径
./iplog_checker -c /path/to/config.yaml
./iplog_checker --config /path/to/config.yaml
```

## 配置说明

完整的配置示例请参考 [config-example.yaml](config-example.yaml)。

### 日志配置 (logging)

| 配置项  | 说明                                       | 默认值 |
| ------- | ------------------------------------------ | ------ |
| `level` | 日志级别: `debug`, `info`, `warn`, `error` | `info` |
| `to`    | 日志文件路径，留空则只输出到控制台         | 空     |

### 安全 IP 列表 (safe_list)

白名单 IP，匹配这些 IP 的日志不会触发告警。

每个列表项需要指定以下来源之一（三选一）：

- `ips`: 直接在配置文件中指定 IP 或 CIDR
- `file`: 从本地文件加载
- `url`: 从远程 URL 下载

| 配置项            | 说明                             | 默认值 | 适用来源  |
| ----------------- | -------------------------------- | ------ | --------- |
| `name`            | 列表名称（必填）                 | -      | 全部      |
| `ips`             | IP 地址列表（支持单 IP 和 CIDR） | -      | ips       |
| `file`            | 本地文件路径                     | -      | file      |
| `url`             | 远程 URL                         | -      | url       |
| `format`          | 文件格式: `text`, `csv`, `json`  | `text` | file/url  |
| `update_interval` | 更新间隔（支持 d/h/m/s）         | `2h`   | file/url  |
| `timeout`         | 请求超时                         | `30s`  | url       |
| `retry_count`     | 重试次数                         | `3`    | url       |
| `csv_column`      | CSV 列名                         | -      | csv 格式  |
| `json_path`       | JSON 路径                        | -      | json 格式 |
| `custom_headers`  | 自定义 HTTP 请求头               | -      | url       |

### 风险 IP 列表 (risk_list)

黑名单 IP，匹配这些 IP 的日志将触发告警。配置项与 `safe_list` 相同。

推荐使用 [stamparm/ipsum](https://github.com/stamparm/ipsum) 项目的威胁情报数据：

- Level 8: 最高风险
- Level 7: 高风险
- Level 1-6: 按风险递减

### 目标日志文件 (target_logs)

| 配置项             | 说明                                     | 默认值  |
| ------------------ | ---------------------------------------- | ------- |
| `name`             | 文件名称（用于日志标识）                 | -       |
| `path`             | 日志文件路径（必填）                     | -       |
| `read_mode`        | 读取模式: `tail`（实时）, `once`（定时） | `once`  |
| `read_interval`    | 读取间隔（仅 once 模式）                 | `2h`    |
| `clean_after_read` | 读取后清空文件（仅 once 模式）           | `false` |

### 通知配置 (notifications)

| 配置项        | 说明         | 默认值 |
| ------------- | ------------ | ------ |
| `timeout`     | 请求超时     | `10s`  |
| `retry_count` | 重试次数     | `5`    |
| `services`    | 通知服务列表 | -      |

每个通知服务的配置：

| 配置项             | 说明                         | 默认值 |
| ------------------ | ---------------------------- | ------ |
| `service`          | 服务类型（见下方支持列表）   | -      |
| `threshold`        | 触发阈值（同一 IP 命中次数） | `5`    |
| `payload_template` | 消息模板（Go 模板语法）      | -      |
| `config`           | 服务特定配置                 | -      |

#### 消息模板变量

| 变量             | 说明                             |
| ---------------- | -------------------------------- |
| `{{.IP}}`        | 风险 IP 地址                     |
| `{{.Count}}`     | 命中次数                         |
| `{{.Source}}`    | 来源日志文件名                   |
| `{{.Timestamp}}` | Unix 时间戳                      |
| `{{.Time}}`      | 格式化时间 (2006-01-02 15:04:05) |

### 支持的通知服务

| 服务         | 说明                | 必需配置                                |
| ------------ | ------------------- | --------------------------------------- |
| `webhook`    | 自定义 HTTP Webhook | `url`                                   |
| `slack`      | Slack               | `token`, `channel`                      |
| `discord`    | Discord             | `token`, `channel`                      |
| `telegram`   | Telegram            | `token`, `chat_id`                      |
| `bark`       | Bark (iOS)          | `key`                                   |
| `pushover`   | Pushover            | `token`, `user_key`                     |
| `pushbullet` | Pushbullet          | `token`                                 |
| `rocketchat` | Rocket.Chat         | `url`, `user_id`, `token`, `channel`    |
| `wechat`     | 微信公众号/企业微信 | `app_id`, `app_secret`, `open_id`       |
| `dingding`   | DingTalk (钉钉)     | `token`, `secret`                       |
| `webpush`    | 浏览器推送          | `vapid_public_key`, `vapid_private_key` |

## 配置示例

### 基础配置示例

```yaml
# 日志配置
logging:
  level: "info"
  to: "iplog_checker.log"

# 白名单: 内网 IP
safe_list:
  - name: "LAN"
    ips:
      - "192.168.0.0/16"
      - "10.0.0.0/8"

# 黑名单: 使用 stamparm/ipsum
risk_list:
  - name: "ipsum_level8"
    url: "https://github.com/stamparm/ipsum/raw/refs/heads/master/levels/8.txt"
    update_interval: "6h"
    format: "text"

# 监控日志
target_logs:
  - name: "nginx"
    path: "/var/log/nginx/access.log"
    read_mode: "tail"

# 通知: Webhook
notifications:
  services:
    - service: "webhook"
      threshold: 5
      payload_template: '{"ip": "{{.IP}}", "count": {{.Count}}}'
      config:
        url: "https://your-webhook-url.com"
```

### Telegram 通知示例

```yaml
notifications:
  services:
    - service: "telegram"
      threshold: 3
      payload_template: "🚨 风险 IP 告警\nIP: {{.IP}}\n次数: {{.Count}}\n来源: {{.Source}}\n时间: {{.Time}}"
      config:
        token: "your-bot-token"
        chat_id: "your-chat-id"
```

### Slack 通知示例

```yaml
notifications:
  services:
    - service: "slack"
      threshold: 5
      payload_template: ":warning: Risk IP detected: {{.IP}} ({{.Count}} hits) from {{.Source}}"
      config:
        token: "xoxb-your-slack-token"
        channel: "#security-alerts"
```

### DingTalk 通知示例

```yaml
notifications:
  services:
    - service: "dingding"
      threshold: 5
      payload_template: "风险IP告警\nIP: {{.IP}}\n次数: {{.Count}}\n来源: {{.Source}}\n时间: {{.Time}}"
      config:
        token: "your-dingtalk-token"
        secret: "your-dingtalk-secret"
```

## 注意事项

1. **配置文件安全**：`config.yaml` 可能包含敏感信息（API Token 等），请勿提交到版本控制系统
2. **远程推送**：如果你的仓库曾经包含敏感配置，建议强制推送以清除历史记录：
   ```bash
   git push origin --force --all
   ```
3. **IP 列表格式**：
   - `text` 格式：每行一个 IP 或 CIDR
   - `csv` 格式：需指定 `csv_column`
   - `json` 格式：需指定 `json_path`

## 依赖项目

- [github.com/nikoksr/notify](https://github.com/nikoksr/notify) - 多渠道通知库
- [github.com/hpcloud/tail](https://github.com/hpcloud/tail) - 日志文件 tail 实现
- [github.com/sirupsen/logrus](https://github.com/sirupsen/logrus) - 日志库
- [github.com/fsnotify/fsnotify](https://github.com/fsnotify/fsnotify) - 文件系统监控

## License

MIT License
