# WhatsApp AI Agent - Product Specification

> Version: 1.0 | Date: 2026-05-07 | Status: Approved

## 1. Overview

### 1.1 Product Description

基于现有 google-maps-scraper 项目（Go + Playwright + Wails Desktop），构建智能 WhatsApp Agent。Agent 自动接收客户消息，基于 RAG 知识库回答问题，调用 Google Maps 爬虫工具获取信息，并通过 Wails Desktop 聊天界面与用户交互。

### 1.2 Target Users

个人/小团队，用于管理客户 WhatsApp 沟通。

### 1.3 Tech Stack

| 层                  | 技术            | 说明                              |
| ------------------ | ------------- | ------------------------------- |
| Backend            | Go 1.26+      | 主开发语言                           |
| LLM                | DeepSeek V4   | OpenAI 兼容 API，通过 `go-openai` 调用 |
| Browser Automation | Playwright Go | WhatsApp Web 消息收发               |
| Desktop            | Wails v2      | 聊天界面集成                          |
| Database           | SQLite        | 对话历史、向量存储、统计                    |
| RAG                | 自建            | SQLite 向量 + DeepSeek Embedding  |
| Frontend           | HTML/CSS/JS   | 无框架，延续现有模式                      |

***

## 2. Core Features

### 2.1 Auto-Reply (WhatsApp Agent)

**功能**: 自动接收客户 WhatsApp 消息并回复。

**流程**:

1. WhatsApp Web Listener 每 3-5 秒轮询聊天列表
2. 检测未读消息，解析发送者手机号、姓名、消息文本
3. 消息入队 buffered channel (capacity 100)
4. Agent 顺序处理每条消息：
   - 加载该联系人对话历史 (最近 20 条)
   - 搜索 RAG 知识库获取相关文档片段
   - 构建系统提示词 (公司信息 + RAG 上下文 + 对话历史)
   - 调用 DeepSeek V4 API 生成回复
   - 如有 tool\_call，执行工具后重新调用 LLM
   - 保存消息到 SQLite
   - 通过 Playwright 发送回复到 WhatsApp Web
5. 实时通知 Wails 前端更新

**安全措施**:

- 每小时每联系人最大回复数: 10
- 回复前随机延迟 3-10 秒
- 打字模拟 (30ms/字符)
- 关键词守卫: 投诉/法律相关词汇触发安全回复
- DeepSeek API 限流: 2 req/s，429 响应指数退避

### 2.2 RAG Knowledge Base

**功能**: 上传公司文档 (PDF/TXT/Word)，自动向量化，Agent 基于文档内容回答客户问题。

**文档处理流程**:

1. 用户上传文档 (PDF/DOCX/TXT)
2. 提取纯文本
3. 递归字符分块 (chunk\_size=500, overlap=50)
4. 调用 DeepSeek Embedding API 生成向量
5. 存储到 SQLite (BLOB 格式)

**检索流程**:

1. 客户消息作为 query
2. 生成 query embedding
3. 余弦相似度搜索 top-5 文档片段
4. 注入到 LLM 系统提示词

**文件格式支持**:

- PDF: `github.com/ledongthuc/pdf` (纯 Go)
- DOCX: `github.com/nguyenthenguyen/docx` (纯 Go)
- TXT: Go 标准库

**管理功能**:

- 上传文档
- 列出已索引文档 (文件名、类型、分块数、更新时间)
- 删除文档 (级联删除分块)
- 重新索引 (基于 SHA-256 变更检测)

### 2.3 Tool Calling

Agent 可调用以下工具:

| 工具                      | 描述                  | 参数                                    |
| ----------------------- | ------------------- | ------------------------------------- |
| `search_knowledge`      | 搜索 RAG 知识库          | `query: string`, `top_k: int`         |
| `scrape_google_maps`    | 爬取 Google Maps 商家信息 | `keyword: string`, `location: string` |
| `send_whatsapp_message` | 向指定联系人发送消息          | `phone: string`, `text: string`       |
| `get_progress_stats`    | 获取今日进度统计            | (无参数)                                 |

### 2.4 Chat Interface (Wails Desktop)

