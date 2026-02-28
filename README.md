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
git clone https://github.com/xfzka/iplog_checker.git
cd iplog_checker

# 构建
CI：GitHub Actions 会在推送到 `main` 分支或创建 `v*` tag 时自动构建并在 Release 中上传构建产物。

# 本地构建（如需）：
VERSION=$(git describe --tags --exact-match 2>/dev/null || echo "$(git describe --tags --abbrev=0)-$(git rev-parse --short=8 HEAD)") && \
  go build -ldflags "-X main.Version=$VERSION" -o iplog_checker .

# 查看版本
./iplog_checker -v
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
  **注意：** `IPList.Level` 默认值为 **1**，表示列表中 IP 的风险等级（数值越大风险越高）。对于白名单（`safe_list`）里的条目，其风险等级始终被视为 **0**，即白名单内 IP 不会触发告警。

### 目标日志文件 (target_logs)

| 配置项             | 说明                                             | 默认值  |
| ------------------ | ------------------------------------------------ | ------- |
| `name`             | 文件名称（用于日志标识）                         | -       |
| `path`             | 日志文件路径（必填）                             | -       |
| `read_mode`        | 读取模式: `tail`（实时）, `once`（定时）         | `once`  |
| `read_interval`    | 读取间隔（仅 once 模式）                         | `2h`    |
| `clean_after_read` | 读取后清空文件（仅 once 模式）                   | `false` |
| `ignore_keys`      | 忽略的关键字，当日志行包含这些关键字时跳过检测   | -       |
| `level`            | 日志文件等级，标记日志重要程度（数值越大越重要） | `1`     |

### 通知配置 (notifications)

| 配置项        | 说明         | 默认值 |
| ------------- | ------------ | ------ |
| `timeout`     | 请求超时     | `10s`  |
| `retry_count` | 重试次数     | `5`    |
| `services`    | 通知服务列表 | -      |

每个通知服务的配置：

| 配置项             | 说明                                                                 | 默认值 |
| ------------------ | -------------------------------------------------------------------- | ------ |
| `service`          | 服务类型（见下方支持列表）                                           | -      |
| `threshold`        | 触发阈值（同一 IP 命中次数）                                         | `5`    |
| `level`            | 通知关注的日志文件等级阈值，只有日志文件等级 >= 此值时才会触发该通知 | `1`    |
| `risk_level`       | 通知关注的 IP 风险等级阈值，只有 IP 风险等级 >= 此值时才会触发该通知 | `1`    |
| `payload_template` | 消息模板（Go 模板语法）                                              | -      |
| `config`           | 服务特定配置                                                         | -      |

#### 消息模板变量

| 变量                        | 说明                             |
| --------------------------- | -------------------------------- |
| `{{.IP}}`                   | 风险 IP 地址                     |
| `{{.Count}}`                | 命中次数                         |
| `{{.SourceListInfo.Name}}`  | 风险 IP 来源列表名称             |
| `{{.SourceListInfo.Level}}` | 风险 IP 来源列表等级             |
| `{{.SourceLogInfo.Name}}`   | 检测到该 IP 的日志文件名称       |
| `{{.SourceLogInfo.Level}}`  | 检测到该 IP 的日志文件等级       |
| `{{.Timestamp}}`            | Unix 时间戳                      |
| `{{.Time}}`                 | 格式化时间 (2006-01-02 15:04:05) |

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

````yaml
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
      level: 1
      risk_level: 1
      payload_template: '{"ip": "{{.IP}}", "count": {{.Count}}, "list_name": "{{.SourceListInfo.Name}}", "list_level": {{.SourceListInfo.Level}}, "log_name": "{{.SourceLogInfo.Name}}", "log_level": {{.SourceLogInfo.Level}}}'
      config:
        url: "https://your-webhook-url.com"


### 通知触发逻辑

- 可以配置多项 `notifications.services`，每项都是独立的通知规则。只要满足任意一项规则，就会向该项指定的服务发送通知。✅
- 触发条件（全部满足时才通知）：
  1. 同一 IP 的命中次数 >= `threshold`（每个通知项可独立设置）
  2. 日志文件等级 (`target_logs[].level`) >= 通知项的 `level`
  3. IP 风险等级 (`IPList.Level`) >= 通知项的 `risk_level`

示例：
- 配置了两个通知：
  - Bark: `level=1`, `risk_level=2`
  - DingTalk: `level=3`, `risk_level=4`
- 检测到若干次风险 IP，IP 风险等级为 3，来源日志文件的等级为 2：
  - 满足 Bark 条件（2 >= 1 且 3 >= 2），发送 Bark 通知
  - 不满足 DingTalk（2 < 3 或 3 < 4），不发送 DingTalk
- 检测到高风险 IP，风险等级为 5，来源日志文件等级为 4：
  - 满足 Bark，发送 Bark
  - 满足 DingTalk，发送 DingTalk

# 通知: Curl (内置 curl 功能，基于 req/v3) 🔧

> 行为说明：
> - 当 `method` 为 `POST` 时，直接将 `message` 作为请求 body 发送, 如果你的数据内容想以 json 形式发送, 记得加入 `Content-Type: application/json` header 。
> - 其它 method（如 `GET` / `PUT` / `DELETE`）会将 `title` 与 `message` URL 编码并追加到 URL 的查询参数中发送。

```yaml
notifications:
  services:
    - service: "curl"
      threshold: 5
      payload_title: "Risk IP Alert"
      payload_template: "{{.IP}} - {{.Count}} hits from {{.SourceLogInfo.Name}} (log_level: {{.SourceLogInfo.Level}}, list: {{.SourceListInfo.Name}}, risk_level: {{.SourceListInfo.Level}}) at {{.Time}}"
      config:
        url: "https://example.com/curl_endpoint"
        method: "GET"
        headers:
          Authorization: "Bearer your_token"
````

### Telegram 通知示例

```yaml
notifications:
  services:
    - service: "telegram"
      threshold: 3
      payload_template: "🚨 风险 IP 告警\nIP: {{.IP}}\n次数: {{.Count}}\n风险列表: {{.SourceListInfo.Name}} (Level {{.SourceListInfo.Level}})\n日志来源: {{.SourceLogInfo.Name}} (Level {{.SourceLogInfo.Level}})\n时间: {{.Time}}"
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
      payload_template: ":warning: Risk IP detected: {{.IP}} ({{.Count}} hits) from log {{.SourceLogInfo.Name}}, risk list: {{.SourceListInfo.Name}}"
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
      payload_template: "风险IP告警\nIP: {{.IP}}\n次数: {{.Count}}\n风险列表: {{.SourceListInfo.Name}} (Level {{.SourceListInfo.Level}})\n日志来源: {{.SourceLogInfo.Name}} (Level {{.SourceLogInfo.Level}})\n时间: {{.Time}}"
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
