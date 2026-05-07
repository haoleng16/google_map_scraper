/* ── State ───────────────────────────────────────────── */
const state = {
  contacts: [],
  messages: [],
  countries: [],
  jobs: [],
  results: [],
  resultCategories: [],
  expandedJobs: new Set(),
  expandedResults: {},
  selectedResultJobs: new Set(),
  initializedResultJobs: new Set(),
  visibleResultJobs: new Set(),
  scraperMode: "country",
  activeJobID: null,
  loggedIn: false,
  sending: false,
  loginPollTimer: null,
  jobPollTimer: null,
};

let msgIdCounter = 0;
function nextMsgId() { return ++msgIdCounter; }

/* ── DOM refs ────────────────────────────────────────── */
const $ = (sel) => document.querySelector(sel);
const statusDot = $("#statusDot");
const statusText = $("#statusText");
const loginBtn = $("#loginBtn");
const logoutBtn = $("#logoutBtn");
const csvDropZone = $("#csvDropZone");
const csvFileInput = $("#csvFileInput");
const contactControls = $("#contactControls");
const contactCount = $("#contactCount");
const contactList = $("#contactList");
const selectAllBtn = $("#selectAllBtn");
const deselectAllBtn = $("#deselectAllBtn");
const invertBtn = $("#invertBtn");
const messageCards = $("#messageCards");
const addMsgBtn = $("#addMsgBtn");
const sendBtn = $("#sendBtn");
const stopBtn = $("#stopBtn");
const contactDelayMinInput = $("#contactDelayMinInput");
const contactDelayMaxInput = $("#contactDelayMaxInput");
const batchSizeInput = $("#batchSizeInput");
const batchDelayMinInput = $("#batchDelayMinInput");
const batchDelayMaxInput = $("#batchDelayMaxInput");
const maxFailuresInput = $("#maxFailuresInput");
const progressPanel = $("#progressPanel");
const progressCounter = $("#progressCounter");
const progressBarFill = $("#progressBarFill");
const progressList = $("#progressList");
const tabButtons = document.querySelectorAll(".tab");
const tabPanels = document.querySelectorAll(".tab-panel");
const scraperMode = $("#scraperMode");
const countryField = $("#countryField");
const countrySelect = $("#countrySelect");
const locationField = $("#locationField");
const locationInput = $("#locationInput");
const keywordsInput = $("#keywordsInput");
const startScraperBtn = $("#startScraperBtn");
const cancelScraperBtn = $("#cancelScraperBtn");
const refreshJobsBtn = $("#refreshJobsBtn");
const scraperSummary = $("#scraperSummary");
const jobList = $("#jobList");
const resultQueryInput = $("#resultQueryInput");
const resultCategorySelect = $("#resultCategorySelect");
const hasPhoneFilter = $("#hasPhoneFilter");
const hasEmailFilter = $("#hasEmailFilter");
const hasWebsiteFilter = $("#hasWebsiteFilter");
const notImportedFilter = $("#notImportedFilter");
const searchResultsBtn = $("#searchResultsBtn");
const importResultsBtn = $("#importResultsBtn");
const resultTaskList = $("#resultTaskList");
const resultsSummary = $("#resultsSummary");
const settingsStatus = $("#settingsStatus");
const checkSettingsBtn = $("#checkSettingsBtn");
const installBrowsersBtn = $("#installBrowsersBtn");
const replaceCitiesBtn = $("#replaceCitiesBtn");

/* ── Wails Go bindings ──────────────────────────────── */
const go = window.go.main.App;

/* ── Top Tabs ───────────────────────────────────────── */
tabButtons.forEach((button) => {
  button.addEventListener("click", () => switchTab(button.dataset.tab));
});

function switchTab(tabID) {
  tabButtons.forEach((button) => button.classList.toggle("tab--active", button.dataset.tab === tabID));
  tabPanels.forEach((panel) => panel.classList.toggle("tab-panel--active", panel.id === tabID));
}

/* ── Information Retrieval ──────────────────────────── */
scraperMode?.addEventListener("click", (e) => {
  const button = e.target.closest("[data-mode]");
  if (!button) return;
  setScraperMode(button.dataset.mode);
});

function setScraperMode(mode) {
  state.scraperMode = mode;
  scraperMode.querySelectorAll("[data-mode]").forEach((button) => {
    button.classList.toggle("segmented__btn--active", button.dataset.mode === mode);
  });
  countryField.classList.toggle("field--hidden", mode !== "country");
  locationField.classList.toggle("field--hidden", mode !== "location");
}

async function loadCountries() {
  try {
    const countries = await go.GeoCountries();
    state.countries = countries || [];
    countrySelect.innerHTML = state.countries.map((country) => {
      const label = esc(country.display_name || `${country.name || ""}（${country.code || ""}）`);
      return `<option value="${esc(country.value || country.code)}" data-city-count="${Number(country.city_count || 0)}">${label}</option>`;
    }).join("");
    if (!state.countries.length) {
      countrySelect.innerHTML = `<option value="">未找到国家库</option>`;
    } else {
      updateCountryCoverageHint();
    }
  } catch (e) {
    countrySelect.innerHTML = `<option value="">国家库加载失败</option>`;
    scraperSummary.textContent = "国家库加载失败: " + e;
  }
}

countrySelect?.addEventListener("change", updateCountryCoverageHint);

function updateCountryCoverageHint() {
  const option = countrySelect.options[countrySelect.selectedIndex];
  const cityCount = option ? Number(option.dataset.cityCount || 0) : 0;
  scraperSummary.textContent = cityCount > 0 ? `国家覆盖：最多 ${cityCount} 个城市` : "国家覆盖：使用最大城市覆盖";
}

function buildScraperRequest() {
  return {
    mode: state.scraperMode,
    country: countrySelect.value,
    location: locationInput.value.trim(),
    keywords: keywordsInput.value.split(/\r?\n/).map((value) => value.trim()).filter(Boolean),
  };
}

