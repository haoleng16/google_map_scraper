/* ── State ───────────────────────────────────────────── */
const state = {
  contacts: [],
  messages: [],  // { id, text, image_id, image_name, pdf_id, pdf_name }
  loggedIn: false,
  sending: false,
  loginPollTimer: null,
  activeTag: null,
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
const progressPanel = $("#progressPanel");
const progressCounter = $("#progressCounter");
const progressBarFill = $("#progressBarFill");
const progressList = $("#progressList");

/* ── API helpers ─────────────────────────────────────── */
async function api(method, path, body) {
  const requestPath = apiBase + path;
  const opts = { method, headers: {} };
  if (body instanceof FormData) {
    opts.body = body;
  } else if (body) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(requestPath, opts);
  const data = await res.json();
  if (!res.ok) {
    const errMsg = data.detail?.error || data.detail || "请求失败";
    throw new Error(errMsg);
  }
  return data;
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

async function pollLoginStatus() {
  try {
    const data = await api("GET", "/login/status");
    updateLoginUI(data.logged_in);
    if (data.logged_in && state.loginPollTimer) {
      clearInterval(state.loginPollTimer);
      state.loginPollTimer = null;
    }
  } catch {
    /* ignore */
  }
}

loginBtn.addEventListener("click", async () => {
  loginBtn.disabled = true;
  statusText.textContent = "启动浏览器...";
  try {
    await api("POST", "/login/start");
    statusText.textContent = "等待扫码...";
    state.loginPollTimer = setInterval(pollLoginStatus, 2000);
  } catch (e) {
    alert("启动失败: " + e.message);
    loginBtn.disabled = false;
    statusText.textContent = "未连接";
  }
});

logoutBtn.addEventListener("click", async () => {
  try {
    await api("POST", "/login/logout");
    updateLoginUI(false);
  } catch (e) {
    alert("登出失败: " + e.message);
  }
});

/* ── CSV Upload ──────────────────────────────────────── */
function setupDropZone(zone, inputEl, onFile) {
  zone.addEventListener("dragover", (e) => {
    e.preventDefault();
    zone.classList.add("drop-zone--active");
  });
  zone.addEventListener("dragleave", () => {
    zone.classList.remove("drop-zone--active");
  });
  zone.addEventListener("drop", (e) => {
    e.preventDefault();
    zone.classList.remove("drop-zone--active");
    const file = e.dataTransfer.files[0];
    if (file) onFile(file);
  });
  inputEl.addEventListener("change", () => {
    const file = inputEl.files[0];
    if (file) onFile(file);
    inputEl.value = "";
  });
}

setupDropZone(csvDropZone, csvFileInput, uploadCSV);

async function uploadCSV(file) {
  const fd = new FormData();
  fd.append("file", file);
  try {
    const data = await api("POST", "/contacts/upload", fd);
    state.contacts = data.contacts;
    renderContacts();
  } catch (e) {
    alert("CSV 上传失败: " + e.message);
  }
}

function getTopCategories() {
  const counts = {};
  for (const c of state.contacts) {
    if (c.category) {
      counts[c.category] = (counts[c.category] || 0) + 1;
    }
  }
  return Object.entries(counts)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 3)
    .map(([cat]) => cat);
}

function getFilteredContacts() {
  if (!state.activeTag) return state.contacts;
  return state.contacts.filter((c) => c.category === state.activeTag);
}

function renderTags() {
  const container = document.getElementById("tagList");
  if (!container) return;
  container.innerHTML = "";
  const topCats = getTopCategories();
  if (topCats.length === 0) {
    container.style.display = "none";
    return;
  }
  container.style.display = "";

  // "全部" tag
  const allTag = document.createElement("button");
  allTag.className = "tag-btn" + (state.activeTag === null ? " tag-btn--active" : "");
  allTag.textContent = "全部";
  allTag.addEventListener("click", () => {
    state.activeTag = null;
    renderTags();
    renderContactList();
  });
  container.appendChild(allTag);

  for (const cat of topCats) {
    const btn = document.createElement("button");
    btn.className = "tag-btn" + (state.activeTag === cat ? " tag-btn--active" : "");
    btn.textContent = cat;
    btn.addEventListener("click", () => {
      state.activeTag = state.activeTag === cat ? null : cat;
      renderTags();
      renderContactList();
    });
    container.appendChild(btn);
  }
}

function renderContacts() {
  contactControls.style.display = state.contacts.length ? "" : "none";
  renderTags();
  renderContactList();
  updateSendBtn();
}

function renderContactList() {
  const filtered = getFilteredContacts();
  const allSelected = state.contacts.filter((c) => c.selected).length;
  contactCount.textContent = `${allSelected}/${state.contacts.length} 个联系人`;

  contactList.innerHTML = "";
  for (const c of filtered) {
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
}

function updateContactCount() {
  const selected = state.contacts.filter((c) => c.selected).length;
  contactCount.textContent = `${selected}/${state.contacts.length} 个联系人`;
}

selectAllBtn.addEventListener("click", () => {
  state.contacts.forEach((c) => (c.selected = true));
  renderContacts();
});
deselectAllBtn.addEventListener("click", () => {
  state.contacts.forEach((c) => (c.selected = false));
  renderContacts();
});
invertBtn.addEventListener("click", () => {
  state.contacts.forEach((c) => (c.selected = !c.selected));
  renderContacts();
});

/* ── Message Cards ───────────────────────────────────── */
function addMessage() {
  const msg = {
    id: nextMsgId(),
    text: "",
    image_id: null,
    image_name: null,
    pdf_id: null,
    pdf_name: null,
  };
  state.messages.push(msg);
  renderMessages();
}

addMsgBtn.addEventListener("click", addMessage);

function buildAttachmentHtml(msg) {
  const parts = [];
  if (msg.image_id) {
    parts.push(`<div class="file-upload-zone file-upload-zone--has-file" data-kind="image">
      <button class="file-upload-zone__remove" data-msg-id="${msg.id}" data-remove-kind="image" title="删除图片">&times;</button>
      <input type="file" accept="image/*" data-msg-id="${msg.id}" data-kind="image">
      <div class="file-upload-zone__label">图片已选</div>
      <div class="file-upload-zone__preview"><span class="file-name">${esc(msg.image_name)}</span></div>
    </div>`);
  } else {
    parts.push(`<div class="file-upload-zone" data-kind="image">
      <input type="file" accept="image/*" data-msg-id="${msg.id}" data-kind="image">
      <div class="file-upload-zone__label">图片</div>
    </div>`);
  }
  if (msg.pdf_id) {
    parts.push(`<div class="file-upload-zone file-upload-zone--has-file" data-kind="pdf">
      <button class="file-upload-zone__remove" data-msg-id="${msg.id}" data-remove-kind="pdf" title="删除PDF">&times;</button>
      <input type="file" accept=".pdf,application/pdf" data-msg-id="${msg.id}" data-kind="pdf">
      <div class="file-upload-zone__label">PDF 已选</div>
      <div class="file-upload-zone__preview"><span class="file-name">${esc(msg.pdf_name)}</span></div>
    </div>`);
  } else {
    parts.push(`<div class="file-upload-zone" data-kind="pdf">
      <input type="file" accept=".pdf,application/pdf" data-msg-id="${msg.id}" data-kind="pdf">
      <div class="file-upload-zone__label">PDF</div>
    </div>`);
  }
  return parts.join("");
}

function removeMessage(id) {
  state.messages = state.messages.filter((m) => m.id !== id);
  renderMessages();
}

function renderMessages() {
  messageCards.innerHTML = "";
  state.messages.forEach((msg, idx) => {
    const card = document.createElement("div");
    card.className = "msg-card";
    card.draggable = true;
    card.dataset.msgId = msg.id;

    const attachmentHtml = buildAttachmentHtml(msg);

    card.innerHTML = `
      <div class="msg-card__header">
        <span class="msg-card__label">消息 ${idx + 1}</span>
        <button class="msg-card__delete" data-msg-id="${msg.id}">&times;</button>
      </div>
      <textarea placeholder="输入消息内容">${esc(msg.text)}</textarea>
      <div class="msg-card__attachments">
        ${attachmentHtml}
      </div>
    `;

    // Text change
    card.querySelector("textarea").addEventListener("input", (e) => {
      msg.text = e.target.value;
    });

    // Delete
    card.querySelector(".msg-card__delete").addEventListener("click", () => {
      removeMessage(msg.id);
    });

    // File remove buttons
    card.querySelectorAll(".file-upload-zone__remove").forEach((btn) => {
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        e.preventDefault();
        const kind = btn.dataset.removeKind;
        if (kind === "image") {
          msg.image_id = null;
          msg.image_name = null;
        } else {
          msg.pdf_id = null;
          msg.pdf_name = null;
        }
        renderMessages();
      });
    });

    // File uploads
    card.querySelectorAll('input[type="file"]').forEach((inp) => {
      inp.addEventListener("change", async () => {
        const file = inp.files[0];
        if (!file) return;
        const kind = inp.dataset.kind;
        try {
          const fd = new FormData();
          fd.append("file", file);
          const data = await api("POST", "/upload", fd);
          if (kind === "image") {
            msg.image_id = data.id;
            msg.image_name = data.name;
          } else {
            msg.pdf_id = data.id;
            msg.pdf_name = data.name;
          }
          renderMessages();
        } catch (e) {
          alert("上传失败: " + e.message);
        }
        inp.value = "";
      });
    });

    // Drag reorder
    card.addEventListener("dragstart", (e) => {
      card.classList.add("msg-card--dragging");
      e.dataTransfer.setData("text/plain", String(msg.id));
    });
    card.addEventListener("dragend", () => {
      card.classList.remove("msg-card--dragging");
    });
    card.addEventListener("dragover", (e) => {
      e.preventDefault();
      const dragging = messageCards.querySelector(".msg-card--dragging");
      if (dragging && dragging !== card) {
        const rect = card.getBoundingClientRect();
        const mid = rect.top + rect.height / 2;
        if (e.clientY < mid) {
          messageCards.insertBefore(dragging, card);
        } else {
          messageCards.insertBefore(dragging, card.nextSibling);
        }
      }
    });
    card.addEventListener("drop", (e) => {
      e.preventDefault();
      syncMessageOrder();
    });

    messageCards.appendChild(card);
  });
  updateSendBtn();
}

