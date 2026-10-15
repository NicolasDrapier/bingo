"use strict";

const $ = (s) => document.querySelector(s);

const S = {
  game: null,
  proposals: [],
  players: [],
  you: null,
  grid: null,
  checked: null,
  wins: [],
  final: null,
  ws: null,
  wsRetry: null,
};

const isHost = () => S.game && S.you && S.game.host_player_id === S.you.id;

async function api(path, method = "GET", body) {
  const opt = { method, headers: {} };
  if (body) {
    opt.headers["Content-Type"] = "application/json";
    opt.body = JSON.stringify(body);
  }
  const r = await fetch(path, opt);
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(data.error || `HTTP ${r.status}`);
  return data;
}

function showScreen(name) {
  for (const s of document.querySelectorAll(".screen")) s.classList.add("hidden");
  $(`#screen-${name}`).classList.remove("hidden");
  $("#topbar").classList.toggle("hidden", name === "home");
  if (name !== "home" && S.game) {
    $("#game-meta").textContent = `${S.game.name} · code ${S.game.code}`;
  }
}

function toast(msg, ms = 2500) {
  const t = $("#toast");
  t.textContent = msg;
  t.classList.remove("hidden");
  clearTimeout(toast._t);
  toast._t = setTimeout(() => t.classList.add("hidden"), ms);
}

function homeError(msg) {
  const e = $("#home-error");
  e.textContent = msg || "";
}

async function loadState() {
  try {
    const sp = await api("/api/state");
    S.game = sp.game;
    S.players = sp.players;
    S.proposals = sp.proposals || [];
    S.you = sp.you;
    S.grid = sp.grid || null;
    S.checked = sp.checked || null;
    S.wins = sp.wins || [];
    S.final = null;
    route();
  } catch (err) {

    Object.assign(S, { game: null, players: [], you: null, grid: null, checked: null, wins: [] });
    showScreen("home");
    homeError(err.message);
  }
}

function route() {
  if (!S.game) return showScreen("home");
  const name = { lobby: "lobby", playing: "game", finished: "finished" }[S.game.status];
  if (S.game.status === "finished" && !S.final) {

    S.final = { players: S.players.map((p) => ({ id: p.id, name: p.name, wins: winCount(p.id) })) };
  }
  showScreen(name);
  if (name === "lobby") renderLobby();
  if (name === "game") renderGame();
  if (name === "finished") renderFinished();
  openWS();
}

function winCount(pid) {
  return S.wins.filter((w) => w.player_id === pid).length;
}

function openWS() {
  if (S.ws && (S.ws.readyState === WebSocket.OPEN || S.ws.readyState === WebSocket.CONNECTING)) return;
  clearTimeout(S.wsRetry);
  const proto = location.protocol === "https:" ? "wss" : "ws";
  const ws = new WebSocket(`${proto}://${location.host}/ws`);
  S.ws = ws;
  ws.onmessage = (ev) => {
    const m = JSON.parse(ev.data);
    onMessage(m.type, m.payload || {});
  };
  ws.onclose = () => {

    S.ws = null;
    S.wsRetry = setTimeout(openWS, 1500);
  };
  ws.onerror = () => ws.close();
}

function onMessage(type, p) {
  switch (type) {
    case "state":
      S.game = p.game;
      S.players = p.players;
      S.proposals = p.proposals || [];
      S.you = p.you;
      S.grid = p.grid || null;
      S.checked = p.checked || null;
      S.wins = p.wins || [];
      route();
      break;
    case "player_joined":
    case "proposals_updated":
      loadState();
      break;
    case "game_started":
      showGo();
      loadState();
      break;
    case "cell_toggled":
      if (S.checked && p.player_id === S.you.id) {
        S.checked[p.row * S.game.grid_size + p.col] = p.checked;
        updateCell($("#game-grid").children[p.row * S.game.grid_size + p.col], p.checked);
      }
      break;
    case "bingo":
      showBingo(p);
      loadState();
      break;
    case "game_finished":
      S.final = p;
      showScreen("finished");
      renderFinished();
      break;
  }
}

