# Software Requirements Specification (SRS)
## Pyntra - AI-Powered Penetration Testing Assistant

**Version:** 1.5.2  
**Date:** 2026-04-24  
**Document Status:** Draft

---

## 1. Introduction

### 1.1 Project Scope
Pyntra is an AI-powered penetration testing assistant that automates cybersecurity assessment tasks. The system leverages large language models (LLMs) to perform reconnaissance, strategy planning, command generation, and execution for security testing purposes. This SRS documents the functional and non-functional requirements for the Pyntra system.

### 1.2 Design and Implementation Constraints
- **AI Model Provider:** Ollama (local deployment) with support for OpenAI-compatible API
- **Primary Model:** qwen2.5:7b (7B parameter model, 8192 token context window)
- **Alternative Models:** kimi-k2.6:cloud, qwen3.5:9b, and other Ollama-compatible models
- **API Compatibility:** OpenAI protocol compatible (supports multiple providers)
- **Deployment Mode:** Local/on-premise with optional cloud integration
- **Configuration:** YAML-based configuration file (config.yaml)

### 1.3 Assumptions and Dependencies
- **Ollama Service:** Must be running on localhost:11434
- **Model Availability:** Required AI models must be pulled locally via `ollama pull`
- **Network Access:** For initial model downloads and optional cloud provider integration
- **Hardware Requirements:** Minimum 10GB GPU VRAM recommended (12GB+ for optimal performance)
- **Dependencies:** Go runtime environment for backend services

---

## 2. System Features

### 2.1 System Feature 1: Reconnaissance Agent
**Purpose:** Automated information gathering and reconnaissance
- **Functionality:** 
  - Passive and active reconnaissance operations
  - Target information collection (IP ranges, domains, open ports)
  - Vulnerability identification through automated scanning
  - OSINT (Open Source Intelligence) data aggregation
- **AI Integration:** LLM-assisted target analysis and vulnerability prioritization

### 2.2 System Feature 2: Strategy Planner
**Purpose:** Attack path planning and strategy optimization
- **Functionality:**
  - Multi-stage attack path generation
  - Risk assessment and impact analysis
  - Resource allocation and prioritization
  - Privilege escalation planning
- **AI Integration:** LLM-based attack strategy optimization and decision support

### 2.3 System Feature 3: Command Generator
**Purpose:** Automated command and payload generation
- **Functionality:**
  - Metasploit module integration
  - Custom payload generation
  - Shell command automation
  - Exploit code generation
- **AI Integration:** LLM-powered command optimization and safety validation

### 2.4 System Feature 4: Execution Agent
**Purpose:** Task execution and monitoring
- **Functionality:**
  - Command execution automation
  - Result collection and parsing
  - Real-time monitoring and alerting
  - Session management
- **AI Integration:** LLM-assisted result interpretation and next-step recommendations

### 2.5 System Feature 5: Reporting and Dashboard
**Purpose:** Visualization and reporting of security assessments
- **Functionality:**
  - Real-time dashboard with key metrics
  - Comprehensive vulnerability reports
  - Executive summary generation
  - Historical data tracking and trend analysis
  - Export capabilities (PDF, HTML, JSON)
- **AI Integration:** LLM-powered report generation and natural language summaries

---

## 3. External Interface Requirements

### 3.1 User Interfaces
- **Web Interface:** 
  - URL: http://localhost:8080
  - Login required with password authentication
  - Dashboard for real-time monitoring
  - Configuration management interface
  - Report viewing and export functionality
- **API Interface:** RESTful API for programmatic access
- **CLI Interface:** Command-line tools for automation

### 3.2 Hardware Interfaces
- **Minimum Requirements:**
  - CPU: 4+ cores recommended
  - RAM: 16GB minimum (32GB recommended)
  - GPU: 10GB VRAM minimum (12GB+ recommended)
  - Storage: 50GB available space (model-dependent)
- **Recommended Hardware:**
  - Multi-core processor (8+ cores)
  - 32GB+ RAM
  - Dedicated GPU with 12GB+ VRAM
  - Fast storage (NVMe SSD recommended)

