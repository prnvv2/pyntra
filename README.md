<div align="center">

<img src="images/brand/pyntra-lockup.svg" alt="Pyntra" height="72">

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
  <img alt="LLM" src="https://img.shields.io/badge/models-local%20%2B%2013%20providers-DC2626">
</p>

<a href="#-quick-start"><b>Quick start</b></a> ·
<a href="#-features"><b>Features</b></a> ·
<a href="#-architecture"><b>Architecture</b></a> ·
<a href="#-configuration"><b>Configure</b></a> ·
<a href="#-integrations"><b>Integrations</b></a>

</div>

---

**Pyntra** orchestrates **111 security tools** over the [Model Context Protocol (MCP)](https://modelcontextprotocol.io), reasons about findings, builds **attack chains**, retrieves from a **security knowledge base**, enforces **engagement scope**, and keeps every step auditable. Describe the objective in plain language — the agent plans, runs tools, correlates results, maps them to **MITRE ATT&CK**, and generates a report.

It runs **fully offline** against a local model, connects to **any inference provider** (Ollama, OpenAI, Anthropic, Hugging Face, +9 more), integrates **Burp Suite** over MCP, and can itself be consumed as an MCP server by **Claude Code, Cursor, Cline, opencode, Codex, and Windsurf**.

> [!WARNING]
> **Authorized use only.** Pyntra is for penetration testing and security research on systems you **own or are explicitly authorized to test**. You are responsible for complying with all applicable laws and rules of engagement. The built-in **engagement scope guard** helps keep agents inside authorized targets, but it does not replace your own authorization process.

---

## ✨ Features

### Agent & orchestration
- **Single-agent (ReAct)** and **multi-agent** modes — **Deep**, **Plan-Execute**, and **Supervisor** — that plan, act, and self-correct across 16 specialized sub-agents.
- **Kill-switch** to halt all tool execution instantly, plus per-engagement **dry-run** (plan without executing).
- **Attack-chain graph** mapped to **MITRE ATT&CK** tactics and techniques.

### Model integration — API + local, secure
- **13 provider presets**: Ollama / local, OpenAI, Anthropic Claude, Hugging Face, DeepSeek, OpenRouter, Groq, Together, Mistral, LM Studio, vLLM, LocalAI, or any custom OpenAI-compatible endpoint.
- **Local model discovery** (Ollama `/api/tags`, OpenAI `/v1/models`) straight from Settings — no CORS, keys never leave the server.
- **Credential safety**: API keys are redacted from API responses, support `${ENV_VAR}` references, and plain-HTTP to remote hosts is refused.

### Engagements, scope & reporting
- **Engagements** scope findings and targets; the **scope guard** blocks tool calls outside authorized domains/CIDRs/URLs, with an authorization window.
- **Report generator** turns findings into a Markdown / HTML pentest report (exec summary, severity-sorted findings, evidence, remediation).

### Knowledge base (RAG)
- **Hybrid retrieval** — dense vectors **+** BM25/FTS5, fused with Reciprocal Rank Fusion — plus an optional cross-encoder **reranker** and a query cache.
- Multi-format ingestion and retrieval analytics (zero-hit rate, top queries).

### Console
- Redesigned **Command Center** dashboard — KPI cards, activity trend, severity breakdown, tool leaderboard, success gauge, and a Pyntra Assistant.
- Light (white + red) and **pure-black** dark themes, command palette (⌘K), and the Pyntra brand throughout.

### MCP, both ways
- A built-in MCP server, first-class **external MCP servers** (incl. **Burp Suite** and Ghidra), and Pyntra usable **as** an MCP server from AI coding tools.

---

## 🚀 Quick start

### Prerequisites
| Requirement | Notes |
|---|---|
| **[Go 1.25+](https://go.dev/dl/)** | To build the server. |
| **An LLM endpoint** | A local **[Ollama](https://ollama.com)** install (recommended, fully offline), or any OpenAI-compatible / Claude / HF endpoint. |
| **Python 3** *(optional)* | For a few Python-based tools (e.g. `shodan_search`, `censys_search`). |
| **Security CLIs** *(optional)* | `nmap`, `nuclei`, `ffuf`, `amass`, … — install the ones you plan to use; the agent uses whatever is on `PATH`. |

```bash
# 1 — Clone
git clone https://github.com/prnvv2/pyntra.git
cd pyntra

# 2 — Start a local model (recommended, fully offline)
ollama pull llama3.1:8b
#     then set openai.model in config.yaml (base_url already points at Ollama)

# 3 — Build & run
go build -o pyntra ./cmd/server
./pyntra                          # Windows: .\pyntra.exe
```

On first run, `config.yaml` is created from `config.example.yaml` and a **strong random web password is generated**, written to `config.yaml`, and printed to the console. Open **http://localhost:8080** and sign in with that password.

> [!IMPORTANT]
> Keep the generated password safe, or set your own in `auth.password` (`config.yaml`) or from **Settings**. `config.yaml` holds your API keys and password and is **git-ignored** — commit `config.example.yaml` instead. See **[docs/providers.md](docs/providers.md)** for the full offline / provider walkthrough.

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

- **Backend:** Go + [Gin](https://github.com/gin-gonic/gin); multi-agent via [CloudWeGo Eino](https://github.com/cloudwego/eino); SQLite (pure-Go `modernc.org/sqlite`, FTS5).
- **Protocol:** Model Context Protocol (MCP) — server and client.
- **Console:** no-build vanilla JS + CSS, Canvas charts, [Cytoscape](https://js.cytoscape.org/) attack graph, [Motion One](https://motion.dev) animation.
- See [docs/MULTI_AGENT_EINO.md](docs/MULTI_AGENT_EINO.md) and [docs/mcp-clients.md](docs/mcp-clients.md) for details.

---

## ⚙️ Configuration

All configuration lives in `config.yaml` (created from `config.example.yaml` on first run) and can also be edited from the **Settings** page. Secrets can be kept out of the file with environment references, e.g. `api_key: ${OPENAI_API_KEY}`.

| Section | Purpose |
|---|---|
| `server` | Listen host/port. Loopback by default — change `auth.password` before exposing it. |
| `auth` | Web login password and session length — auto-generated on first run; set your own to override. |
| `openai` | Model provider — `provider`, `base_url`, `api_key`, `model` (works with any OpenAI-compatible or Claude endpoint). |
| `agent` / `multi_agent` | Iteration limits, timeouts, orchestration mode. |
| `knowledge` | RAG: embeddings, `retrieval.mode` (`dense`/`lexical`/`hybrid`), `rrf_k`, optional `rerank`. |

See **[docs/providers.md](docs/providers.md)** for the full provider catalogue.

---

## 🔌 Integrations

- **Inference providers** — 13 presets + custom; local and hosted. → [docs/providers.md](docs/providers.md)
- **Burp Suite over MCP** — drive Burp from the agent. → [docs/burp-mcp.md](docs/burp-mcp.md)
- **Pyntra as an MCP server** — connect Claude Code, Cursor, Cline, opencode, Codex, Windsurf. → [docs/mcp-clients.md](docs/mcp-clients.md)
- **Recon API keys** — Shodan, Censys (optional, for those tools).
- **Chat bots** — Telegram, Slack, Discord.

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

Issues and PRs are welcome. Please keep contributions focused and include a clear description. For security tooling, ensure additions are for **authorized** testing use.

## 📄 License

Apache 2.0 — see [LICENSE](LICENSE).

<div align="center">
<br>
<img src="images/brand/pyntra-symbol.svg" alt="" height="28">
<br><sub><b>Pyntra</b> — Security as a Service</sub>
</div>
