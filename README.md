<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/brand/pyntra-symbol-dark.svg">
  <img src="images/brand/pyntra-symbol.svg" alt="Pyntra" width="76" height="76">
</picture>

# Pyntra

### Security as a Service

**An AI-native penetration-testing platform — drive autonomous agents through the full offensive-security lifecycle from one console, a chat message, or your favorite AI coding tool.**

<p>
  <a href="https://go.dev/dl/"><img alt="Go" src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-Apache_2.0-blue.svg"></a>
  <img alt="Platform" src="https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey">
  <img alt="Protocol" src="https://img.shields.io/badge/MCP-native-34d399?logo=modelcontextprotocol&logoColor=white">
  <br>
  <img alt="Tools" src="https://img.shields.io/badge/security_tools-111-ef4444">
  <img alt="Roles" src="https://img.shields.io/badge/roles-13-f59e0b">
  <img alt="Skills" src="https://img.shields.io/badge/skills-23-8b5cf6">
  <img alt="Sub-agents" src="https://img.shields.io/badge/sub--agents-16-06b6d4">
  <img alt="Models" src="https://img.shields.io/badge/models-local%20%2B%2013%20providers-DC2626">
</p>

<a href="#-quick-start"><b>Quick start</b></a> ·
<a href="#-using-pyntra"><b>Usage</b></a> ·
<a href="#-the-dashboard"><b>Dashboard</b></a> ·
<a href="#-configuration"><b>Configure</b></a> ·
<a href="#-integrations"><b>Integrations</b></a>

</div>

---

