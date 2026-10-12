(function () {
  'use strict';

  var canvas = document.getElementById('board');
  var ctx = canvas.getContext('2d');
  var statusEl = document.getElementById('status');
  var scoreEl = document.getElementById('score');
  var foodSlider = document.getElementById('food-slider');
  var foodValue = document.getElementById('food-value');
  var nameInput = document.getElementById('name-input');
  var roomInput = document.getElementById('room-input');
  var joinBtn = document.getElementById('join-btn');
  var roomList = document.getElementById('room-list');
  var speedButtons = Array.prototype.slice.call(document.querySelectorAll('.speed-btn'));

  var playerId = null;
  var view = null;
  var source = null;
  var lastSentFood = null;
  var lastFoodTouch = 0;

  function setStatus(text) {
    statusEl.textContent = text;
  }

  function postJSON(url, payload) {
    return fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    }).then(function (response) {
      return response.json().catch(function () { return {}; }).then(function (data) {
        if (!response.ok) {
          throw new Error(data.error || ('HTTP ' + response.status));
        }
        return data;
      });
    });
  }

  function join(room, name) {
    setStatus('正在加入房间…');
    return postJSON('/api/join', { room: room, name: name }).then(function (data) {
      playerId = data.playerId;
      roomInput.value = data.room;
      applyView(data.state);
      openStream();
    }).catch(function (error) {
      setStatus('加入房间失败：' + error.message);
    });
  }

  function openStream() {
    if (source) {
      source.close();
      source = null;
    }
    source = new EventSource('/api/stream?playerId=' + encodeURIComponent(playerId));
    source.addEventListener('state', function (event) {
      try {
        applyView(JSON.parse(event.data));
      } catch (error) {
        // 忽略无法解析的帧
      }
    });
    source.addEventListener('error', function () {
      setStatus('与服务器的连接中断，正在重试…');
    });
  }

  function selfSnake() {
    if (!view) {
      return null;
    }
    for (var i = 0; i < view.snakes.length; i++) {
      if (view.snakes[i].id === playerId) {
        return view.snakes[i];
      }
    }
    return null;
  }

  function applyView(next) {
    view = next;
    var me = selfSnake();
    scoreEl.textContent = '得分：' + (me ? me.score : 0);
    if (!me) {
      setStatus('已加入房间 ' + view.room);
    } else if (!me.alive) {
      setStatus('撞墙啦！按空格键重新开始');
    } else if (me.paused) {
      setStatus('已暂停，按空格键继续');
    } else {
      setStatus('游戏中 · 房间 ' + view.room);
    }

    if (Date.now() - lastFoodTouch > 800) {
      foodSlider.value = String(view.foodCount);
      foodValue.textContent = String(view.foodCount);
      lastSentFood = view.foodCount;
    }

    speedButtons.forEach(function (button) {
      var active = !!(me && me.speed === button.getAttribute('data-speed'));
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', active ? 'true' : 'false');
    });

    draw();
  }

  function snakeFill(snake, isSelf, isHead) {
    if (!snake.alive) {
      return isHead ? '#7c8a97' : '#55616c';
    }
    if (isSelf) {
      return isHead ? '#9df3bd' : '#3ddc84';
    }
    return isHead ? '#9dc0ff' : '#4c8bf5';
  }

  function draw() {
    var canvasWidth = canvas.width;
    var canvasHeight = canvas.height;
    ctx.fillStyle = '#101820';
    ctx.fillRect(0, 0, canvasWidth, canvasHeight);

    if (!view) {
      ctx.fillStyle = '#8fa3b8';
      ctx.font = '26px system-ui, sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText('正在连接服务器…', canvasWidth / 2, canvasHeight / 2);
      return;
    }

    var padding = 16;
    var cell = Math.floor(Math.min((canvasWidth - padding * 2) / view.width, (canvasHeight - padding * 2) / view.height));
    if (cell < 4) {
      cell = 4;
    }
    var boardWidth = cell * view.width;
    var boardHeight = cell * view.height;
    var offsetX = Math.floor((canvasWidth - boardWidth) / 2);
    var offsetY = Math.floor((canvasHeight - boardHeight) / 2);

    ctx.fillStyle = '#16232e';
    ctx.fillRect(offsetX, offsetY, boardWidth, boardHeight);

    ctx.strokeStyle = 'rgba(255, 255, 255, 0.06)';
    ctx.lineWidth = 1;
    var index;
    for (index = 0; index <= view.width; index++) {
      ctx.beginPath();
      ctx.moveTo(offsetX + index * cell + 0.5, offsetY);
      ctx.lineTo(offsetX + index * cell + 0.5, offsetY + boardHeight);
      ctx.stroke();
    }
    for (index = 0; index <= view.height; index++) {
      ctx.beginPath();
      ctx.moveTo(offsetX, offsetY + index * cell + 0.5);
      ctx.lineTo(offsetX + boardWidth, offsetY + index * cell + 0.5);
      ctx.stroke();
    }

    ctx.strokeStyle = '#4c8bf5';
    ctx.lineWidth = 3;
    ctx.strokeRect(offsetX + 1.5, offsetY + 1.5, boardWidth - 3, boardHeight - 3);

    ctx.fillStyle = '#f5a524';
    view.food.forEach(function (point) {
      ctx.beginPath();
      ctx.arc(offsetX + point.x * cell + cell / 2, offsetY + point.y * cell + cell / 2, Math.max(2, cell * 0.28), 0, Math.PI * 2);
      ctx.fill();
    });

    view.snakes.forEach(function (snake) {
      var isSelf = snake.id === playerId;
      snake.body.forEach(function (point, position) {
        ctx.fillStyle = snakeFill(snake, isSelf, position === 0);
        ctx.fillRect(offsetX + point.x * cell + 1, offsetY + point.y * cell + 1, Math.max(1, cell - 2), Math.max(1, cell - 2));
      });
    });

    var me = selfSnake();
    if (me && (!me.alive || me.paused)) {
      ctx.fillStyle = 'rgba(8, 12, 18, 0.7)';
      ctx.fillRect(offsetX, offsetY, boardWidth, boardHeight);
      ctx.fillStyle = '#f2f6fa';
      ctx.font = '22px system-ui, sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText(me.alive ? '已暂停 — 按空格键继续' : '游戏结束 — 按空格键重新开始', canvasWidth / 2, canvasHeight / 2);
    }
  }

  function sendInput(key) {
    if (!playerId) {
      return;
    }
    postJSON('/api/input', { playerId: playerId, key: key }).then(applyView).catch(function (error) {
      setStatus('操作失败：' + error.message);
    });
  }

  function sendConfig(payload) {
    if (!playerId) {
      return;
    }
    payload.playerId = playerId;
    postJSON('/api/config', payload).then(applyView).catch(function (error) {
      setStatus('配置失败：' + error.message);
    });
  }

  function handleFoodInput() {
    var value = parseInt(foodSlider.value, 10);
    if (isNaN(value)) {
      return;
    }
    foodValue.textContent = String(value);
    lastFoodTouch = Date.now();
    if (value === lastSentFood) {
      return;
    }
    lastSentFood = value;
    sendConfig({ foodCount: value });
  }

  function directionForKey(event) {
    var map = {
      ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right',
      w: 'up', s: 'down', a: 'left', d: 'right',
      W: 'up', S: 'down', A: 'left', D: 'right'
    };
    return map[event.key] || null;
  }

  function handleKeyDown(event) {
    if (event.key === ' ' || event.key === 'Spacebar' || event.code === 'Space') {
      event.preventDefault();
      sendInput('space');
      return;
    }
    var direction = directionForKey(event);
    if (direction) {
      event.preventDefault();
      sendInput(direction);
    }
  }

  function refreshRooms() {
    fetch('/api/rooms').then(function (response) {
      return response.json();
    }).then(function (data) {
      roomList.textContent = '';
      (data.rooms || []).forEach(function (room) {
        var item = document.createElement('li');
        var button = document.createElement('button');
        button.type = 'button';
        button.className = 'room-entry';
        button.setAttribute('data-room', room.room);
        button.textContent = room.room + '（' + room.players + ' 人 · ' + room.foodCount + ' 食物）';
        button.addEventListener('click', function () {
          roomInput.value = room.room;
          join(room.room, nameInput.value || '玩家');
        });
        item.appendChild(button);
        roomList.appendChild(item);
      });
    }).catch(function () {
      // 忽略房间列表的临时错误
    });
  }

  window.addEventListener('keydown', handleKeyDown);

  foodSlider.addEventListener('input', handleFoodInput);
  foodSlider.addEventListener('change', handleFoodInput);

  speedButtons.forEach(function (button) {
    button.addEventListener('click', function () {
      sendConfig({ speed: button.getAttribute('data-speed') });
    });
  });

  joinBtn.addEventListener('click', function () {
    join(roomInput.value, nameInput.value);
  });

  [nameInput, roomInput].forEach(function (input) {
    input.addEventListener('keydown', function (event) {
      if (event.key === 'Enter') {
        join(roomInput.value, nameInput.value);
      }
    });
  });

  window.addEventListener('load', function () {
    if (!nameInput.value) {
      nameInput.value = '玩家' + Math.floor(Math.random() * 900 + 100);
    }
    draw();
    join(roomInput.value, nameInput.value);
    refreshRooms();
    window.setInterval(refreshRooms, 3000);
  });
}());
