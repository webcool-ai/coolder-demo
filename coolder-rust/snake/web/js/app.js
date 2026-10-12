'use strict';

/* Snake multiplayer frontend: no dependencies, one canvas, full-room view.
 * The server is authoritative — this file only renders snapshots and
 * forwards inputs. */

const CELL = 24;

const DIRS = {
  ArrowUp: { x: 0, y: -1 },
  ArrowDown: { x: 0, y: 1 },
  ArrowLeft: { x: -1, y: 0 },
  ArrowRight: { x: 1, y: 0 },
  KeyW: { x: 0, y: -1 },
  KeyS: { x: 0, y: 1 },
  KeyA: { x: -1, y: 0 },
  KeyD: { x: 1, y: 0 },
};

const $ = (id) => document.getElementById(id);

const els = {
  setup: $('setup'), joinBtn: $('joinBtn'), roomInput: $('roomInput'),
  nameInput: $('nameInput'), speedSel: $('speedSel'), foodSel: $('foodSel'),
  roomList: $('roomList'), refreshBtn: $('refreshBtn'), serverStatus: $('serverStatus'),
  lobby: $('lobby'), board: $('board'),
  roomTitle: $('roomTitle'), meName: $('meName'), score: $('score'),
  leaderboard: $('leaderboard'), foodSlider: $('foodSlider'), foodValue: $('foodValue'),
  speedGroup: $('speedGroup'),
  deathOverlay: $('deathOverlay'), deathMsg: $('deathMsg'),
  restartBtn: $('restartBtn'), watchBtn: $('watchBtn'), leaveBtn: $('leaveBtn'),
};

const state = {
  playing: false,
  socket: null,
  meId: null,
  gridW: 42,
  gridH: 28,
  snakes: [],
  food: [],
  tick: 0,
  lastDir: { x: 1, y: 0 },
};

const ctx = els.board.getContext('2d');

/* ------------------------------------------------------------- helpers */

function send(msg) {
  if (state.socket && state.socket.readyState === WebSocket.OPEN) {
    state.socket.send(JSON.stringify(msg));
  }
}

function showLobby() {
  state.playing = false;
  els.lobby.classList.add('hidden');
  els.setup.classList.remove('hidden');
  refreshLobby();
}

function setServerStatus(text, isError) {
  els.serverStatus.textContent = text;
  els.serverStatus.classList.toggle('error', Boolean(isError));
}

/* --------------------------------------------------------------- lobby */

async function refreshLobby() {
  try {
    const resp = await fetch('/api/rooms');
    const rooms = await resp.json();
    els.roomList.innerHTML = '';
    if (!rooms.length) {
      const li = document.createElement('li');
      li.className = 'empty';
      li.textContent = '暂无在线房间，创建一个吧';
      els.roomList.appendChild(li);
      return;
    }
    for (const r of rooms) {
      const li = document.createElement('li');
      const name = document.createElement('span');
      name.textContent = `${r.name} · ${r.players} 人`;
      const btn = document.createElement('button');
      btn.className = 'btn small';
      btn.textContent = '加入';
      btn.addEventListener('click', () => {
        els.roomInput.value = r.name;
        join();
      });
      li.append(name, btn);
      els.roomList.appendChild(li);
    }
    setServerStatus('服务器正常', false);
  } catch (err) {
    setServerStatus('无法连接服务器：' + err.message, true);
  }
}

/* ------------------------------------------------------------ websocket */

function join() {
  const room = els.roomInput.value.trim() || 'room1';
  const name = els.nameInput.value.trim();
  const speed = els.speedSel.value;
  const food = String(els.foodSel.value);

  if (state.socket) {
    try { state.socket.close(); } catch (_) { /* ignore */ }
  }

  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const qs = new URLSearchParams({ room, name, speed, food });
  const socket = new WebSocket(`${proto}//${location.host}/ws?${qs}`);
  state.socket = socket;
  state.playing = true;

  socket.onopen = () => {
    els.setup.classList.add('hidden');
    els.lobby.classList.remove('hidden');
    els.deathOverlay.classList.add('hidden');
    setServerStatus('服务器正常', false);
    rebuildSpeedGroup(speed);
    send({ type: 'food', food: Number(food) || 0 });
  };

  socket.onmessage = (ev) => {
    let msg;
    try { msg = JSON.parse(ev.data); } catch (_) { return; }
    if (msg.type === 'welcome') onWelcome(msg);
    else if (msg.type === 'state') onState(msg);
  };

  socket.onclose = () => {
    if (!state.playing) return;
    showLobby();
    setServerStatus('连接已断开，请重新进入', true);
  };

  socket.onerror = () => {
    if (!state.playing) return;
    setServerStatus('连接出错，请检查服务器', true);
  };
}

