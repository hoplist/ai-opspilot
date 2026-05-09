(function (global) {
  const METRIC_LABEL = {
    all: "全部",
    disk: "Disk",
    cpu: "CPU",
    mem: "Mem",
    pod: "Pod",
    pod_cpu: "Pod CPU",
    pod_mem: "Pod Mem",
  };

  const ZONE_LABEL = {
    alert: "告警",
    watch: "关注",
    safe: "安全",
    unknown: "未知",
  };

  const POD_FLAGS = [
    { key: "abnormalOnly", label: "仅异常" },
    { key: "notrunning", label: "NotRunning" },
    { key: "waiting", label: "Waiting" },
    { key: "terminated", label: "Terminated" },
    { key: "oom", label: "OOM" },
    { key: "restart", label: "重启>0" },
    { key: "throttle", label: "Throttle>20%" },
    { key: "mem", label: "Mem>80%" },
    { key: "fs", label: "FS>80%" },
  ];

  const DEFAULT_LINK_TEMPLATES = {
    logs: "",
    events: "",
    yaml: "",
    shell: "",
    metrics: "",
  };

  const DEFAULT_NOTIFICATION_SETTINGS = {
    enabled: false,
    webhook_url: "",
    webhook_type: "generic",
    targets: [],
    state_file: "data/pod_restart_notify_state.json",
  };

  const DEFAULT_RESOURCE_PREFS = {
    metric: "all",
    cluster: "all",
    search: "",
    filters: {
      zone: "all",
      namespace: "all",
      node: "all",
      podFlags: {
        abnormalOnly: false,
        notrunning: false,
        waiting: false,
        terminated: false,
        oom: false,
        restart: false,
        throttle: false,
        mem: false,
        fs: false,
      },
    },
    columns: {
      combined: [
        "metric",
        "cluster",
        "instance",
        "group",
        "usage",
        "usage_zone",
        "remaining",
        "remaining_zone",
        "peak",
        "trend",
      ],
      pod: [
        "cluster",
        "namespace",
        "pod",
        "phase",
        "node",
        "ready",
        "restarts",
        "cpu",
        "cpu_trend_short",
        "mem",
        "mem_trend_short",
        "signals",
        "terminated",
        "last_terminated",
        "actions",
      ],
    },
    playback: {
      enabled: false,
      endTs: null,
      rangeHours: 24,
    },
    links: { ...DEFAULT_LINK_TEMPLATES },
  };

  const DEFAULT_ALERT_PREFS = {
    category: "all",
    search: "",
  };

  const VIEW_LABELS = {
    default: {
      riskTitle: "清理优先 Top 5",
      riskSub: "按当前使用率排序，优先处理告警区",
      surplusTitle: "可承载 Top 5",
      surplusSub: "按剩余率排序，优先看安全区",
      usageTitle: "当前与剩余",
      usageSub: "统一表格展示当前压力、余量和趋势",
      issuesTitle: "潜在问题清单",
      issuesSub: "告警和关注项，按风险优先级排序",
      capacityTitle: "资源规划清单",
      capacitySub: "安全区资源，按余量从高到低排序",
    },
    pod: {
      riskTitle: "高风险 Pod Top 5",
      riskSub: "按异常严重度和使用率排序",
      surplusTitle: "低风险 Pod Top 5",
      surplusSub: "按余量排序，适合承载流量",
      usageTitle: "Pod 画像",
      usageSub: "聚合展示 Pod 状态、重启、CPU、内存和排障入口",
      issuesTitle: "异常 Pod 清单",
      issuesSub: "状态异常、重启、限流、OOM 等问题优先显示",
      capacityTitle: "稳定 Pod 清单",
      capacitySub: "运行稳定、余量充足的 Pod",
    },
  };

  function byId(id) {
    return document.getElementById(id);
  }

  function deepClone(value) {
    return JSON.parse(JSON.stringify(value));
  }

  function escapeHtml(value) {
    return String(value ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function toNumber(value) {
    const num = Number(value);
    return Number.isFinite(num) ? num : null;
  }

  function fmtPct(value) {
    const num = toNumber(value);
    if (num === null) return "-";
    return `${(num * 100).toFixed(1)}%`;
  }

  function fmtPctDelta(value) {
    const num = toNumber(value);
    if (num === null) return "-";
    const prefix = num > 0 ? "+" : "";
    return `${prefix}${(num * 100).toFixed(1)}pp`;
  }

  function fmtBytes(value) {
    const num = toNumber(value);
    if (num === null) return "-";
    const units = ["B", "KB", "MB", "GB", "TB", "PB"];
    let next = Math.abs(num);
    let index = 0;
    while (next >= 1024 && index < units.length - 1) {
      next /= 1024;
      index += 1;
    }
    return `${num < 0 ? "-" : ""}${next.toFixed(1)}${units[index]}`;
  }

  function fmtRate(value, suffix) {
    const num = toNumber(value);
    if (num === null) return "-";
    return `${num.toFixed(3)}${suffix || "/s"}`;
  }

  function fmtValue(value) {
    if (value === null || value === undefined || value === "") return "-";
    return String(value);
  }

  function fmtRestart(value) {
    const num = toNumber(value);
    if (num === null) return "-";
    return num.toFixed(1);
  }

  function fmt\u5c31\u7eea(count, total, ratio) {
    const safeCount = toNumber(count);
    const safeTotal = toNumber(total);
    if (safeTotal !== null) return `${safeCount === null ? 0 : safeCount}/${safeTotal}`;
    return fmtPct(ratio);
  }

  function fmtHours(value) {
    const num = toNumber(value);
    if (num === null) return "0.00";
    return num.toFixed(2);
  }

  function fmtTime(value) {
    const num = toNumber(value);
    if (num === null || num <= 0) return "-";
    const ms = num > 1e12 ? num : num * 1000;
    const date = new Date(ms);
    if (Number.isNaN(date.getTime())) return "-";
    return date.toLocaleString();
  }

  function fmtDateTimeInput(ts) {
    const num = toNumber(ts);
    if (num === null || num <= 0) return "";
    const date = new Date(num * 1000);
    if (Number.isNaN(date.getTime())) return "";
    const pad = (value) => String(value).padStart(2, "0");
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(
      date.getHours()
    )}:${pad(date.getMinutes())}`;
  }

  function fmtTrend(value) {
    if (!value || value === "-") return "-";
    if (value === "上升") return "↑ 上升";
    if (value === "下降") return "↓ 下降";
    if (value === "持平") return "→ 持平";
    return String(value);
  }

  function mergeDeep(base, extra) {
    if (!extra || typeof extra !== "object") return deepClone(base);
    const output = Array.isArray(base) ? [...base] : { ...base };
    Object.keys(extra).forEach((key) => {
      const value = extra[key];
      if (Array.isArray(value)) output[key] = [...value];
      else if (value && typeof value === "object") output[key] = mergeDeep(base[key] || {}, value);
      else output[key] = value;
    });
    return output;
  }

  function loadPrefs(key, defaults) {
    try {
      const raw = localStorage.getItem(key);
      if (!raw) return mergeDeep(defaults, {});
      return mergeDeep(defaults, JSON.parse(raw));
    } catch (err) {
      return mergeDeep(defaults, {});
    }
  }

  function savePrefs(key, value) {
    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch (err) {
      return;
    }
  }

  function parseGroup(group) {
    if (!group || group === "-") return {};
    const output = {};
    String(group)
      .split(" ")
      .map((part) => part.trim())
      .filter(Boolean)
      .forEach((part) => {
        const index = part.indexOf("=");
        if (index <= 0) return;
        output[part.slice(0, index)] = part.slice(index + 1);
      });
    return output;
  }

  function getCluster(item) {
    if (item.cluster && item.cluster !== "-") return item.cluster;
    const parsed = parseGroup(item.group);
    return parsed.cluster || "-";
  }

  function getGroupSummary(item) {
    const parsed = parseGroup(item.group);
    const summary = Object.entries(parsed)
      .filter(([key]) => key !== "cluster")
      .map(([key, value]) => `${key}=${value}`)
      .join(" ");
    return summary || "-";
  }

  function splitPodInstance(instance) {
    if (!instance) return { namespace: "-", pod: "-" };
    const text = String(instance);
    if (!text.includes("/")) return { namespace: "-", pod: text || "-" };
    const parts = text.split("/");
    return { namespace: parts[0] || "-", pod: parts.slice(1).join("/") || "-" };
  }

  function uniqueSorted(values) {
    return Array.from(new Set(values.filter(Boolean))).sort((a, b) => String(a).localeCompare(String(b)));
  }

  function zoneRank(zone) {
    if (zone === "alert") return 0;
    if (zone === "watch") return 1;
    if (zone === "safe") return 2;
    return 3;
  }

  function zoneBadge(zone) {
    const safeZone = ZONE_LABEL[zone] ? zone : "unknown";
    return `<span class="badge ${safeZone}">${ZONE_LABEL[safeZone]}</span>`;
  }

  function moreSevereZone(left, right) {
    return zoneRank(left) <= zoneRank(right) ? left : right;
  }

  function applyTemplate(template, vars) {
    let output = String(template || "");
    Object.keys(vars)
      .sort((a, b) => b.length - a.length)
      .forEach((key) => {
        output = output.split(`{${key}}`).join(String(vars[key] ?? ""));
      });
    return output;
  }

  function copyText(text) {
    if (!text) return Promise.resolve(false);
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text).then(() => true).catch(() => false);
    }
    return new Promise((resolve) => {
      try {
        const node = document.createElement("textarea");
        node.value = text;
        node.setAttribute("readonly", "readonly");
        node.style.position = "fixed";
        node.style.opacity = "0";
        document.body.appendChild(node);
        node.select();
        const success = document.execCommand("copy");
        document.body.removeChild(node);
        resolve(success);
      } catch (err) {
        resolve(false);
      }
    });
  }

  function metricLabel(metric) {
    return METRIC_LABEL[metric] || fmtValue(metric);
  }

  function getThresholds(resources) {
    const usage = resources && resources.usage_thresholds ? resources.usage_thresholds : {};
    const remaining = resources && resources.remaining_thresholds ? resources.remaining_thresholds : {};
    return {
      usage: {
        alert: toNumber(usage.alert) ?? 0.8,
        watch: toNumber(usage.watch) ?? 0.7,
      },
      remaining: {
        alert: toNumber(remaining.alert) ?? 0.2,
        watch: toNumber(remaining.watch) ?? 0.3,
      },
    };
  }

  function getPodTrendRules(resources) {
    const trend = resources && resources.trend ? resources.trend : {};
    const rules = resources && resources.pod_trend_rules ? resources.pod_trend_rules : {};
    return {
      enabled: rules.enabled !== undefined ? !!rules.enabled : true,
      watchRatio: toNumber(rules.watch_ratio) ?? 1.5,
      alertRatio: toNumber(rules.alert_ratio) ?? 2.0,
      watchDelta: toNumber(rules.watch_delta) ?? 0.1,
      alertDelta: toNumber(rules.alert_delta) ?? 0.2,
      minCurrent: toNumber(rules.min_current) ?? 0.3,
      baselineFloor: toNumber(rules.baseline_floor) ?? 0.05,
      days: toNumber(rules.days) ?? toNumber(trend.days) ?? 14,
    };
  }

  function getPodShortTrendConfig(resources) {
    const shortTrend = resources && resources.pod_short_trend ? resources.pod_short_trend : {};
    return {
      windowMinutes: Math.max(5, Math.round(toNumber(shortTrend.window_minutes) ?? 30)),
      watchDelta: 0.03,
      alertDelta: 0.08,
      watchRatio: 1.2,
      alertRatio: 1.5,
      baselineFloor: 0.01,
    };
  }

  function buildPodShortTrend(currentValue, recentAvg, prevAvg, config) {
    const current = toNumber(currentValue);
    const recent = toNumber(recentAvg);
    const previous = toNumber(prevAvg);
    const compare = current !== null ? current : recent;
    if (compare === null || previous === null) {
      return {
        current,
        recent,
        previous,
        delta: null,
        ratio: null,
        score: 0,
        zone: "unknown",
        direction: "-",
      };
    }
    const baseline = Math.max(previous, toNumber(config && config.baselineFloor) ?? 0.01);
    const delta = compare - previous;
    const ratio = baseline > 0 ? compare / baseline : null;
    let zone = "safe";
    if (delta > 0) {
      if (delta >= config.alertDelta || (ratio !== null && ratio >= config.alertRatio)) zone = "alert";
      else if (delta >= config.watchDelta || (ratio !== null && ratio >= config.watchRatio)) zone = "watch";
    }
    if (delta === null) zone = "unknown";
    const direction = delta === null ? "-" : delta > 0.01 ? "↑" : delta < -0.01 ? "↓" : "→";
    return {
      current,
      recent,
      previous,
      delta,
      ratio,
      score: delta !== null && delta > 0 ? delta * 100 + Math.max((ratio || 1) - 1, 0) * 10 : 0,
      zone,
      direction,
    };
  }

  function buildPodShortTrendIssue(metricKey, label, trend, config) {
    if (!trend || !trend.zone || trend.zone === "safe" || trend.zone === "unknown" || trend.delta === null) return null;
    const minutes = Math.max(5, Math.round(toNumber(config && config.windowMinutes) ?? 30));
    const ratioText = trend.ratio !== null ? `${trend.ratio.toFixed(1)}x` : "-";
    return {
      key: `${metricKey}_short_rise`,
      severity: trend.zone,
      title: `${label} 短时上涨`,
      detail: `近${minutes}分钟 ${label} 变化 ${fmtPctDelta(trend.delta)}，当前 ${fmtPct(trend.current)}，前窗口均值 ${fmtPct(
        trend.previous
      )}，相对基线 ${ratioText}。`,
    };
  }

  function getPodShortTrendIssues(item, config) {
    return [
      buildPodShortTrendIssue("cpu", "CPU", item.cpu_short_trend, config),
      buildPodShortTrendIssue("mem", "内存", item.mem_short_trend, config),
    ].filter(Boolean);
  }

  function contextSignalToIssue(signal) {
    if (!signal) return null;
    const type = String(signal.type || "signal");
    const metric = type.indexOf("mem") >= 0 ? "Mem" : type.indexOf("cpu") >= 0 ? "CPU" : type;
    const delta = toNumber(signal.delta);
    const ratio = toNumber(signal.ratio);
    return {
      key: `context_${type}`,
      severity: signal.severity || "info",
      title: type === "cpu_short_trend" || type === "mem_short_trend" ? `${metric} short rise` : String(signal.message || type),
      detail:
        delta !== null
          ? `${metric} delta ${fmtPctDelta(delta)} / ${ratio !== null ? ratio.toFixed(1) : "-"}x`
          : String(signal.message || type),
    };
  }

  function renderPodSignals(item) {
    const signals = Array.isArray(item.context_signals) ? item.context_signals : [];
    if (!signals.length) return '<span class="muted">-</span>';
    return `<div class="signal-stack">${signals
      .slice(0, 3)
      .map((signal) => {
        const label =
          signal.type === "cpu_short_trend"
            ? "CPU↑"
            : signal.type === "mem_short_trend"
            ? "Mem↑"
            : String(signal.type || "signal");
        const delta = toNumber(signal.delta);
        const ratio = toNumber(signal.ratio);
        const suffix = delta !== null ? ` ${fmtPctDelta(delta)} ${ratio !== null ? ratio.toFixed(1) + "x" : ""}` : "";
        return `<span class="signal-pill ${escapeHtml(signal.severity || "info")}">${escapeHtml(label + suffix)}</span>`;
      })
      .join("")}</div>`;
  }

  function formatPodShortTrendCell(trend, config) {
    if (!trend || trend.delta === null) return '<div class="cell-multi"><span>-</span><span class="muted">短期基线不足</span></div>';
    const minutes = Math.max(5, Math.round(toNumber(config && config.windowMinutes) ?? 30));
    const deltaText = fmtPctDelta(trend.delta);
    const ratioText = trend.ratio !== null ? `${trend.ratio.toFixed(1)}x` : "-";
    return `<div class="cell-multi"><span>${zoneBadge(trend.zone)} ${escapeHtml(`${trend.direction} ${deltaText}`)}</span><span class="muted">${escapeHtml(
      `当前 ${fmtPct(trend.current)} · 前${minutes}m ${fmtPct(trend.previous)} · ${ratioText}`
    )}</span></div>`;
  }

  function buildPodTrendIssue(metricKey, label, currentValue, recentAvg, prevAvg, trendValue, rules) {
    if (!rules || !rules.enabled) return null;
    const current = toNumber(currentValue);
    const recent = toNumber(recentAvg);
    const previous = toNumber(prevAvg);
    if (recent === null || previous === null) return null;

    const baseline = Math.max(previous, toNumber(rules.baselineFloor) ?? 0.05);
    const now = current !== null ? current : recent;
    const recentDelta = recent - previous;
    const currentDelta = now - previous;
    const recentRatio = baseline > 0 ? recent / baseline : null;
    const currentRatio = baseline > 0 ? now / baseline : null;
    const peakDelta = Math.max(recentDelta, currentDelta);
    const peakRatio = Math.max(recentRatio || 0, currentRatio || 0);

    if (peakDelta <= 0) return null;
    if (Math.max(now, recent) < (toNumber(rules.minCurrent) ?? 0.3)) return null;

    let severity = null;
    if (peakDelta >= rules.alertDelta || peakRatio >= rules.alertRatio) severity = "alert";
    else if (peakDelta >= rules.watchDelta || peakRatio >= rules.watchRatio) severity = "watch";
    if (!severity) return null;

    const days = Math.max(1, Math.round(toNumber(rules.days) ?? 14));
    const direction = fmtTrend(trendValue);
    return {
      key: `${metricKey}_trend`,
      severity,
      title: `${label} 基线${severity === "alert" ? "快速抬升" : "持续上升"}`,
      detail: `当前 ${fmtPct(now)}，近${days}天均值 ${fmtPct(recent)}，前${days}天均值 ${fmtPct(previous)}，变化 ${fmtPctDelta(
        peakDelta
      )} / ${peakRatio.toFixed(1)}x${direction !== "-" ? `，趋势 ${direction}` : ""}。`,
    };
  }

  function getPodTrendIssues(item, trendRules) {
    const memCurrent = toNumber(item.mem_usage_ratio ?? item.mem_ratio);
    return [buildPodTrendIssue("cpu", "CPU", item.cpu_usage_ratio, item.cpu_recent_avg, item.cpu_prev_avg, item.cpu_trend, trendRules), buildPodTrendIssue("mem", "内存", memCurrent, item.mem_recent_avg, item.mem_prev_avg, item.mem_trend, trendRules)].filter(Boolean);
  }

  function zoneByUsage(value, thresholds) {
    const num = toNumber(value);
    if (num === null) return "unknown";
    if (num >= thresholds.alert) return "alert";
    if (num >= thresholds.watch) return "watch";
    return "safe";
  }

  function zoneByRemaining(value, thresholds) {
    const num = toNumber(value);
    if (num === null) return "unknown";
    if (num <= thresholds.alert) return "alert";
    if (num <= thresholds.watch) return "watch";
    return "safe";
  }

  function buildSearchBlob(item) {
    return [
      item.metric,
      item.cluster,
      item.instance,
      item.mountpoint,
      item.group,
      item.group_summary,
      item.namespace,
      item.pod,
      item.node_name,
      item.phase,
      item.waiting_reason,
      item.terminated_reason,
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
  }

  function getPodIssues(item, trendRules) {
    const issues = [];
    const phase = String(item.phase || item.pod_status || "").trim();
    const restarts = toNumber(item.restarts_total ?? item.restarts);
    const throttle = toNumber(item.cpu_throttle_ratio);
    const memRatio = toNumber(item.mem_ratio ?? item.mem_usage_ratio);
    const fsRatio = toNumber(item.fs_usage_ratio);
    const oomEvents = toNumber(item.oom_events_total);
    const readyCount = toNumber(item.ready_count);
    const readyTotal = toNumber(item.ready_total);

    if (phase && phase !== "Running") {
      issues.push({
        key: "phase",
        severity: "alert",
        title: `运行状态为 ${phase}`,
        detail: item.pending_reason || "需要先确认调度、镜像拉取或探针失败原因。",
      });
    }
    if (readyTotal !== null && readyCount !== null && readyCount < readyTotal) {
      issues.push({
        key: "ready",
        severity: readyCount === 0 ? "alert" : "watch",
        title: `\u5c31\u7eea ${readyCount}/${readyTotal}`,
        detail: "容器未全部就绪，建议先看探针和最近事件。",
      });
    }
    if (item.waiting_reason) {
      issues.push({
        key: "waiting",
        severity: "watch",
        title: `Waiting: ${item.waiting_reason}`,
        detail: "建议先看 Pod 事件和最近容器日志。",
      });
    }
    if (item.terminated_reason) {
      issues.push({
        key: "terminated",
        severity:
          item.terminated_reason === "OOMKilled" || toNumber(item.terminated_exitcode) === 137 ? "alert" : "watch",
        title: `最近终止: ${item.terminated_reason}`,
        detail:
          item.terminated_exitcode !== null && item.terminated_exitcode !== undefined
            ? `退出码 ${Math.round(Number(item.terminated_exitcode))}。`
            : "建议先看 previous 日志和事件。",
      });
    }
    if (item.oom || (oomEvents !== null && oomEvents > 0)) {
      issues.push({
        key: "oom",
        severity: "alert",
        title: "发生过 OOM",
        detail: "优先核对内存 working set、limit 和最近重启时间。",
      });
    }
    if (restarts !== null && restarts > 0) {
      issues.push({
        key: "restart",
        severity: restarts >= 3 ? "alert" : "watch",
        title: `重启次数 ${fmtRestart(restarts)}`,
        detail: "建议结合最近终止原因、事件和 previous 日志判断。",
      });
    }
    if (throttle !== null && throttle >= 0.2) {
      issues.push({
        key: "throttle",
        severity: throttle >= 0.4 ? "alert" : "watch",
        title: `CPU Throttle ${fmtPct(throttle)}`,
        detail: "CPU 限流明显，建议检查 limit 和 CFS 限流速率。",
      });
    }
    if (memRatio !== null && memRatio >= 0.8) {
      issues.push({
        key: "mem",
        severity: memRatio >= 0.9 ? "alert" : "watch",
        title: `内存使用 ${fmtPct(memRatio)}`,
        detail: "建议确认 working set 是否持续贴近 request/limit。",
      });
    }
    if (fsRatio !== null && fsRatio >= 0.8) {
      issues.push({
        key: "fs",
        severity: fsRatio >= 0.9 ? "alert" : "watch",
        title: `FS 使用 ${fmtPct(fsRatio)}`,
        detail: "建议清理临时文件，确认日志落盘和磁盘写入峰值。",
      });
    }
    getPodTrendIssues(item, trendRules).forEach((entry) => issues.push(entry));
    return issues;
  }

  function isPodAbnormal(item, trendRules) {
    return getPodIssues(item, trendRules).length > 0;
  }

  function inferPodZone(item, thresholds, trendRules) {
    let zone = zoneByUsage(item.usage_current, thresholds.usage);
    const issues = getPodIssues(item, trendRules);
    if (!issues.length) return zone === "unknown" ? "safe" : zone;
    const issueZone = issues.some((entry) => entry.severity === "alert") ? "alert" : "watch";
    if (zone === "unknown") return issueZone;
    return moreSevereZone(issueZone, zone);
  }

  function enrichItems(items, thresholds, trendRules) {
    const usage = thresholds.usage;
    const remaining = thresholds.remaining;
    return (items || []).map((item) => {
      const podInfo = item.metric === "pod" ? splitPodInstance(item.instance) : { namespace: "-", pod: "-" };
      const usageZone =
        item.metric === "pod"
          ? inferPodZone(item, thresholds, trendRules)
          : item.usage_zone || zoneByUsage(item.usage_current, usage);
      const remainingZone = item.remaining_zone || zoneByRemaining(item.remaining_current, remaining);
      const enriched = {
        ...item,
        metric: item.metric || item.key || "-",
        cluster: getCluster(item),
        group_summary: getGroupSummary(item),
        namespace: podInfo.namespace,
        pod: podInfo.pod,
        usage_zone: usageZone,
        remaining_zone: remainingZone,
      };
      enriched.search_blob = buildSearchBlob(enriched);
      return enriched;
    });
  }

  function buildPodKey(item) {
    return [item.cluster || "-", item.namespace || "-", item.pod || item.instance || "-"].join("|");
  }

  function buildPodItems(resources, thresholds, trendRules, shortTrendConfig) {
    const map = new Map();
    const signalsByPod = new Map();
    ((resources && resources.summary && resources.summary.top_signals) || []).forEach((signal) => {
      const key = String(signal.pod || "").trim();
      if (!key) return;
      const list = signalsByPod.get(key) || [];
      list.push(signal);
      signalsByPod.set(key, list);
    });
    const itemRows = enrichItems((resources && resources.items) || [], thresholds, trendRules).filter((item) => item.metric === "pod");
    const stateRows = enrichItems((resources && resources.pod_states) || [], thresholds, trendRules).map((item) => ({
      ...item,
      metric: "pod",
    }));

    function mergeRow(row) {
      const podInfo = row.namespace && row.pod ? { namespace: row.namespace, pod: row.pod } : splitPodInstance(row.instance);
      const merged = {
        ...row,
        cluster: row.cluster || getCluster(row),
        namespace: podInfo.namespace,
        pod: podInfo.pod,
        group_summary: row.group_summary || getGroupSummary(row),
        metric: "pod",
      };
      const key = buildPodKey(merged);
      const current = map.get(key) || {};
      const usageCurrent =
        toNumber(merged.usage_current) ??
        Math.max(toNumber(merged.cpu_usage_ratio) ?? -1, toNumber(merged.mem_usage_ratio) ?? -1);
      const usageMax =
        toNumber(merged.usage_max) ?? Math.max(toNumber(merged.cpu_max) ?? -1, toNumber(merged.mem_max) ?? -1);
      const next = {
        ...current,
        ...merged,
        usage_current: usageCurrent >= 0 ? usageCurrent : toNumber(current.usage_current),
        usage_max: usageMax >= 0 ? usageMax : toNumber(current.usage_max),
      };
      next.remaining_current =
        next.usage_current !== null && next.usage_current !== undefined ? 1 - next.usage_current : toNumber(next.remaining_current);
      next.remaining_min =
        next.usage_max !== null && next.usage_max !== undefined ? 1 - next.usage_max : toNumber(next.remaining_min);
      next.cluster = next.cluster || "-";
      next.group = next.group || "-";
      next.group_summary = next.group_summary || getGroupSummary(next);
      next.namespace = next.namespace || "-";
      next.pod = next.pod || "-";
      next.short_trend_config = shortTrendConfig;
      next.cpu_short_trend = buildPodShortTrend(
        next.cpu_usage_ratio,
        next.cpu_short_recent_avg,
        next.cpu_short_prev_avg,
        shortTrendConfig
      );
      next.mem_short_trend = buildPodShortTrend(
        next.mem_usage_ratio ?? next.mem_ratio,
        next.mem_short_recent_avg,
        next.mem_short_prev_avg,
        shortTrendConfig
      );
      next.cpu_surge_score = next.cpu_short_trend.score;
      next.mem_surge_score = next.mem_short_trend.score;
      next.surge_score = Math.max(next.cpu_surge_score || 0, next.mem_surge_score || 0);
      next.surge_metric = (next.cpu_surge_score || 0) >= (next.mem_surge_score || 0) ? "cpu" : "mem";
      next.context_signals = (signalsByPod.get(next.pod) || signalsByPod.get(next.instance) || []).slice().sort((left, right) => {
        const bySeverity = zoneRank(left.severity) - zoneRank(right.severity);
        if (bySeverity !== 0) return bySeverity;
        return Math.abs(toNumber(right.delta) || 0) - Math.abs(toNumber(left.delta) || 0);
      });
      next.search_blob = buildSearchBlob(next);
      next.pod_issues = getPodIssues(next, trendRules)
        .concat(getPodShortTrendIssues(next, shortTrendConfig))
        .concat(next.context_signals.map(contextSignalToIssue).filter(Boolean));
      next.pod_issue_keys = next.pod_issues.map((entry) => entry.key);
      next.is_abnormal = next.pod_issues.length > 0;
      const baseZone = zoneByUsage(next.usage_current, thresholds.usage);
      const issueZone = next.pod_issues.some((entry) => entry.severity === "alert")
        ? "alert"
        : next.pod_issues.length
        ? "watch"
        : baseZone === "unknown"
        ? "safe"
        : baseZone;
      next.usage_zone = baseZone === "unknown" ? issueZone : moreSevereZone(issueZone, baseZone);
      next.remaining_zone = zoneByRemaining(next.remaining_current, thresholds.remaining);
      map.set(key, next);
    }

    itemRows.forEach(mergeRow);
    stateRows.forEach(mergeRow);
    return Array.from(map.values()).sort((left, right) => {
      const bySurge = (toNumber(right.surge_score) || 0) - (toNumber(left.surge_score) || 0);
      if (bySurge !== 0) return bySurge;
      if ((left.cluster || "") !== (right.cluster || "")) return String(left.cluster || "").localeCompare(String(right.cluster || ""));
      if ((left.namespace || "") !== (right.namespace || "")) {
        return String(left.namespace || "").localeCompare(String(right.namespace || ""));
      }
      return String(left.pod || "").localeCompare(String(right.pod || ""));
    });
  }

  function ensureResourceOverlays() {
    if (!byId("filter-drawer-modal")) {
      document.body.insertAdjacentHTML(
        "beforeend",
        `
          <div class="modal" id="filter-drawer-modal">
            <div class="modal-backdrop" data-close-modal="filter-drawer-modal"></div>
            <div class="modal-card settings-card drawer-card">
              <div class="modal-header">
                <div class="modal-title">高级筛选</div>
                <button class="modal-close" type="button" data-close-modal="filter-drawer-modal">关闭</button>
              </div>
              <div class="modal-body" id="filter-drawer-content"></div>
            </div>
          </div>
        `
      );
    }
    if (!byId("settings-drawer-modal")) {
      document.body.insertAdjacentHTML(
        "beforeend",
        `
          <div class="modal" id="settings-drawer-modal">
            <div class="modal-backdrop" data-close-modal="settings-drawer-modal"></div>
            <div class="modal-card settings-card drawer-card">
              <div class="modal-header">
                <div class="modal-title">视图与集成设置</div>
                <button class="modal-close" type="button" data-close-modal="settings-drawer-modal">关闭</button>
              </div>
              <div class="modal-body" id="settings-drawer-content"></div>
            </div>
          </div>
        `
      );
    }
    if (!byId("pod-detail-modal")) {
      document.body.insertAdjacentHTML(
        "beforeend",
        `
          <div class="modal" id="pod-detail-modal">
            <div class="modal-backdrop" data-close-modal="pod-detail-modal"></div>
            <div class="modal-card">
              <div class="modal-header">
                <div class="modal-title" id="pod-detail-title">Pod 详情</div>
                <button class="modal-close" type="button" data-close-modal="pod-detail-modal">关闭</button>
              </div>
              <div class="modal-body" id="pod-detail-body"></div>
            </div>
          </div>
        `
      );
    }
  }

  function openModal(id) {
    const node = byId(id);
    if (node) node.classList.add("show");
  }

  function closeModal(id) {
    const node = byId(id);
    if (node) node.classList.remove("show");
  }

  function normalizeDashboardSettings(raw, fallbackLinks) {
    const source = raw && raw.settings ? raw.settings : raw || {};
    const notification = mergeDeep(DEFAULT_NOTIFICATION_SETTINGS, source.notification || {});
    const links = mergeDeep(DEFAULT_LINK_TEMPLATES, source.links || fallbackLinks || {});
    const metaSource = source.meta || {};
    return {
      notification: {
        ...notification,
        targets: Array.isArray(notification.targets)
          ? notification.targets.map((item) => String(item).trim()).filter(Boolean)
          : [],
      },
      links,
      meta: {
        config_path: String(metaSource.config_path || ""),
        prometheus: Array.isArray(metaSource.prometheus)
          ? metaSource.prometheus
              .filter((item) => item && typeof item === "object")
              .map((item) => ({
                cluster: String(item.cluster || "").trim(),
                url: String(item.url || "").trim(),
              }))
          : [],
      },
    };
  }

  function serializeSettings(settings) {
    return {
      notification: {
        enabled: !!(settings.notification && settings.notification.enabled),
        webhook_url: String((settings.notification && settings.notification.webhook_url) || "").trim(),
        webhook_type: String((settings.notification && settings.notification.webhook_type) || "generic").trim(),
        targets: Array.isArray(settings.notification && settings.notification.targets)
          ? settings.notification.targets.map((item) => String(item).trim()).filter(Boolean)
          : [],
        state_file: String((settings.notification && settings.notification.state_file) || "").trim(),
      },
      links: {
        logs: String((settings.links && settings.links.logs) || "").trim(),
        events: String((settings.links && settings.links.events) || "").trim(),
        yaml: String((settings.links && settings.links.yaml) || "").trim(),
        shell: String((settings.links && settings.links.shell) || "").trim(),
        metrics: String((settings.links && settings.links.metrics) || "").trim(),
      },
    };
  }

  function describeResourceItem(item) {
    if (item.metric === "alert") {
      return {
        title: fmtValue(item.instance),
        subtitle: fmtValue(item.cluster),
      };
    }
    if (item.metric === "pod") {
      return {
        title: `${fmtValue(item.namespace)}/${fmtValue(item.pod)}`,
        subtitle: `${fmtValue(item.cluster)} · ${fmtValue(item.node_name || "-")}`,
      };
    }
    if (item.metric === "disk") {
      return {
        title: `${fmtValue(item.instance)} · ${fmtValue(item.mountpoint)}`,
        subtitle: `${metricLabel(item.metric)} · ${fmtValue(item.cluster)}`,
      };
    }
    return {
      title: fmtValue(item.instance),
      subtitle: `${metricLabel(item.metric)} · ${fmtValue(item.cluster)}`,
    };
  }

  function buildQueryState(prefs, options) {
    if (!options.supportPlayback || !prefs.playback.enabled || !prefs.playback.endTs) {
      return { end: null, range_hours: null };
    }
    return {
      end: prefs.playback.endTs,
      range_hours: prefs.playback.rangeHours,
    };
  }

  function buildPodActionContext(item, settings) {
    const prom = Array.isArray(settings.meta.prometheus) ? settings.meta.prometheus : [];
    const matched = prom.find((entry) => entry.cluster === item.cluster && entry.url);
    const fallback = prom.find((entry) => entry.url);
    const prometheus = matched ? matched.url : fallback ? fallback.url : "";
    const expr = `sum by (pod) (container_memory_working_set_bytes{namespace="${item.namespace}",pod="${item.pod}"})`;
    return {
      cluster: item.cluster || "",
      namespace: item.namespace || "",
      pod: item.pod || "",
      node: item.node_name || "",
      instance: item.instance || "",
      metric: item.metric || "pod",
      prometheus,
      expr,
      cluster_url: encodeURIComponent(item.cluster || ""),
      namespace_url: encodeURIComponent(item.namespace || ""),
      pod_url: encodeURIComponent(item.pod || ""),
      node_url: encodeURIComponent(item.node_name || ""),
      instance_url: encodeURIComponent(item.instance || ""),
      prometheus_url: encodeURIComponent(prometheus),
      expr_url: encodeURIComponent(expr),
    };
  }

  function buildPodActions(item, settings) {
    const context = buildPodActionContext(item, settings);
    const promBase = context.prometheus ? String(context.prometheus).replace(/\/$/, "") : "";
    const metricsUrl = settings.links.metrics
      ? applyTemplate(settings.links.metrics, context)
      : promBase
      ? `${promBase}/graph?g0.expr=${encodeURIComponent(context.expr)}&g0.tab=1`
      : "";
    return [
      {
        label: "日志",
        url: settings.links.logs ? applyTemplate(settings.links.logs, context) : "",
        copy:
          `kubectl logs -n ${context.namespace} ${context.pod} --all-containers --tail=200\n` +
          `kubectl logs -n ${context.namespace} ${context.pod} --previous --all-containers --tail=200`,
      },
      {
        label: "事件",
        url: settings.links.events ? applyTemplate(settings.links.events, context) : "",
        copy: `kubectl get events -n ${context.namespace} --field-selector involvedObject.kind=Pod,involvedObject.name=${context.pod} --sort-by=.lastTimestamp`,
      },
      {
        label: "YAML",
        url: settings.links.yaml ? applyTemplate(settings.links.yaml, context) : "",
        copy: `kubectl get pod -n ${context.namespace} ${context.pod} -o yaml`,
      },
      {
        label: "Shell",
        url: settings.links.shell ? applyTemplate(settings.links.shell, context) : "",
        copy: `kubectl exec -n ${context.namespace} -it ${context.pod} -- sh`,
      },
      {
        label: "指标",
        url: metricsUrl,
        copy: `kubectl top pod -n ${context.namespace} ${context.pod}\nkubectl describe pod -n ${context.namespace} ${context.pod}`,
      },
    ];
  }

  function renderActionButton(action) {
    if (action.url) {
      return `<a class="detail-action" href="${escapeHtml(action.url)}" target="_blank" rel="noreferrer">${escapeHtml(
        action.label
      )}</a>`;
    }
    return `<button class="detail-action" type="button" data-copy="${escapeHtml(action.copy)}">${escapeHtml(
      action.label
    )}</button>`;
  }

  function buildPodBaselineRows(label, currentValue, recentAvg, prevAvg, trendValue, trendRules) {
    const current = toNumber(currentValue);
    const recent = toNumber(recentAvg);
    const previous = toNumber(prevAvg);
    const days = Math.max(1, Math.round(toNumber(trendRules && trendRules.days) ?? 14));
    const baseline = previous === null ? null : Math.max(previous, toNumber(trendRules && trendRules.baselineFloor) ?? 0.05);
    const compareValue = current !== null ? current : recent;
    const ratio = baseline !== null && compareValue !== null && baseline > 0 ? `${(compareValue / baseline).toFixed(1)}x` : "-";
    const change = previous !== null && compareValue !== null ? fmtPctDelta(compareValue - previous) : "-";
    const trendText = fmtTrend(trendValue);
    return `
      <div class="detail-item"><span>${escapeHtml(label)} 当前</span><span>${escapeHtml(fmtPct(compareValue))}</span></div>
      <div class="detail-item"><span>${escapeHtml(`${label} 近${days}天均值`)}</span><span>${escapeHtml(fmtPct(recent))}</span></div>
      <div class="detail-item"><span>${escapeHtml(`${label} 前${days}天均值`)}</span><span>${escapeHtml(fmtPct(previous))}</span></div>
      <div class="detail-item"><span>${escapeHtml(`${label} 相对基线`)}</span><span>${escapeHtml(`${change} / ${ratio} / ${trendText}`)}</span></div>
    `;
  }

  function renderHighlightList(target, items, emptyText) {
    if (!target) return;
    if (!items.length) {
      target.innerHTML = `<div class="muted">${escapeHtml(emptyText)}</div>`;
      return;
    }
    target.innerHTML = items
      .map((item) => {
        const desc = {
          ...describeResourceItem(item),
          title: item.display_title || describeResourceItem(item).title,
          subtitle: item.display_subtitle || describeResourceItem(item).subtitle,
        };
        const value = item.display_value || (item.metric === "alert" ? `${fmtHours(item.usage_current)}h` : fmtPct(item.usage_current));
        return `
          <div class="list-item">
            <div class="list-main">
              <div class="list-title">${escapeHtml(desc.title)}</div>
              <div class="list-sub">${escapeHtml(desc.subtitle)}</div>
            </div>
            <div class="list-side">
              <div class="list-value">${escapeHtml(value)}</div>
              ${item.display_meta ? `<div class="list-sub">${escapeHtml(item.display_meta)}</div>` : ""}
              ${item.metric === "pod" ? `<button class="detail-action" type="button" data-open-pod="${escapeHtml(buildPodKey(item))}">详情</button>` : ""}
            </div>
          </div>
        `;
      })
      .join("");
  }

  function resolveResourceEmptyText(state, fallback, loadingText) {
    if (state && state.loading) return loadingText || "\u8d44\u6e90\u6570\u636e\u52a0\u8f7d\u4e2d...";
    if (state && state.flash && state.flash.level === "error" && !state.resources) {
      return state.flash.text || fallback;
    }
    return fallback;
  }

  function renderKpis(target, derived, state) {
    if (!target) return;
    const loading = !!(state && state.loading && !derived.rows.length);
    const loadError = state && state.flash && state.flash.level === "error" && !derived.rows.length ? state.flash.text : "";
    const alerts = derived.rows.filter((item) => item.usage_zone === "alert").length;
    const watches = derived.rows.filter((item) => item.usage_zone === "watch").length;
    const restartPods = derived.podRows.filter((item) => (toNumber(item.restarts_total ?? item.restarts) || 0) > 0).length;
    const cpuRisers = derived.podRows.filter((item) => (item.cpu_surge_score || 0) > 0).length;
    const memRisers = derived.podRows.filter((item) => (item.mem_surge_score || 0) > 0).length;
    const worst = derived.rows[0];
    const cards =
      derived.viewMode === "pod"
        ? [
            {
              title: "筛选后 Pod",
              value: loading ? "-" : String(derived.rows.length),
              sub: loading ? "资源数据加载中..." : loadError || "当前筛选条件命中的 Pod 数量",
            },
            {
              title: "告警",
              value: loading ? "-" : String(alerts),
              sub: loading ? "等待资源结果返回" : "风险区资源或异常 Pod",
            },
            {
              title: `CPU上涨(${derived.shortTrendConfig.windowMinutes}m)`,
              value: loading ? "-" : String(cpuRisers),
              sub: loading ? "加载统计中..." : "短期 CPU 持续抬升的 Pod 数量",
            },
            {
              title: `内存上涨(${derived.shortTrendConfig.windowMinutes}m)`,
              value: loading ? "-" : String(memRisers),
              sub: loading ? "加载统计中..." : "短期内存持续抬升的 Pod 数量",
            },
          ]
        : [
            {
              title: "\u7b5b\u9009\u540e\u8d44\u6e90",
              value: loading ? "-" : String(derived.rows.length),
              sub: loading ? "\u8d44\u6e90\u6570\u636e\u52a0\u8f7d\u4e2d..." : loadError || "\u5f53\u524d\u7b5b\u9009\u6761\u4ef6\u547d\u4e2d\u7684\u8d44\u6e90\u6761\u76ee\u6570",
            },
            {
              title: "\u544a\u8b66",
              value: loading ? "-" : String(alerts),
              sub: loading ? "\u7b49\u5f85\u8d44\u6e90\u7ed3\u679c\u8fd4\u56de" : "\u98ce\u9669\u533a\u8d44\u6e90\u6216\u5f02\u5e38 Pod",
            },
            {
              title: "\u5173\u6ce8",
              value: loading ? "-" : String(watches),
              sub: loading ? "\u52a0\u8f7d\u7edf\u8ba1\u4e2d..." : "\u5173\u6ce8\u533a\u8d44\u6e90\u6570\u91cf",
            },
            {
              title: "\u6700\u9ad8\u5f53\u524d\u4f7f\u7528\u7387",
              value: loading ? "-" : worst ? fmtPct(worst.usage_current) : "-",
              sub: loading ? "Prometheus \u67e5\u8be2\u4e2d" : worst ? describeResourceItem(worst).title : "\u6682\u65e0\u6570\u636e",
            },
          ];
    target.innerHTML = cards
      .map(
        (item) => `
          <div class="kpi-card">
            <div class="kpi-title">${escapeHtml(item.title)}</div>
            <div class="kpi-value">${escapeHtml(item.value)}</div>
            <div class="kpi-sub">${escapeHtml(item.sub)}</div>
          </div>
        `
      )
      .join("");
  }

  function buildClusterRows(items) {
    const map = new Map();
    items.forEach((item) => {
      const key = item.cluster || "-";
      const current =
        map.get(key) ||
        {
          cluster: key,
          metrics: new Set(),
          total: 0,
          alert: 0,
          watch: 0,
          safe: 0,
          usageSum: 0,
          usageCount: 0,
          remainingSum: 0,
          remainingCount: 0,
          worst: null,
        };
      current.metrics.add(metricLabel(item.metric));
      current.total += 1;
      current[item.usage_zone] = (current[item.usage_zone] || 0) + 1;
      if (toNumber(item.usage_current) !== null) {
        current.usageSum += Number(item.usage_current);
        current.usageCount += 1;
      }
      if (toNumber(item.remaining_current) !== null) {
        current.remainingSum += Number(item.remaining_current);
        current.remainingCount += 1;
      }
      if (!current.worst || zoneRank(item.usage_zone) < zoneRank(current.worst.usage_zone)) current.worst = item;
      else if (
        current.worst &&
        zoneRank(item.usage_zone) === zoneRank(current.worst.usage_zone) &&
        (toNumber(item.usage_current) || 0) > (toNumber(current.worst.usage_current) || 0)
      ) current.worst = item;
      map.set(key, current);
    });
    return Array.from(map.values())
      .map((item) => ({
        cluster: item.cluster,
        metrics: Array.from(item.metrics).join(", "),
        total: item.total,
        alert: item.alert || 0,
        watch: item.watch || 0,
        safe: item.safe || 0,
        avgUsage: item.usageCount ? item.usageSum / item.usageCount : null,
        avgRemaining: item.remainingCount ? item.remainingSum / item.remainingCount : null,
        worst: item.worst,
      }))
      .sort((left, right) => {
        if (left.alert !== right.alert) return right.alert - left.alert;
        if (left.watch !== right.watch) return right.watch - left.watch;
        return (toNumber(right.avgUsage) || 0) - (toNumber(left.avgUsage) || 0);
      });
  }

  function renderClusterTable(target, rows, emptyText) {
    if (!target) return;
    target.innerHTML = `
      <thead>
        <tr><th>集群</th><th>覆盖指标</th><th>总数</th><th>告警</th><th>关注</th><th>平均使用率</th><th>平均余量</th><th>最高风险</th></tr>
      </thead>
      <tbody>
        ${
          rows.length
            ? rows
                .map((item) => {
                  const worst = item.worst ? describeResourceItem(item.worst).title : "-";
                  const worstZone = item.worst ? zoneBadge(item.worst.usage_zone) : zoneBadge("unknown");
                  return `
                    <tr>
                      <td>${escapeHtml(item.cluster)}</td>
                      <td>${escapeHtml(item.metrics || "-")}</td>
                      <td>${item.total}</td>
                      <td>${item.alert}</td>
                      <td>${item.watch}</td>
                      <td>${fmtPct(item.avgUsage)}</td>
                      <td>${fmtPct(item.avgRemaining)}</td>
                      <td><div class="cell-multi"><span>${escapeHtml(worst)}</span><span>${worstZone}</span></div></td>
                    </tr>
                  `;
                })
                .join("")
            : `<tr><td colspan="8" class="empty-state">${escapeHtml(emptyText || "\u6682\u65e0\u5339\u914d\u6570\u636e")}</td></tr>`
        }
      </tbody>
    `;
  }

  const RESOURCE_COLUMN_DEFS = {
    combined: {
      metric: { label: "指标", render: (item) => escapeHtml(metricLabel(item.metric)) },
      cluster: { label: "集群", render: (item) => escapeHtml(fmtValue(item.cluster)) },
      instance: {
        label: "对象",
        render: (item) => {
          const main = item.metric === "disk" ? `${fmtValue(item.instance)} · ${fmtValue(item.mountpoint)}` : fmtValue(item.instance);
          return `<div class="cell-multi"><span>${escapeHtml(main)}</span><span class="muted">${escapeHtml(
            fmtValue(item.mountpoint || item.namespace || "")
          )}</span></div>`;
        },
      },
      group: { label: "分组", render: (item) => escapeHtml(fmtValue(item.group_summary || item.group)) },
      usage: {
        label: "当前",
        render: (item) => `<div class="cell-multi"><span>${escapeHtml(fmtPct(item.usage_current))}</span><span class="muted">${escapeHtml(
          item.metric === "disk" && item.used_bytes !== undefined ? `${fmtBytes(item.used_bytes)} / ${fmtBytes(item.size_bytes)}` : "-"
        )}</span></div>`,
      },
      usage_zone: { label: "风险区", render: (item) => zoneBadge(item.usage_zone) },
      remaining: {
        label: "余量",
        render: (item) => `<div class="cell-multi"><span>${escapeHtml(fmtPct(item.remaining_current))}</span><span class="muted">${escapeHtml(
          item.avail_bytes !== undefined && item.avail_bytes !== null ? fmtBytes(item.avail_bytes) : "-"
        )}</span></div>`,
      },
      remaining_zone: { label: "余量区", render: (item) => zoneBadge(item.remaining_zone) },
      peak: {
        label: "峰值",
        render: (item) => `<div class="cell-multi"><span>${escapeHtml(fmtPct(item.usage_max))}</span><span class="muted">最小余量 ${escapeHtml(
          fmtPct(item.remaining_min)
        )}</span></div>`,
      },
      trend: { label: "趋势", render: (item) => escapeHtml(fmtTrend(item.trend)) },
      forecast: {
        label: "预计触线",
        render: (item) => {
          const days = toNumber(item.forecast_alert_days);
          return days === null ? "-" : `${days.toFixed(1)} 天`;
        },
      },
    },
    pod: {
      cluster: { label: "集群", render: (item) => escapeHtml(fmtValue(item.cluster)) },
      namespace: { label: "命名空间", render: (item) => escapeHtml(fmtValue(item.namespace)) },
      pod: {
        label: "Pod",
        render: (item) => `<div class="cell-multi"><span>${escapeHtml(fmtValue(item.pod))}</span><span class="muted">${escapeHtml(
          item.is_abnormal ? "异常" : "稳定"
        )}</span></div>`,
      },
      phase: {
        label: "状态",
        render: (item) =>
          `<div class="cell-multi"><span>${zoneBadge(item.usage_zone)}</span><span class="muted">${escapeHtml(
            fmtValue(item.phase || item.pod_status || "-")
          )}</span></div>`,
      },
      node: { label: "节点", render: (item) => escapeHtml(fmtValue(item.node_name)) },
      ready: { label: "\u5c31\u7eea", render: (item) => escapeHtml(fmt\u5c31\u7eea(item.ready_count, item.ready_total, item.ready_ratio)) },
      restarts: {
        label: "重启",
        render: (item) => `<div class="cell-multi"><span>${escapeHtml(fmtRestart(item.restarts_total ?? item.restarts))}</span><span class="muted">速率 ${escapeHtml(
          fmtRate(item.restarts_rate)
        )}</span></div>`,
      },
      cpu: {
        label: "CPU",
        render: (item) => `<div class="cell-multi"><span>${escapeHtml(fmtPct(item.cpu_usage_ratio))}</span><span class="muted">${escapeHtml(
          toNumber(item.cpu_usage_cores) !== null ? `${Number(item.cpu_usage_cores).toFixed(3)} cores` : "-"
        )}</span></div>`,
      },
      cpu_trend_short: {
        label: "CPU趋势",
        render: (item) => formatPodShortTrendCell(item.cpu_short_trend, item.short_trend_config),
      },
      mem: {
        label: "Mem",
        render: (item) => `<div class="cell-multi"><span>${escapeHtml(
          fmtPct(item.mem_usage_ratio ?? item.mem_ratio)
        )}</span><span class="muted">${escapeHtml(fmtBytes(item.mem_working_set_bytes))}</span></div>`,
      },
      mem_trend_short: {
        label: "Mem趋势",
        render: (item) => formatPodShortTrendCell(item.mem_short_trend, item.short_trend_config),
      },
      signals: {
        label: "Signals",
        render: (item) => renderPodSignals(item),
      },
      terminated: {
        label: "最近终止",
        render: (item) =>
          escapeHtml(
            item.terminated_reason
              ? `${item.terminated_reason}${
                  item.terminated_exitcode !== null && item.terminated_exitcode !== undefined
                    ? ` (${Math.round(Number(item.terminated_exitcode))})`
                    : ""
                }`
              : "-"
          ),
      },
      last_terminated: { label: "终止时间", render: (item) => escapeHtml(fmtTime(item.last_terminated_time)) },
      actions: {
        label: "动作",
        render: (item) =>
          `<button class="detail-action" type="button" data-open-pod="${escapeHtml(buildPodKey(item))}">查看详情</button>`,
      },
    },
  };

  function pickVisibleColumns(type, prefs) {
    const selected = Array.isArray(prefs.columns[type]) ? prefs.columns[type].filter((key) => RESOURCE_COLUMN_DEFS[type][key]) : [];
    return selected.length ? selected : DEFAULT_RESOURCE_PREFS.columns[type].filter((key) => RESOURCE_COLUMN_DEFS[type][key]);
  }

  function renderTable(target, rows, type, prefs, emptyText) {
    if (!target) return;
    const keys = pickVisibleColumns(type, prefs);
    const defs = RESOURCE_COLUMN_DEFS[type];
    target.innerHTML = `
      <thead>
        <tr>${keys.map((key) => `<th>${escapeHtml(defs[key].label)}</th>`).join("")}</tr>
      </thead>
      <tbody>
        ${
          rows.length
            ? rows
                .map(
                  (row) =>
                    `<tr class="row-${row.usage_zone || "unknown"}">${keys.map((key) => `<td>${defs[key].render(row)}</td>`).join("")}</tr>`
                )
                .join("")
            : `<tr><td colspan="${keys.length || 1}" class="empty-state">${escapeHtml(emptyText)}</td></tr>`
        }
      </tbody>
    `;
  }

  function render\u544a\u8b66MiniTable(target, alerts) {
    if (!target) return;
    const rows = (alerts || []).slice(0, 10);
    target.innerHTML = `
      <thead><tr><th>分类</th><th>对象</th><th>累计时长</th></tr></thead>
      <tbody>
        ${
          rows.length
            ? rows
                .map(
                  (item) => `<tr class="row-alert"><td>${escapeHtml(fmtValue(item.category))}</td><td>${escapeHtml(
                    fmtValue(item.object)
                  )}</td><td>${escapeHtml(`${fmtHours(item.hours)}h`)}</td></tr>`
                )
                .join("")
            : '<tr><td colspan="3" class="empty-state">暂无告警数据</td></tr>'
        }
      </tbody>
    `;
  }

  function hasActivePodFilter(prefs) {
    return Object.values((prefs.filters && prefs.filters.podFlags) || {}).some(Boolean);
  }

  function matchesPodFlags(item, podFlags, trendRules) {
    const issues = Array.isArray(item.pod_issues) ? item.pod_issues : getPodIssues(item, trendRules);
    const byKey = new Set(issues.map((entry) => entry.key));
    if (podFlags.abnormalOnly && !issues.length) return false;
    if (podFlags.notrunning && String(item.phase || item.pod_status || "") === "Running") return false;
    if (podFlags.waiting && !item.waiting_reason) return false;
    if (podFlags.terminated && !item.terminated_reason) return false;
    if (podFlags.oom && !(item.oom || (toNumber(item.oom_events_total) || 0) > 0)) return false;
    if (podFlags.restart && (toNumber(item.restarts_total ?? item.restarts) || 0) <= 0) return false;
    if (podFlags.throttle && !byKey.has("throttle")) return false;
    if (podFlags.mem && !byKey.has("mem")) return false;
    if (podFlags.fs && !byKey.has("fs")) return false;
    return true;
  }

  function deriveResourceState(resources, alerts, prefs) {
    const thresholds = getThresholds(resources || {});
    const trendRules = getPodTrendRules(resources || {});
    const shortTrendConfig = getPodShortTrendConfig(resources || {});
    const enriched = enrichItems((resources && resources.items) || [], thresholds, trendRules);
    const podRows = buildPodItems(resources || {}, thresholds, trendRules, shortTrendConfig);
    const forcePodView =
      prefs.metric === "pod" || prefs.filters.namespace !== "all" || prefs.filters.node !== "all" || hasActivePodFilter(prefs);
    const metric = prefs.metric === "all" && forcePodView ? "pod" : prefs.metric;
    const sourceRows = metric === "pod" ? podRows : enriched.filter((item) => metric === "all" || item.metric === metric);
    const search = String(prefs.search || "").trim().toLowerCase();
    const filtered = sourceRows.filter((item) => {
      if (prefs.cluster !== "all" && item.cluster !== prefs.cluster) return false;
      if (prefs.filters.zone !== "all" && item.usage_zone !== prefs.filters.zone) return false;
      if (metric === "pod") {
        if (prefs.filters.namespace !== "all" && item.namespace !== prefs.filters.namespace) return false;
        if (prefs.filters.node !== "all" && (item.node_name || "-") !== prefs.filters.node) return false;
        if (!matchesPodFlags(item, prefs.filters.podFlags, trendRules)) return false;
      }
      if (search && !String(item.search_blob || "").includes(search)) return false;
      return true;
    });
    const rows = filtered.slice().sort((left, right) => {
      const byZone = zoneRank(left.usage_zone) - zoneRank(right.usage_zone);
      if (byZone !== 0) return byZone;
      if (metric === "pod") {
        const bySurge = (toNumber(right.surge_score) || 0) - (toNumber(left.surge_score) || 0);
        if (bySurge !== 0) return bySurge;
      }
      return (toNumber(right.usage_current) || 0) - (toNumber(left.usage_current) || 0);
    });
    const capacityRows = filtered
      .filter((item) => item.usage_zone === "safe")
      .slice()
      .sort((left, right) => (toNumber(right.remaining_current) || -1) - (toNumber(left.remaining_current) || -1));
    const podScope = metric === "pod" ? filtered : podRows;
    const cpuRiseRows = podScope
      .filter((item) => item.cpu_short_trend && toNumber(item.cpu_short_trend.delta) !== null && (item.cpu_surge_score || 0) > 0)
      .slice()
      .sort((left, right) => (right.cpu_surge_score || 0) - (left.cpu_surge_score || 0))
      .slice(0, 5)
      .map((item) => ({
        ...item,
        display_value: `${fmtPctDelta(item.cpu_short_trend.delta)} / ${item.cpu_short_trend.ratio?.toFixed(1) || "-"}x`,
        display_meta: `当前 ${fmtPct(item.cpu_short_trend.current)} · 前${shortTrendConfig.windowMinutes}m ${fmtPct(
          item.cpu_short_trend.previous
        )}`,
      }));
    const memRiseRows = podScope
      .filter((item) => item.mem_short_trend && toNumber(item.mem_short_trend.delta) !== null && (item.mem_surge_score || 0) > 0)
      .slice()
      .sort((left, right) => (right.mem_surge_score || 0) - (left.mem_surge_score || 0))
      .slice(0, 5)
      .map((item) => ({
        ...item,
        display_value: `${fmtPctDelta(item.mem_short_trend.delta)} / ${item.mem_short_trend.ratio?.toFixed(1) || "-"}x`,
        display_meta: `当前 ${fmtPct(item.mem_short_trend.current)} · 前${shortTrendConfig.windowMinutes}m ${fmtPct(
          item.mem_short_trend.previous
        )}`,
      }));
    const labels =
      metric === "pod"
        ? {
            ...VIEW_LABELS.pod,
            riskTitle: "CPU快速上涨 Top 5",
            riskSub: `按近${shortTrendConfig.windowMinutes}分钟 CPU 抬升幅度排序`,
            surplusTitle: "内存快速上涨 Top 5",
            surplusSub: `按近${shortTrendConfig.windowMinutes}分钟内存抬升幅度排序`,
            usageTitle: "Pod 画像",
            usageSub: "聚合展示 Pod 状态、重启、CPU、内存和短期上涨趋势",
            issuesTitle: "异常 Pod 清单",
            issuesSub: "状态异常、重启、限流、OOM 和短时上涨的 Pod 优先显示",
            capacityTitle: "稳定 Pod 清单",
            capacitySub: "运行稳定、余量充足的 Pod",
          }
        : VIEW_LABELS.default;
    return {
      metric,
      viewMode: metric === "pod" ? "pod" : "default",
      rows,
      issueRows: rows.filter((item) => item.usage_zone === "alert" || item.usage_zone === "watch"),
      capacityRows,
      riskRows: metric === "pod" ? cpuRiseRows : rows.slice(0, 5),
      surplusRows: metric === "pod" ? memRiseRows : capacityRows.slice(0, 5),
      clusterRows: buildClusterRows(filtered),
      podRows,
      clusters: uniqueSorted(enriched.map((item) => item.cluster).concat(podRows.map((item) => item.cluster))),
      namespaces: uniqueSorted(podRows.map((item) => item.namespace).filter((item) => item && item !== "-")),
      nodes: uniqueSorted(podRows.map((item) => item.node_name).filter((item) => item && item !== "-")),
      trendRules,
      shortTrendConfig,
      labels,
      alerts: (alerts && alerts.items ? alerts.items : []).slice().sort((left, right) => (right.hours || 0) - (left.hours || 0)),
    };
  }

  function renderActiveFilters(target, prefs) {
    if (!target) return;
    const chips = [];
    if (prefs.metric !== "all") chips.push({ key: "metric", label: `指标: ${metricLabel(prefs.metric)}` });
    if (prefs.cluster !== "all") chips.push({ key: "cluster", label: `集群: ${prefs.cluster}` });
    if (prefs.filters.zone !== "all") chips.push({ key: "zone", label: `风险区: ${ZONE_LABEL[prefs.filters.zone] || prefs.filters.zone}` });
    if (prefs.filters.namespace !== "all") chips.push({ key: "namespace", label: `命名空间: ${prefs.filters.namespace}` });
    if (prefs.filters.node !== "all") chips.push({ key: "node", label: `节点: ${prefs.filters.node}` });
    POD_FLAGS.forEach((item) => {
      if (prefs.filters.podFlags[item.key]) chips.push({ key: `podFlag:${item.key}`, label: item.label });
    });
    if (prefs.search) chips.push({ key: "search", label: `搜索: ${prefs.search}` });
    if (prefs.playback.enabled && prefs.playback.endTs) {
      chips.push({ key: "playback", label: `回放: ${fmtTime(prefs.playback.endTs)} / ${prefs.playback.rangeHours}h` });
    }
    if (!chips.length) {
      target.innerHTML = `<span class="muted">当前只启用一级筛选。</span>`;
      return;
    }
    target.innerHTML = chips
      .map(
        (item) => `
          <button class="filter-badge" type="button" data-clear-filter="${escapeHtml(item.key)}">
            <span>${escapeHtml(item.label)}</span>
            <span>×</span>
          </button>
        `
      )
      .join("");
  }

  function renderTopMeta(metaNode, statusNode, resources, prefs, settings, flash, options, state) {
    const loading = !!(state && state.loading);
    if (metaNode) {
      if (!resources) metaNode.textContent = loading ? "\u8d44\u6e90\u6570\u636e\u52a0\u8f7d\u4e2d..." : "\u5c1a\u672a\u52a0\u8f7d\u6570\u636e";
      else {
        const metaLines = [
          `\u751f\u6210\u65f6\u95f4: ${escapeHtml(fmtValue(resources.generated_at))}`,
          `\u7edf\u8ba1\u7a97\u53e3: ${escapeHtml(fmtValue(resources.window && resources.window.start))} ~ ${escapeHtml(
            fmtValue(resources.window && resources.window.end)
          )}`,
        ];
        if (resources.meta && toNumber(resources.meta.query_seconds) !== null) {
          metaLines.push(`\u67e5\u8be2\u8017\u65f6: ${escapeHtml(Number(resources.meta.query_seconds).toFixed(2))}s`);
        }
        metaNode.innerHTML = metaLines.join("<br/>");
      }
    }
    if (!statusNode) return;
    if (flash && flash.text) {
      statusNode.textContent = flash.text;
      statusNode.dataset.level = flash.level || "info";
      return;
    }
    if (loading) {
      statusNode.textContent = "\u8d44\u6e90\u6570\u636e\u52a0\u8f7d\u4e2d...";
      statusNode.dataset.level = "info";
      return;
    }
    const parts = [];
    if (options.supportPlayback) parts.push(prefs.playback.enabled ? "\u56de\u653e\u6a21\u5f0f" : "\u5b9e\u65f6\u6a21\u5f0f");
    if (settings && settings.notification && settings.notification.enabled) {
      parts.push(`\u901a\u77e5\u5df2\u542f\u7528 (${settings.notification.targets.length})`);
    } else if (options.saveSettings) {
      parts.push("\u901a\u77e5\u672a\u542f\u7528");
    }
    statusNode.textContent = parts.join(" | ") || "\u5c31\u7eea";
    statusNode.dataset.level = "info";
  }

  function renderFilterDrawer(state, derived, options, rerender, refreshData) {
    const target = byId("filter-drawer-content");
    if (!target) return;
    const podFlags = state.prefs.filters.podFlags;
    target.innerHTML = `
      <div class="drawer-stack">
        <section class="drawer-section">
          <div class="settings-subtitle">风险筛选</div>
          <label class="settings-field">
            <span>风险区</span>
            <select id="filter-zone-select">
              <option value="all">全部</option>
              <option value="alert"${state.prefs.filters.zone === "alert" ? " selected" : ""}>告警</option>
              <option value="watch"${state.prefs.filters.zone === "watch" ? " selected" : ""}>关注</option>
              <option value="safe"${state.prefs.filters.zone === "safe" ? " selected" : ""}>安全</option>
              <option value="unknown"${state.prefs.filters.zone === "unknown" ? " selected" : ""}>未知</option>
            </select>
          </label>
        </section>
        <section class="drawer-section">
          <div class="settings-subtitle">Pod 高级筛选</div>
          <div class="settings-grid">
            <label class="settings-field">
              <span>命名空间</span>
              <select id="filter-namespace-select">
                <option value="all">全部</option>
                ${derived.namespaces
                  .map(
                    (item) =>
                      `<option value="${escapeHtml(item)}"${state.prefs.filters.namespace === item ? " selected" : ""}>${escapeHtml(item)}</option>`
                  )
                  .join("")}
              </select>
            </label>
            <label class="settings-field">
              <span>节点</span>
              <select id="filter-node-select">
                <option value="all">全部</option>
                ${derived.nodes
                  .map(
                    (item) =>
                      `<option value="${escapeHtml(item)}"${state.prefs.filters.node === item ? " selected" : ""}>${escapeHtml(item)}</option>`
                  )
                  .join("")}
              </select>
            </label>
          </div>
          <div class="checkbox-grid">
            ${POD_FLAGS.map(
              (item) => `
                <label class="checkbox-item">
                  <input type="checkbox" data-pod-flag="${escapeHtml(item.key)}"${podFlags[item.key] ? " checked" : ""} />
                  <span>${escapeHtml(item.label)}</span>
                </label>
              `
            ).join("")}
          </div>
        </section>
        ${
          options.supportPlayback
            ? `
              <section class="drawer-section">
                <div class="settings-subtitle">回放</div>
                <label class="settings-switch">
                  <input type="checkbox" id="playback-enabled"${state.prefs.playback.enabled ? " checked" : ""} />
                  <span>按历史时间窗口回放</span>
                </label>
                <div class="settings-grid">
                  <label class="settings-field">
                    <span>结束时间</span>
                    <input id="playback-end" type="datetime-local" value="${escapeHtml(fmtDateTimeInput(state.prefs.playback.endTs))}" />
                  </label>
                  <label class="settings-field">
                    <span>回放窗口（小时）</span>
                    <input id="playback-hours" type="number" min="1" max="720" value="${escapeHtml(
                      String(state.prefs.playback.rangeHours || 24)
                    )}" />
                  </label>
                </div>
                <div class="detail-actions">
                  <button class="detail-action" id="playback-apply" type="button">应用回放</button>
                  <button class="detail-action" id="playback-reset" type="button">恢复实时</button>
                </div>
              </section>
            `
            : ""
        }
        <section class="drawer-section">
          <div class="detail-actions">
            <button class="detail-action" id="filter-reset" type="button">清空高级筛选</button>
          </div>
        </section>
      </div>
    `;

    const zoneSelect = byId("filter-zone-select");
    const namespaceSelect = byId("filter-namespace-select");
    const nodeSelect = byId("filter-node-select");
    if (zoneSelect) {
      zoneSelect.addEventListener("change", () => {
        state.prefs.filters.zone = zoneSelect.value || "all";
        state.persistPrefs();
        rerender();
      });
    }
    if (namespaceSelect) {
      namespaceSelect.addEventListener("change", () => {
        state.prefs.filters.namespace = namespaceSelect.value || "all";
        state.persistPrefs();
        rerender();
      });
    }
    if (nodeSelect) {
      nodeSelect.addEventListener("change", () => {
        state.prefs.filters.node = nodeSelect.value || "all";
        state.persistPrefs();
        rerender();
      });
    }
    target.querySelectorAll("[data-pod-flag]").forEach((node) => {
      node.addEventListener("change", () => {
        const key = node.getAttribute("data-pod-flag");
        state.prefs.filters.podFlags[key] = !!node.checked;
        state.persistPrefs();
        rerender();
      });
    });
    const resetButton = byId("filter-reset");
    if (resetButton) {
      resetButton.addEventListener("click", () => {
        state.prefs.filters.zone = "all";
        state.prefs.filters.namespace = "all";
        state.prefs.filters.node = "all";
        state.prefs.filters.podFlags = mergeDeep(DEFAULT_RESOURCE_PREFS.filters.podFlags, {});
        state.persistPrefs();
        rerender();
      });
    }
    if (options.supportPlayback) {
      const applyButton = byId("playback-apply");
      const resetPlayback = byId("playback-reset");
      if (applyButton) {
        applyButton.addEventListener("click", () => {
          const enabledNode = byId("playback-enabled");
          const endNode = byId("playback-end");
          const hoursNode = byId("playback-hours");
          state.prefs.playback.enabled = !!(enabledNode && enabledNode.checked);
          state.prefs.playback.endTs = endNode && endNode.value ? Math.floor(new Date(endNode.value).getTime() / 1000) : null;
          state.prefs.playback.rangeHours = Math.max(1, Math.min(720, Number(hoursNode && hoursNode.value ? hoursNode.value : 24) || 24));
          state.persistPrefs();
          refreshData();
        });
      }
      if (resetPlayback) {
        resetPlayback.addEventListener("click", () => {
          state.prefs.playback.enabled = false;
          state.prefs.playback.endTs = null;
          state.prefs.playback.rangeHours = 24;
          state.persistPrefs();
          refreshData();
        });
      }
    }
  }

  function renderSettingsDrawer(state, options, rerender) {
    const target = byId("settings-drawer-content");
    if (!target) return;
    const settings = state.settings || normalizeDashboardSettings({}, state.prefs.links);
    target.innerHTML = `
      <div class="drawer-stack">
        <section class="drawer-section">
          <div class="settings-subtitle">列显示</div>
          <div class="settings-help">列偏好实时保存到浏览器，本地和实时页面共用同一套表格定义。</div>
          <div class="settings-subtitle">资源表</div>
          <div class="checkbox-grid">
            ${Object.keys(RESOURCE_COLUMN_DEFS.combined)
              .map(
                (key) => `
                  <label class="checkbox-item">
                    <input type="checkbox" data-column-group="combined" data-column-key="${escapeHtml(key)}"${
                      pickVisibleColumns("combined", state.prefs).includes(key) ? " checked" : ""
                    } />
                    <span>${escapeHtml(RESOURCE_COLUMN_DEFS.combined[key].label)}</span>
                  </label>
                `
              )
              .join("")}
          </div>
          <div class="settings-subtitle">Pod 表</div>
          <div class="checkbox-grid">
            ${Object.keys(RESOURCE_COLUMN_DEFS.pod)
              .map(
                (key) => `
                  <label class="checkbox-item">
                    <input type="checkbox" data-column-group="pod" data-column-key="${escapeHtml(key)}"${
                      pickVisibleColumns("pod", state.prefs).includes(key) ? " checked" : ""
                    } />
                    <span>${escapeHtml(RESOURCE_COLUMN_DEFS.pod[key].label)}</span>
                  </label>
                `
              )
              .join("")}
          </div>
        </section>
        <section class="drawer-section">
          <div class="settings-subtitle">跳转模板</div>
          <div class="settings-help">支持占位符：{cluster} {namespace} {pod} {node} {instance} {prometheus} {expr_url}。</div>
          <div class="settings-grid">
            ${["logs", "events", "yaml", "shell", "metrics"]
              .map(
                (key) => `
                  <label class="settings-field">
                    <span>${escapeHtml({ logs: "日志链接", events: "事件链接", yaml: "YAML 链接", shell: "Shell 链接", metrics: "指标链接" }[key])}</span>
                    <input id="link-${escapeHtml(key)}" type="text" value="${escapeHtml(settings.links[key])}" placeholder="https://..." />
                  </label>
                `
              )
              .join("")}
          </div>
        </section>
        ${
          options.saveSettings
            ? `
              <section class="drawer-section">
                <div class="settings-subtitle">Pod 重启通知</div>
                <label class="settings-switch">
                  <input id="notify-enabled" type="checkbox"${settings.notification.enabled ? " checked" : ""} />
                  <span>启用指定 Pod 重启自动通知</span>
                </label>
                <div class="settings-grid">
                  <label class="settings-field">
                    <span>Webhook URL</span>
                    <input id="notify-webhook-url" type="url" value="${escapeHtml(
                      settings.notification.webhook_url
                    )}" placeholder="https://example.com/webhook" />
                  </label>
                  <label class="settings-field">
                    <span>Webhook 类型</span>
                    <select id="notify-webhook-type">
                      ${["generic", "feishu", "wecom", "dingtalk"]
                        .map(
                          (item) =>
                            `<option value="${item}"${
                              settings.notification.webhook_type === item ? " selected" : ""
                            }>${item}</option>`
                        )
                        .join("")}
                    </select>
                  </label>
                </div>
                <label class="settings-field">
                  <span>监控 Pod</span>
                  <textarea id="notify-targets" rows="7" placeholder="每行一个，支持 pod / namespace/pod / cluster/namespace/pod">${escapeHtml(
                    settings.notification.targets.join("\n")
                  )}</textarea>
                </label>
                <div class="settings-help">首次启用只会建立基线，不会回放历史重启。测试通知不会改动状态文件。</div>
                ${
                  settings.meta.config_path
                    ? `<div class="settings-help">当前配置文件：${escapeHtml(settings.meta.config_path)}</div>`
                    : ""
                }
              </section>
            `
            : ""
        }
        <section class="drawer-section">
          <div class="settings-status ${state.settingsStatus.level || ""}" id="settings-status">${escapeHtml(
            state.settingsStatus.text || ""
          )}</div>
          <div class="detail-actions">
            ${options.testNotification ? '<button class="detail-action" type="button" id="settings-test">发送测试</button>' : ""}
            ${options.saveSettings ? '<button class="detail-action" type="button" id="settings-save">保存设置</button>' : ""}
            <button class="detail-action" type="button" id="settings-reset">重置视图偏好</button>
          </div>
        </section>
      </div>
    `;

    target.querySelectorAll("[data-column-group]").forEach((node) => {
      node.addEventListener("change", () => {
        const group = node.getAttribute("data-column-group");
        const next = Object.keys(RESOURCE_COLUMN_DEFS[group]).filter((columnKey) => {
          const checkbox = target.querySelector(`[data-column-group="${group}"][data-column-key="${columnKey}"]`);
          return checkbox && checkbox.checked;
        });
        state.prefs.columns[group] = next.length ? next : DEFAULT_RESOURCE_PREFS.columns[group].slice();
        state.persistPrefs();
        rerender();
      });
    });

    ["logs", "events", "yaml", "shell", "metrics"].forEach((key) => {
      const node = byId(`link-${key}`);
      if (!node) return;
      node.addEventListener("input", () => {
        state.settings.links[key] = node.value.trim();
        state.prefs.links[key] = state.settings.links[key];
        state.persistPrefs();
      });
    });

    const resetButton = byId("settings-reset");
    if (resetButton) {
      resetButton.addEventListener("click", () => {
        state.prefs.columns = mergeDeep(DEFAULT_RESOURCE_PREFS.columns, {});
        state.prefs.links = mergeDeep(DEFAULT_LINK_TEMPLATES, {});
        state.settings.links = mergeDeep(DEFAULT_LINK_TEMPLATES, {});
        state.persistPrefs();
        rerender();
        renderSettingsDrawer(state, options, rerender);
      });
    }

    if (!options.saveSettings) return;

    function syncNotification() {
      const enabledNode = byId("notify-enabled");
      const urlNode = byId("notify-webhook-url");
      const typeNode = byId("notify-webhook-type");
      const targetsNode = byId("notify-targets");
      state.settings.notification.enabled = !!(enabledNode && enabledNode.checked);
      state.settings.notification.webhook_url = urlNode ? urlNode.value.trim() : "";
      state.settings.notification.webhook_type = typeNode ? typeNode.value : "generic";
      state.settings.notification.targets = targetsNode
        ? targetsNode.value
            .split(/\r?\n|,/)
            .map((item) => item.trim())
            .filter(Boolean)
        : [];
    }

    ["notify-enabled", "notify-webhook-url", "notify-webhook-type", "notify-targets"].forEach((id) => {
      const node = byId(id);
      if (!node) return;
      node.addEventListener("input", syncNotification);
      node.addEventListener("change", syncNotification);
    });

    const saveButton = byId("settings-save");
    if (saveButton) {
      saveButton.addEventListener("click", async () => {
        syncNotification();
        state.settingsStatus = { text: "保存中...", level: "info" };
        renderSettingsDrawer(state, options, rerender);
        try {
          const saved = await options.saveSettings(serializeSettings(state.settings));
          state.settings = normalizeDashboardSettings(saved, state.prefs.links);
          state.prefs.links = mergeDeep(DEFAULT_LINK_TEMPLATES, state.settings.links);
          state.persistPrefs();
          state.settingsStatus = { text: "设置已保存。", level: "success" };
          rerender();
          renderSettingsDrawer(state, options, rerender);
        } catch (err) {
          state.settingsStatus = { text: err.message || "保存失败。", level: "error" };
          renderSettingsDrawer(state, options, rerender);
        }
      });
    }

    const testButton = byId("settings-test");
    if (testButton) {
      testButton.addEventListener("click", async () => {
        syncNotification();
        state.settingsStatus = { text: "测试发送中...", level: "info" };
        renderSettingsDrawer(state, options, rerender);
        try {
          const response = await options.testNotification(serializeSettings(state.settings));
          state.settingsStatus = { text: response.message || "测试通知已发送。", level: "success" };
        } catch (err) {
          state.settingsStatus = { text: err.message || "测试发送失败。", level: "error" };
        }
        renderSettingsDrawer(state, options, rerender);
      });
    }
  }

  function renderPodDetail(state) {
    const title = byId("pod-detail-title");
    const body = byId("pod-detail-body");
    const item = state.activePod;
    const trendRules = (state.latestDerived && state.latestDerived.trendRules) || getPodTrendRules(state.resources || {});
    if (!title || !body) return;
    if (!item) {
      title.textContent = "Pod 详情";
      body.innerHTML = `<div class="muted">未选择 Pod。</div>`;
      return;
    }
    const actions = buildPodActions(item, state.settings);
    const issues = Array.isArray(item.pod_issues) ? item.pod_issues : getPodIssues(item, trendRules);
    title.textContent = `${item.namespace}/${item.pod}`;
    body.innerHTML = `
      <div class="summary-grid">
        <div class="summary-card">
          <div class="summary-title">状态</div>
          <div class="summary-main ${escapeHtml(item.usage_zone || "unknown")}">${escapeHtml(
            fmtValue(item.phase || item.pod_status || "-")
          )}</div>
          <div class="summary-sub">${escapeHtml(fmtValue(item.cluster))} · ${escapeHtml(fmtValue(item.node_name))}</div>
        </div>
        <div class="summary-card">
          <div class="summary-title">重启</div>
          <div class="summary-main">${escapeHtml(fmtRestart(item.restarts_total ?? item.restarts))}</div>
          <div class="summary-sub">速率 ${escapeHtml(fmtRate(item.restarts_rate))}</div>
        </div>
        <div class="summary-card">
          <div class="summary-title">CPU / Mem</div>
          <div class="summary-main">${escapeHtml(
            fmtPct(Math.max(toNumber(item.cpu_usage_ratio) || 0, toNumber(item.mem_usage_ratio ?? item.mem_ratio) || 0))
          )}</div>
          <div class="summary-sub">CPU ${escapeHtml(fmtPct(item.cpu_usage_ratio))} · Mem ${escapeHtml(
            fmtPct(item.mem_usage_ratio ?? item.mem_ratio)
          )}</div>
        </div>
        <div class="summary-card">
          <div class="summary-title">最近终止</div>
          <div class="summary-main">${escapeHtml(fmtValue(item.terminated_reason || "-"))}</div>
          <div class="summary-sub">${escapeHtml(fmtTime(item.last_terminated_time))}</div>
        </div>
      </div>
      <div class="detail-actions">${actions.map(renderActionButton).join("")}</div>
      <div class="diagnosis">
        <strong>诊断建议</strong>
        <div class="diagnosis-actions">
          ${
            issues.length
              ? issues
                  .map(
                    (entry) =>
                      `<div class="diagnosis-action">${zoneBadge(entry.severity)} ${escapeHtml(entry.title)}：${escapeHtml(
                        entry.detail
                      )}</div>`
                  )
                  .join("")
              : '<div class="diagnosis-action">当前没有发现显著异常，建议继续关注趋势和最近变更。</div>'
          }
        </div>
      </div>
      <div class="detail-grid">
        <div class="detail-card">
          <h4>运行指标</h4>
          <div class="detail-list">
            <div class="detail-item"><span>\u5c31\u7eea</span><span>${escapeHtml(
              fmt\u5c31\u7eea(item.ready_count, item.ready_total, item.ready_ratio)
            )}</span></div>
            <div class="detail-item"><span>CPU 使用</span><span>${escapeHtml(fmtPct(item.cpu_usage_ratio))}</span></div>
            <div class="detail-item"><span>CPU 实际</span><span>${escapeHtml(
              toNumber(item.cpu_usage_cores) !== null ? `${Number(item.cpu_usage_cores).toFixed(3)} cores` : "-"
            )}</span></div>
            <div class="detail-item"><span>Mem 使用</span><span>${escapeHtml(fmtPct(item.mem_usage_ratio ?? item.mem_ratio))}</span></div>
            <div class="detail-item"><span>Working Set</span><span>${escapeHtml(fmtBytes(item.mem_working_set_bytes))}</span></div>
            <div class="detail-item"><span>FS 使用</span><span>${escapeHtml(fmtPct(item.fs_usage_ratio))}</span></div>
          </div>
        </div>
        <div class="detail-card">
          <h4>历史基线</h4>
          <div class="detail-list">
            ${buildPodBaselineRows("CPU", item.cpu_usage_ratio, item.cpu_recent_avg, item.cpu_prev_avg, item.cpu_trend, trendRules)}
            ${buildPodBaselineRows(
              "内存",
              item.mem_usage_ratio ?? item.mem_ratio,
              item.mem_recent_avg,
              item.mem_prev_avg,
              item.mem_trend,
              trendRules
            )}
          </div>
        </div>
        <div class="detail-card">
          <h4>限流与网络</h4>
          <div class="detail-list">
            <div class="detail-item"><span>CPU Throttle</span><span>${escapeHtml(fmtPct(item.cpu_throttle_ratio))}</span></div>
            <div class="detail-item"><span>Throttled Rate</span><span>${escapeHtml(
              fmtRate(item.cpu_throttled_seconds_rate)
            )}</span></div>
            <div class="detail-item"><span>Net RX</span><span>${escapeHtml(fmtRate(item.net_rx_bytes_rate, "B/s"))}</span></div>
            <div class="detail-item"><span>Net TX</span><span>${escapeHtml(fmtRate(item.net_tx_bytes_rate, "B/s"))}</span></div>
            <div class="detail-item"><span>RX Error</span><span>${escapeHtml(fmtRate(item.net_rx_errors_rate))}</span></div>
            <div class="detail-item"><span>TX Error</span><span>${escapeHtml(fmtRate(item.net_tx_errors_rate))}</span></div>
          </div>
        </div>
        <div class="detail-card">
          <h4>节点上下文</h4>
          <div class="detail-list">
            <div class="detail-item"><span>Node CPU</span><span>${escapeHtml(fmtPct(item.node_cpu_usage_ratio))}</span></div>
            <div class="detail-item"><span>Node OOM Rate</span><span>${escapeHtml(fmtRate(item.node_oom_rate))}</span></div>
            <div class="detail-item"><span>Node Load1</span><span>${escapeHtml(
              toNumber(item.node_load1) !== null ? Number(item.node_load1).toFixed(2) : "-"
            )}</span></div>
            <div class="detail-item"><span>Node 内存余量</span><span>${escapeHtml(fmtBytes(item.node_mem_available_bytes))}</span></div>
            <div class="detail-item"><span>Node Up</span><span>${escapeHtml(
              toNumber(item.node_up) === null ? "-" : Number(item.node_up) > 0 ? "UP" : "DOWN"
            )}</span></div>
          </div>
        </div>
      </div>
    `;
  }

  function bindSharedModalEvents(state, rerender) {
    if (state.sharedEventsBound) return;
    document.addEventListener("click", async (event) => {
      const closeNode = event.target.closest("[data-close-modal]");
      if (closeNode) {
        closeModal(closeNode.getAttribute("data-close-modal"));
        return;
      }
      const podNode = event.target.closest("[data-open-pod]");
      if (podNode) {
        const key = podNode.getAttribute("data-open-pod");
        state.activePod = state.latestDerived.podRows.find((item) => buildPodKey(item) === key) || null;
        renderPodDetail(state);
        openModal("pod-detail-modal");
        return;
      }
      const copyNode = event.target.closest("[data-copy]");
      if (copyNode) {
        const ok = await copyText(copyNode.getAttribute("data-copy"));
        state.setFlash(ok ? "命令已复制。" : "复制失败。", ok ? "success" : "error");
        rerender();
        return;
      }
      const clearNode = event.target.closest("[data-clear-filter]");
      if (!clearNode) return;
      const key = clearNode.getAttribute("data-clear-filter");
      if (key === "metric") state.prefs.metric = "all";
      if (key === "cluster") state.prefs.cluster = "all";
      if (key === "zone") state.prefs.filters.zone = "all";
      if (key === "namespace") state.prefs.filters.namespace = "all";
      if (key === "node") state.prefs.filters.node = "all";
      if (key === "search") state.prefs.search = "";
      if (key === "playback") {
        state.prefs.playback.enabled = false;
        state.prefs.playback.endTs = null;
        state.prefs.playback.rangeHours = 24;
      }
      if (key && key.startsWith("podFlag:")) state.prefs.filters.podFlags[key.split(":")[1]] = false;
      state.persistPrefs();
      rerender();
    });
    state.sharedEventsBound = true;
  }

  function createResourceDashboard(options) {
    ensureResourceOverlays();
    const storageKey = options.storageKey || "dashboard-resource-prefs";
    const state = {
      prefs: loadPrefs(storageKey, DEFAULT_RESOURCE_PREFS),
      resources: null,
      alerts: null,
      loading: false,
      settings: normalizeDashboardSettings({ links: loadPrefs(storageKey, DEFAULT_RESOURCE_PREFS).links }, DEFAULT_RESOURCE_PREFS.links),
      settingsStatus: { text: "", level: "" },
      flash: null,
      flashTimer: null,
      activePod: null,
      latestDerived: {
        viewMode: "default",
        rows: [],
        issueRows: [],
        capacityRows: [],
        riskRows: [],
        surplusRows: [],
        clusterRows: [],
        podRows: [],
        clusters: [],
        namespaces: [],
        nodes: [],
        trendRules: getPodTrendRules({}),
        shortTrendConfig: getPodShortTrendConfig({}),
        labels: VIEW_LABELS.default,
        alerts: [],
      },
      sharedEventsBound: false,
      persistPrefs() {
        savePrefs(storageKey, state.prefs);
      },
      setFlash(text, level) {
        state.flash = { text, level };
        if (state.flashTimer) clearTimeout(state.flashTimer);
        state.flashTimer = setTimeout(() => {
          state.flash = null;
          render();
        }, 3000);
      },
    };

    const refs = {
      meta: byId("meta"),
      topStatus: byId("top-status"),
      metricFilter: byId("metric-filter"),
      clusterSelect: byId("cluster-select"),
      search: byId("search"),
      activeFilters: byId("active-filters"),
      kpis: byId("kpis"),
      topRisk: byId("top-risk"),
      topSurplus: byId("top-surplus"),
      clusterTable: byId("cluster-table"),
      issueTable: byId("issue-table"),
      capacityTable: byId("capacity-table"),
      usageTable: byId("usage-table"),
      alertPanel: byId("alerts-panel"),
      alertTable: byId("alert-table"),
      riskTitle: byId("risk-title"),
      riskSub: byId("risk-sub"),
      surplusTitle: byId("surplus-title"),
      surplusSub: byId("surplus-sub"),
      usageTitle: byId("usage-title"),
      usageSub: byId("usage-sub"),
      issuesTitle: byId("issues-title"),
      issuesSub: byId("issues-sub"),
      capacityTitle: byId("capacity-title"),
      capacitySub: byId("capacity-sub"),
      filterButton: byId("filter-drawer-open"),
      settingsButton: byId("settings-drawer-open"),
      reloadButton: byId("reload-button"),
    };

    function renderControls(derived) {
      if (refs.metricFilter) {
        refs.metricFilter.querySelectorAll("[data-metric]").forEach((node) => {
          node.classList.toggle("active", node.getAttribute("data-metric") === state.prefs.metric);
        });
      }
      if (refs.clusterSelect) {
        refs.clusterSelect.innerHTML = [`<option value="all">全部集群</option>`]
          .concat(
            derived.clusters.map(
              (cluster) =>
                `<option value="${escapeHtml(cluster)}"${state.prefs.cluster === cluster ? " selected" : ""}>${escapeHtml(cluster)}</option>`
            )
          )
          .join("");
      }
      if (refs.search) refs.search.value = state.prefs.search || "";
    }

    function render() {
      const derived = deriveResourceState(state.resources || {}, state.alerts || {}, state.prefs);
      state.latestDerived = derived;
      renderControls(derived);
      renderTopMeta(refs.meta, refs.topStatus, state.resources, state.prefs, state.settings, state.flash, options, state);
      renderActiveFilters(refs.activeFilters, state.prefs);
      renderKpis(refs.kpis, derived, state);
      renderHighlightList(refs.topRisk, derived.riskRows, resolveResourceEmptyText(state, "\u6ca1\u6709\u9ad8\u98ce\u9669\u6761\u76ee"));
      renderHighlightList(refs.topSurplus, derived.surplusRows, resolveResourceEmptyText(state, "\u6ca1\u6709\u53ef\u627f\u8f7d\u6761\u76ee"));
      renderClusterTable(refs.clusterTable, derived.clusterRows, resolveResourceEmptyText(state, "\u6682\u65e0\u5339\u914d\u6570\u636e"));
      renderTable(refs.issueTable, derived.issueRows, derived.viewMode, state.prefs, resolveResourceEmptyText(state, "\u6ca1\u6709\u98ce\u9669\u9879"));
      renderTable(refs.capacityTable, derived.capacityRows, derived.viewMode, state.prefs, resolveResourceEmptyText(state, "\u6ca1\u6709\u5b89\u5168\u533a\u6761\u76ee"));
      renderTable(refs.usageTable, derived.rows, derived.viewMode, state.prefs, resolveResourceEmptyText(state, "\u6682\u65e0\u5339\u914d\u6570\u636e"));
      if (refs.riskTitle) refs.riskTitle.textContent = derived.labels.riskTitle;
      if (refs.riskSub) refs.riskSub.textContent = derived.labels.riskSub;
      if (refs.surplusTitle) refs.surplusTitle.textContent = derived.labels.surplusTitle;
      if (refs.surplusSub) refs.surplusSub.textContent = derived.labels.surplusSub;
      if (refs.usageTitle) refs.usageTitle.textContent = derived.labels.usageTitle;
      if (refs.usageSub) refs.usageSub.textContent = derived.labels.usageSub;
      if (refs.issuesTitle) refs.issuesTitle.textContent = derived.labels.issuesTitle;
      if (refs.issuesSub) refs.issuesSub.textContent = derived.labels.issuesSub;
      if (refs.capacityTitle) refs.capacityTitle.textContent = derived.labels.capacityTitle;
      if (refs.capacitySub) refs.capacitySub.textContent = derived.labels.capacitySub;
      if (refs.alertPanel) refs.alertPanel.style.display = options.show\u544a\u8b66sPanel === false ? "none" : "";
      if (options.show\u544a\u8b66sPanel !== false) render\u544a\u8b66MiniTable(refs.alertTable, derived.alerts);
      if (state.activePod) {
        const latest = derived.podRows.find((item) => buildPodKey(item) === buildPodKey(state.activePod));
        if (latest) {
          state.activePod = latest;
          renderPodDetail(state);
        }
      }
      renderFilterDrawer(state, derived, options, render, refreshData);
      renderSettingsDrawer(state, options, render);
    }

    async function refreshData() {
      state.loading = true;
      state.flash = null;
      render();
      try {
        const query = buildQueryState(state.prefs, options);
        const tasks = [options.loadResources(query)];
        if (options.load\u544a\u8b66s) tasks.push(options.load\u544a\u8b66s(query));
        const results = await Promise.allSettled(tasks);
        if (results[0].status !== "fulfilled") throw results[0].reason || new Error("Failed to load resource data.");
        state.resources = results[0].value;
        state.alerts = options.load\u544a\u8b66s ? (results[1].status === "fulfilled" ? results[1].value : { items: [] }) : { items: [] };
        state.flash = null;
      } finally {
        state.loading = false;
      }
      render();
    }

    async function loadSettings() {
      if (!options.loadSettings) {
        state.settings = normalizeDashboardSettings({ links: state.prefs.links }, state.prefs.links);
        return;
      }
      try {
        const saved = await options.loadSettings();
        state.settings = normalizeDashboardSettings(saved, state.prefs.links);
        state.prefs.links = mergeDeep(DEFAULT_LINK_TEMPLATES, state.settings.links);
        state.persistPrefs();
      } catch (err) {
        state.settings = normalizeDashboardSettings({ links: state.prefs.links }, state.prefs.links);
        state.settingsStatus = { text: err.message || "设置加载失败。", level: "error" };
      }
    }

    bindSharedModalEvents(state, render);

    if (refs.metricFilter) {
      refs.metricFilter.addEventListener("click", (event) => {
        const node = event.target.closest("[data-metric]");
        if (!node) return;
        state.prefs.metric = node.getAttribute("data-metric") || "all";
        if (state.prefs.metric !== "pod" && state.prefs.metric !== "all") {
          state.prefs.filters.namespace = "all";
          state.prefs.filters.node = "all";
          state.prefs.filters.podFlags = mergeDeep(DEFAULT_RESOURCE_PREFS.filters.podFlags, {});
        }
        state.persistPrefs();
        render();
      });
    }
    if (refs.clusterSelect) {
      refs.clusterSelect.addEventListener("change", () => {
        state.prefs.cluster = refs.clusterSelect.value || "all";
        state.persistPrefs();
        render();
      });
    }
    if (refs.search) {
      refs.search.addEventListener("input", () => {
        state.prefs.search = refs.search.value || "";
        state.persistPrefs();
        render();
      });
    }
    if (refs.filterButton) {
      refs.filterButton.addEventListener("click", () => {
        renderFilterDrawer(state, state.latestDerived, options, render, refreshData);
        openModal("filter-drawer-modal");
      });
    }
    if (refs.settingsButton) {
      refs.settingsButton.addEventListener("click", () => {
        renderSettingsDrawer(state, options, render);
        openModal("settings-drawer-modal");
      });
    }
    if (refs.reloadButton) {
      refs.reloadButton.addEventListener("click", () => {
        refreshData().catch((err) => {
          state.setFlash(err.message || "刷新失败。", "error");
          render();
        });
      });
    }

    Promise.all([loadSettings(), refreshData()])
      .then(() => render())
      .catch((err) => {
        state.flash = { text: err.message || "数据加载失败。", level: "error" };
        render();
      });
  }

  function create\u544a\u8b66sDashboard(options) {
    const storageKey = options.storageKey || "dashboard-alerts-prefs";
    const state = {
      prefs: loadPrefs(storageKey, DEFAULT_ALERT_PREFS),
      data: null,
    };
    const refs = {
      meta: byId("meta"),
      topStatus: byId("top-status"),
      categoryFilter: byId("category-filter"),
      search: byId("search"),
      activeFilters: byId("active-filters"),
      kpis: byId("kpis"),
      alertTable: byId("alert-table"),
      topRisk: byId("top-risk"),
      topSurplus: byId("top-surplus"),
    };

    function getItems() {
      const items = ((state.data && state.data.items) || []).slice().sort((left, right) => (right.hours || 0) - (left.hours || 0));
      return items.filter((item) => {
        if (state.prefs.category !== "all" && item.category !== state.prefs.category) return false;
        if (state.prefs.search) {
          const search = state.prefs.search.toLowerCase();
          const blob = `${item.category || ""} ${item.object || ""}`.toLowerCase();
          if (!blob.includes(search)) return false;
        }
        return true;
      });
    }

    function renderCategoryFilter() {
      if (!refs.categoryFilter) return;
      const categories = uniqueSorted(((state.data && state.data.items) || []).map((item) => item.category));
      refs.categoryFilter.innerHTML = [`<button data-category="all"${state.prefs.category === "all" ? ' class="active"' : ""}>全部</button>`]
        .concat(
          categories.map(
            (item) =>
              `<button data-category="${escapeHtml(item)}"${
                state.prefs.category === item ? ' class="active"' : ""
              }>${escapeHtml(item)}</button>`
          )
        )
        .join("");
    }

    function renderActiveFilters(items) {
      if (!refs.activeFilters) return;
      const chips = [];
      if (state.prefs.category !== "all") chips.push({ key: "category", label: `分类: ${state.prefs.category}` });
      if (state.prefs.search) chips.push({ key: "search", label: `搜索: ${state.prefs.search}` });
      refs.activeFilters.innerHTML = chips.length
        ? chips
            .map(
              (item) => `
                <button class="filter-badge" type="button" data-alert-clear="${escapeHtml(item.key)}">
                  <span>${escapeHtml(item.label)}</span>
                  <span>×</span>
                </button>
              `
            )
            .join("")
        : `<span class="muted">当前显示全部告警分类。</span>`;
      return items;
    }

    function render() {
      const items = getItems();
      if (refs.meta) {
        if (!state.data) refs.meta.textContent = "加载中...";
        else {
          refs.meta.innerHTML = `生成时间：${escapeHtml(fmtValue(state.data.generated_at))}<br/>统计窗口：${escapeHtml(
            fmtValue(state.data.window && state.data.window.start)
          )} ~ ${escapeHtml(fmtValue(state.data.window && state.data.window.end))}`;
        }
      }
      if (refs.topStatus) refs.topStatus.textContent = items.length ? `命中 ${items.length} 条告警` : "无告警";
      renderCategoryFilter();
      if (refs.search) refs.search.value = state.prefs.search || "";
      renderActiveFilters(items);
      if (refs.kpis) {
        const top = items[0];
        const totalHours = items.reduce((sum, item) => sum + (toNumber(item.hours) || 0), 0);
        refs.kpis.innerHTML = `
          <div class="kpi-card">
            <div class="kpi-title">筛选后告警</div>
            <div class="kpi-value">${items.length}</div>
            <div class="kpi-sub">当前分类和搜索条件命中的告警条目</div>
          </div>
          <div class="kpi-card">
            <div class="kpi-title">最长单条</div>
            <div class="kpi-value">${top ? `${fmtHours(top.hours)}h` : "0.00h"}</div>
            <div class="kpi-sub">${escapeHtml(top ? `${top.category} · ${top.object}` : "暂无数据")}</div>
          </div>
          <div class="kpi-card">
            <div class="kpi-title">累计时长</div>
            <div class="kpi-value">${fmtHours(totalHours)}h</div>
            <div class="kpi-sub">筛选后所有告警累计 firing 时长</div>
          </div>
        `;
      }
      renderHighlightList(
        refs.topRisk,
        items.slice(0, 5).map((item) => ({
          metric: "alert",
          cluster: item.category,
          instance: item.object,
          usage_current: toNumber(item.hours),
          usage_zone: "alert",
          display_value: `${fmtHours(item.hours)}h`,
        })),
        "暂无长时告警。"
      );
      renderHighlightList(
        refs.topSurplus,
        Array.from(
          ((state.data && state.data.items) || []).reduce((map, item) => {
            const key = item.category || "未分类";
            map.set(key, (map.get(key) || 0) + 1);
            return map;
          }, new Map()).entries()
        )
          .map(([category, count]) => ({
            metric: "alert",
            cluster: category,
            instance: `${count} 条`,
            usage_current: count / 10,
            usage_zone: "watch",
            display_value: `${count} 条`,
          }))
          .sort((left, right) => (toNumber(right.usage_current) || 0) - (toNumber(left.usage_current) || 0))
          .slice(0, 5),
        "暂无分类统计。"
      );
      if (refs.alertTable) {
        refs.alertTable.innerHTML = `
          <thead><tr><th>分类</th><th>对象</th><th>累计时长(小时)</th></tr></thead>
          <tbody>
            ${
              items.length
                ? items
                    .map(
                      (item) =>
                        `<tr class="row-alert"><td>${escapeHtml(fmtValue(item.category))}</td><td>${escapeHtml(
                          fmtValue(item.object)
                        )}</td><td>${escapeHtml(fmtHours(item.hours))}</td></tr>`
                    )
                    .join("")
                : '<tr><td colspan="3" class="empty-state">暂无告警数据</td></tr>'
            }
          </tbody>
        `;
      }
    }

    if (refs.categoryFilter) {
      refs.categoryFilter.addEventListener("click", (event) => {
        const node = event.target.closest("[data-category]");
        if (!node) return;
        state.prefs.category = node.getAttribute("data-category") || "all";
        savePrefs(storageKey, state.prefs);
        render();
      });
    }
    if (refs.search) {
      refs.search.addEventListener("input", () => {
        state.prefs.search = refs.search.value || "";
        savePrefs(storageKey, state.prefs);
        render();
      });
    }
    document.addEventListener("click", (event) => {
      const clearNode = event.target.closest("[data-alert-clear]");
      if (!clearNode) return;
      const key = clearNode.getAttribute("data-alert-clear");
      if (key === "category") state.prefs.category = "all";
      if (key === "search") state.prefs.search = "";
      savePrefs(storageKey, state.prefs);
      render();
    });

    options
      .load\u544a\u8b66s()
      .then((data) => {
        state.data = data;
        render();
      })
      .catch((err) => {
        if (refs.meta) refs.meta.textContent = err.message || "告警数据加载失败。";
        if (refs.topStatus) refs.topStatus.textContent = "加载失败";
      });
  }

  global.DashboardCore = {
    createResourceDashboard,
    create\u544a\u8b66sDashboard,
  };
})(window);