startScraperBtn?.addEventListener("click", async () => {
  const req = buildScraperRequest();
  if (!req.keywords.length) {
    alert("请输入关键词");
    return;
  }
  if (req.mode === "country" && !req.country) {
    alert("请选择国家");
    return;
  }
  if (req.mode === "location" && !req.location) {
    alert("请输入城市或地区");
    return;
  }

  startScraperBtn.disabled = true;
  scraperSummary.textContent = "正在创建任务...";
  try {
    const job = await go.ScraperStartJob(req);
    state.activeJobID = job.id;
    cancelScraperBtn.disabled = false;
    scraperSummary.textContent = "任务已创建，后端开始获取信息";
    await refreshJobs();
    startJobPolling();
  } catch (e) {
    alert("启动失败: " + e);
    scraperSummary.textContent = "启动失败";
  } finally {
    startScraperBtn.disabled = false;
  }
});

cancelScraperBtn?.addEventListener("click", async () => {
  if (!state.activeJobID) return;
  try {
    await go.ScraperCancelJob(state.activeJobID);
    cancelScraperBtn.disabled = true;
    scraperSummary.textContent = "已请求取消任务";
    await refreshJobs();
  } catch (e) {
    alert("取消失败: " + e);
  }
});

refreshJobsBtn?.addEventListener("click", refreshJobs);

function startJobPolling() {
  if (state.jobPollTimer) clearInterval(state.jobPollTimer);
  state.jobPollTimer = setInterval(refreshJobs, 4000);
}

async function refreshJobs() {
  try {
    const jobs = await go.ScraperListJobs();
    state.jobs = jobs || [];
    renderJobs(state.jobs);
    await searchResults();
  } catch (e) {
    scraperSummary.textContent = "任务刷新失败: " + e;
  }
}

function renderJobs(jobs) {
  if (!jobs.length) {
    jobList.innerHTML = `<li class="empty-row">暂无任务</li>`;
    scraperSummary.textContent = "暂无任务";
    return;
  }
  const running = jobs.find((job) => job.status === "working" || job.status === "pending");
  state.activeJobID = running ? running.id : state.activeJobID;
  cancelScraperBtn.disabled = !running;
  scraperSummary.textContent = `${jobs.length} 个任务`;
  jobList.innerHTML = jobs.map((job) => `
    <li class="job-item" data-job-id="${esc(job.id)}">
      <div>
        <div class="job-item__name">${esc(job.name)}</div>
        <div class="job-item__meta">${esc(statusLabel(job.status))} · ${esc(job.location || "")} · ${esc((job.keywords || []).join(", "))}</div>
      </div>
      <div class="job-item__actions">
        <span class="job-item__count">${Number(job.imported_count || 0)} 条</span>
        <button class="icon-btn icon-btn--danger" type="button" data-delete-job="${esc(job.id)}" title="删除任务">×</button>
      </div>
    </li>
  `).join("");
}

jobList?.addEventListener("click", async (e) => {
  const deleteButton = e.target.closest("[data-delete-job]");
  if (!deleteButton) return;
  e.preventDefault();
  e.stopPropagation();
  await deleteJob(deleteButton.dataset.deleteJob, deleteButton);
});

function statusLabel(status) {
  const labels = { pending: "等待中", working: "获取中", ok: "完成", failed: "失败", interrupted: "已取消" };
  return labels[status] || status || "未知";
}

/* ── Results Library ────────────────────────────────── */
function buildResultFilter() {
  return {
    query: resultQueryInput.value.trim(),
    category: resultCategorySelect.value,
    has_phone: hasPhoneFilter.checked,
    has_email: hasEmailFilter.checked,
    has_website: hasWebsiteFilter.checked,
    not_imported: notImportedFilter.checked,
    limit: 500,
  };
}

function buildResultFilterForJob(jobID) {
  return { ...buildResultFilter(), job_id: jobID, limit: 500 };
}

function buildResultFilterForJobs(jobIDs) {
  return { ...buildResultFilter(), job_ids: jobIDs, limit: 50000 };
}

function buildResultCategoryFilter() {
  const filter = buildResultFilter();
  filter.category = "";
  return filter;
}

searchResultsBtn?.addEventListener("click", searchResults);
resultQueryInput?.addEventListener("keydown", (e) => { if (e.key === "Enter") searchResults(); });
[resultCategorySelect, hasPhoneFilter, hasEmailFilter, hasWebsiteFilter, notImportedFilter].forEach((input) => input?.addEventListener("change", searchResults));

async function searchResults() {
  try {
    await loadResultCategories();
    const results = await go.ResultsSearch(buildResultFilter());
    state.results = results || [];
    state.expandedResults = {};
    for (const jobID of state.expandedJobs) {
      state.expandedResults[jobID] = await go.ResultsSearch(buildResultFilterForJob(jobID));
    }
    renderResults();
  } catch (e) {
    resultsSummary.textContent = "结果读取失败: " + e;
  }
}

async function loadResultCategories() {
  const selected = resultCategorySelect.value;
  const categories = await go.ResultsCategories(buildResultCategoryFilter());
  state.resultCategories = categories || [];
  if (selected && !state.resultCategories.includes(selected)) {
    resultCategorySelect.value = "";
  }

  const current = resultCategorySelect.value;
  resultCategorySelect.innerHTML = [
    `<option value="">全部分类</option>`,
    ...state.resultCategories.map((category) => `<option value="${esc(category)}">${esc(category)}</option>`),
  ].join("");
  resultCategorySelect.value = state.resultCategories.includes(current) ? current : "";
}

function renderResults() {
  const matchingJobIDs = new Set(state.results.map((item) => item.job_id));
  const visibleJobs = state.jobs.filter((job) => {
    if (!hasActiveResultFilters()) return true;
    return matchingJobIDs.has(job.id);
  });
  state.visibleResultJobs = new Set(visibleJobs.map((job) => job.id));
  visibleJobs.forEach((job) => {
    if (!state.initializedResultJobs.has(job.id)) {
      state.initializedResultJobs.add(job.id);
      state.selectedResultJobs.add(job.id);
    }
  });

  const selectedCount = selectedVisibleResultJobIDs().length;
  resultsSummary.textContent = `${visibleJobs.length} 个任务 / ${state.results.length} 条匹配结果 / 已选 ${selectedCount} 个任务`;
  if (!visibleJobs.length) {
    resultTaskList.innerHTML = `<div class="empty-row">暂无任务结果</div>`;
    importResultsBtn.disabled = true;
    return;
  }

  importResultsBtn.disabled = selectedCount === 0;
  resultTaskList.innerHTML = visibleJobs.map((job, index) => renderResultTask(job, index)).join("");
}

