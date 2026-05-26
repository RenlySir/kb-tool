const TOKEN_KEY = "kbToolApiToken";

const state = {
  authRequired: false,
  username: "admin",
  documents: [],
  selectedId: null,
  selectedTag: "",
  selectedIds: new Set(),
  activePanel: "",
  connections: [],
  currentConnection: null,
  overview: null,
};

const els = {
  loginView: document.querySelector("#loginView"),
  appView: document.querySelector("#appView"),
  loginForm: document.querySelector("#loginForm"),
  usernameInput: document.querySelector("#usernameInput"),
  passwordInput: document.querySelector("#passwordInput"),
  loginMessage: document.querySelector("#loginMessage"),
  sessionUser: document.querySelector("#sessionUser"),
  currentConnectionLabel: document.querySelector("#currentConnectionLabel"),
  logoutBtn: document.querySelector("#logoutBtn"),
  refreshBtn: document.querySelector("#refreshBtn"),
  navItems: document.querySelectorAll(".nav-item"),
  moduleDrawer: document.querySelector("#moduleDrawer"),
  drawerTitle: document.querySelector("#drawerTitle"),
  drawerSubtitle: document.querySelector("#drawerSubtitle"),
  closeDrawerBtn: document.querySelector("#closeDrawerBtn"),
  drawerPanels: document.querySelectorAll(".drawer-panel"),
  connectionList: document.querySelector("#connectionList"),
  connectionForm: document.querySelector("#connectionForm"),
  connectionIdInput: document.querySelector("#connectionIdInput"),
  connectionNameInput: document.querySelector("#connectionNameInput"),
  connectionHostInput: document.querySelector("#connectionHostInput"),
  connectionPortInput: document.querySelector("#connectionPortInput"),
  connectionUserInput: document.querySelector("#connectionUserInput"),
  connectionPasswordInput: document.querySelector("#connectionPasswordInput"),
  connectionDatabaseInput: document.querySelector("#connectionDatabaseInput"),
  testConnectionBtn: document.querySelector("#testConnectionBtn"),
  resetConnectionBtn: document.querySelector("#resetConnectionBtn"),
  connectionMessage: document.querySelector("#connectionMessage"),
  sourceType: document.querySelector("#sourceType"),
  sourceInput: document.querySelector("#sourceInput"),
  ingestBtn: document.querySelector("#ingestBtn"),
  ingestResult: document.querySelector("#ingestResult"),
  batchTagInput: document.querySelector("#batchTagInput"),
  batchTagBtn: document.querySelector("#batchTagBtn"),
  batchTagMessage: document.querySelector("#batchTagMessage"),
  selectionSummary: document.querySelector("#selectionSummary"),
  tagList: document.querySelector("#tagList"),
  clearTagBtn: document.querySelector("#clearTagBtn"),
  searchInput: document.querySelector("#searchInput"),
  searchBtn: document.querySelector("#searchBtn"),
  documentCount: document.querySelector("#documentCount"),
  activeFilter: document.querySelector("#activeFilter"),
  documentList: document.querySelector("#documentList"),
  detailEmpty: document.querySelector("#detailEmpty"),
  detailView: document.querySelector("#detailView"),
  detailTitle: document.querySelector("#detailTitle"),
  detailPath: document.querySelector("#detailPath"),
  detailType: document.querySelector("#detailType"),
  detailTags: document.querySelector("#detailTags"),
  tagInput: document.querySelector("#tagInput"),
  addTagBtn: document.querySelector("#addTagBtn"),
  assetPreview: document.querySelector("#assetPreview"),
  textPreview: document.querySelector("#textPreview"),
  overviewDocumentCount: document.querySelector("#overviewDocumentCount"),
  overviewTagCount: document.querySelector("#overviewTagCount"),
  overviewAssetCount: document.querySelector("#overviewAssetCount"),
  overviewSelectionCount: document.querySelector("#overviewSelectionCount"),
  overviewFilterLabel: document.querySelector("#overviewFilterLabel"),
  overviewSourceTypes: document.querySelector("#overviewSourceTypes"),
  overviewLanguages: document.querySelector("#overviewLanguages"),
  overviewRecentDocuments: document.querySelector("#overviewRecentDocuments"),
};

