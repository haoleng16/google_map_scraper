# Google Maps Scraper + WhatsApp AI Agent

> Fork of [gosom/google-maps-scraper](https://github.com/gosom/google-maps-scraper) with an integrated **WhatsApp Desktop App** and **AI Agent** powered by DeepSeek / OpenAI-compatible LLMs.

<p align="center">
  <a href="https://github.com/gosom/google-maps-scraper/stargazers"><img src="https://img.shields.io/github/stars/gosom/google-maps-scraper?style=social" alt="GitHub Stars"></a>
  <a href="https://github.com/gosom/google-maps-scraper/network/members"><img src="https://img.shields.io/github/forks/gosom/google-maps-scraper?style=social" alt="GitHub Forks"></a>
</p>

Extract Google Maps business leads, auto-reply WhatsApp customer messages with AI, manage contacts, and send bulk messages — all from a single Wails Desktop application.

| Goal | Start here |
|---|---|
| Get leads into CSV/JSON | [Command Line](#command-line) |
| Run a browser UI locally | [Web UI](#web-ui) |
| Automate scraping from your app | [REST API](#rest-api) |
| WhatsApp Desktop + AI Agent | [WhatsApp Desktop App](#whatsapp-desktop-app) |

If this project is useful to you, a GitHub star helps others discover it. Sponsorships help fund maintenance and new work.

---

## End-to-End Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    Wails Desktop App                      │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────┐  │
│  │ Scraper  │ │ Results  │ │ WhatsApp │ │  AI Agent  │  │
│  │   Tab    │ │   Tab    │ │ Send Tab │ │    Tab     │  │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └─────┬──────┘  │
│       │            │            │              │          │
│  ┌────▼────────────▼────────────▼──────────────▼──────┐  │
│  │               cmd/whatsapp-desktop                  │  │
│  │   App (Wails bindings)  →  desktop_agent.go        │  │
│  │   desktop_scraper.go    →  desktop_results.go      │  │
│  └──┬──────────┬──────────┬──────────┬────────────────┘  │
│     │          │          │          │                     │
│  ┌──▼───┐  ┌──▼───┐  ┌──▼───┐  ┌───▼────┐              │
│  │agent/│  │ rag/ │  │whats │  │ geodata│              │
│  │      │  │      │  │ app/ │  │        │              │
│  │Agent │  │ RAG  │  │Send  │  │Cities  │              │
│  │LLM   │  │Embed │  │Listen│  │DB      │              │
│  │Tools │  │Chunk │  │      │  │        │              │
│  └──┬───┘  └──────┘  └──┬───┘  └────────┘              │
│     │                     │                               │
│     │  DeepSeek/OpenAI    │  Playwright                   │
│     │  Compatible API     │  WhatsApp Web                 │
│     ▼                     ▼                               │
│  ┌─────────┐       ┌───────────┐                         │
│  │  LLM    │       │ WhatsApp  │                         │
│  │  API    │       │   Web     │                         │
│  └─────────┘       └───────────┘                         │
└──────────────────────────────────────────────────────────┘
```

### Data Flow

```
1. Scraper:  User selects country → keywords → Scraper runs via Playwright → Results stored in SQLite/CSV
2. Results:  User filters/browses scraped leads → Selects contacts → Imports to WhatsApp tab
3. WhatsApp: Load CSV contacts → Compose message → Batch send with throttling & delays
4. AI Agent: WhatsApp Listener polls for new messages → Agent processes via LLM
             → Optionally calls scraper tool → Auto-replies via WhatsApp
5. RAG:      Upload PDF/DOCX/TXT → Chunked → Embedded → Stored in SQLite vectors
             → Agent searches knowledge base to answer customer questions
```

---

## Project Structure

```
google-maps-scraper/
├── agent/                         # AI Agent core
│   ├── agent.go                   # Agent struct, message loop, tool dispatch
│   ├── config.go                  # AgentConfig (LLM, safety, rate limits)
│   ├── conversation.go            # ConversationStore (sliding window per phone)
│   ├── llm.go                     # OpenAI-compatible LLM client (DeepSeek)
│   ├── prompt.go                  # System prompt builder with RAG context
│   ├── safety.go                  # Keyword guard, content filter, sanitize
│   └── tools_scraper.go           # Google Maps scraper tool for agent
├── rag/                           # RAG knowledge base
│   ├── rag.go                     # Ingest, search, list, delete documents
│   ├── embedder.go                # Embedding API + cosine similarity
│   ├── chunker.go                 # Recursive text chunker
│   ├── pdf.go                     # PDF parser
│   ├── docx.go                    # DOCX parser
│   └── txt.go                     # Plain text passthrough
├── whatsapp/                      # WhatsApp automation
│   ├── service.go                 # Service (Playwright page pool)
│   ├── sender.go                  # Send messages, open chats, type text
│   ├── listener.go                # Poll for new incoming messages
│   ├── listener_selectors.go      # DOM selectors for reading messages
│   ├── contacts.go                # Contact parsing utilities
│   ├── selectors.go               # DOM selectors for WhatsApp Web
│   └── types.go                   # IncomingMessage struct
├── cmd/whatsapp-desktop/          # Wails Desktop app entry point
│   ├── main.go                    # App struct, Wails lifecycle, model config
│   ├── desktop_agent.go           # Agent Wails bindings
│   ├── desktop_agent_db.go        # Agent SQLite schema (conversations, messages, KB)
│   ├── desktop_agent_scraper.go   # Scraper adapter for agent tool calls
│   ├── desktop_scraper.go         # Scraper tab Wails bindings
│   ├── desktop_results.go         # Results tab Wails bindings
│   ├── desktop_geodata.go         # Country/city database
│   ├── main_test.go               # Integration tests
│   └── frontend/
│       ├── index.html             # 5-tab UI (Scraper/Results/WhatsApp/Agent/Settings)
│       ├── app.js                 # Frontend logic (all tabs)
│       └── style.css              # UI styles
├── geodata/                       # Global country/city SQLite database
├── web/                           # Web UI + REST API (upstream)
├── runner/                        # Scraping engine (upstream)
├── config/                        # Configuration types (upstream)
├── logging/                       # Logging utilities (upstream)
├── spec.md                        # Full product specification
└── README.md
```

---

## WhatsApp Desktop App

### Features

| Feature | Description |
|---------|-------------|
| **Google Maps Scraper** | Select country, enter keywords, scrape leads into CSV |
| **Results Browser** | Filter, search, and manage scraped business data |
| **WhatsApp Bulk Send** | Load CSV contacts, compose messages, batch send with throttling |
| **AI Auto-Reply** | DeepSeek/OpenAI LLM auto-replies to incoming WhatsApp messages |
| **RAG Knowledge Base** | Upload PDF/DOCX/TXT, agent answers questions from your documents |
| **Scraper Tool** | Agent can call Google Maps scraper to find businesses on demand |
| **Human Takeover** | Switch from auto-reply to manual mode for any conversation |
| **Model Config** | Configure API key, base URL, and model name in Settings |

### Prerequisites

- Go 1.25+
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation)
- Playwright browsers (`go run github.com/playwright-community/playwright-go/cmd/playwright@latest install --with-deps chromium`)

### Build & Run

```bash
# Install Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Clone the repo
git clone https://github.com/haoleng16/google_map_scraper.git
cd google_map_scraper

# Run in development mode (hot reload)
wails dev

# Build production binary
wails build
```

### AI Agent Setup

1. Open the app, go to **Settings** tab
2. Enter your **API Key**, **Base URL** (e.g. `https://api.deepseek.com`), and **Model** (e.g. `deepseek-chat`)
3. Click **Save**
4. Login to WhatsApp via the top-right button
5. Go to **AI Agent** tab, click **Start**
6. Upload documents (PDF/DOCX/TXT) to the knowledge base
7. The agent will now auto-reply to incoming messages using your knowledge base

---

## Table of Contents

- [Quick Start](#quick-start)
  - [Command Line](#command-line)
  - [Web UI](#web-ui)
  - [REST API](#rest-api)
  - [SaaS Edition](#saas-edition)
- [AI Agent Skill](#ai-agent-skill)
- [Recipes](docs/recipes.md)
- [Proxy Sponsors](docs/proxies.md)
- [Installation](#installation)
- [Features](#features)
- [Extracted Data Points](#extracted-data-points)
- [Configuration](#configuration)
  - [Command Line Options](#command-line-options)
  - [Using Proxies](#using-proxies)
  - [Email Extraction](#email-extraction)
  - [Fast Mode](#fast-mode)
- [Export to LeadsDB](#export-to-leadsdb)
- [Advanced Usage](#advanced-usage)
  - [PostgreSQL Database Provider](#postgresql-database-provider)
  - [Kubernetes Deployment](#kubernetes-deployment)
  - [Custom Writer Plugins](#custom-writer-plugins)
- [Performance](#performance)
- [Support the Project](#support-the-project)
- [Community](#community)
- [Contributing](#contributing)
- [License](#license)

---

## Quick Start

### Command Line (upstream)

```bash
mkdir -p gmaps-output

docker run \
  -v gmaps-playwright-cache:/opt \
  -v "$PWD/example-queries.txt:/queries.txt:ro" \
  -v "$PWD/gmaps-output:/out" \
  gosom/google-maps-scraper \
  -input /queries.txt \
  -results /out/results.csv \
  -depth 1 \
  -exit-on-inactivity 3m
```

### Web UI (upstream)

```bash
mkdir -p gmapsdata

docker run \
  -v "$PWD/gmapsdata:/gmapsdata" \
  -p 8080:8080 \
  gosom/google-maps-scraper \
  -data-folder /gmapsdata
```

Then open http://localhost:8080 in your browser.

---

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.25+ |
| Desktop Framework | Wails v2 |
| Browser Automation | Playwright (Go) |
| LLM | DeepSeek / OpenAI-compatible API |
| Embeddings | DeepSeek embedding API |
| Vector Store | SQLite BLOB + cosine similarity |
| Document Parsing | PDF, DOCX, TXT |
| Database | SQLite (agent data, conversations, knowledge base) |
| Frontend | Vanilla HTML/CSS/JS |

---

## License

This project is licensed under the [MIT License](LICENSE).

---

## Legal Notice

Please use this scraper responsibly and in accordance with applicable laws and regulations. Unauthorized scraping may violate terms of service.
