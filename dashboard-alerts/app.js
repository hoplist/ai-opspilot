(function () {
  async function requestJson(url, timeoutMs = 30000) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    try {
      const response = await fetch(url, { cache: "no-store", signal: controller.signal });
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

  window.DashboardCore.createAlertsDashboard({
    storageKey: "dashboard-alerts-prefs",
    loadAlerts: () => requestJson("/api/alerts"),
  });
})();
