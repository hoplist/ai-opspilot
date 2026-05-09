(function () {
  function buildQuery(params) {
    const search = new URLSearchParams();
    if (params && params.end) search.set("end", String(params.end));
    if (params && params.range_hours) search.set("range_hours", String(params.range_hours));
    const text = search.toString();
    return text ? `?${text}` : "";
  }

  async function requestJson(url, options) {
    const { timeoutMs = 45000, ...fetchOptions } = options || {};
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    try {
      const response = await fetch(url, {
        cache: "no-store",
        headers: { "Content-Type": "application/json" },
        signal: controller.signal,
        ...fetchOptions,
      });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error || `${response.status} ${response.statusText}`);
      }
      if (payload && payload.error) {
        throw new Error(payload.error);
      }
      return payload;
    } catch (error) {
      if (error && error.name === "AbortError") {
        throw new Error(`请求超时（${Math.round(timeoutMs / 1000)}s）：${url}`);
      }
      throw error;
    } finally {
      clearTimeout(timer);
    }
  }

  window.DashboardCore.createResourceDashboard({
    storageKey: "dashboard-live-prefs",
    showAlertsPanel: true,
    supportPlayback: true,
    loadResources: (query) => requestJson(`/api/resources${buildQuery(query)}`, { timeoutMs: 60000 }),
    loadAlerts: (query) => requestJson(`/api/alerts${buildQuery(query)}`, { timeoutMs: 30000 }),
    loadSettings: () => requestJson("/api/dashboard-settings", { timeoutMs: 10000 }).then((payload) => payload.settings || payload),
    saveSettings: (settings) =>
      requestJson("/api/dashboard-settings", {
        method: "POST",
        body: JSON.stringify(settings),
        timeoutMs: 10000,
      }).then((payload) => payload.settings || payload),
    testNotification: (settings) =>
      requestJson("/api/dashboard-settings/test-notification", {
        method: "POST",
        body: JSON.stringify(settings),
        timeoutMs: 10000,
      }),
  });
})();