$("#form-create").addEventListener("submit", async (e) => {
  e.preventDefault();
  homeError("");
  const f = new FormData(e.target);
  try {
    await api("/api/games", "POST", {
      name: f.get("name").trim(),
      player_name: f.get("player").trim(),
      grid_size: Number(f.get("size")),
      free_center: f.get("free") === "on",
    });
    await loadState();
  } catch (err) {
    homeError(err.message);
  }
});

$("#form-join").addEventListener("submit", async (e) => {
  e.preventDefault();
  homeError("");
  const f = new FormData(e.target);
  const code = f.get("code").trim().toUpperCase();
  try {
    await api(`/api/games/${code}/join`, "POST", { player_name: f.get("player").trim() });
    await loadState();
  } catch (err) {
    homeError(err.message);
  }
});

const codeParam = new URLSearchParams(location.search).get("code");
if (codeParam) $("#form-join [name=code]").value = codeParam.toUpperCase();

function renderLobby() {
  renderPlayers($("#players"));
  const required = S.game.grid_size ** 2;
  const count = S.proposals.length;
  $("#proposal-progress").textContent = `${count} / ${required} propositions minimum`;
  $("#btn-start").classList.toggle("hidden", !isHost());
  $("#btn-start").disabled = count < required;
  $("#start-hint").textContent = isHost()
    ? `Relis la liste. Dès qu’elle contient au moins ${required} propositions, valide-la pour distribuer les grilles et lancer la partie.`
    : "L’hôte valide la liste complète et lance la partie. Ta grille sera distribuée automatiquement.";
  const list = $("#proposals");
  list.replaceChildren();
  if (!count) {
    const empty = document.createElement("li");
    empty.className = "hint";
    empty.textContent = "Pas encore de proposition. Lance la première idée !";
    list.appendChild(empty);
  }
  for (const proposal of S.proposals) {
    const li = document.createElement("li");
    const copy = document.createElement("div");
    const label = document.createElement("strong");
    label.textContent = proposal.label;
    const author = document.createElement("small");
    author.className = "proposal-author";
    author.textContent = `Proposé par ${proposal.player_name}`;
    copy.append(label, author);
    li.appendChild(copy);
    list.appendChild(li);
  }
}
$("#form-proposal").addEventListener("submit", async (event) => {
  event.preventDefault();
  const input = $("#proposal-label");
  const label = input.value.trim();
  if (!label) return;
  const button = event.target.querySelector("button"); button.disabled = true;
  try {
    await api("/api/proposals", "POST", { label });
    if (input.value.trim() === label) input.value = "";
    await loadState(); input.focus();
  } catch (err) { toast(err.message); }
  finally { button.disabled = false; }
});
$("#btn-start").addEventListener("click", async () => {
  const button = $("#btn-start"); button.disabled = true;
  try { await api("/api/start", "POST"); await loadState(); }
  catch (err) { toast(err.message); renderLobby(); }
});

function renderPlayers(el) {
  el.innerHTML = "";
  for (const p of S.players) {
    const li = document.createElement("li");
    li.className = "player" + (p.id === S.you.id ? " me" : "");
    li.innerHTML = `<span class="p-name"></span><span class="badge ${S.game.status === "playing" ? "ready" : "writing"}"></span>`;
    li.querySelector(".p-name").textContent = p.name + (p.host ? " 👑" : "");
    li.querySelector(".badge").textContent = S.game.status === "playing" ? "En jeu" : "Dans le lobby";
    el.appendChild(li);
  }
}

function rankingData() {
  return S.players
    .map((p) => ({ ...p, wins: winCount(p.id) }))
    .sort((a, b) => b.wins - a.wins || (a.id - b.id));
}

