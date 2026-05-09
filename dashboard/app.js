(function () {
  const DATA_URLS = ["./data.json", "/outputs/dashboard/data.json", "../outputs/dashboard/data.json"];

  async function fetchFirstJson(urls) {
    let lastError = null;
    for (const url of urls) {
      try {
        const response = await fetch(url, { cache: "no-store" });
        if (!response.ok) continue;
        return await response.json();
      } catch (err) {
        lastError = err;
      }
    }
    throw lastError || new Error("无法加载离线看板数据。");
  }

  window.DashboardCore.createResourceDashboard({
    storageKey: "dashboard-offline-prefs",
    showAlertsPanel: false,
    supportPlayback: false,
    loadResources: () => fetchFirstJson(DATA_URLS),
  });
})();