**Pyntra** orchestrates **111 security tools** over the [Model Context Protocol (MCP)](https://modelcontextprotocol.io), reasons about findings, builds **attack chains**, retrieves from a **security knowledge base**, enforces **engagement scope**, and keeps every step auditable. Describe the objective in plain language — the agent plans, runs tools, correlates results, maps them to **MITRE ATT&CK**, and produces a report.

Run it **fully offline** against a local model, or connect **any inference provider** (Ollama, OpenAI, Anthropic, Hugging Face, +9 more). Integrate **Burp Suite** over MCP, and use Pyntra itself as an MCP server from **Claude Code, Cursor, Cline, opencode, Codex, and Windsurf**.

> [!WARNING]
> **Authorized use only.** Pyntra is for penetration testing and security research on systems you **own or are explicitly authorized to test**. You are responsible for complying with all applicable laws and rules of engagement. The built-in **engagement scope guard** helps keep agents inside authorized targets — it supports, but does not replace, your own authorization process.

---

## ✨ Features

<table>
<tr>
<td width="50%" valign="top">

**🤖 Agents & orchestration**
- Single-agent (ReAct) + multi-agent: **Deep**, **Plan-Execute**, **Supervisor**
- 16 specialized sub-agents that plan, act, self-correct
- **Attack-chain graph** → **MITRE ATT&CK** mapping
- **Kill-switch** to halt everything · per-engagement **dry-run**

**🧠 Any model — local or API, secure**
- **13 providers**: Ollama/local, OpenAI, Claude, HF, DeepSeek, OpenRouter, Groq, Together, Mistral, LM Studio, vLLM, LocalAI, custom
- **Local model discovery** from Settings (Ollama/`/v1/models`)
- Keys **redacted** from the API, `${ENV_VAR}` refs, HTTPS enforced to remote

</td>
<td width="50%" valign="top">

**🎯 Engagements, scope & reporting**
- **Engagements** scope findings & targets
- **Scope guard** blocks tools outside authorized domains/CIDRs/URLs
- **Report generator** → Markdown / HTML pentest reports

**📚 Knowledge base (RAG)**
- **Hybrid** retrieval: dense vectors + BM25/FTS5, RRF fusion
- Optional cross-encoder **reranker** + query cache

**🖥️ Console**
- Redesigned **Command Center** dashboard
- Light (white+red) & **pure-black** dark · ⌘K palette

</td>
</tr>
</table>

---

## 🚀 Quick start

### 1. Prerequisites

| Requirement | Why | Install |
|---|---|---|
| **[Go 1.25+](https://go.dev/dl/)** | Builds the server | [go.dev/dl](https://go.dev/dl/) |
| **An LLM endpoint** | The agent's brain | **[Ollama](https://ollama.com)** (local, offline) or any API below |
| **Python 3** *(optional)* | A few Python-based tools | [python.org](https://www.python.org/) |
| **Security CLIs** *(optional)* | `nmap`, `nuclei`, `ffuf`, `amass`, … | your package manager; the agent uses whatever is on `PATH` |

### 2. Install & run

```bash
# Clone
git clone https://github.com/prnvv2/pyntra.git
cd pyntra

# (recommended) start a local model — fully offline
ollama pull llama3.1:8b

# Build
go build -o pyntra ./cmd/server      # Windows: go build -o pyntra.exe ./cmd/server

# Run
./pyntra                             # Windows: .\pyntra.exe
```

<details>
<summary><b>Platform notes (Linux / macOS / Windows)</b></summary>

- **Linux/macOS:** install Go and (optionally) the security CLIs via your package manager (`apt`, `brew`, …). Run `./pyntra`.
- **Windows:** install Go from the MSI, build with `go build -o pyntra.exe ./cmd/server`, run `.\pyntra.exe`. Git Bash or PowerShell both work.
- **Docker / remote:** keep `server.host: 127.0.0.1` (loopback) unless you've set a strong `auth.password` and understand the exposure.
</details>

### 3. First sign-in

On first run, Pyntra creates `config.yaml` from `config.example.yaml`, **generates a strong random web password**, writes it to `config.yaml`, and prints it to the console:

```
[Pyntra] ✅ Auto-generated and written web login password for you.
Password: ••••••••••••••••••••••••
```

Open **http://localhost:8080** and sign in with that password.

> [!IMPORTANT]
> `config.yaml` holds your API keys and password and is **git-ignored** — commit `config.example.yaml` instead. Change the password anytime in **Settings** or `auth.password`. See **[docs/providers.md](docs/providers.md)** for the full provider walkthrough.

### 4. Point it at a model

Open **Settings → Model**, pick a **provider** (e.g. *Ollama · local*), click **Fetch models** to list what's installed, choose one, and **Test connection**. That's it — you're ready.

---

## 📖 Using Pyntra

> Always operate within an authorized **engagement scope** (see below). Pyntra executes real tools against the targets you give it.

### Define an engagement (recommended first step)
Create an **engagement** with your authorized **scope** (domains / CIDRs / URLs, with optional exclusions and an authorization window), then make it active. The **scope guard** will block any tool call whose target falls outside it. Use **dry-run** to have the agent plan without executing.

### Start work — three ways
| Entry point | Best for |
|---|---|
| **Chat** | Conversational testing — describe the objective, watch the agent plan and run tools, ask follow-ups. |
| **Tasks / batch** | Queue many targets or repeatable jobs; each runs the agent loop. |
| **MCP client** | Drive Pyntra's tools from Claude Code, Cursor, Cline, etc. (see [docs/mcp-clients.md](docs/mcp-clients.md)). |

### Pick a role & mode
- **Roles** (13) scope the agent's prompt and restrict which tools it may use — e.g. *Information-Collection*, *Web-Application-Scanning*, *Cloud-Security-Audit*, *CTF*, *Digital-Forensics*. Choose one that matches the task.
- **Mode**: *single* (fast, linear) or *multi* (*Deep* / *Plan-Execute* / *Supervisor*) for complex, multi-step objectives.

### Review results
- **Vulnerabilities** — findings with severity, target, status, evidence and remediation. Confirm, triage, or mark fixed.
- **Attack graph** — how findings chain together, tagged to MITRE ATT&CK.
- **Report** — export a Markdown/HTML pentest report from the findings.
- **Knowledge & Skills monitors** — what the agent retrieved and which skills it used.

### Safety controls
- **Kill-switch** (top bar) halts all tool execution instantly.
- **Scope guard** + **authorization window** keep the agent inside bounds.
- Every tool call and finding is recorded for audit.

---

## 📊 The dashboard

The **Command Center** gives a live operational view:

| Panel | Shows |
|---|---|
| **KPI cards** | Running tasks · Vulnerabilities · Tool invocations · Tool success rate, each with trend vs. the last refresh. |
| **Findings & activity trend** | Session activity over time (area chart with hover detail). |
| **Severity breakdown** | Findings by Critical / High / Medium / Low / Info. |
| **Recent findings** | Latest vulnerabilities with severity, target and status. |
| **Top tools** | Most-used tools by invocation count. |
| **Tool success rate** | Live gauge with success / failed split. |
| **Pyntra Assistant** | Jump a question straight into Chat. |

The model/engine status (provider, model, agent mode, reachability) and a one-click **Export report** are always in view. Light and pure-black dark themes, plus a **⌘K** command palette for fast navigation.

---

## ⚙️ Configuration

Everything lives in `config.yaml` (created from `config.example.yaml` on first run) and is also editable from **Settings**. Keep secrets out of the file with env references, e.g. `api_key: ${OPENAI_API_KEY}`.

| Section | Purpose |
|---|---|
| `server` | Listen host/port. Loopback by default — set a strong `auth.password` before exposing it. |
| `auth` | Web password & session length — auto-generated on first run; override anytime. |
| `openai` | Model: `provider`, `base_url`, `api_key`, `model` (any OpenAI-compatible or Claude endpoint). |
| `agent` / `multi_agent` | Iteration limits, timeouts, orchestration mode. |
| `knowledge` | RAG: embeddings, `retrieval.mode` (`dense`/`lexical`/`hybrid`), `rrf_k`, optional `rerank`. |

→ Full catalogue in **[docs/providers.md](docs/providers.md)**.

---

## 🧭 Architecture

```
┌──────────────┐   HTTP / SSE / WS   ┌───────────────────────────────────────┐
│  Web console │◀───────────────────▶│  Gin HTTP server                      │
│ (Command     │                     │  • Agent loop (ReAct)                 │
│  Center)     │                     │  • Multi-agent (CloudWeGo Eino)       │
└──────────────┘                     │  • Scope guard · kill-switch          │
                                     │  • Knowledge (hybrid RAG · rerank)    │
   AI coding tools ──MCP──▶          │  • Reporting · attack chains          │
   (Claude Code, Cursor…)            └────────────────┬──────────────────────┘
                                                      │ MCP
                     ┌────────────────────────────────┼────────────────────────┐
                     ▼                                 ▼                        ▼
             Built-in tools (111)          External MCP (Burp, Ghidra)   Inference provider
             nmap · nuclei · ffuf …        over stdio / SSE              (local or API)
```

- **Backend:** Go + [Gin](https://github.com/gin-gonic/gin); multi-agent via [CloudWeGo Eino](https://github.com/cloudwego/eino); SQLite (`modernc.org/sqlite`, FTS5).
- **Console:** no-build vanilla JS + CSS, Canvas charts, [Cytoscape](https://js.cytoscape.org/) attack graph, [Motion One](https://motion.dev).
- Deep dives: [docs/MULTI_AGENT_EINO.md](docs/MULTI_AGENT_EINO.md) · [docs/mcp-clients.md](docs/mcp-clients.md)

---

## 🔌 Integrations

- **Inference providers** — 13 presets + custom, local & hosted → [docs/providers.md](docs/providers.md)
- **Burp Suite over MCP** → [docs/burp-mcp.md](docs/burp-mcp.md)
- **Pyntra as an MCP server** (Claude Code, Cursor, Cline, opencode, Codex, Windsurf) → [docs/mcp-clients.md](docs/mcp-clients.md)
- **Recon API keys** — Shodan, Censys (optional)
- **Chat bots** — Telegram · Slack · Discord

---

## 📁 Project structure

```
cmd/            Entrypoints (server, MCP stdio, test helpers)
internal/
  agent/        Single-agent ReAct loop
  multiagent/   Eino multi-agent orchestration
  mcp/          MCP server + external MCP manager
  knowledge/    RAG: embeddings, hybrid search, rerank, indexing
  security/     Tool executor + scope guard + auth
  handler/      HTTP handlers (config, engagements, reports, vulns…)
  database/     SQLite models (findings, attack chains, engagements…)
roles/ skills/ agents/ tools/   YAML/Markdown definitions
web/            Console (templates + static: css, js, brand, vendor)
docs/           Provider, MCP, multi-agent, i18n docs
```

---

## 🤝 Contributing

Issues and PRs welcome — keep them focused with a clear description. Security tooling contributions must be for **authorized** testing use.

## 📄 License

Apache 2.0 — see [LICENSE](LICENSE).

<div align="center">
<br>
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/brand/pyntra-symbol-dark.svg">
  <img src="images/brand/pyntra-symbol.svg" alt="" height="26">
</picture>
<br><sub><b>Pyntra</b> — Security as a Service</sub>
</div>