function hasActiveResultFilters() {
  const filter = buildResultFilter();
  return Boolean(filter.query || filter.category || filter.has_phone || filter.has_email || filter.has_website || filter.not_imported);
}

function renderResultTask(job, index) {
  const isExpanded = state.expandedJobs.has(job.id);
  const isSelected = state.selectedResultJobs.has(job.id);
  const loadedResults = state.expandedResults[job.id] || [];
  const resultCount = hasActiveResultFilters() ? countMatchingResults(job.id) : Number(job.imported_count || 0);
  return `
    <section class="result-task ${isExpanded ? "result-task--expanded" : ""}" data-job-id="${esc(job.id)}">
      <label class="result-task__select" title="选择此任务用于导入">
        <input type="checkbox" data-select-result-job="${esc(job.id)}" ${isSelected ? "checked" : ""}>
      </label>
      <button class="result-task__header" type="button" data-toggle-job="${esc(job.id)}">
        <span class="result-task__chevron">${isExpanded ? "⌃" : "⌄"}</span>
        <span class="result-task__title">任务${index + 1}：${esc(job.name)}</span>
        <span class="result-task__meta">${esc(statusLabel(job.status))} · ${esc(job.location || "")} · ${resultCount} 条</span>
      </button>
      <button class="icon-btn icon-btn--danger result-task__delete" type="button" data-delete-job="${esc(job.id)}" title="删除任务">×</button>
      ${isExpanded ? renderResultsTable(loadedResults) : ""}
    </section>
  `;
}

function countMatchingResults(jobID) {
  return state.results.filter((item) => item.job_id === jobID).length;
}

function selectedVisibleResultJobIDs() {
  return [...state.selectedResultJobs].filter((jobID) => state.visibleResultJobs.has(jobID));
}

function renderResultsTable(results) {
  if (!results.length) {
    return `<div class="empty-row">此任务暂无匹配结果</div>`;
  }
  return `
    <div class="table-wrap">
      <table class="results-table">
        <thead>
          <tr>
            <th>商家</th>
            <th>电话</th>
            <th>分类</th>
            <th>评分</th>
            <th>邮箱</th>
            <th>官网</th>
            <th>谷歌地图</th>
          </tr>
        </thead>
        <tbody>
          ${results.map((item) => `
            <tr>
              <td>
                <div class="table-title">${esc(item.shop_name || "")}</div>
                <div class="table-subtitle">${esc(item.address || "")}</div>
              </td>
              <td>${esc(item.phone || "")}</td>
              <td>${esc(item.category || "")}</td>
              <td>${esc(item.rating || "")}</td>
              <td>${esc(item.email || "")}</td>
              <td>${item.website ? `<button class="text-link" type="button" data-result-id="${Number(item.id)}" data-url-kind="website">打开</button>` : ""}</td>
              <td>${item.map_url ? `<button class="text-link" type="button" data-result-id="${Number(item.id)}" data-url-kind="map">地图</button>` : ""}</td>
            </tr>
          `).join("")}
        </tbody>
      </table>
    </div>
  `;
}

resultTaskList?.addEventListener("click", async (e) => {
  const selectInput = e.target.closest("[data-select-result-job]");
  if (selectInput) {
    e.stopPropagation();
    const jobID = selectInput.dataset.selectResultJob;
    if (selectInput.checked) {
      state.selectedResultJobs.add(jobID);
    } else {
      state.selectedResultJobs.delete(jobID);
    }
    renderResults();
    return;
  }

  const deleteButton = e.target.closest("[data-delete-job]");
  if (deleteButton) {
    e.preventDefault();
    e.stopPropagation();
    await deleteJob(deleteButton.dataset.deleteJob, deleteButton);
    return;
  }

  const toggleButton = e.target.closest("[data-toggle-job]");
  if (toggleButton) {
    e.preventDefault();
    e.stopPropagation();
    const jobID = toggleButton.dataset.toggleJob;
    if (state.expandedJobs.has(jobID)) {
      state.expandedJobs.delete(jobID);
      delete state.expandedResults[jobID];
    } else {
      state.expandedJobs.add(jobID);
      state.expandedResults[jobID] = await go.ResultsSearch(buildResultFilterForJob(jobID));
    }
    renderResults();
    return;
  }

  const button = e.target.closest("[data-result-id][data-url-kind]");
  if (!button) return;
  e.preventDefault();
  e.stopPropagation();

  const resultID = Number(button.dataset.resultId || 0);
  const expandedRows = Object.values(state.expandedResults).flat();
  const item = [...state.results, ...expandedRows].find((result) => Number(result.id) === resultID);
  if (!item) {
    console.warn("结果未找到, resultID:", resultID);
    return;
  }
  const rawURL = button.dataset.urlKind === "map" ? item.map_url : item.website;
  const url = normalizeExternalURL(rawURL || "");
  if (!url) {
    console.warn("URL 为空, kind:", button.dataset.urlKind, "rawURL:", rawURL);
    return;
  }
  try {
    await go.OpenExternalURL(url);
  } catch (err) {
    console.error("打开网址失败:", err);
    alert("打开网址失败: " + err);
  }
});

async function deleteJob(jobID, button) {
  if (!jobID) return;
  if (button) {
    button.disabled = true;
    button.textContent = "…";
  }
  try {
    await go.ScraperDeleteJob(jobID);
    state.expandedJobs.delete(jobID);
    delete state.expandedResults[jobID];
    state.selectedResultJobs.delete(jobID);
    state.initializedResultJobs.delete(jobID);
    state.visibleResultJobs.delete(jobID);
    await refreshJobs();
  } catch (e) {
    if (button) {
      button.disabled = false;
      button.textContent = "×";
    }
    alert("删除失败: " + e);
  }
}