function onWelcome(msg) {
  state.meId = msg.id;
  state.gridW = msg.width;
  state.gridH = msg.height;
  els.board.width = msg.width * CELL;
  els.board.height = msg.height * CELL;
  els.roomTitle.textContent = msg.room;
  const name = els.nameInput.value.trim();
  els.meName.textContent = name ? name : '自动命名';
  els.foodSlider.max = msg.maxFood || 40;
  els.foodSlider.value = msg.foodCount;
  els.foodValue.textContent = msg.foodCount;
  rebuildSpeedGroup(msg.speed);
}

function onState(msg) {
  state.snakes = msg.snakes || [];
  state.food = msg.food || [];
  state.tick = msg.tick;

  const me = state.snakes.find((s) => s.id === state.meId);
  if (me) {
    els.score.textContent = me.score;
    if (!me.alive) {
      els.deathMsg.textContent = `本局得分 ${me.score}`;
      els.deathOverlay.classList.remove('hidden');
    } else {
      els.deathOverlay.classList.add('hidden');
    }
  }
  updateLeaderboard();
}

function updateLeaderboard() {
  const sorted = [...state.snakes].sort((a, b) => b.score - a.score);
  let html = '';
  for (const s of sorted) {
    const cls = [s.id === state.meId ? 'me' : '', s.alive ? '' : 'dead'].join(' ').trim();
    html += `<li class="${cls}">` +
      `<span class="dot" style="background:${s.alive ? s.color : '#4b5563'}"></span>` +
      `<span class="name">${escapeHTML(s.name)}</span>` +
      `<span>${s.score}${s.alive ? '' : ' 💀'}</span></li>`;
  }
  if (els.leaderboard.dataset.sig !== html) {
    els.leaderboard.dataset.sig = html;
    els.leaderboard.innerHTML = html;
  }
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}

/* -------------------------------------------------------------- inputs */