function renderGame() {
  renderGameGrid();
  renderPlayers($("#players-game"));
  const rk = $("#ranking");
  rk.innerHTML = "";
  for (const p of rankingData()) {
    const li = document.createElement("li");
    li.textContent = `${p.name} — ${p.wins} BINGO`;
    rk.appendChild(li);
  }
  $("#btn-finish").classList.toggle("hidden", !isHost());
}

function updateCell(btn, checked, animate = false) {
  if (!btn) return;
  btn.classList.toggle("checked", checked);
  btn.setAttribute("aria-pressed", String(checked));
  if (!checked) btn.querySelector(".stamp")?.remove();
  if (animate && checked && !btn.classList.contains("free")) {
    btn.querySelector(".stamp")?.remove();
    const stamp = document.createElement("span");
    stamp.className = "stamp";
    stamp.textContent = "POP!";
    stamp.setAttribute("aria-hidden", "true");
    btn.appendChild(stamp);
    setTimeout(() => stamp.remove(), 280);
  }
}

function renderGameGrid() {
  const n = S.game.grid_size;
  const el = $("#game-grid");
  const key = `${S.game.code}:${S.you.id}:${n}`;
  if (el.dataset.key !== key) {
    el.replaceChildren();
    el.dataset.key = key;
    el.style.gridTemplateColumns = `repeat(${n}, minmax(0, 1fr))`;
    for (let i = 0; i < n * n; i++) {
      const r = Math.floor(i / n), c = i % n;
      const free = S.game.free_center && r === (n - 1) / 2 && c === (n - 1) / 2;
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "cell" + (free ? " free" : "");
      const label = document.createElement("span");
      label.className = "cell-label";
      btn.appendChild(label);
      if (free) btn.setAttribute("aria-disabled", "true");
      else btn.addEventListener("click", () => toggleCell(r, c, btn));
      el.appendChild(btn);
    }
  }
  Array.from(el.children).forEach((btn, i) => {
    const free = btn.classList.contains("free");
    const label = free ? "GRATUIT" : (S.grid?.[i] || "");
    btn.querySelector(".cell-label").textContent = label;
    btn.setAttribute("aria-label", free ? "Case centrale gratuite, déjà cochée" : label);
    updateCell(btn, free || !!S.checked?.[i]);
  });
}

async function toggleCell(r, c, btn) {
  if (btn.dataset.pending) return;
  const i = r * S.game.grid_size + c;
  const previous = !!S.checked[i];
  btn.dataset.pending = "true";
  S.checked[i] = !previous;
  updateCell(btn, S.checked[i], true);
  try {
    const result = await api("/api/cells/toggle", "POST", { row: r, col: c });
    S.checked[i] = result.checked;
    updateCell(btn, result.checked);
  } catch (err) {
    S.checked[i] = previous;
    updateCell(btn, previous);
    toast(err.message);
  } finally {
    delete btn.dataset.pending;
  }
}

function renderFinished() {
  const f = S.final || {
    players: S.players.map((p) => ({ id: p.id, name: p.name, wins: winCount(p.id) })),
  };
  const rk = $("#final-ranking");
  rk.innerHTML = "";
  const ordered = [...f.players].sort((a, b) => b.wins - a.wins || a.id - b.id);
  ordered.forEach((p, i) => {
    const li = document.createElement("li");
    li.textContent = `${p.name} — ${p.wins} BINGO`;
    if (i === 0 && p.wins > 0) li.classList.add("winner");
    rk.appendChild(li);
  });
  const wrap = $("#final-grids");
  wrap.innerHTML = "";
  for (const p of f.players) {
    if (!p.grid) continue;
    const n = S.game.grid_size;
    const div = document.createElement("div");
    div.className = "panel mini-grid-panel";
    div.innerHTML = `<h3 class="mini-title"></h3>`;
    div.querySelector("h3").textContent = p.name;
    const g = document.createElement("div");
    g.className = "grid grid-mini";
    g.style.gridTemplateColumns = `repeat(${n}, minmax(0, 1fr))`;
    const labels = p.grid.flat();
    const checked = p.checked?.flat();
    for (let i = 0; i < n * n; i++) {
      const cell = document.createElement("div");
      cell.className = "cell mini" + (checked && checked[i] ? " checked" : "");
      const label = document.createElement("span");
      label.className = "cell-label";
      label.textContent = labels[i];
      cell.appendChild(label);
      g.appendChild(cell);
    }
    div.appendChild(g);
    wrap.appendChild(div);
  }
}