importResultsBtn?.addEventListener("click", async () => {
  try {
    const jobIDs = selectedVisibleResultJobIDs();
    if (!jobIDs.length) {
      alert("请先勾选要导入的任务");
      return;
    }
    const data = await go.ResultsImportToWhatsApp(buildResultFilterForJobs(jobIDs));
    applyContactsResult(data);
    switchTab("whatsappTab");
    await searchResults();
  } catch (e) {
    alert("导入失败: " + e);
  }
});

/* ── Settings ───────────────────────────────────────── */
checkSettingsBtn?.addEventListener("click", checkSettings);
installBrowsersBtn?.addEventListener("click", async () => {
  installBrowsersBtn.disabled = true;
  try {
    await go.SettingsInstallBrowsers();
    alert("浏览器环境已安装/修复");
  } catch (e) {
    alert("安装失败: " + e);
  } finally {
    installBrowsersBtn.disabled = false;
    await checkSettings();
  }
});
replaceCitiesBtn?.addEventListener("click", async () => {
  try {
    const data = await go.SettingsSelectCitiesDB();
    if (data) renderSettings({ cities_db: data, scraper_ok: true });
  } catch (e) {
    alert("替换失败: " + e);
  }
});

async function checkSettings() {
  try {
    const data = await go.SettingsCheckEnvironment();
    renderSettings(data);
  } catch (e) {
    settingsStatus.innerHTML = `<dt>检测失败</dt><dd>${esc(String(e))}</dd>`;
  }
}

function renderSettings(data) {
  const city = data.cities_db || {};
  settingsStatus.innerHTML = `
    <dt>数据目录</dt><dd>${esc(data.data_folder || "")}</dd>
    <dt>国家库</dt><dd>${city.error ? esc(city.error) : `${Number(city.country_count || 0)} 个国家 / ${Number(city.city_count || 0)} 个城市`}</dd>
    <dt>cities.db</dt><dd>${esc(city.path || "")}</dd>
    <dt>爬虫后端</dt><dd>${data.scraper_ok ? "已集成" : "初始化异常"}</dd>
  `;
}

/* ── Model Config ─────────────────────────────────── */
const modelApiKeyInput = document.getElementById("modelApiKey");
const modelBaseUrlInput = document.getElementById("modelBaseUrl");
const modelModelInput = document.getElementById("modelModel");
const saveModelConfigBtn = document.getElementById("saveModelConfigBtn");
const modelConfigStatus = document.getElementById("modelConfigStatus");

saveModelConfigBtn?.addEventListener("click", saveModelConfig);

async function loadModelConfig() {
  try {
    const cfg = await go.SettingsGetModelConfig();
    if (modelApiKeyInput && cfg.api_key) modelApiKeyInput.value = cfg.api_key;
    if (modelBaseUrlInput) modelBaseUrlInput.value = cfg.base_url || "https://api.deepseek.com";
    if (modelModelInput) modelModelInput.value = cfg.model || "deepseek-chat";
  } catch {
    // use defaults
  }
}

async function saveModelConfig() {
  const cfg = {
    api_key: modelApiKeyInput ? modelApiKeyInput.value.trim() : "",
    base_url: modelBaseUrlInput ? modelBaseUrlInput.value.trim() : "",
    model: modelModelInput ? modelModelInput.value.trim() : "",
  };
  if (!cfg.api_key) {
    if (modelConfigStatus) modelConfigStatus.textContent = "API Key 不能为空";
    return;
  }
  try {
    await go.SettingsSaveModelConfig(cfg);
    if (modelConfigStatus) modelConfigStatus.textContent = "已保存";
    setTimeout(() => { if (modelConfigStatus) modelConfigStatus.textContent = ""; }, 3000);
  } catch (e) {
    if (modelConfigStatus) modelConfigStatus.textContent = "保存失败: " + e;
  }
}

async function getSavedModelConfig() {
  try {
    return await go.SettingsGetModelConfig();
  } catch {
    return null;
  }
}

/* ── Login ───────────────────────────────────────────── */
function updateLoginUI(loggedIn) {
  state.loggedIn = loggedIn;
  statusDot.classList.toggle("status-dot--online", loggedIn);
  statusText.textContent = loggedIn ? "已登录" : "未登录";
  loginBtn.disabled = loggedIn;
  logoutBtn.disabled = !loggedIn;
  updateSendBtn();
}

async function pollLoginStatus(options = {}) {
  try {
    const data = await go.WhatsAppLoginStatus();
    const loggedIn = Boolean(data.logged_in);
    updateLoginUI(loggedIn);
    if (loggedIn) {
      stopLoginPolling();
    } else if (options.waiting) {
      statusText.textContent = "等待扫码...";
      loginBtn.disabled = true;
    }
  } catch {}
}

function stopLoginPolling() {
  if (state.loginPollTimer) {
    clearInterval(state.loginPollTimer);
    state.loginPollTimer = null;
  }
}

loginBtn.addEventListener("click", async () => {
  stopLoginPolling();
  loginBtn.disabled = true;
  statusText.textContent = "启动浏览器...";
  try {
    const data = await go.WhatsAppLoginStart();
    updateLoginUI(Boolean(data?.logged_in));
    if (!data?.logged_in) {
      statusText.textContent = "等待扫码...";
      await pollLoginStatus({ waiting: true });
      if (!state.loggedIn) {
        state.loginPollTimer = setInterval(() => pollLoginStatus({ waiting: true }), 2000);
      }
    }
  } catch (e) {
    stopLoginPolling();
    alert("启动失败: " + e);
    loginBtn.disabled = false;
    statusText.textContent = "未连接";
  }
});

logoutBtn.addEventListener("click", async () => {
  try {
    stopLoginPolling();
    await go.WhatsAppLogout();
    updateLoginUI(false);
  } catch (e) {
    alert("登出失败: " + e);
  }
});

/* ── CSV Upload ──────────────────────────────────────── */
csvDropZone.addEventListener("click", async (e) => {
  e.preventDefault();
  await selectCSVFile();
});

// Label click triggers zone click
csvFileInput.parentElement.addEventListener("click", (e) => {
  e.preventDefault();
  e.stopPropagation();
  csvDropZone.click();
});

csvFileInput.addEventListener("change", async () => {
  const file = csvFileInput.files[0];
  if (file) await uploadCSVFile(file);
  csvFileInput.value = "";
});