window.addEventListener('keydown', (ev) => {
  if (!state.playing) return;
  if (ev.ctrlKey || ev.metaKey || ev.altKey) return;
  const tag = document.activeElement && document.activeElement.tagName;
  if (tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA') return;

  const dir = DIRS[ev.code];
  if (dir) {
    ev.preventDefault();
    state.lastDir = dir;
    send({ type: 'dir', dx: dir.x, dy: dir.y });
    return;
  }
  if (ev.code === 'Space') {
    ev.preventDefault();
    const me = state.snakes.find((s) => s.id === state.meId);
    if (me && me.alive) send({ type: 'pause', paused: !me.paused });
    return;
  }
  if (ev.code === 'KeyR') {
    const me = state.snakes.find((s) => s.id === state.meId);
    if (me && !me.alive) send({ type: 'restart' });
  }
});

/* Swipe support for touch devices */
let touchStart = null;
els.board.addEventListener('touchstart', (ev) => {
  touchStart = { x: ev.touches[0].clientX, y: ev.touches[0].clientY };
}, { passive: true });
els.board.addEventListener('touchend', (ev) => {
  if (!touchStart) return;
  const dx = ev.changedTouches[0].clientX - touchStart.x;
  const dy = ev.changedTouches[0].clientY - touchStart.y;
  touchStart = null;
  if (Math.max(Math.abs(dx), Math.abs(dy)) < 24) return;
  const dir = Math.abs(dx) > Math.abs(dy)
    ? { x: Math.sign(dx), y: 0 }
    : { x: 0, y: Math.sign(dy) };
  state.lastDir = dir;
  send({ type: 'dir', dx: dir.x, dy: dir.y });
});

/* ---------------------------------------------------------- hud wiring */

let foodSendTimer = null;
els.foodSlider.addEventListener('input', () => {
  els.foodValue.textContent = els.foodSlider.value;
  clearTimeout(foodSendTimer);
  foodSendTimer = setTimeout(() => {
    send({ type: 'food', food: Number(els.foodSlider.value) });
  }, 120);
});
els.foodSlider.addEventListener('change', () => {
  clearTimeout(foodSendTimer);
  send({ type: 'food', food: Number(els.foodSlider.value) });
});

function rebuildSpeedGroup(checked) {
  els.speedGroup.innerHTML = '';
  for (const [value, label] of [['slow', '慢'], ['medium', '中'], ['fast', '快']]) {
    const id = 'spd_' + value;
    const input = document.createElement('input');
    input.type = 'radio';
    input.name = 'speed';
    input.id = id;
    input.value = value;
    input.checked = value === checked;
    input.addEventListener('change', () => {
      if (input.checked) send({ type: 'speed', speed: value });
    });
    const labelEl = document.createElement('label');
    labelEl.htmlFor = id;
    labelEl.textContent = label;
    els.speedGroup.append(input, labelEl);
  }
}

els.joinBtn.addEventListener('click', join);
els.nameInput.addEventListener('keydown', (ev) => { if (ev.key === 'Enter') join(); });
els.roomInput.addEventListener('keydown', (ev) => { if (ev.key === 'Enter') join(); });
els.refreshBtn.addEventListener('click', refreshLobby);
els.leaveBtn.addEventListener('click', () => {
  state.playing = false;
  if (state.socket) { try { state.socket.close(); } catch (_) { /* ignore */ } }
  showLobby();
});
els.restartBtn.addEventListener('click', () => send({ type: 'restart' }));
els.watchBtn.addEventListener('click', () => els.deathOverlay.classList.add('hidden'));

/* ------------------------------------------------------------- drawing */

function draw() {
  requestAnimationFrame(draw);
  if (!state.playing) return;

  const w = state.gridW * CELL;
  const h = state.gridH * CELL;
  if (els.board.width !== w) els.board.width = w;
  if (els.board.height !== h) els.board.height = h;

  ctx.fillStyle = '#0d1424';
  ctx.fillRect(0, 0, w, h);

  // subtle grid
  ctx.strokeStyle = 'rgba(148, 163, 184, 0.07)';
  ctx.lineWidth = 1;
  ctx.beginPath();
  for (let x = CELL; x < w; x += CELL) {
    ctx.moveTo(x + 0.5, 0);
    ctx.lineTo(x + 0.5, h);
  }
  for (let y = CELL; y < h; y += CELL) {
    ctx.moveTo(0, y + 0.5);
    ctx.lineTo(w, y + 0.5);
  }
  ctx.stroke();

  drawFood();
  for (const s of state.snakes) drawSnake(s);
  drawPauseBanner();
}

function drawFood() {
  const t = (performance.now() / 500) % (Math.PI * 2);
  const pulse = 0.82 + 0.18 * Math.sin(t);
  const colors = ['#f87171', '#fbbf24', '#4ade80', '#60a5fa', '#f472b6', '#2dd4bf'];
  for (let i = 0; i < state.food.length; i++) {
    const f = state.food[i];
    const r = (CELL / 2 - 5) * pulse;
    ctx.fillStyle = colors[i % colors.length];
    ctx.beginPath();
    ctx.arc(f.x * CELL + CELL / 2, f.y * CELL + CELL / 2, r, 0, Math.PI * 2);
    ctx.fill();
  }
}

function drawSnake(s) {
  if (!s.body || !s.body.length) return;
  ctx.globalAlpha = s.alive ? 1 : 0.35;
  for (let i = s.body.length - 1; i >= 0; i--) {
    const p = s.body[i];
    const pad = 2 + 1.5 * (i / s.body.length); // tail tapers slightly
    roundRect(
      p.x * CELL + pad,
      p.y * CELL + pad,
      CELL - pad * 2,
      CELL - pad * 2,
      6,
    );
    ctx.fillStyle = s.alive ? shade(s.color, i) : '#4b5563';
    ctx.fill();
  }
  // eyes on the head
  const head = s.body[0];
  const cx = head.x * CELL + CELL / 2;
  const cy = head.y * CELL + CELL / 2;
  const dx = s.dir.x, dy = s.dir.y;
  const px = dy !== 0 ? 5 : 0, py = dx !== 0 ? 5 : 0;
  ctx.fillStyle = '#0b1220';
  ctx.beginPath();
  ctx.arc(cx + dx * 5 + px, cy + dy * 5 + py, 2.2, 0, Math.PI * 2);
  ctx.arc(cx + dx * 5 - px, cy + dy * 5 - py, 2.2, 0, Math.PI * 2);
  ctx.fill();
  ctx.globalAlpha = 1;
}

function shade(hex, i) {
  if (i === 0) return hex; // head keeps full color
  const f = Math.max(0.45, 1 - i * 0.022);
  const n = parseInt(hex.slice(1), 16);
  const r = Math.round(((n >> 16) & 255) * f);
  const g = Math.round(((n >> 8) & 255) * f);
  const b = Math.round((n & 255) * f);
  return `rgb(${r},${g},${b})`;
}

function roundRect(x, y, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}

function drawPauseBanner() {
  const me = state.snakes.find((s) => s.id === state.meId);
  if (!me || !me.alive || !me.paused) return;
  ctx.fillStyle = 'rgba(13, 20, 36, 0.55)';
  ctx.fillRect(0, 0, els.board.width, els.board.height);
  ctx.fillStyle = '#e7edf7';
  ctx.font = 'bold 34px system-ui, sans-serif';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText('⏸ 已暂停（空格继续）', els.board.width / 2, els.board.height / 2);
}

/* ---------------------------------------------------------------- boot */

refreshLobby();
requestAnimationFrame(draw);
