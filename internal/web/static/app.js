const state = {
  documents: [],
  selectedId: null,
  selectedTag: "",
};

const els = {
  refreshBtn: document.querySelector("#refreshBtn"),
  sourceInput: document.querySelector("#sourceInput"),
  ingestBtn: document.querySelector("#ingestBtn"),
  ingestResult: document.querySelector("#ingestResult"),
  tagList: document.querySelector("#tagList"),
  searchInput: document.querySelector("#searchInput"),
  searchBtn: document.querySelector("#searchBtn"),
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
    return localStorage.getItem("kbToolApiToken") || "";
  } catch (error) {
    return "";
  }
}

function storeApiToken(token) {
  try {
    localStorage.setItem("kbToolApiToken", token);
  } catch (error) {
    return;
  }
}

function apiHeaders(options = {}) {
  const token = storedApiToken();
  return {
    "Content-Type": "application/json",
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...(options.headers || {}),
  };
}

async function api(path, options = {}, allowTokenPrompt = true) {
  const response = await fetch(path, {
    headers: apiHeaders(options),
    ...options,
  });
  if (response.status === 401 && allowTokenPrompt) {
    const message = storedApiToken() ? "API Token 无效，请重新输入" : "请输入 API Token";
    const token = window.prompt(message);
    if (token) {
      storeApiToken(token.trim());
      return api(path, options, false);
    }
  }
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || response.statusText);
  }
  return response.json();
}

function assetURL(id) {
  const token = storedApiToken();
  const suffix = token ? `?access_token=${encodeURIComponent(token)}` : "";
  return `/api/documents/${id}/asset${suffix}`;
}

function tagHtml(tags = []) {
  return tags.map((tag) => `<span class="tag">${escapeHtml(tag)}</span>`).join("");
}

function escapeHtml(value = "") {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

async function loadDocuments() {
  const q = els.searchInput.value.trim();
  try {
    const payload = await api(`/api/documents?q=${encodeURIComponent(q)}&limit=100`);
    state.documents = payload.documents || [];
  } catch (error) {
    state.documents = [];
    els.documentList.innerHTML = `<div class="empty">API 暂不可用，请确认 kb-tool server 已连接 TiDB。</div>`;
    return;
  }
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
    ? tags.map((tag) => `<span class="tag clickable" data-tag="${escapeHtml(tag.name)}">${escapeHtml(tag.name)} (${tag.count})</span>`).join("")
    : `<span class="status">暂无标签</span>`;
  els.tagList.querySelectorAll("[data-tag]").forEach((node) => {
    node.addEventListener("click", () => {
      state.selectedTag = node.dataset.tag;
      renderDocuments();
    });
  });
}

function renderDocuments() {
  const docs = state.selectedTag
    ? state.documents.filter((doc) => (doc.tags || []).includes(state.selectedTag))
    : state.documents;
  els.documentList.innerHTML = docs.length
    ? docs.map((doc) => `
      <article class="document-row ${doc.id === state.selectedId ? "active" : ""}" data-id="${doc.id}">
        <div class="document-title">${escapeHtml(doc.title || doc.path)}</div>
        <div class="document-meta">${escapeHtml(doc.source_type)} · ${escapeHtml(doc.mime_type || "text/plain")} · ${escapeHtml(doc.path)}</div>
        <div class="tag-list">${tagHtml(doc.tags)}</div>
      </article>
    `).join("")
    : `<div class="empty">没有匹配的知识内容。</div>`;
  els.documentList.querySelectorAll("[data-id]").forEach((node) => {
    node.addEventListener("click", () => selectDocument(Number(node.dataset.id)));
  });
}

async function selectDocument(id) {
  state.selectedId = id;
  const doc = await api(`/api/documents/${id}`);
  els.detailEmpty.classList.add("hidden");
  els.detailView.classList.remove("hidden");
  els.detailTitle.textContent = doc.title || doc.path;
  els.detailPath.textContent = doc.path || "";
  els.detailType.textContent = doc.mime_type || doc.language || "document";
  els.detailTags.innerHTML = tagHtml(doc.tags || []);
  els.assetPreview.innerHTML = "";
  if (doc.is_binary && (doc.mime_type || "").startsWith("image/")) {
    els.assetPreview.innerHTML = `<img src="${assetURL(doc.id)}" alt="${escapeHtml(doc.title || "image preview")}">`;
    els.textPreview.textContent = "";
  } else {
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
  els.ingestResult.textContent = "正在导入...";
  const payload = await api("/api/ingest", {
    method: "POST",
    body: JSON.stringify({ sources }),
  });
  els.ingestResult.textContent = (payload.results || []).map((result) => {
    if (result.error) return `${result.source}: ${result.error}`;
    return `${result.source}: ${result.documents} documents, ${result.tags} tags`;
  }).join("\n");
  await refresh();
}

async function addTags() {
  if (!state.selectedId) return;
  const tags = els.tagInput.value.split(",").map((tag) => tag.trim()).filter(Boolean);
  if (!tags.length) return;
  await api(`/api/documents/${state.selectedId}/tags`, {
    method: "POST",
    body: JSON.stringify({ tags }),
  });
  els.tagInput.value = "";
  await refresh();
  await selectDocument(state.selectedId);
}

async function refresh() {
  await Promise.all([loadDocuments(), loadTags()]);
}

els.refreshBtn.addEventListener("click", refresh);
els.searchBtn.addEventListener("click", loadDocuments);
els.searchInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") loadDocuments();
});
els.ingestBtn.addEventListener("click", ingestSources);
els.addTagBtn.addEventListener("click", addTags);

refresh().catch((error) => {
  els.documentList.innerHTML = `<div class="empty">${escapeHtml(error.message)}</div>`;
});