// Drag and drop
csvDropZone.addEventListener("dragover", (e) => {
  e.preventDefault();
  csvDropZone.classList.add("drop-zone--active");
});
csvDropZone.addEventListener("dragleave", () => {
  csvDropZone.classList.remove("drop-zone--active");
});
csvDropZone.addEventListener("drop", async (e) => {
  e.preventDefault();
  csvDropZone.classList.remove("drop-zone--active");
  const file = e.dataTransfer.files[0];
  if (!file) return;
  await uploadCSVFile(file);
});

if (window.runtime?.OnFileDrop) {
  window.runtime.OnFileDrop(async (x, y, paths) => {
    const target = document.elementFromPoint(x, y);
    if (!target || !csvDropZone.contains(target)) return;
    const path = paths.find((value) => value.toLowerCase().endsWith(".csv"));
    if (!path) {
      alert("请拖入 CSV 文件");
      return;
    }
    await uploadCSVPath(path);
  }, false);
}

async function selectCSVFile() {
  csvDropZone.classList.add("drop-zone--active");
  try {
    const data = await go.WhatsAppSelectContactsFile();
    applyContactsResult(data);
  } catch (e) {
    alert("CSV 上传失败: " + e);
  } finally {
    csvDropZone.classList.remove("drop-zone--active");
  }
}

async function uploadCSVFile(file) {
  try {
    const buffer = await file.arrayBuffer();
    const data = await go.WhatsAppUploadContacts(Array.from(new Uint8Array(buffer)));
    applyContactsResult(data);
  } catch (e) {
    alert("CSV 上传失败: " + e);
  }
}

async function uploadCSVPath(path) {
  csvDropZone.classList.add("drop-zone--active");
  try {
    const data = await go.WhatsAppUploadContactsFile(path);
    applyContactsResult(data);
  } catch (e) {
    alert("CSV 上传失败: " + e);
  } finally {
    csvDropZone.classList.remove("drop-zone--active");
  }
}

function applyContactsResult(data) {
  if (!data) return;
  if (data.error) {
    alert("CSV 上传失败: " + data.error);
    return;
  }
  if (!data.contacts || data.contacts.length === 0) {
    alert("CSV 解析结果为空");
    return;
  }
  state.contacts = data.contacts;
  renderContacts();
}

/* ── Contacts ────────────────────────────────────────── */
function renderContacts() {
  contactControls.style.display = state.contacts.length ? "" : "none";
  const selected = state.contacts.filter((c) => c.selected).length;
  contactCount.textContent = `${selected}/${state.contacts.length} 个联系人`;
  contactList.innerHTML = "";
  for (const c of state.contacts) {
    const li = document.createElement("li");
    li.className = "contact-item";
    li.innerHTML = `
      <input type="checkbox" data-id="${c.id}" ${c.selected ? "checked" : ""}>
      <div class="contact-item__info">
        <div class="contact-item__name">${esc(c.shop_name || c.phone)}</div>
        <div class="contact-item__phone">${esc(c.phone)}</div>
      </div>
      ${c.category ? `<span class="contact-item__category">${esc(c.category)}</span>` : ""}
    `;
    const cb = li.querySelector("input");
    cb.addEventListener("change", () => {
      c.selected = cb.checked;
      updateContactCount();
      updateSendBtn();
    });
    contactList.appendChild(li);
  }
  updateSendBtn();
}

function updateContactCount() {
  const selected = state.contacts.filter((c) => c.selected).length;
  contactCount.textContent = `${selected}/${state.contacts.length} 个联系人`;
}

selectAllBtn.addEventListener("click", () => { state.contacts.forEach((c) => (c.selected = true)); renderContacts(); });
deselectAllBtn.addEventListener("click", () => { state.contacts.forEach((c) => (c.selected = false)); renderContacts(); });
invertBtn.addEventListener("click", () => { state.contacts.forEach((c) => (c.selected = !c.selected)); renderContacts(); });

/* ── Message Cards ───────────────────────────────────── */
function addMessage() {
  state.messages.push({ id: nextMsgId(), text: "", image_id: null, image_name: null, pdf_id: null, pdf_name: null });
  renderMessages();
}
addMsgBtn.addEventListener("click", addMessage);

function buildAttachmentHtml(msg) {
  const parts = [];
  if (msg.image_id) {
    parts.push(`<div class="file-upload-zone file-upload-zone--has-file" data-kind="image">
      <button class="file-upload-zone__remove" data-msg-id="${msg.id}" data-remove-kind="image" title="删除图片">&times;</button>
      <div class="file-upload-zone__label">图片已选</div>
      <div class="file-upload-zone__preview"><span class="file-name">${esc(msg.image_name)}</span></div>
    </div>`);
  } else {
    parts.push(`<div class="file-upload-zone" data-kind="image"><div class="file-upload-zone__label">图片</div></div>`);
  }
  if (msg.pdf_id) {
    parts.push(`<div class="file-upload-zone file-upload-zone--has-file" data-kind="pdf">
      <button class="file-upload-zone__remove" data-msg-id="${msg.id}" data-remove-kind="pdf" title="删除PDF">&times;</button>
      <div class="file-upload-zone__label">PDF 已选</div>
      <div class="file-upload-zone__preview"><span class="file-name">${esc(msg.pdf_name)}</span></div>
    </div>`);
  } else {
    parts.push(`<div class="file-upload-zone" data-kind="pdf"><div class="file-upload-zone__label">PDF</div></div>`);
  }
  return parts.join("");
}

function removeMessage(id) {
  state.messages = state.messages.filter((m) => m.id !== id);
  renderMessages();
}

let filePollTimer = null;

