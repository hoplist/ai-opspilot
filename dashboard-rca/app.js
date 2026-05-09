(function () {
  const STORAGE_KEY = "dashboard-rca-form";
  let runtimeConfig = null;

  function byId(id) {
    return document.getElementById(id);
  }

  function escapeHtml(value) {
    return String(value ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function requestJson(url, options) {
    const opts = options || {};
    const timeoutMs = opts.timeoutMs || 120000;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    return fetch(url, {
      method: opts.method || "GET",
      headers: { "Content-Type": "application/json" },
      body: opts.body,
      cache: "no-store",
      signal: controller.signal,
    })
      .then(async (response) => {
        const payload = await response.json().catch(() => ({}));
        if (!response.ok) {
          throw new Error(payload.error || `${response.status} ${response.statusText}`);
        }
        if (payload && payload.error) {
          throw new Error(payload.error);
        }
        return payload;
      })
      .finally(() => clearTimeout(timer));
  }

  async function loadRuntimeConfig() {
    if (runtimeConfig) return runtimeConfig;
    runtimeConfig = await requestJson("/api/config").catch(() => ({}));
    return runtimeConfig;
  }

  function setStatus(text, mode) {
    const badge = byId("status-badge");
    if (!badge) return;
    badge.textContent = text;
    badge.className = `status-badge${mode ? ` ${mode}` : ""}`;
  }

  function setText(id, text) {
    const node = byId(id);
    if (!node) return;
    node.textContent = text || "-";
  }

  function saveFormState() {
    const value = {
      namespace: byId("namespace").value.trim(),
      pod: byId("pod").value.trim(),
      workload_name: byId("workload_name").value.trim(),
      range_hours: byId("range_hours").value.trim(),
      question: byId("question").value.trim(),
      query: byId("query").value.trim(),
      use_ai: byId("use_ai").checked,
    };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
  }

  function loadFormState() {
    const params = new URLSearchParams(location.search);
    let saved = {};
    try {
      saved = JSON.parse(localStorage.getItem(STORAGE_KEY) || "{}");
    } catch (error) {
      saved = {};
    }

    byId("namespace").value = params.get("namespace") || saved.namespace || "langfuse";
    byId("pod").value = params.get("pod") || saved.pod || "langfuse-clickhouse-shard0-0";
    byId("workload_name").value = params.get("workload_name") || saved.workload_name || "";
    byId("range_hours").value = params.get("range_hours") || saved.range_hours || "6";
    byId("question").value = params.get("question") || saved.question || "这个 Pod 为什么一直重启？";
    byId("query").value = params.get("query") || saved.query || "";
    byId("use_ai").checked = params.get("use_ai") ? params.get("use_ai") === "true" : Boolean(saved.use_ai);
  }

  function renderSimpleList(containerId, items, formatter, emptyText) {
    const node = byId(containerId);
    const list = Array.isArray(items) ? items : [];
    if (!node) return;
    if (!list.length) {
      node.innerHTML = `<div class="empty-state">${escapeHtml(emptyText || "暂无内容")}</div>`;
      return;
    }
    node.innerHTML = list.map(formatter).join("");
  }

  function renderTextList(containerId, items, emptyText) {
    const node = byId(containerId);
    const list = Array.isArray(items) ? items.filter(Boolean) : [];
    if (!node) return;
    node.innerHTML = list.length
      ? list.map((item) => `<li>${escapeHtml(item)}</li>`).join("")
      : `<li class="empty-state">${escapeHtml(emptyText || "暂无内容")}</li>`;
  }

  function renderRootCauses(items) {
    renderSimpleList(
      "root-cause-list",
      items,
      (item) => {
        const evidence = (item.evidence || []).map((line) => `<li>${escapeHtml(line)}</li>`).join("");
        const counter = (item.counter_evidence || []).map((line) => `<li>${escapeHtml(line)}</li>`).join("");
        return `
          <li>
            <h3>${escapeHtml(item.title || "未命名根因")}</h3>
            <div class="confidence">置信度 ${Math.round((Number(item.confidence) || 0) * 100)}%</div>
            <div class="evidence-block"><strong>支持证据</strong><ul class="plain-list">${evidence || "<li>暂无</li>"}</ul></div>
            <div class="evidence-block"><strong>反证</strong><ul class="plain-list">${counter || "<li>暂无</li>"}</ul></div>
          </li>
        `;
      },
      "暂无根因结论"
    );
  }

  function renderDashboardsLinks(links) {
    const ordered = [
      ["overview_dashboard", "总览 Dashboard"],
      ["rca_dashboard", "RCA Dashboard"],
      ["logs", "日志视图"],
      ["events", "事件视图"],
    ];
    const items = ordered
      .filter(([key]) => links && links[key])
      .map(([key, label]) => ({ label, href: links[key] }));
    renderSimpleList(
      "dashboards-links",
      items,
      (item) => `
        <a href="${escapeHtml(item.href)}" target="_blank" rel="noreferrer">
          <span>${escapeHtml(item.label)}</span>
          <small>${escapeHtml(item.href)}</small>
        </a>
      `,
      "暂无可用跳转"
    );
  }

  function renderRecentInvestigations(items) {
    renderSimpleList(
      "recent-investigations",
      items,
      (item) => `
        <a href="?investigation_id=${encodeURIComponent(item.investigation_id)}">
          <span>
            <strong>${escapeHtml(item.namespace || "-")}/${escapeHtml(item.pod || item.workload_name || "-")}</strong><br />
            <small>${escapeHtml(item.summary || "暂无摘要")}</small>
          </span>
          <small>${escapeHtml(item.generated_at || "-")}</small>
        </a>
      `,
      "暂无调查记录"
    );
  }

  function renderTargetMeta(item) {
    const parts = [];
    if (item.risk_level) parts.push(`风险 ${item.risk_level}`);
    if (item.dominant_risk) parts.push(`主风险 ${item.dominant_risk}`);
    if (item.lifecycle) parts.push(`生命周期 ${item.lifecycle}`);
    if (Array.isArray(item.source_types) && item.source_types.length) parts.push(item.source_types.join(" + "));
    return parts.join(" · ");
  }

  function riskClass(item) {
    const level = String(item.risk_level || item.final_risk_level || "").toLowerCase();
    return level ? `risk-${level}` : "";
  }

  function buildInvestigationHref(item) {
    const params = new URLSearchParams();
    if (item.namespace) params.set("namespace", item.namespace);
    if (item.pod) params.set("pod", item.pod);
    if (item.workload_name) params.set("workload_name", item.workload_name);

    const title = item.namespace
      ? `${item.namespace}/${item.pod || item.workload_name || "-"}`
      : (item.instance || item.event_key || "-");
    const reasonParts = [];
    if (item.dominant_risk) reasonParts.push(item.dominant_risk);
    if (item.lifecycle) reasonParts.push(item.lifecycle);
    if (Array.isArray(item.signals) && item.signals.length) {
      reasonParts.push(item.signals.slice(0, 3).join(","));
    }

    params.set("question", `排查 ${title} 为什么出现 ${reasonParts.join(" / ") || "异常"}`);
    if (item.dominant_risk) {
      params.set("query", item.dominant_risk);
    } else if (Array.isArray(item.signals) && item.signals.length) {
      params.set("query", item.signals[0]);
    }
    params.set("range_hours", "24");
    return `?${params.toString()}`;
  }

  function dashboardsBaseUrl() {
    return (((runtimeConfig || {}).opensearch || {}).dashboards_url || "").replace(/\/$/, "");
  }

  function discoverSavedSearchUrl(objectId) {
    const base = dashboardsBaseUrl();
    if (!base || !objectId) return "";
    return `${base}/app/discover#/view/${encodeURIComponent(objectId)}`;
  }

  function dashboardsViewUrl(objectId) {
    const base = dashboardsBaseUrl();
    if (!base || !objectId) return "";
    return `${base}/app/dashboards#/view/${encodeURIComponent(objectId)}`;
  }

  function buildIncidentActionLinks(item) {
    const dashboards = ((item.links || {}).dashboards) || {};
    return {
      runRca: item.investigation_supported ? buildInvestigationHref(item) : "",
      openLogs: dashboards.logs || discoverSavedSearchUrl("search-k8s-logs-recent-errors"),
      openEvents: dashboards.events || discoverSavedSearchUrl("search-k8s-events-warnings"),
      openDashboard: dashboards.overview_dashboard || dashboardsViewUrl("dashboard-auto-inspection-overview"),
    };
  }

  function renderIncidentAction(label, href, kind) {
    if (!href) {
      return `<span class="incident-button disabled ${escapeHtml(kind || "")}">${escapeHtml(label)}</span>`;
    }
    const external = !href.startsWith("?");
    return `<a class="incident-button ${escapeHtml(kind || "")}" href="${escapeHtml(href)}"${external ? ' target="_blank" rel="noreferrer"' : ""}>${escapeHtml(label)}</a>`;
  }

  function renderInvestigationTargets(items) {
    renderSimpleList(
      "investigation-targets",
      items,
      (item) => {
        const supported = Boolean(item.investigation_supported);
        const href = supported ? buildInvestigationHref(item) : "";
        const title = item.namespace
          ? `${item.namespace}/${item.pod || item.workload_name || "-"}`
          : (item.instance || item.workload_name || item.pod || "-");
        const content = `
          <span>
            <strong>${escapeHtml(title)}</strong><br />
            <small>${escapeHtml(item.latest_summary || item.recommended_reason || "暂无摘要")}</small><br />
            <small>${escapeHtml(renderTargetMeta(item) || "暂无元信息")}</small>
            ${item.runbook_title ? `<br /><small>Runbook: ${escapeHtml(item.runbook_title)}</small>` : ""}
            ${item.mapped_by ? `<br /><small>Mapped by: ${escapeHtml(item.mapped_by)}</small>` : ""}
          </span>
          <small>${escapeHtml(String(item.count || 0))} 次${supported ? "" : " · 仅展示"}</small>
        `;
        if (!supported) {
          return `<div class="item-card muted ${riskClass(item)}">${content}</div>`;
        }
        return `<a class="${riskClass(item)}" href="${href}">${content}</a>`;
      },
      "暂无推荐对象"
    );
  }

  function renderCurrentIncidents(items) {
    renderSimpleList(
      "current-incidents",
      items,
      (item) => {
        const title = item.namespace
          ? `${item.namespace}/${item.pod || item.workload_name || "-"}`
          : (item.instance || item.event_key || "-");
        const links = buildIncidentActionLinks(item);
        const risk = item.final_risk_level || item.risk_level || "-";
        const summary = item.runbook && item.runbook.title
          ? item.runbook.title
          : (item.dominant_risk || "未知风险");
        const mappingMeta = [];
        if (item.mapped_by) mappingMeta.push(`映射方式 ${item.mapped_by}`);
        if (item.service) mappingMeta.push(`服务 ${item.service}`);
        if (item.node) mappingMeta.push(`节点 ${item.node}`);
        return `
          <div class="item-card incident-card ${riskClass(item)}${item.investigation_supported ? "" : " muted"}">
            <span class="incident-body">
              <strong>${escapeHtml(title)}</strong>
              <small>${escapeHtml(summary)}</small>
              <small class="incident-meta">${escapeHtml((item.signals || []).join(", ") || "无信号")} · ${escapeHtml(risk)} · ${escapeHtml(item.lifecycle || "-")}</small>
              ${mappingMeta.length ? `<small class="incident-meta">${escapeHtml(mappingMeta.join(" · "))}</small>` : ""}
            </span>
            <span class="incident-actions">
              ${renderIncidentAction("Run RCA", links.runRca, "primary")}
              ${renderIncidentAction("Open Logs", links.openLogs, "secondary")}
              ${renderIncidentAction("Open Events", links.openEvents, "secondary")}
              ${renderIncidentAction("Open Dashboard", links.openDashboard, "secondary")}
            </span>
          </div>
        `;
      },
      "暂无高风险事件"
    );
  }

  function renderPodCards(pods) {
    const node = byId("pod-cards");
    const list = Array.isArray(pods) ? pods : [];
    if (!node) return;
    if (!list.length) {
      node.innerHTML = '<div class="empty-state">暂无 Pod 快照</div>';
      return;
    }
    node.innerHTML = list.map((pod) => `
      <div class="pod-card">
        <h3>${escapeHtml(pod.namespace || "-")}/${escapeHtml(pod.name || "-")}</h3>
        <div class="pod-meta">
          <span class="tag">Phase ${escapeHtml(pod.phase || "-")}</span>
          <span class="tag">Node ${escapeHtml(pod.node || "-")}</span>
          <span class="tag">Owner ${escapeHtml(pod.owner_kind || "-")}/${escapeHtml(pod.owner_name || "-")}</span>
        </div>
        ${(pod.containers || []).map((container) => `
          <div class="container-card">
            <h4>${escapeHtml(container.name || "container")}</h4>
            <div class="container-meta">
              <span class="tag">Restart ${escapeHtml(container.restart_count ?? "-")}</span>
              <span class="tag">State ${escapeHtml((container.state || {}).kind || "-")}</span>
              <span class="tag">Reason ${escapeHtml((container.state || {}).reason || (container.last_terminated || {}).reason || "-")}</span>
              <span class="tag">Exit ${escapeHtml((container.last_terminated || {}).exit_code ?? "-")}</span>
            </div>
            <div class="evidence-block"><strong>Resources</strong> requests=${escapeHtml(JSON.stringify((container.resources || {}).requests || {}))} limits=${escapeHtml(JSON.stringify((container.resources || {}).limits || {}))}</div>
          </div>
        `).join("")}
      </div>
    `).join("");
  }

  function formatNumber(value, digits) {
    const num = Number(value);
    if (!Number.isFinite(num)) return "-";
    return num.toFixed(digits ?? 0);
  }

  function formatBytes(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return "-";
    const units = ["B", "KiB", "MiB", "GiB", "TiB"];
    let current = num;
    let index = 0;
    while (current >= 1024 && index < units.length - 1) {
      current /= 1024;
      index += 1;
    }
    return `${current.toFixed(current >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
  }

  function formatCores(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return "-";
    return `${num.toFixed(num >= 1 ? 2 : 3)} cores`;
  }

  function topReason(value) {
    if (!value || typeof value !== "object") return "-";
    const entries = Object.entries(value).sort((a, b) => Number(b[1] || 0) - Number(a[1] || 0));
    if (!entries.length) return "-";
    return `${entries[0][0]} (${formatNumber(entries[0][1], 0)})`;
  }

  function fallbackContainerSignal(targetPods, field) {
    const pods = Array.isArray(targetPods) ? targetPods : [];
    for (const pod of pods) {
      for (const container of pod.containers || []) {
        if (field === "waiting_reason" && container.state && container.state.reason) {
          return container.state.reason;
        }
        if (field === "last_terminated_reason" && container.last_terminated && container.last_terminated.reason) {
          return container.last_terminated.reason;
        }
      }
    }
    return "-";
  }

  function signalCard(label, value, tone, detail) {
    return `
      <div class="signal-card ${escapeHtml(tone || "")}">
        <div class="signal-label">${escapeHtml(label)}</div>
        <div class="signal-value">${escapeHtml(value || "-")}</div>
        <div class="signal-detail">${escapeHtml(detail || "")}</div>
      </div>
    `;
  }

  function renderSignalSummary(prometheusContext, targetPods) {
    const node = byId("signal-summary-grid");
    const pods = (prometheusContext && prometheusContext.pods) || [];
    const primary = pods[0] || {};
    if (!node) return;

    const restartTotal = primary.restart_total;
    const restartIncrease = primary.restart_increase;
    const waitingReason = topReason(primary.waiting_reasons);
    const waitingFallback = fallbackContainerSignal(targetPods, "waiting_reason");
    const terminatedReason = topReason(primary.last_terminated_reasons);
    const terminatedFallback = fallbackContainerSignal(targetPods, "last_terminated_reason");
    const effectiveWaiting = waitingReason && waitingReason !== "-" ? waitingReason : waitingFallback;
    const effectiveTerminated = terminatedReason && terminatedReason !== "-" ? terminatedReason : terminatedFallback;
    const memPair = `${formatBytes(primary.memory_request_bytes)} / ${formatBytes(primary.memory_limit_bytes)}`;
    const cpuPair = `${formatCores(primary.cpu_request_cores)} / ${formatCores(primary.cpu_limit_cores)}`;
    const waitingTone = String(effectiveWaiting).includes("CrashLoopBackOff") ? "critical" : "warn";
    const terminatedTone = String(effectiveTerminated).includes("OOMKilled") ? "critical" : "warn";
    const restartTone = Number(restartIncrease || 0) >= 5 ? "critical" : (Number(restartIncrease || 0) > 0 ? "warn" : "ok");

    node.innerHTML = [
      signalCard("Restart Total", formatNumber(restartTotal, 0), Number(restartTotal || 0) >= 100 ? "critical" : "warn", "累计重启次数"),
      signalCard("Restart Increase", formatNumber(restartIncrease, 1), restartTone, "调查时间窗内增量"),
      signalCard("Waiting Reason", effectiveWaiting || "-", waitingTone, "来自 Prometheus / Pod 状态"),
      signalCard("Last Terminated", effectiveTerminated || "-", terminatedTone, "最近终止原因"),
      signalCard("Memory Req / Limit", memPair, String(effectiveTerminated).includes("OOMKilled") ? "critical" : "ok", "Pod 内存请求 / 限额"),
      signalCard("CPU Req / Limit", cpuPair, "ok", "Pod CPU 请求 / 限额"),
    ].join("");
  }

  function renderPrometheusCards(context) {
    const node = byId("prometheus-cards");
    const items = (context && context.pods) || [];
    if (!node) return;
    if (!items.length) {
      node.innerHTML = '<div class="empty-state">这次调查没有拿到可用的 Pod 维度 Prometheus 数据。</div>';
      return;
    }
    node.innerHTML = items.map((item) => `
      <div class="metric-card">
        <h3>${escapeHtml(item.namespace)}/${escapeHtml(item.pod)}</h3>
        <p>CPU: ${escapeHtml(item.cpu_cores ?? "-")}</p>
        <p>Memory: ${escapeHtml(item.memory_working_set_bytes ?? "-")}</p>
        <p>Restart Total: ${escapeHtml(item.restart_total ?? "-")}</p>
        <p>Restart Increase: ${escapeHtml(item.restart_increase ?? "-")}</p>
        <p>Waiting Reasons: ${escapeHtml(JSON.stringify(item.waiting_reasons || {}))}</p>
      </div>
    `).join("");
  }

  function highlightLogMessage(message) {
    const text = String(message ?? "");
    if (!text) return "-";
    const pattern = /\b(ERROR|ERR|FATAL|PANIC|CRITICAL|WARN|WARNING|EXCEPTION|OOMKILLED|CRASHLOOPBACKOFF|FAILED|FAILURE)\b/gi;
    let lastIndex = 0;
    let html = "";
    let match;

    while ((match = pattern.exec(text)) !== null) {
      const token = match[0];
      const start = match.index;
      const end = start + token.length;
      if (start > lastIndex) {
        html += escapeHtml(text.slice(lastIndex, start));
      }
      const upper = token.toUpperCase();
      const tone = /ERROR|ERR|FATAL|PANIC|CRITICAL|EXCEPTION|OOMKILLED|CRASHLOOPBACKOFF|FAILED|FAILURE/.test(upper)
        ? "critical"
        : "warn";
      html += `<span class="log-token ${tone}">${escapeHtml(token)}</span>`;
      lastIndex = end;
    }

    if (lastIndex < text.length) {
      html += escapeHtml(text.slice(lastIndex));
    }

    return html || "-";
  }

  function renderLogMessageCell(row) {
    const severity = String(row.severity || "").toUpperCase();
    const logger = row.logger || "";
    const parts = [];
    const prefixParts = [];

    if (row.timestamp) prefixParts.push(row.timestamp);
    if (row.pod) prefixParts.push(row.pod);
    if (row.mode) prefixParts.push(row.mode);

    if (severity) {
      const tone = /ERROR|ERR|FATAL|PANIC|CRITICAL/.test(severity) ? "critical" : /WARN/.test(severity) ? "warn" : "info";
      parts.push(`<span class="log-badge ${tone}">${escapeHtml(severity)}</span>`);
    }

    if (logger) {
      parts.push(`<span class="log-logger">${escapeHtml(logger)}</span>`);
    }

    const prefix = prefixParts.length ? `<div class="log-prefix">${escapeHtml(prefixParts.join("  ·  "))}</div>` : "";
    const meta = parts.length ? `<div class="log-meta">${parts.join("")}</div>` : "";
    return `
      <div class="log-cell">
        ${prefix}
        ${meta}
        <pre class="log-line">${highlightLogMessage(row.message)}</pre>
      </div>
    `;
  }

  function renderTable(tableId, columns, rows) {
    const table = byId(tableId);
    const items = Array.isArray(rows) ? rows : [];
    if (!table) return;
    const thead = `<thead><tr>${columns.map((col) => `<th>${escapeHtml(col.label)}</th>`).join("")}</tr></thead>`;
    if (!items.length) {
      table.innerHTML = `${thead}<tbody><tr><td colspan="${columns.length}" class="empty-state">暂无数据</td></tr></tbody>`;
      return;
    }
    const tbody = `<tbody>${items.map((row) => `<tr>${columns.map((col) => {
      const content = typeof col.render === "function"
        ? col.render(row)
        : escapeHtml(col.getter(row) || "-");
      const className = col.className ? ` class="${escapeHtml(col.className)}"` : "";
      return `<td${className}>${content}</td>`;
    }).join("")}</tr>`).join("")}</tbody>`;
    table.innerHTML = `${thead}${tbody}`;
  }

  function renderInvestigation(payload) {
    const analysis = payload.analysis || {};
    const evidence = payload.evidence || {};
    const target = payload.target || {};
    const storage = payload.storage || {};
    const dashboardLinks = ((payload.links || {}).dashboards) || {};
    const request = payload.request || {};

    setText("investigation-id", payload.investigation_id);
    setText("generated-at", payload.generated_at);
    setText("evidence-sources", `${evidence.logs_source || "-"} / ${evidence.events_source || "-"}`);
    setText("evidence-counts", `logs ${((evidence.logs || []).length)} · events ${((evidence.events || []).length)}`);
    setText("target-object", `${request.namespace || "-"}/${request.pod || request.workload_name || "-"}`);
    setText("target-meta", `pods ${((target.pod_names || []).length)} · cluster ${request.cluster || "-"}`);

    const storageSummary = [];
    if (storage.opensearch && storage.opensearch.indexed) storageSummary.push("OpenSearch");
    if (storage.hot_store && storage.hot_store.stored) storageSummary.push(`热存储:${storage.hot_store.driver}`);
    if (storage.cold_store && storage.cold_store.stored) storageSummary.push(`冷归档:${storage.cold_store.driver}`);
    setText("storage-status", storageSummary.join(" · ") || "仅本地文件");
    setText("storage-meta", storage.local_path || "-");

    byId("summary-text").textContent = analysis.summary || "暂无结论";
    byId("analysis-prompt").textContent = payload.analysis_prompt || "未启用 AI 或未记录提示词。";

    renderDashboardsLinks(dashboardLinks);
    renderRootCauses(analysis.root_cause || []);
    renderTextList("actions-list", analysis.actions || [], "暂无建议动作");
    renderTextList("human-check-list", analysis.need_human_check || [], "暂无待确认项");
    renderTextList("timeline-list", analysis.timeline || [], "暂无关键时间线");
    renderSignalSummary(evidence.prometheus || {}, target.pods || []);
    renderPodCards(target.pods || []);
    renderPrometheusCards(evidence.prometheus || {});

    renderTable(
      "logs-table",
      [
        { label: "Log", render: renderLogMessageCell, className: "log-message-column" },
      ],
      evidence.logs || []
    );

    renderTable(
      "events-table",
      [
        { label: "Timestamp", getter: (row) => row.timestamp },
        { label: "Type", getter: (row) => row.type },
        { label: "Reason", getter: (row) => row.reason },
        { label: "Object", getter: (row) => `${row.object_kind || "-"} / ${row.object_name || "-"}` },
        { label: "Message", getter: (row) => row.message },
      ],
      evidence.events || []
    );

    const params = new URLSearchParams(location.search);
    params.set("investigation_id", payload.investigation_id);
    params.set("namespace", request.namespace || "");
    if (request.pod) params.set("pod", request.pod);
    if (request.workload_name) params.set("workload_name", request.workload_name);
    history.replaceState({}, "", `${location.pathname}?${params.toString()}`);
  }

  async function loadRecentLists() {
    await loadRuntimeConfig();
    const [recent, targets, incidents] = await Promise.all([
      requestJson("/api/investigations?limit=12"),
      requestJson("/api/investigation-targets?limit=12"),
      requestJson("/api/incidents/list?limit=12"),
    ]);
    renderRecentInvestigations(recent.items || []);
    renderInvestigationTargets(targets.items || []);
    renderCurrentIncidents(incidents.items || []);
  }

  async function loadInvestigationById(id) {
    setStatus("加载调查结果中...", "running");
    const payload = await requestJson(`/api/investigations/${encodeURIComponent(id)}`);
    renderInvestigation(payload);
    setStatus("调查结果已加载", "running");
  }

  async function loadLatestInvestigation() {
    setStatus("加载最近一次调查中...", "running");
    const payload = await requestJson("/api/investigations/latest");
    renderInvestigation(payload);
    setStatus("已加载最近一次调查", "running");
  }

  async function runInvestigation() {
    saveFormState();
    setStatus("调查执行中...", "running");
    byId("run-button").disabled = true;
    try {
      const payload = {
        namespace: byId("namespace").value.trim(),
        pod: byId("pod").value.trim(),
        workload_name: byId("workload_name").value.trim(),
        range_hours: Number(byId("range_hours").value || 6),
        question: byId("question").value.trim(),
        query: byId("query").value.trim(),
        use_ai: byId("use_ai").checked,
      };
      const result = await requestJson("/api/investigate", {
        method: "POST",
        body: JSON.stringify(payload),
      });
      renderInvestigation(result);
      await loadRecentLists();
      setStatus("调查完成", "running");
    } catch (error) {
      setStatus(`失败：${error.message}`, "error");
    } finally {
      byId("run-button").disabled = false;
    }
  }

  function bindEvents() {
    byId("investigation-form").addEventListener("submit", (event) => {
      event.preventDefault();
      runInvestigation();
    });
    byId("load-latest-button").addEventListener("click", () => {
      loadLatestInvestigation().catch((error) => setStatus(`失败：${error.message}`, "error"));
    });
    ["namespace", "pod", "workload_name", "range_hours", "question", "query", "use_ai"].forEach((id) => {
      byId(id).addEventListener("change", saveFormState);
      byId(id).addEventListener("input", saveFormState);
    });
  }

  async function init() {
    loadFormState();
    bindEvents();
    await loadRecentLists().catch(() => {});
    const params = new URLSearchParams(location.search);
    const investigationId = params.get("investigation_id");
    try {
      if (investigationId) {
        await loadInvestigationById(investigationId);
        return;
      }
      await loadLatestInvestigation();
    } catch (error) {
      setStatus(`待执行：${error.message}`, "error");
    }
  }

  init();
})();