function storedApiToken() {
  try {
    return localStorage.getItem(TOKEN_KEY) || "";
  } catch (error) {
    return "";
  }
}

function storeApiToken(token) {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
  } catch (error) {
    return;
  }
}

function apiHeaders(options = {}) {
  const headers = {
    "Content-Type": "application/json",
    ...(options.headers || {}),
  };
  const token = storedApiToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  return headers;
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: apiHeaders(options),
  });
  if (response.status === 401) {
    storeApiToken("");
    showLogin("登录已失效，请重新登录。");
    throw new Error("unauthorized");
  }
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || response.statusText);
  }
  return response.json();
}

async function loadSession() {
  const response = await fetch("/api/session");
  if (!response.ok) throw new Error("无法连接服务端");
  const payload = await response.json();
  state.authRequired = Boolean(payload.auth_required);
  state.username = payload.username || "admin";
  els.usernameInput.value = state.username;

  if (state.authRequired && !storedApiToken()) {
    showLogin("");
    return;
  }
  showApp();
  await refresh();
}

async function login(event) {
  event.preventDefault();
  els.loginMessage.textContent = "";
  const username = els.usernameInput.value.trim();
  const password = els.passwordInput.value;
  if (!username || !password) {
    els.loginMessage.textContent = "请输入账号和密码。";
    return;
  }
  try {
    const payload = await fetch("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });
    if (!payload.ok) {
      els.loginMessage.textContent = "账号或密码不正确。";
      return;
    }
    const body = await payload.json();
    storeApiToken(body.token || "");
    state.username = body.username || username;
    els.passwordInput.value = "";
    showApp();
    await refresh();
  } catch (error) {
    els.loginMessage.textContent = "无法连接服务端。";
  }
}

function logout() {
  storeApiToken("");
  state.selectedId = null;
  state.selectedIds.clear();
  state.documents = [];
  if (state.authRequired) showLogin("");
  else refresh().catch(showLoadError);
}

function showLogin(message) {
  els.loginView.classList.remove("hidden");
  els.appView.classList.add("hidden");
  els.loginMessage.textContent = message || "";
}

function showApp() {
  els.loginView.classList.add("hidden");
  els.appView.classList.remove("hidden");
  els.sessionUser.textContent = state.authRequired ? `已登录：${state.username}` : "本地免登录模式";
}

function openPanel(panelId) {
  if (state.activePanel === panelId && panelId !== "overviewPanel") {
    closePanel();
    return;
  }
  state.activePanel = panelId;
  els.moduleDrawer.classList.remove("hidden");
  els.drawerPanels.forEach((panel) => {
    panel.classList.toggle("hidden", panel.id !== panelId);
  });
  els.navItems.forEach((item) => {
    item.classList.toggle("active", item.dataset.panel === panelId);
  });
  const panel = document.querySelector(`#${panelId}`);
  els.drawerTitle.textContent = panel?.dataset.title || "功能面板";
  els.drawerSubtitle.textContent = panel?.dataset.subtitle || "";
  if (panelId === "databasePanel") loadConnections().catch(showConnectionError);
  if (panelId === "overviewPanel") updateOverview();
}

function closePanel() {
  state.activePanel = "";
  els.moduleDrawer.classList.add("hidden");
  els.drawerPanels.forEach((panel) => panel.classList.add("hidden"));
  els.navItems.forEach((item) => item.classList.remove("active"));
}

function assetURL(id) {
  const token = storedApiToken();
  const suffix = token ? `?access_token=${encodeURIComponent(token)}` : "";
  return `/api/documents/${id}/asset${suffix}`;
}

function escapeHtml(value = "") {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function tagHtml(tags = []) {
  return tags.map((tag) => `<span class="tag">${escapeHtml(tag)}</span>`).join("");
}

function parseTags(raw) {
  return raw
    .split(/[,\n，]+/)
    .map((tag) => tag.trim())
    .filter(Boolean);
}

function visibleDocuments() {
  if (!state.selectedTag) return state.documents;
  return state.documents.filter((doc) => (doc.tags || []).includes(state.selectedTag));
}

async function loadDocuments() {
  const q = els.searchInput.value.trim();
  const payload = await api(`/api/documents?q=${encodeURIComponent(q)}&limit=100`);
  state.documents = payload.documents || [];
  const knownIDs = new Set(state.documents.map((doc) => doc.id));
  state.selectedIds = new Set([...state.selectedIds].filter((id) => knownIDs.has(id)));
  renderDocuments();
  updateOverview();
}

async function loadTags() {
  let tags = [];
  try {
    const payload = await api("/api/tags");
    tags = payload.tags || [];
  } catch (error) {
    tags = [];
  }
  els.tagList.innerHTML = tags.length
    ? tags.map((tag) => {
        const active = tag.name === state.selectedTag ? " active" : "";
        return `<button class="tag clickable${active}" data-tag="${escapeHtml(tag.name)}" type="button">${escapeHtml(tag.name)} (${tag.count})</button>`;
      }).join("")
    : `<span class="muted-text">暂无标签</span>`;
  els.tagList.querySelectorAll("[data-tag]").forEach((node) => {
    node.addEventListener("click", () => {
      state.selectedTag = node.dataset.tag;
      renderDocuments();
    });
  });
}

async function loadOverview() {
  const payload = await api("/api/overview");
  state.overview = payload.overview || null;
  updateOverview();
}

async function loadConnections() {
  const payload = await api("/api/connections");
  state.connections = payload.connections || [];
  state.currentConnection = payload.current || null;
  renderConnections();
}

function renderConnections() {
  const current = state.currentConnection;
  els.currentConnectionLabel.textContent = current
    ? `当前数据库：${current.name} (${current.host}:${current.port}/${current.database})`
    : "当前数据库：未选择";
  els.connectionList.innerHTML = state.connections.length
    ? state.connections.map((conn) => {
        const active = conn.active ? " active" : "";
        const passwordState = conn.has_password ? "已配置密码" : "无密码";
        return `
          <article class="connection-row${active}" data-connection-id="${escapeHtml(conn.id)}">
            <button class="connection-main" type="button" data-edit-connection="${escapeHtml(conn.id)}">
              <strong>${escapeHtml(conn.name)}</strong>
              <span>${escapeHtml(conn.host)}:${conn.port} / ${escapeHtml(conn.database)}</span>
              <small>${escapeHtml(conn.user)} · ${passwordState}</small>
            </button>
            <button class="mini-button" type="button" data-activate-connection="${escapeHtml(conn.id)}">${conn.active ? "当前" : "使用"}</button>
          </article>
        `;
      }).join("")
    : `<div class="empty-state compact">暂无连接配置。</div>`;

  els.connectionList.querySelectorAll("[data-edit-connection]").forEach((node) => {
    node.addEventListener("click", () => fillConnectionForm(node.dataset.editConnection));
  });
  els.connectionList.querySelectorAll("[data-activate-connection]").forEach((node) => {
    node.addEventListener("click", () => activateConnection(node.dataset.activateConnection));
  });
}

function fillConnectionForm(id) {
  const conn = state.connections.find((item) => item.id === id);
  if (!conn) return;
  els.connectionIdInput.value = conn.id;
  els.connectionNameInput.value = conn.name || "";
  els.connectionHostInput.value = conn.host || "";
  els.connectionPortInput.value = conn.port || 4000;
  els.connectionUserInput.value = conn.user || "";
  els.connectionPasswordInput.value = "";
  els.connectionDatabaseInput.value = conn.database || "";
  els.connectionMessage.textContent = conn.has_password ? "已加载连接，密码留空将保留原密码。" : "已加载连接。";
}

function resetConnectionForm() {
  els.connectionIdInput.value = "";
  els.connectionNameInput.value = "";
  els.connectionHostInput.value = "";
  els.connectionPortInput.value = "4000";
  els.connectionUserInput.value = "";
  els.connectionPasswordInput.value = "";
  els.connectionDatabaseInput.value = "";
  els.connectionMessage.textContent = "";
}

function connectionPayload() {
  return {
    id: els.connectionIdInput.value.trim(),
    name: els.connectionNameInput.value.trim(),
    host: els.connectionHostInput.value.trim(),
    port: Number(els.connectionPortInput.value || 4000),
    user: els.connectionUserInput.value.trim(),
    password: els.connectionPasswordInput.value,
    database: els.connectionDatabaseInput.value.trim(),
  };
}

async function saveConnection(event) {
  event.preventDefault();
  els.connectionMessage.textContent = "正在保存...";
  try {
    await api("/api/connections", {
      method: "POST",
      body: JSON.stringify(connectionPayload()),
    });
    resetConnectionForm();
    await loadConnections();
    els.connectionMessage.textContent = "连接已保存。";
  } catch (error) {
    if (error.message !== "unauthorized") showConnectionError(error);
  }
}

async function testConnection() {
  els.connectionMessage.textContent = "正在测试连接...";
  try {
    await api("/api/connections/test", {
      method: "POST",
      body: JSON.stringify(connectionPayload()),
    });
    els.connectionMessage.textContent = "连接测试通过。";
  } catch (error) {
    if (error.message !== "unauthorized") showConnectionError(error);
  }
}

async function activateConnection(id) {
  els.connectionMessage.textContent = "正在切换数据库...";
  try {
    await api(`/api/connections/${encodeURIComponent(id)}/activate`, { method: "POST" });
    els.connectionMessage.textContent = "当前数据库已切换。";
    state.selectedId = null;
    state.selectedIds.clear();
    els.detailEmpty.classList.remove("hidden");
    els.detailView.classList.add("hidden");
    await Promise.all([loadConnections(), refresh()]);
  } catch (error) {
    if (error.message !== "unauthorized") showConnectionError(error);
  }
}

function showConnectionError(error) {
  els.connectionMessage.textContent = error.message || "数据库连接操作失败。";
}

function renderDocuments() {
  const docs = visibleDocuments();
  els.documentCount.textContent = `${docs.length} 条内容`;
  els.activeFilter.textContent = state.selectedTag ? `标签：${state.selectedTag}` : "";
  els.selectionSummary.textContent = state.selectedIds.size ? `已选 ${state.selectedIds.size}` : "未选择";
  updateOverview();

  els.documentList.innerHTML = docs.length
    ? docs.map((doc) => {
        const checked = state.selectedIds.has(doc.id) ? "checked" : "";
        const active = doc.id === state.selectedId ? " active" : "";
        const title = escapeHtml(doc.title || doc.path || `Document ${doc.id}`);
        const meta = `${doc.source_type || "file"} · ${doc.mime_type || "text/plain"} · ${doc.path || ""}`;
        return `
          <article class="document-row${active}" data-id="${doc.id}">
            <label class="row-check" title="选择用于批量打标签">
              <input type="checkbox" data-select-id="${doc.id}" ${checked}>
            </label>
            <button class="row-main" type="button" data-open-id="${doc.id}">
              <span class="document-title">${title}</span>
              <span class="document-meta">${escapeHtml(meta)}</span>
              <span class="tag-list">${tagHtml(doc.tags)}</span>
            </button>
          </article>
        `;
      }).join("")
    : `<div class="empty-state">没有匹配的知识内容。</div>`;

  els.documentList.querySelectorAll("[data-open-id]").forEach((node) => {
    node.addEventListener("click", () => selectDocument(Number(node.dataset.openId)));
  });
  els.documentList.querySelectorAll("[data-select-id]").forEach((node) => {
    node.addEventListener("change", () => toggleSelection(Number(node.dataset.selectId), node.checked));
  });
}

function renderCountBars(container, items = []) {
  if (!container) return;
  if (!items.length) {
    container.innerHTML = `<div class="empty-state compact">暂无统计数据。</div>`;
    return;
  }
  const max = Math.max(...items.map((item) => Number(item.count) || 0), 1);
  container.innerHTML = items.slice(0, 6).map((item) => {
    const count = Number(item.count) || 0;
    const width = Math.max(6, Math.round((count / max) * 100));
    return `
      <div class="mini-bar">
        <div class="mini-bar-label">
          <span>${escapeHtml(item.name || "unknown")}</span>
          <strong>${count}</strong>
        </div>
        <div class="mini-bar-track">
          <span style="width:${width}%"></span>
        </div>
      </div>
    `;
  }).join("");
}

function renderRecentDocuments(docs = []) {
  if (!els.overviewRecentDocuments) return;
  if (!docs.length) {
    els.overviewRecentDocuments.innerHTML = `<div class="empty-state compact">暂无最近入库内容。</div>`;
    return;
  }
  els.overviewRecentDocuments.innerHTML = docs.slice(0, 6).map((doc) => {
    const title = doc.title || doc.path || `Document ${doc.id}`;
    const meta = `${doc.source_type || "file"} · ${doc.mime_type || "unknown"}`;
    return `
      <button class="recent-row" type="button" data-recent-id="${doc.id}">
        <span>
          <strong>${escapeHtml(title)}</strong>
          <small>${escapeHtml(meta)}</small>
        </span>
        <span class="tag-list">${tagHtml(doc.tags || [])}</span>
      </button>
    `;
  }).join("");
  els.overviewRecentDocuments.querySelectorAll("[data-recent-id]").forEach((node) => {
    node.addEventListener("click", () => selectDocument(Number(node.dataset.recentId)));
  });
}

function updateOverview() {
  if (!els.overviewDocumentCount) return;
  const overview = state.overview;
  const assetCount = overview ? Number(overview.image_documents || overview.binary_documents || 0) : 0;
  els.overviewDocumentCount.textContent = String(overview?.total_documents ?? visibleDocuments().length);
  els.overviewTagCount.textContent = String(overview?.total_tags ?? 0);
  els.overviewAssetCount.textContent = String(assetCount);
  els.overviewSelectionCount.textContent = String(state.selectedIds.size);
  els.overviewFilterLabel.textContent = state.selectedTag || "全部";
  renderCountBars(els.overviewSourceTypes, overview?.source_types || []);
  renderCountBars(els.overviewLanguages, overview?.languages || []);
  renderRecentDocuments(overview?.recent_documents || []);
}

function toggleSelection(id, selected) {
  if (selected) state.selectedIds.add(id);
  else state.selectedIds.delete(id);
  renderDocuments();
}

async function selectDocument(id) {
  state.selectedId = id;
  const doc = await api(`/api/documents/${id}`);
  els.detailEmpty.classList.add("hidden");
  els.detailView.classList.remove("hidden");
  els.detailTitle.textContent = doc.title || doc.path || `Document ${id}`;
  els.detailPath.textContent = doc.path || "";
  els.detailType.textContent = doc.mime_type || doc.language || doc.source_type || "document";
  els.detailTags.innerHTML = tagHtml(doc.tags || []);
  els.assetPreview.innerHTML = "";

  if (doc.is_binary && (doc.mime_type || "").startsWith("image/")) {
    els.assetPreview.innerHTML = `<img src="${assetURL(doc.id)}" alt="${escapeHtml(doc.title || "image preview")}">`;
    els.textPreview.classList.add("hidden");
    els.textPreview.textContent = "";
  } else {
    els.textPreview.classList.remove("hidden");
    els.textPreview.textContent = doc.content || "";
  }
  renderDocuments();
}

async function ingestSources() {
  const sources = els.sourceInput.value.split(/\n+/).map((line) => line.trim()).filter(Boolean);
  if (!sources.length) {
    els.ingestResult.textContent = "请输入至少一个来源。";
    return;
  }
  els.ingestBtn.disabled = true;
  els.ingestResult.textContent = "正在导入...";
  try {
    const payload = await api("/api/ingest", {
      method: "POST",
      body: JSON.stringify({ source_type: els.sourceType.value, sources }),
    });
    els.ingestResult.textContent = (payload.results || []).map((result) => {
      if (result.error) return `${result.source}: ${result.error}`;
      return `${result.source}: ${result.documents} documents, ${result.tags} tags`;
    }).join("\n") || "没有可导入内容。";
    await refresh();
  } catch (error) {
    if (error.message !== "unauthorized") els.ingestResult.textContent = error.message;
  } finally {
    els.ingestBtn.disabled = false;
  }
}

async function addTags() {
  if (!state.selectedId) return;
  const tags = parseTags(els.tagInput.value);
  if (!tags.length) return;
  await api(`/api/documents/${state.selectedId}/tags`, {
    method: "POST",
    body: JSON.stringify({ tags }),
  });
  els.tagInput.value = "";
  await refresh();
  await selectDocument(state.selectedId);
}

async function addBatchTags() {
  const tags = parseTags(els.batchTagInput.value);
  const documentIDs = [...state.selectedIds];
  if (!documentIDs.length) {
    els.batchTagMessage.textContent = "请先勾选要打标签的内容。";
    return;
  }
  if (!tags.length) {
    els.batchTagMessage.textContent = "请输入至少一个标签。";
    return;
  }
  els.batchTagBtn.disabled = true;
  try {
    await api("/api/documents/tags", {
      method: "POST",
      body: JSON.stringify({ document_ids: documentIDs, tags }),
    });
    els.batchTagMessage.textContent = `已为 ${documentIDs.length} 条内容添加标签。`;
    els.batchTagInput.value = "";
    await refresh();
    if (state.selectedId) await selectDocument(state.selectedId);
  } catch (error) {
    if (error.message !== "unauthorized") els.batchTagMessage.textContent = error.message;
  } finally {
    els.batchTagBtn.disabled = false;
  }
}

async function refresh() {
  await Promise.all([loadDocuments(), loadTags(), loadConnections(), loadOverview()]);
  updateOverview();
}

function showLoadError(error) {
  els.documentList.innerHTML = `<div class="empty-state">${escapeHtml(error.message)}</div>`;
}

els.loginForm.addEventListener("submit", login);
els.logoutBtn.addEventListener("click", logout);
els.refreshBtn.addEventListener("click", () => refresh().catch(showLoadError));
els.navItems.forEach((item) => {
  item.addEventListener("click", () => openPanel(item.dataset.panel));
});
els.closeDrawerBtn.addEventListener("click", closePanel);
els.connectionForm.addEventListener("submit", saveConnection);
els.testConnectionBtn.addEventListener("click", testConnection);
els.resetConnectionBtn.addEventListener("click", resetConnectionForm);
els.searchBtn.addEventListener("click", () => loadDocuments().catch(showLoadError));
els.searchInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") loadDocuments().catch(showLoadError);
});
els.clearTagBtn.addEventListener("click", () => {
  state.selectedTag = "";
  renderDocuments();
});
els.ingestBtn.addEventListener("click", ingestSources);
els.batchTagBtn.addEventListener("click", addBatchTags);
els.addTagBtn.addEventListener("click", () => addTags().catch(showLoadError));

loadSession().catch((error) => {
  showLogin(error.message);
});
