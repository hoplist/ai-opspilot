# RCA Architecture

## High-level Architecture

```mermaid
flowchart LR
    subgraph K8S["Kubernetes Cluster"]
        A["Container Logs"]
        B["Kubernetes Events"]
        C["Pod / Node Metadata"]
        D["Prometheus + kube-state-metrics"]
    end

    A --> FB["Fluent Bit (logs)"]
    B --> FBE["Fluent Bit (events)"]
    C --> OTel["OTel Collector"]
    FB --> OS["OpenSearch"]
    FBE --> OS
    OTel --> OS
    D --> API["auto_inspection backend"]

    OS --> API
    API --> RCA["AI Investigation / RCA"]
    API --> MCP["MCP Server"]
    API --> UI["RCA Workbench"]
    OS --> Dash["OpenSearch Dashboards"]

    RCA --> OS
    UI --> User["Operator / SRE"]
    Dash --> User
    MCP --> Codex["Codex Skill / MCP Client"]
```

## Core Flows

### 1. Data Ingestion

- container logs are shipped by Fluent Bit into `logs-k8s-*`
- Kubernetes events are shipped by Fluent Bit into `events-k8s-*`
- Prometheus and kube-state-metrics provide Pod / Node metric context

### 2. Incident and Investigation Flow

- the pipeline generates incidents into `inspection-incidents-*`
- the backend reads incidents, logs, events, and Prometheus context
- the backend writes investigation results into `inspection-investigations-*`

### 3. User-facing Entry Points

- Dashboards provides search and charts
- the RCA page provides summary cards and one-click investigation
- MCP / Skill provides conversational access