function renderMessages() {
  messageCards.innerHTML = "";
  state.messages.forEach((msg, idx) => {
    const card = document.createElement("div");
    card.className = "msg-card";
    card.draggable = true;
    card.dataset.msgId = msg.id;
    card.innerHTML = `
      <div class="msg-card__header">
        <span class="msg-card__label">消息 ${idx + 1}</span>
        <button class="msg-card__delete" data-msg-id="${msg.id}">&times;</button>
      </div>
      <textarea placeholder="输入消息内容">${esc(msg.text)}</textarea>
      <div class="msg-card__attachments">${buildAttachmentHtml(msg)}</div>
    `;

    card.querySelector("textarea").addEventListener("input", (e) => { msg.text = e.target.value; });
    card.querySelector(".msg-card__delete").addEventListener("click", () => { removeMessage(msg.id); });

    card.querySelectorAll(".file-upload-zone__remove").forEach((btn) => {
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        e.preventDefault();
        const kind = btn.dataset.removeKind;
        if (kind === "image") { msg.image_id = null; msg.image_name = null; }
        else { msg.pdf_id = null; msg.pdf_name = null; }
        renderMessages();
      });
    });

    card.querySelectorAll(".file-upload-zone").forEach((zone) => {
      const kind = zone.dataset.kind;
      if ((kind === "image" && msg.image_id) || (kind === "pdf" && msg.pdf_id)) return;
      zone.style.cursor = "pointer";
      zone.addEventListener("click", (e) => {
        e.stopPropagation();
        go.OpenFileDialogForAttachment(kind, msg.id);
        startFilePoll();
      });
    });

    card.addEventListener("dragstart", (e) => { card.classList.add("msg-card--dragging"); e.dataTransfer.setData("text/plain", String(msg.id)); });
    card.addEventListener("dragend", () => { card.classList.remove("msg-card--dragging"); });
    card.addEventListener("dragover", (e) => {
      e.preventDefault();
      const dragging = messageCards.querySelector(".msg-card--dragging");
      if (dragging && dragging !== card) {
        const rect = card.getBoundingClientRect();
        const mid = rect.top + rect.height / 2;
        messageCards.insertBefore(dragging, e.clientY < mid ? card : card.nextSibling);
      }
    });
    card.addEventListener("drop", (e) => { e.preventDefault(); syncMessageOrder(); });
    messageCards.appendChild(card);
  });
  updateSendBtn();
}

function startFilePoll() {
  if (filePollTimer) clearInterval(filePollTimer);
  filePollTimer = setInterval(async () => {
    try {
      const data = await go.PollFileResult();
      if (!data) return;
      clearInterval(filePollTimer);
      filePollTimer = null;
      if (data.error) { alert("上传失败: " + data.error); return; }
      const msg = state.messages.find((m) => m.id === data.msg_id);
      if (!msg) return;
      if (data.kind === "image") { msg.image_id = data.id; msg.image_name = data.name; }
      else { msg.pdf_id = data.id; msg.pdf_name = data.name; }
      renderMessages();
    } catch (e) {
      clearInterval(filePollTimer);
      filePollTimer = null;
    }
  }, 500);
}

function syncMessageOrder() {
  const cards = messageCards.querySelectorAll(".msg-card");
  const map = new Map(state.messages.map((m) => [m.id, m]));
  state.messages = Array.from(cards).map((c) => parseInt(c.dataset.msgId)).map((id) => map.get(id)).filter(Boolean);
}

/* ── Send ────────────────────────────────────────────── */
function updateSendBtn() {
  const hasContacts = state.contacts.some((c) => c.selected);
  const hasContent = state.messages.some((m) => m.text.trim() || m.image_id || m.pdf_id);
  sendBtn.disabled = !state.loggedIn || !hasContacts || !hasContent || state.sending;
  stopBtn.disabled = !state.sending;
}

sendBtn.addEventListener("click", async () => {
  const contact_ids = state.contacts.filter((c) => c.selected).map((c) => c.id);
  const messages = state.messages.filter((m) => m.text.trim() || m.image_id || m.pdf_id).map((m) => ({ text: m.text, image_id: m.image_id, pdf_id: m.pdf_id }));
  try {
    state.sending = true;
    updateSendBtn();
    startProgressListener();
    await go.WhatsAppSend(contact_ids, messages, buildSendOptions());
  } catch (e) {
    state.sending = false;
    updateSendBtn();
    alert("发送失败: " + e);
  }
});

stopBtn.addEventListener("click", async () => {
  try { await go.WhatsAppStop(); } catch (e) { alert("停止失败: " + e); }
});

function readNumberInput(input, fallback) {
  const value = Number(input?.value || 0);
  return Number.isFinite(value) && value > 0 ? Math.floor(value) : fallback;
}

function buildSendOptions() {
  return {
    contact_delay_min_seconds: readNumberInput(contactDelayMinInput, 1),
    contact_delay_max_seconds: readNumberInput(contactDelayMaxInput, 5),
    batch_size: readNumberInput(batchSizeInput, 20),
    batch_delay_min_seconds: readNumberInput(batchDelayMinInput, 5),
    batch_delay_max_seconds: readNumberInput(batchDelayMaxInput, 10),
    max_consecutive_failures: readNumberInput(maxFailuresInput, 5),
  };
}

/* ── Progress ────────────────────────────────────────── */
let cancelProgress = null;
function startProgressListener() {
  progressPanel.style.display = "";
  progressList.innerHTML = "";
  progressBarFill.style.width = "0%";
  progressCounter.textContent = "0/0";
  if (cancelProgress) cancelProgress();
  cancelProgress = window.runtime.EventsOn("whatsapp:progress", (data) => {
    if (data.type === "progress" || data.type === "error") {
      progressCounter.textContent = `${data.current}/${data.total}`;
      progressBarFill.style.width = `${(data.current / data.total) * 100}%`;
      const li = document.createElement("li");
      li.className = `progress-item progress-item--${data.type === "progress" ? "success" : "error"}`;
      li.innerHTML = `<span class="progress-item__icon">${data.type === "progress" ? "✓" : "✗"}</span><span>${esc(data.contact)}</span>${data.type === "error" ? `<span>— ${esc(data.error)}</span>` : ""}`;
      progressList.appendChild(li);
      progressList.scrollTop = progressList.scrollHeight;
    }
    if (data.type === "wait") {
      progressCounter.textContent = `${data.current}/${data.total}`;
      progressBarFill.style.width = `${(data.current / data.total) * 100}%`;
      const li = document.createElement("li");
      li.className = "progress-item progress-item--wait";
      li.innerHTML = `<span class="progress-item__icon">…</span><span>${esc(data.reason || "等待")}</span><span>${Number(data.seconds || 0)} 秒</span>`;
      progressList.appendChild(li);
      progressList.scrollTop = progressList.scrollHeight;
    }
    if (data.type === "paused") {
      progressCounter.textContent = `已暂停 ${data.success} 成功 / ${data.failed} 失败`;
      progressBarFill.style.width = `${data.total ? (data.current / data.total) * 100 : 0}%`;
      const li = document.createElement("li");
      li.className = "progress-item progress-item--error";
      li.innerHTML = `<span class="progress-item__icon">!</span><span>${esc(data.reason || "发送已暂停")}</span>`;
      progressList.appendChild(li);
      progressList.scrollTop = progressList.scrollHeight;
      state.sending = false;
      updateSendBtn();
    }
    if (data.type === "complete") {
      progressCounter.textContent = `完成 ${data.success} 成功 / ${data.failed} 失败`;
      progressBarFill.style.width = "100%";
      state.sending = false;
      updateSendBtn();
    }
  });
}