function syncMessageOrder() {
  const cards = messageCards.querySelectorAll(".msg-card");
  const orderedIds = Array.from(cards).map((c) => parseInt(c.dataset.msgId));
  const map = new Map(state.messages.map((m) => [m.id, m]));
  state.messages = orderedIds.map((id) => map.get(id)).filter(Boolean);
}

/* ── Send ────────────────────────────────────────────── */
function updateSendBtn() {
  const hasContacts = state.contacts.some((c) => c.selected);
  const hasContent = state.messages.some(
    (m) => m.text.trim() || m.image_id || m.pdf_id
  );
  sendBtn.disabled = !state.loggedIn || !hasContacts || !hasContent || state.sending;
  stopBtn.disabled = !state.sending;
}

sendBtn.addEventListener("click", async () => {
  const contact_ids = state.contacts.filter((c) => c.selected).map((c) => c.id);
  const messages = state.messages
    .filter((m) => m.text.trim() || m.image_id || m.pdf_id)
    .map((m) => ({
      text: m.text,
      image_id: m.image_id,
      pdf_id: m.pdf_id,
    }));

  try {
    await api("POST", "/send", { contact_ids, messages });
    state.sending = true;
    updateSendBtn();
    startSSE();
  } catch (e) {
    alert("发送失败: " + e.message);
  }
});