### 3.3 Software Interfaces
- **Primary APIs:**
  - Ollama API (http://localhost:11434/v1)
  - OpenAI-compatible API interface
  - Internal agent communication protocols
- **Integration Points:**
  - Knowledge base systems
  - Vulnerability databases
  - CI/CD pipelines
  - Security operations platforms

### 3.4 Communication Interfaces
- **Internal Communication:** 
  - Inter-agent messaging protocols
  - Task distribution and coordination
- **External Communication:**
  - Cloud service integration (optional)
  - API gateway communication
  - Webhook notifications
- **Data Protocols:**
  - JSON-based message format
  - RESTful HTTP/HTTPS
  - WebSocket for real-time updates

---

## 4. Nonfunctional Requirements

### 4.1 Performance Requirements
- **Response Time:** 
  - API responses: < 5 seconds (95th percentile)
  - Model inference: < 30 seconds for typical queries
  - Dashboard loading: < 3 seconds
- **Throughput:**
  - Concurrent users: 50+ supported
  - API requests per second: 100+ capacity
- **Scalability:** Horizontal scaling support for multi-agent operations

### 4.2 Safety Requirements
- **Access Control:**
  - Password-based authentication (configurable)
  - Session management with 12-hour timeout
  - Role-based access control (future enhancement)
- **Data Protection:**
  - Local data processing (privacy-preserving)
  - Secure API communications (HTTPS)
  - Input validation and sanitization
- **Operational Safety:**
  - Command execution sandboxing
  - Rate limiting and abuse prevention
  - Error handling and graceful degradation

### 4.3 Security Requirements
- **Authentication:** Password-based authentication with strong password policy
- **Authorization:** Session-based access control
- **Data Encryption:** HTTPS for all external communications
- **Audit Logging:** Comprehensive activity logging
- **Vulnerability Management:** Regular security updates and patches

### 4.4 Software Quality Attributes
- **Reliability:** 99.9% uptime target
- **Availability:** 24/7 operational capability
- **Maintainability:** Modular architecture with clear separation of concerns
- **Portability:** Cross-platform compatibility (Windows, Linux, macOS)
- **Usability:** Intuitive web interface with comprehensive documentation

---

## 5. Other Requirements

### 5.1 Database Requirements
- **Primary Database:** SQLite (embedded)
- **Data Storage:** 
  - Conversation history
  - User configurations
  - Knowledge base entries
  - Audit logs
- **Storage Location:** Local file system with configurable paths
- **Backup:** Automated backup mechanisms

### 5.2 Legal Requirements
- **Compliance:** Adherence to data protection regulations
- **Licensing:** Open-source dependencies with appropriate licensing
- **Usage Rights:** Clear terms of service and acceptable use policies
- **Data Sovereignty:** Local data processing ensures compliance with regional laws

---

## 6. Analysis Model

### 6.1 Data Flow Diagram
```
[User] → [Web Interface/API] → [Authentication]
    ↓
[Agent Coordinator]
    ↓
[Reconnaissance Agent] → [Data Collection] → [Storage]
    ↓
[Strategy Planner] → [Attack Path Generation] → [Execution Agent]
    ↓
[Command Generator] → [Payload Creation] → [Execution]
    ↓
[Execution Agent] → [Result Processing] → [Reporting]
    ↓
[Dashboard/Reporting] ← [All Agents]
```

### 6.2 Class Diagram
```
+---------------------+
|      User           |
+---------------------+
| - credentials       |
| - session           |
+----------+----------+
           |
           v
+---------------------+
|  AgentCoordinator   |
+---------------------+
| - agents: List      |
| - config: Config    |
+----------+----------+
           |
    +-----+------+
    |            |
    v            v
+-----------+  +----------------+
| Recon     |  | StrategyPlanner  |
| Agent     |  +----------------+  
+-----------+  | +--------------+ |
              | | CommandGen   | |
              | +--------------+ |
              | | ExecAgent    | |
              | +--------------+ |
              +----------------+
                           |
                           v
                  +----------------+
                  |   Reporting    |
                  |   & Dashboard  |
                  +----------------+
```

---

## 7. System Architecture

### 4.1 System Architecture

The Pyntra system follows a modular, agent-based architecture with the following components:

#### 7.1.1 Core Architecture
- **Configuration Layer:** YAML-based configuration (config.yaml)
- **Agent Framework:** Multi-agent system using Eino framework
- **AI Integration:** OpenAI-compatible API with Ollama backend
- **Communication:** RESTful APIs and inter-process messaging

#### 7.1.2 Component Diagram
```
+---------------------+
|    Configuration    |
|    (config.yaml)    |
+---------------------+
           |
           v
+---------------------+
|  Agent Orchestrator |
+---------------------+
    |       |       |
    v       v       v
+-----------+  +-----------+  +-----------+
| Recon     |  | Strategy  |  | Execution |
| Agent     |  | Planner   |  | Agent     |
+-----------+  +-----------+  +-----------+
    |               |               |
    +-------+-------+-------+-------+
            |
            v
    +----------------+
    | Command Gen    |
    | Agent          |
    +----------------+
            |
            v
    +----------------+
    | Reporting &    |
    | Dashboard      |
    +----------------+
```

#### 7.1.3 Deployment Architecture
- **Local Deployment:** Single-machine installation
- **Distributed Deployment:** Multi-agent across network
- **Cloud Integration:** Optional cloud service connectivity
- **Container Support:** Docker containerization capability

#### 7.1.4 Technology Stack
- **Backend:** Go (Golang)
- **AI Integration:** OpenAI-compatible API client
- **Database:** SQLite (embedded)
- **Web Framework:** Custom HTTP server
- **Configuration:** YAML parsing
- **Agent Framework:** Eino (Go-based multi-agent framework)

---

## 8. Configuration Management

### Current Configuration (config.yaml)
- **Version:** v1.5.2
- **Server:** Port 8080, all interfaces
- **Authentication:** Password-based (auto-generated strong password on first run; configurable)
- **AI Model:** qwen2.5:7b (Ollama)
- **Token Limit:** 8192 tokens
- **Embedding Model:** nomic-embed-text (Ollama)
- **Provider:** OpenAI-compatible API

### Configuration Options
- Provider selection (openai, claude, etc.)
- Base URL configuration
- API key management
- Model selection
- Token limits
- Multi-agent settings
- Database configuration
- Security settings

---

## 9. Compliance and Standards

### 9.1 Security Standards
- OWASP Top 10 compliance
- Secure authentication practices
- Input validation requirements
- Secure API communication

### 9.2 Data Standards
- Data privacy compliance
- Local data processing
- Secure data storage
- Audit logging requirements

### 9.3 Operational Standards
- Monitoring and alerting
- Performance metrics
- Error tracking
- System health checks

---

## 10. Future Enhancements

### 10.1 Planned Features
- Multi-user support
- Role-based access control
- Advanced reporting analytics
- Plugin system for additional tools
- Containerized deployment options
- REST API v2 with enhanced capabilities

### 10.2 Integration Points
- SIEM platform integration
- Vulnerability management systems
- Ticketing systems
- Cloud security platforms

---

**Document Version History:**
- v1.0: Initial draft
- v1.5.2: Updated for Ollama configuration with qwen2.5:7b model

**Approval:**
- Technical Lead: _________________
- Date: _________________________

**Distribution:**
- Development Team
- Security Operations
- Project Management
- Executive Stakeholders