$("#btn-finish").addEventListener("click", async () => {
  if (!confirm("Terminer la partie ?")) return;
  try {
    await api("/api/finish", "POST");
  } catch (err) {
    toast(err.message);
  }
});

$("#btn-new-game").addEventListener("click", async () => {
  const button = $("#btn-new-game");
  button.disabled = true;
  try {
    await api("/api/session/leave", "POST");
    location.href = "/";
  } catch (err) {
    toast(err.message);
    button.disabled = false;
  }
});

let bingoTimer = null;
function showBingo(p) {
  const ov = $("#bingo-overlay");
  ov.classList.add("hidden");
  void ov.offsetWidth;
  $("#bingo-winner").textContent = `${p.player_name} a complété : ${patternLabel(p.pattern)}`;

  ov.classList.remove("hidden");
  ov.classList.remove("shake");
  void ov.offsetWidth;
  ov.classList.add("shake");
  clearTimeout(bingoTimer);
  bingoTimer = setTimeout(() => ov.classList.add("hidden"), 3000);
}

function patternLabel(pat) {
  const [kind, n] = pat.split(":");
  if (kind === "row") return `la ligne ${Number(n) + 1}`;
  if (kind === "col") return `la colonne ${Number(n) + 1}`;
  return kind === "main" ? "la diagonale" : "l'autre diagonale";
}

function starSVG(spikes = 12, { id = "starGrad", r1 = 132, r2 = 86 } = {}) {
  const cx = 150, cy = 150;
  const pts = [];
  for (let i = 0; i < spikes * 2; i++) {
    const R = (i % 2 ? r2 : r1) * (0.82 + ((i * 7) % 11) / 11 * 0.18);
    const a = (Math.PI * i) / spikes - Math.PI / 2 + Math.sin(i * 2) * 0.07;
    pts.push(`${(cx + R * Math.cos(a)).toFixed(1)},${(cy + R * Math.sin(a)).toFixed(1)}`);
  }
  return `<svg viewBox="0 0 300 300" role="img" aria-label="explosion">
    <defs><radialGradient id="${id}" cx="50%" cy="42%" r="75%">
      <stop offset="0%" stop-color="#ffe14d"/><stop offset="100%" stop-color="#ff7a00"/>
    </radialGradient></defs>
    <polygon points="${pts.join(" ")}" fill="url(#${id})" stroke="#141414" stroke-width="10" stroke-linejoin="round"/>
  </svg>`;
}

function renderLogos() {
  const logo = (slot, gradId) => {
    slot.innerHTML = `<div class="star">${starSVG(12, { id: gradId })}</div>
      <span class="logo-text">BINGO!</span>`;
  };
  logo($("#home-logo"), "logoGradHome");
  logo($("#logo-slot"), "logoGradTop");
}

function dismissBingo() {
  clearTimeout(bingoTimer);
  $("#bingo-overlay").classList.add("hidden");
}
$("#btn-dismiss-bingo").addEventListener("click", dismissBingo);
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") dismissBingo();
});
function showGo() {
  const el = $("#go-effect");
  el.classList.add("hidden");
  void el.offsetWidth;
  el.classList.remove("hidden");
  clearTimeout(showGo.timer);
  showGo.timer = setTimeout(() => el.classList.add("hidden"), 850);
}
const sizeSelect = $("#form-create [name=size]");
sizeSelect.addEventListener("change", () => {
  const free = $("#form-create [name=free]");
  free.disabled = sizeSelect.value === "4";
  free.checked = sizeSelect.value !== "4";
});

renderLogos();
loadState();