**功能**: 在 Wails Desktop 应用中添加 "AI Agent" Tab，提供 GPT 风格的对话界面。

**布局**:

```
┌──────────────────────────────────────────┐
│  AI Agent Tab                            │
├──────────┬───────────────────────────────┤
│ 对话列表  │  聊天视图                     │
│          │                               │
│ [张三] ●  │  客户: 你们公司做什么的？     │
│ [李四]    │  Agent: 我们是...             │
│ [王五] ●  │                               │
│          │  ┌─────────────────────────┐  │
│          │  │ 输入消息...              │  │
│          │  └─────────────────────────┘  │
├──────────┴───────────────────────────────┤
│ 今日: 23 条消息 | 平均响应 2.3s | Token: 5.2k │
└──────────────────────────────────────────┘
```

**交互功能**:

- 查看所有活跃对话
- 点击对话查看完整消息历史
- 在输入框直接与 Agent 对话 (不经过 WhatsApp)
- 快捷指令: "今日进度"、"你有什么想法？"
- 暂停/恢复单个对话
- 人工接管模式 (切换对话为手动)

### 2.5 Progress Statistics

**追踪指标**:

- 消息接收/发送数
- 平均响应延迟
- LLM Token 消耗
- API 调用次数
- 各对话状态 (活跃/暂停/关闭)
- 知识库文档数

**展示方式**:

- 底部统计条 (实时)
- "今日进度" 快捷指令 (Agent 描述性回答)

***

## 3. Architecture

### 3.1 Package Structure

```
agent/                         # Agent 核心
  agent.go                     # Agent struct, 对话循环, 工具调度
  config.go                    # AgentConfig 配置
  llm.go                       # DeepSeek API 客户端 (go-openai)
  tools.go                     # Tool 接口定义
  tools_scraper.go             # Google Maps 爬虫工具
  tools_rag.go                 # RAG 知识库搜索工具
  tools_send.go                # WhatsApp 发送工具
  prompt.go                    # 系统提示词构建
  conversation.go              # 对话历史管理 (SQLite)

rag/                           # RAG 知识库
  rag.go                       # RAG 服务接口
  store.go                     # SQLite 向量存储
  embedder.go                  # Embedding API 客户端
  chunker.go                   # 文本分块器
  pdf.go                       # PDF 解析
  docx.go                      # Word 文档解析

whatsapp/                      # 扩展现有包
  listener.go                  # (NEW) 接收消息轮询
  listener_selectors.go        # (NEW) 读取消息 DOM 选择器
  types.go                     # (扩展) IncomingMessage 类型
  sender.go                    # (现有) 发送消息
  service.go                   # (现有) 服务层
  selectors.go                 # (现有) DOM 选择器

cmd/whatsapp-desktop/          # 扩展 Wails 应用
  main.go                      # (扩展) App struct 添加 agent 字段
  desktop_agent.go             # (NEW) Agent Wails 绑定方法
  desktop_agent_db.go          # (NEW) Agent SQLite 初始化
  frontend/
    index.html                 # (扩展) 添加 AI Agent tab
    app.js                     # (扩展) Agent 聊天逻辑
    style.css                  # (扩展) 聊天 UI 样式
```

### 3.2 Data Flow

```
WhatsApp Web DOM
       │
       ▼
whatsapp.Listener.Poll()  (每 3-5 秒)
       │
       ▼
解析未读消息 (phone, name, text, timestamp)
       │
       ▼
入队 buffered channel (capacity 100)
       │
       ▼
agent.Agent.HandleIncomingMessage(ctx, msg)
       │
       ├─→ conversation.Store.GetHistory(phone, 20)
       ├─→ rag.Service.Search(query, 5)
       ├─→ 构建 messages: [system+RAG, ...history, user_msg]
       ├─→ llm.Client.ChatCompletion(messages, tools)
       │       │
       │       ├─→ 文本回复 → 直接使用
       │       └─→ tool_call → 执行工具 → 重新调用 LLM
       │
       ├─→ conversation.Store.SaveMessage(inbound + outbound)
       ├─→ whatsapp.Sender.SendInCurrentChat(reply)
       └─→ wails.EventEmit("agent:message", event)
```

