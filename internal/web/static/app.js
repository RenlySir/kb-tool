const TOKEN_KEY = "kbToolApiToken";

const state = {
  authRequired: false,
  username: "admin",
  documents: [],
  selectedId: null,
  selectedTag: "",
  selectedIds: new Set(),
};

const els = {
  loginView: document.querySelector("#loginView"),
  appView: document.querySelector("#appView"),
  loginForm: document.querySelector("#loginForm"),
  usernameInput: document.querySelector("#usernameInput"),
  passwordInput: document.querySelector("#passwordInput"),
  loginMessage: document.querySelector("#loginMessage"),
  sessionUser: document.querySelector("#sessionUser"),
  logoutBtn: document.querySelector("#logoutBtn"),
  refreshBtn: document.querySelector("#refreshBtn"),
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

function renderDocuments() {
  const docs = visibleDocuments();
  els.documentCount.textContent = `${docs.length} 条内容`;
  els.activeFilter.textContent = state.selectedTag ? `标签：${state.selectedTag}` : "";
  els.selectionSummary.textContent = state.selectedIds.size ? `已选 ${state.selectedIds.size}` : "未选择";

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
  await Promise.all([loadDocuments(), loadTags()]);
}

function showLoadError(error) {
  els.documentList.innerHTML = `<div class="empty-state">${escapeHtml(error.message)}</div>`;
}

els.loginForm.addEventListener("submit", login);
els.logoutBtn.addEventListener("click", logout);
els.refreshBtn.addEventListener("click", () => refresh().catch(showLoadError));
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