stopBtn.addEventListener("click", async () => {
  try {
    await api("POST", "/send/stop");
  } catch (e) {
    alert("停止失败: " + e.message);
  }
});

/* ── SSE Progress ────────────────────────────────────── */
function startSSE() {
  progressPanel.style.display = "";
  progressList.innerHTML = "";
  progressBarFill.style.width = "0%";
  progressCounter.textContent = "0/0";

  const es = new EventSource(apiBase + "/send/events");

  es.onmessage = (e) => {
    const data = JSON.parse(e.data);

    if (data.type === "progress" || data.type === "error") {
      const total = data.total;
      const current = data.current;
      progressCounter.textContent = `${current}/${total}`;
      progressBarFill.style.width = `${(current / total) * 100}%`;

      const li = document.createElement("li");
      li.className = `progress-item progress-item--${data.type === "progress" ? "success" : "error"}`;
      li.innerHTML = `
        <span class="progress-item__icon">${data.type === "progress" ? "✓" : "✗"}</span>
        <span>${esc(data.contact)}</span>
        ${data.type === "error" ? `<span>— ${esc(data.error)}</span>` : ""}
      `;
      progressList.appendChild(li);
      progressList.scrollTop = progressList.scrollHeight;
    }

    if (data.type === "complete") {
      progressCounter.textContent = `完成 ${data.success} 成功 / ${data.failed} 失败`;
      progressBarFill.style.width = "100%";
      state.sending = false;
      updateSendBtn();
      es.close();
    }
  };

  es.onerror = () => {
    state.sending = false;
    updateSendBtn();
    es.close();
  };
}

/* ── Utilities ───────────────────────────────────────── */
function esc(str) {
  if (!str) return "";
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

/* ── Init ────────────────────────────────────────────── */
(async () => {
  try {
    const data = await api("GET", "/login/status");
    updateLoginUI(data.logged_in);
  } catch {
    updateLoginUI(false);
  }
  addMessage();
})();