### 3.3 Wails Bindings

```go
// Agent 控制
func (a *App) AgentStart(config AgentConfig) error
func (a *App) AgentStop() error
func (a *App) AgentStatus() map[string]any

// 对话管理
func (a *App) AgentConversations() ([]map[string]any, error)
func (a *App) AgentMessages(phone string, limit int) ([]map[string]any, error)
func (a *App) AgentSendManual(phone, text string) error
func (a *App) AgentSetConversationStatus(phone, status string) error

// Agent 聊天 (不经过 WhatsApp)
func (a *App) AgentChat(message string) (string, error)

// 知识库管理
func (a *App) AgentUploadKnowledge(path string) (map[string]any, error)
func (a *App) AgentListKnowledge() ([]map[string]any, error)
func (a *App) AgentDeleteKnowledge(docID string) error

// 统计
func (a *App) AgentStats() (map[string]any, error)
```

### 3.4 Wails Events

| Event           | Payload                                         | 触发时机        |
| --------------- | ----------------------------------------------- | ----------- |
| `agent:message` | `{phone, direction, content, timestamp}`        | 收到/发送消息     |
| `agent:status`  | `{status, error}`                               | Agent 启停/错误 |
| `agent:stats`   | `{messages_today, avg_latency_ms, tokens_used}` | 每 60 秒      |

***

## 4. Database Schema

所有表存储在 `~/.whatsapp-desktop/agent/agent.db`。

### 4.1 conversations

| 列           | 类型      | 说明                       |
| ----------- | ------- | ------------------------ |
| phone       | TEXT PK | 联系人手机号                   |
| name        | TEXT    | 联系人姓名                    |
| status      | TEXT    | active / paused / closed |
| created\_at | INTEGER | Unix timestamp           |
| updated\_at | INTEGER | Unix timestamp           |

### 4.2 messages

| 列            | 类型                       | 说明                               |
| ------------ | ------------------------ | -------------------------------- |
| id           | INTEGER PK AUTOINCREMENT | 消息 ID                            |
| phone        | TEXT                     | 联系人手机号                           |
| direction    | TEXT                     | inbound / outbound               |
| content      | TEXT                     | 消息内容                             |
| role         | TEXT                     | user / assistant / system / tool |
| tool\_name   | TEXT                     | 工具调用名称 (空字符串表示无工具)               |
| tokens\_used | INTEGER                  | LLM 消耗 token 数                   |
| created\_at  | INTEGER                  | Unix timestamp                   |

### 4.3 kb\_documents

| 列             | 类型      | 说明               |
| ------------- | ------- | ---------------- |
| id            | TEXT PK | UUID             |
| filename      | TEXT    | 原始文件名            |
| source\_type  | TEXT    | pdf / docx / txt |
| content\_hash | TEXT    | SHA-256          |
| chunk\_count  | INTEGER | 分块数量             |
| created\_at   | INTEGER | Unix timestamp   |
| updated\_at   | INTEGER | Unix timestamp   |

### 4.4 kb\_chunks

| 列            | 类型                       | 说明                |
| ------------ | ------------------------ | ----------------- |
| id           | INTEGER PK AUTOINCREMENT | 分块 ID             |
| document\_id | TEXT FK                  | 关联文档              |
| chunk\_index | INTEGER                  | 分块序号              |
| content      | TEXT                     | 分块文本              |
| embedding    | BLOB                     | 序列化 \[]float64 向量 |
| created\_at  | INTEGER                  | Unix timestamp    |

### 4.5 agent\_stats

| 列            | 类型                       | 说明                                                   |
| ------------ | ------------------------ | ---------------------------------------------------- |
| id           | INTEGER PK AUTOINCREMENT | 统计 ID                                                |
| event\_type  | TEXT                     | message\_received / reply\_sent / tool\_call / error |
| phone        | TEXT                     | 关联手机号                                                |
| detail       | TEXT                     | 事件详情                                                 |
| tokens\_used | INTEGER                  | Token 消耗                                             |
| latency\_ms  | INTEGER                  | 响应延迟                                                 |
| created\_at  | INTEGER                  | Unix timestamp                                       |