/* ── Utilities ───────────────────────────────────────── */
function esc(str) {
  if (!str) return "";
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

function normalizeExternalURL(url) {
  const value = String(url || "").trim();
  if (!value) return "";
  if (/^https?:\/\//i.test(value)) return value;
  return `https://${value}`;
}

/* ── Agent State & DOM Refs ──────────────────────────── */
const agentState = {
  running: false,
  conversations: [],
  selectedPhone: "",
  takeoverMode: false,
  statsInterval: null,
};

function agentDom() {
  return {
    startBtn: document.getElementById("agentStartBtn"),
    stopBtn: document.getElementById("agentStopBtn"),
    statusDot: document.getElementById("agentStatusDot"),
    statusText: document.getElementById("agentStatusText"),
    convList: document.getElementById("agentConvList"),
    chatMessages: document.getElementById("agentChatMessages"),
    chatInput: document.getElementById("agentChatInput"),
    sendBtn: document.getElementById("agentChatSendBtn"),
    statsBar: document.getElementById("agentStatsBar"),
    statsMsg: document.getElementById("agentStatsMsg"),
    statsLatency: document.getElementById("agentStatsLatency"),
    statsTokens: document.getElementById("agentStatsTokens"),
    statsDocs: document.getElementById("agentStatsDocs"),
    toolbar: document.getElementById("agentChatToolbar"),
    chatPhone: document.getElementById("agentChatPhone"),
    takeoverBtn: document.getElementById("agentTakeoverBtn"),
    autoResumeBtn: document.getElementById("agentAutoResumeBtn"),
    uploadDocBtn: document.getElementById("agentUploadDocBtn"),
  };
}

/* ── Agent Tab Init ─────────────────────────────────── */
function initAgentTab() {
  const dom = agentDom();
  if (dom.startBtn) dom.startBtn.addEventListener("click", agentStart);
  if (dom.stopBtn) dom.stopBtn.addEventListener("click", agentStop);
  if (dom.sendBtn) dom.sendBtn.addEventListener("click", agentSendChat);
  if (dom.takeoverBtn) dom.takeoverBtn.addEventListener("click", agentToggleTakeover);
  if (dom.autoResumeBtn) dom.autoResumeBtn.addEventListener("click", agentToggleTakeover);
  if (dom.uploadDocBtn) dom.uploadDocBtn.addEventListener("click", agentUploadDoc);
  if (dom.chatInput) {
    dom.chatInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        agentSendChat();
      }
    });
  }

  // Quick command buttons
  document.querySelectorAll(".agent-cmd").forEach((btn) => {
    btn.addEventListener("click", () => {
      const cmd = btn.dataset.cmd;
      if (cmd) {
        const dom = agentDom();
        if (dom.chatInput) dom.chatInput.value = cmd;
        agentSendChat();
      }
    });
  });

  // Subscribe to Wails runtime events
  if (window.runtime) {
    window.runtime.EventsOn("agent:message", (data) => {
      appendAgentMessage(data);
      loadAgentConversations();
    });
    window.runtime.EventsOn("agent:status", (data) => {
      agentState.running = data.running === true;
      updateAgentUI();
    });
    window.runtime.EventsOn("agent:stats", (data) => {
      renderAgentStats(data);
    });
  }

  updateAgentUI();
  loadAgentConversations();
  loadAgentStats();
  loadAgentDocsCount();
}

async function agentStart() {
  // Load saved config first
  const cfg = await getSavedModelConfig();
  if (!cfg || !cfg.api_key) {
    alert("请先在「设置」页面配置 API Key");
    return;
  }
  try {
    await go.AgentStart({
      deepseek_api_key: cfg.api_key,
      deepseek_base_url: cfg.base_url || "",
      model: cfg.model || "",
    });
    agentState.running = true;
    updateAgentUI();
    if (agentState.statsInterval) clearInterval(agentState.statsInterval);
    agentState.statsInterval = setInterval(loadAgentStats, 60000);
  } catch (err) {
    alert("启动失败: " + err);
  }
}

async function agentStop() {
  try {
    await go.AgentStop();
    agentState.running = false;
    if (agentState.statsInterval) {
      clearInterval(agentState.statsInterval);
      agentState.statsInterval = null;
    }
    updateAgentUI();
  } catch (err) {
    alert("停止失败: " + err);
  }
}

function updateAgentUI() {
  const dom = agentDom();
  if (dom.startBtn) dom.startBtn.disabled = agentState.running;
  if (dom.stopBtn) dom.stopBtn.disabled = !agentState.running;
  if (dom.statusDot) {
    dom.statusDot.className = agentState.running
      ? "status-dot status-dot--active"
      : "status-dot";
  }
  if (dom.statusText) {
    dom.statusText.textContent = agentState.running ? "运行中" : "未启动";
  }
  if (dom.chatInput) dom.chatInput.disabled = !agentState.running;
  if (dom.sendBtn) dom.sendBtn.disabled = !agentState.running;
}

async function loadAgentConversations() {
  try {
    const convs = await go.AgentConversations();
    agentState.conversations = convs || [];
    renderAgentConvList();
  } catch {
    agentState.conversations = [];
    renderAgentConvList();
  }
}

