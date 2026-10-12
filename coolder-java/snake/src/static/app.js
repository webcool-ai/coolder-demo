/* 多人贪吃蛇前端：大厅 → WebSocket 加入房间 → Canvas 渲染 + 键盘/触屏操作 */
(() => {
  'use strict';

  const $ = (id) => document.getElementById(id);
  const lobbyEl = $('lobby'), gameEl = $('game');
  const canvas = $('board'), ctx = canvas.getContext('2d');
  const foodSlider = $('foodSlider'), foodCount = $('foodCount');
  const overlay = $('overlay'), overlayText = $('overlayText'), overlayBtn = $('overlayBtn');
  const connOverlay = $('connOverlay'), connText = $('connText');
  const scoreList = $('scoreList'), eventLog = $('eventLog');
  const roomBadge = $('roomBadge'), onlineBadge = $('onlineBadge');
  const speedBtns = [...document.querySelectorAll('.speed-btn')];

  let ws = null;
  let state = null;      // 最近一次服务端快照
  let myId = 0;          // 我的玩家 ID
  let draggingSlider = false;
  let foodSendTimer = null;

  // ---------- 大厅 ----------

  async function refreshRooms() {
    if (lobbyEl.classList.contains('hidden')) return;
    try {
      const res = await fetch('/api/rooms');
      const rooms = await res.json();
      const dl = $('roomList');
      const chips = $('roomChips');
      dl.innerHTML = '';
      chips.innerHTML = '';
      for (const r of rooms) {
        const opt = document.createElement('option');
        opt.value = r.name;
        dl.appendChild(opt);
        const chip = document.createElement('button');
        chip.className = 'chip';
        chip.textContent = `${r.name} · ${r.players}/${r.max} 人${r.running ? '' : ' · ⏸'}`;
        chip.onclick = () => {
          $('roomInput').value = r.name;
          joinGame();
        };
        chips.appendChild(chip);
      }
      if (!rooms.length) {
        chips.innerHTML = '<span class="chip-empty">暂无活跃房间，输入房间名创建一个吧！</span>';
      }
    } catch (e) { /* 大厅轮询失败可忽略 */ }
  }
  setInterval(refreshRooms, 3000);
  refreshRooms();

  function joinGame() {
    const name = $('nameInput').value.trim() || '玩家';
    const room = $('roomInput').value.trim() || '大厅';
    try {
      localStorage.setItem('snakeName', name);
      localStorage.setItem('snakeRoom', room);
    } catch (e) { /* 隐私模式忽略 */ }
    connect(room, name);
  }

  $('joinBtn').onclick = joinGame;
  $('nameInput').addEventListener('keydown', (e) => { if (e.key === 'Enter') joinGame(); });
  $('roomInput').addEventListener('keydown', (e) => { if (e.key === 'Enter') joinGame(); });
  try {
    $('nameInput').value = localStorage.getItem('snakeName') || '';
    $('roomInput').value = localStorage.getItem('snakeRoom') || '';
  } catch (e) { /* ignore */ }

  $('leaveBtn').onclick = backToLobby;
  $('backBtn').onclick = backToLobby;

  function backToLobby() {
    if (ws) { try { ws.close(); } catch (e) { /* ignore */ } ws = null; }
    state = null;
    myId = 0;
    gameEl.classList.add('hidden');
    lobbyEl.classList.remove('hidden');
    overlay.classList.add('hidden');
    connOverlay.classList.add('hidden');
    refreshRooms();
  }

  // ---------- 连接 ----------

  function connect(room, name) {
    if (ws) { try { ws.close(); } catch (e) { /* ignore */ } }
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(
      `${proto}://${location.host}/ws?room=${encodeURIComponent(room)}&name=${encodeURIComponent(name)}`
    );
    ws.onopen = () => {
      lobbyEl.classList.add('hidden');
      gameEl.classList.remove('hidden');
      roomBadge.textContent = `🏠 ${room}`;
      eventLog.innerHTML = '';
      overlay.classList.add('hidden');
      connOverlay.classList.add('hidden');
    };
    ws.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      handleMsg(msg);
    };
    ws.onclose = () => {
      if (!gameEl.classList.contains('hidden')) {
        connText.textContent = '与服务器的连接已断开';
        connOverlay.classList.remove('hidden');
      }
    };
    ws.onerror = () => { /* onclose 会接管 */ };
  }

  function handleMsg(msg) {
    switch (msg.type) {
      case 'init':
        myId = msg.you;
        break;
      case 'state':
        state = msg;
        syncUI();
        draw();
        break;
      case 'event':
        addEvent(msg.text);
        break;
      case 'error':
        connText.textContent = msg.text || '发生错误';
        connOverlay.classList.remove('hidden');
        if (ws) { try { ws.close(); } catch (e) { /* ignore */ } }
        break;
      case 'pong':
        break;
    }
  }

  function addEvent(text) {
    const li = document.createElement('li');
    li.textContent = text;
    eventLog.prepend(li);
    while (eventLog.children.length > 8) {
      eventLog.removeChild(eventLog.lastChild);
    }
  }

  // ---------- UI 同步 ----------

  speedBtns.forEach((b) => {
    b.onclick = () => send({ type: 'speed', speed: b.dataset.speed });
  });

  function syncUI() {
    if (!state) return;
    onlineBadge.textContent = `在线 ${state.snakes.length} 人`;
    speedBtns.forEach((b) => b.classList.toggle('active', b.dataset.speed === state.speed));
    foodCount.textContent = state.foodTarget;
    if (!draggingSlider && +foodSlider.value !== state.foodTarget) {
      foodSlider.value = state.foodTarget; // 其他人改了滑杆 → 同步过来
    }

    // 排行榜（按得分排序）
    const sorted = [...state.snakes].sort((a, b) => b.score - a.score);
    scoreList.innerHTML = '';
    for (const s of sorted) {
      const li = document.createElement('li');
      if (s.id === myId) li.classList.add('me');
      if (!s.alive) li.classList.add('dead');
      const dot = document.createElement('span');
      dot.className = 'dot';
      dot.style.background = s.color;
      const name = document.createElement('span');
      name.className = 'pname';
      name.textContent = s.name;
      const score = document.createElement('span');
      score.className = 'pscore';
      score.textContent = s.alive ? s.score : `💀 ${s.score}`;
      li.append(dot, name, score);
      scoreList.appendChild(li);
    }

    // 覆盖层：死亡 → 空格重开；暂停 → 空格继续
    const me = state.snakes.find((s) => s.id === myId);
    if (me && !me.alive) {
      overlayText.textContent = '💀 你死了！按空格重新开始';
      overlayBtn.classList.remove('hidden');
      overlay.classList.remove('hidden');
    } else if (!state.running) {
      overlayText.textContent = '⏸ 已暂停 · 按空格继续';
      overlayBtn.classList.add('hidden');
      overlay.classList.remove('hidden');
    } else {
      overlay.classList.add('hidden');
    }
  }

  function send(obj) {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(obj));
    }
  }

  // ---------- 输入：键盘 ----------

  const keyDirs = {
    ArrowUp: 'up', KeyW: 'up',
    ArrowDown: 'down', KeyS: 'down',
    ArrowLeft: 'left', KeyA: 'left',
    ArrowRight: 'right', KeyD: 'right',
  };

  document.addEventListener('keydown', (e) => {
    if (ws && e.code === 'Space') {
      // 空格：暂停/继续；死亡后=重新开始（服务端判定）
      e.preventDefault();
      send({ type: 'space' });
      return;
    }
    const d = keyDirs[e.code];
    if (d && !gameEl.classList.contains('hidden')) {
      e.preventDefault();
      send({ type: 'dir', dir: d });
    }
  });

  // ---------- 输入：触屏（滑动=方向，点按=空格） ----------

  let touchStart = null;
  canvas.addEventListener('touchstart', (e) => {
    touchStart = { x: e.touches[0].clientX, y: e.touches[0].clientY };
  }, { passive: true });
  canvas.addEventListener('touchend', (e) => {
    if (!touchStart) return;
    const t = e.changedTouches[0];
    const dx = t.clientX - touchStart.x;
    const dy = t.clientY - touchStart.y;
    touchStart = null;
    if (Math.hypot(dx, dy) < 24) {
      send({ type: 'space' });
      return;
    }
    const d = Math.abs(dx) > Math.abs(dy)
      ? (dx > 0 ? 'right' : 'left')
      : (dy > 0 ? 'down' : 'up');
    send({ type: 'dir', dir: d });
  });

  overlayBtn.onclick = () => send({ type: 'space' });
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) send({ type: 'space' });
  });

  // ---------- 食物数量滑杆（在线可配置） ----------

  foodSlider.addEventListener('pointerdown', () => { draggingSlider = true; });
  window.addEventListener('pointerup', () => { draggingSlider = false; });
  foodSlider.addEventListener('input', () => {
    foodCount.textContent = foodSlider.value;
    clearTimeout(foodSendTimer);
    foodSendTimer = setTimeout(() => {
      send({ type: 'food', count: +foodSlider.value });
    }, 150); // 拖动过程中做轻量节流
  });
  foodSlider.addEventListener('change', () => {
    send({ type: 'food', count: +foodSlider.value });
  });

  // 应用层心跳（每 20 秒，服务端会顺带刷新读超时）
  setInterval(() => send({ type: 'ping' }), 20000);

  // ---------- 渲染 ----------

  const CELL = 20;

  function draw() {
    if (!state) return;
    const w = state.w * CELL, h = state.h * CELL;
    if (canvas.width !== w) canvas.width = w;
    if (canvas.height !== h) canvas.height = h;

    // 棋盘底色（棋盘格）
    ctx.fillStyle = '#0f172a';
    ctx.fillRect(0, 0, w, h);
    ctx.fillStyle = '#16213b';
    for (let y = 0; y < state.h; y++) {
      for (let x = 0; x < state.w; x++) {
        if ((x + y) % 2 === 0) ctx.fillRect(x * CELL, y * CELL, CELL, CELL);
      }
    }
    // 边框：越界即死（撞墙）
    ctx.strokeStyle = '#ef4444';
    ctx.lineWidth = 2;
    ctx.strokeRect(1, 1, w - 2, h - 2);

    for (const [fx, fy] of state.foods) drawFood(fx, fy);
    for (const s of state.snakes) drawSnake(s);
  }

  function drawFood(x, y) {
    const cx = x * CELL + CELL / 2, cy = y * CELL + CELL / 2;
    ctx.fillStyle = '#ef4444';
    ctx.beginPath();
    ctx.arc(cx, cy + 1, CELL * 0.34, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = '#22c55e';
    ctx.fillRect(cx - 1.5, cy - CELL * 0.42, 3, 5);
  }

  function roundRect(x, y, w2, h2, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w2, y, x + w2, y + h2, r);
    ctx.arcTo(x + w2, y + h2, x, y + h2, r);
    ctx.arcTo(x, y + h2, x, y, r);
    ctx.arcTo(x, y, x + w2, y, r);
    ctx.closePath();
  }

  function drawSnake(s) {
    const pad = 2;
    for (let i = s.cells.length - 1; i >= 0; i--) {
      const [x, y] = s.cells[i];
      ctx.fillStyle = s.alive ? (i === 0 ? lighten(s.color) : s.color) : '#4b5563';
      roundRect(x * CELL + pad, y * CELL + pad, CELL - pad * 2, CELL - pad * 2, 5);
      ctx.fill();
    }
    if (!s.alive || !s.cells.length) return;

    const [hx, hy] = s.cells[0];
    const cx = hx * CELL + CELL / 2, cy = hy * CELL + CELL / 2;
    // 依据第二段推断朝向，画眼睛
    let ex = 1, ey = 0;
    if (s.cells.length > 1) {
      const [nx, ny] = s.cells[1];
      ex = hx - nx; ey = hy - ny;
    }
    ctx.fillStyle = '#0f172a';
    ctx.beginPath();
    ctx.arc(cx + ex * 3 + ey * 3.5, cy + ey * 3 + ex * 3.5, 2, 0, Math.PI * 2);
    ctx.fill();
    ctx.beginPath();
    ctx.arc(cx + ex * 3 - ey * 3.5, cy + ey * 3 - ex * 3.5, 2, 0, Math.PI * 2);
    ctx.fill();

    if (s.id === myId) { // 高亮自己的蛇头
      ctx.strokeStyle = '#ffffff';
      ctx.lineWidth = 1.5;
      roundRect(hx * CELL + 1, hy * CELL + 1, CELL - 2, CELL - 2, 6);
      ctx.stroke();
    }
  }

  function lighten(hex) {
    const n = parseInt(hex.slice(1), 16);
    const r = Math.min(255, (n >> 16) + 70);
    const g = Math.min(255, ((n >> 8) & 255) + 70);
    const b = Math.min(255, (n & 255) + 70);
    return `rgb(${r},${g},${b})`;
  }
})();