***

## 5. Configuration

### 5.1 AgentConfig

```go
type AgentConfig struct {
    DeepSeekAPIKey  string  `json:"deepseek_api_key"`
    DeepSeekBaseURL string  `json:"deepseek_base_url"`  // 默认 https://api.deepseek.com
    Model           string  `json:"model"`               // 默认 deepseek-chat
    EmbeddingModel  string  `json:"embedding_model"`     // 默认 deepseek-chat

    // 安全设置
    MaxRepliesPerHour     int `json:"max_replies_per_hour"`      // 默认 10
    ReplyDelayMinSeconds  int `json:"reply_delay_min_seconds"`   // 默认 3
    ReplyDelayMaxSeconds  int `json:"reply_delay_max_seconds"`   // 默认 10
    ContextWindowSize     int `json:"context_window_size"`       // 默认 20

    // RAG 设置
    ChunkSize     int `json:"chunk_size"`      // 默认 500
    ChunkOverlap  int `json:"chunk_overlap"`   // 默认 50
    TopK          int `json:"top_k"`           // 默认 5

    // 速率限制
    RateLimitPerSecond float64 `json:"rate_limit_per_second"` // 默认 2.0
}
```

### 5.2 Storage

```
~/.whatsapp-desktop/
├── agent/
│   └── agent.db          # 对话、向量、统计
├── whatsapp/
│   ├── session/          # Playwright 浏览器会话
│   ├── uploads/          # 上传文件
│   └── debug_screenshots/
├── geodata/
│   └── cities.db
└── results.db
```

***

## 6. Key Risks & Mitigations

| 风险                 | 严重度    | 缓解措施                         |
| ------------------ | ------ | ---------------------------- |
| WhatsApp 封号        | HIGH   | 打字模拟、随机延迟、频率限制、每联系人每小时上限     |
| LLM 幻觉发送错误信息       | HIGH   | 关键词守卫、保守系统提示词、可切换人工确认模式      |
| WhatsApp Web UI 更新 | MEDIUM | 多 fallback 选择器、健康检查、截图调试     |
| DeepSeek API 不可用   | MEDIUM | 指数退避重试、队列不丢消息                |
| 并发消息处理             | MEDIUM | 顺序处理 + buffered channel 队列   |
| 上下文窗口溢出            | MEDIUM | 滑动窗口 20 条 + 摘要压缩             |
| RAG 检索质量差          | LOW    | 调优 chunk size/overlap，显示引用来源 |

***

## 7. Implementation Phases

### Phase 1: 基础设施

- `cmd/whatsapp-desktop/desktop_agent.go` + `desktop_agent_db.go`
- `whatsapp/listener.go` + `listener_selectofrs.go`
- Wails frontend "AI Agent" tab 骨架
- `IncomingMessage` 类型

### Phase 2: LLM 集成

- `agent/` 包 (config, llm, conversation, prompt)
- 对话循环 + 工具调度
- Listener → Agent → Sender 管道
- 实时事件 + Wails 集成

### Phase 3: RAG 知识库

- `rag/` 包 (embedder, chunker, store, parsers)
- 知识管理 UI (上传/列表/删除)
- RAG 工具集成

### Phase 4: 工具 + 统计 + 安全

- Google Maps 爬虫工具
- 统计追踪 + 面板
- 安全措施 (限流/守卫/退避)

### Phase 5: 聊天界面完善

- AgentChat() 直接对话
- 快捷指令
- 人工接管模式
- RAG 来源显示

***

## 8. Verification Checklist

- [ ] Phase 1: Wails 启动，AI Agent tab 可见；WhatsApp 发消息，Listener 检测到
- [ ] Phase 2: 客户发"你好"，Agent 自动回复；界面实时显示消息
- [ ] Phase 3: 上传公司 PDF，客户问"公司做什么"，Agent 基于文档回答
- [ ] Phase 4: 界面问"今天进度"，返回统计；连续消息验证限流正常
- [ ] Phase 5: 直接与 agent 对话，快捷指令正常工作