function renderAgentConvList() {
  const dom = agentDom();
  if (!dom.convList) return;
  if (!agentState.conversations.length) {
    dom.convList.innerHTML =
      '<li class="agent-empty">暂无对话</li>';
    return;
  }
  dom.convList.innerHTML = agentState.conversations
    .map(
      (c) =>
        `<li class="agent-conv-item ${c.phone === agentState.selectedPhone ? "active" : ""}" data-phone="${esc(c.phone)}">
          <div class="agent-conv-name">${esc(c.name || c.phone)}</div>
          <div class="agent-conv-time">${formatTimestamp(c.updated_at)}</div>
        </li>`
    )
    .join("");
  dom.convList.querySelectorAll(".agent-conv-item").forEach((el) => {
    el.addEventListener("click", () =>
      selectAgentConversation(el.dataset.phone)
    );
  });
}

async function selectAgentConversation(phone) {
  agentState.selectedPhone = phone;
  agentState.takeoverMode = false;
  renderAgentConvList();
  const dom = agentDom();
  if (dom.chatMessages) dom.chatMessages.innerHTML = "";
  if (dom.toolbar) {
    dom.toolbar.style.display = "flex";
    if (dom.chatPhone) dom.chatPhone.textContent = phone;
    if (dom.takeoverBtn) dom.takeoverBtn.style.display = "";
    if (dom.autoResumeBtn) dom.autoResumeBtn.style.display = "none";
  }
  try {
    const msgs = await go.AgentMessages(phone, 50);
    (msgs || []).reverse().forEach((m) => appendAgentMessage(m));
  } catch {
    // no messages yet
  }
  if (dom.chatMessages) {
    dom.chatMessages.scrollTop = dom.chatMessages.scrollHeight;
  }
}

async function agentToggleTakeover() {
  agentState.takeoverMode = !agentState.takeoverMode;
  const dom = agentDom();
  if (dom.takeoverBtn) dom.takeoverBtn.style.display = agentState.takeoverMode ? "none" : "";
  if (dom.autoResumeBtn) dom.autoResumeBtn.style.display = agentState.takeoverMode ? "" : "none";

  if (agentState.selectedPhone) {
    const status = agentState.takeoverMode ? "paused" : "active";
    try {
      await go.AgentSetConversationStatus(agentState.selectedPhone, status);
    } catch {
      // ignore
    }
  }
}

function appendAgentMessage(msg) {
  const dom = agentDom();
  if (!dom.chatMessages) return;
  const isUser = msg.direction === "inbound";
  const cls = isUser ? "agent-msg inbound" : "agent-msg outbound";
  const html = `<div class="${cls}">
    <div class="agent-msg-text">${esc(msg.content)}</div>
    <div class="agent-msg-time">${formatTimestamp(msg.created_at)}</div>
  </div>`;
  dom.chatMessages.insertAdjacentHTML("beforeend", html);
  dom.chatMessages.scrollTop = dom.chatMessages.scrollHeight;
}

async function agentSendChat() {
  const dom = agentDom();
  if (!dom.chatInput) return;
  const text = dom.chatInput.value.trim();
  if (!text) return;
  dom.chatInput.value = "";

  // If a conversation is selected and in takeover mode, send via WhatsApp
  if (agentState.selectedPhone && agentState.takeoverMode) {
    appendAgentMessage({
      direction: "outbound",
      content: text,
      created_at: Math.floor(Date.now() / 1000),
    });
    try {
      await go.AgentSendManual(agentState.selectedPhone, text);
    } catch (err) {
      appendAgentMessage({
        direction: "inbound",
        content: "[发送失败] " + err,
        created_at: Math.floor(Date.now() / 1000),
      });
    }
    return;
  }

  // Otherwise, chat directly with the agent
  appendAgentMessage({
    direction: "outbound",
    content: text,
    created_at: Math.floor(Date.now() / 1000),
  });
  try {
    const reply = await go.AgentChat(text);
    appendAgentMessage({
      direction: "inbound",
      content: reply,
      created_at: Math.floor(Date.now() / 1000),
    });
  } catch (err) {
    appendAgentMessage({
      direction: "inbound",
      content: "[错误] " + err,
      created_at: Math.floor(Date.now() / 1000),
    });
  }
}

async function agentUploadDoc() {
  // Wails doesn't have a direct file dialog binding, so we prompt for path
  const path = prompt("请输入文档路径（支持 .pdf, .docx, .txt, .md）：");
  if (!path) return;
  try {
    const result = await go.AgentUploadKnowledge(path);
    alert(`上传成功！文档: ${result.filename}, 分块数: ${result.chunk_count}`);
    loadAgentDocsCount();
  } catch (err) {
    alert("上传失败: " + err);
  }
}

async function loadAgentStats() {
  try {
    const stats = await go.AgentStats();
    renderAgentStats(stats);
  } catch {
    // stats not available yet
  }
}

function renderAgentStats(data) {
  if (!data) return;
  const dom = agentDom();
  if (dom.statsMsg) dom.statsMsg.textContent = `消息: ${data.messages_received || 0} 收 / ${data.replies_sent || 0} 发`;
  if (dom.statsLatency) {
    const ms = data.avg_latency_ms ? Math.round(data.avg_latency_ms) : "--";
    dom.statsLatency.textContent = `平均响应: ${ms}ms`;
  }
  if (dom.statsTokens) dom.statsTokens.textContent = `Token: ${data.total_tokens || 0}`;
}

async function loadAgentDocsCount() {
  try {
    const docs = await go.AgentListKnowledge();
    const count = (docs || []).length;
    const dom = agentDom();
    if (dom.statsDocs) dom.statsDocs.textContent = `知识库: ${count} 文档`;
  } catch {
    // not available
  }
}

function formatTimestamp(ts) {
  if (!ts) return "";
  const d = new Date(ts * 1000);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

/* ── Init ────────────────────────────────────────────── */
(async () => {
  try {
    const data = await go.WhatsAppLoginStatus();
    updateLoginUI(data.logged_in);
  } catch { updateLoginUI(false); }
  await loadCountries();
  await refreshJobs();
  await searchResults();
  await checkSettings();
  addMessage();
  loadModelConfig();
  initAgentTab();
})();
