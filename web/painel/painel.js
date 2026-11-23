/* Painel do Hallstream. Sem build, sem bibliotecas. O servidor guarda tudo;
   o browser só guarda o último separador aberto. */
'use strict';

(function () {
  var MOCK = /[?&]mock=1\b/.test(location.search);
  // "inicio" is the Configuração inicial: a tab with no button in the nav.
  var TABS = ['jogo', 'ecras', 'plantel', 'patrocinadores', 'promo', 'transmissoes', 'definicoes', 'caixa', 'ajuda', 'inicio'];
  var SCREENS = [
    { id: 'marcador', label: 'Marcador', hint: 'Resultado no canto' },
    { id: 'abertura', label: 'Abertura', hint: 'Antes do jogo' },
    { id: 'plantel', label: 'Plantel', hint: 'Jogadores de hoje' },
    { id: 'desconto', label: 'Desconto de tempo', hint: 'Durante o desconto' },
    { id: 'intervalo', label: 'Intervalo', hint: 'Resultado ao intervalo' },
    { id: 'final', label: 'Final', hint: 'Resultado final' },
    { id: 'apoiantes', label: 'Apoiantes', hint: 'Os patrocinadores' },
    { id: 'proximo', label: 'Próximo jogo', hint: 'O jogo seguinte' },
    { id: 'voltamos', label: 'Voltamos já', hint: 'Pausa técnica' },
    { id: 'nada', label: 'Nada', hint: 'Ecrã limpo' }
  ];
  var SWATCHES = ['#2E6FD8', '#1F6E3A', '#5B2A86', '#B4410C', '#0F5C6E', '#181841', '#5C6170', '#111111'];
  var MAX_SQUAD = 12;

  var S = {
    settings: null, groups: [], status: null, state: null, game: null, draft: null,
    players: {}, sponsors: {}, plantelGid: null, sponsorGid: 'geral',
    fpbGames: null, fpbGamesErr: '', fpbTeams: null, importResult: null, linking: {},
    devices: null, devicesErr: '', tries: {},
    tab: 'jogo'
  };

  /* ---------------- helpers ---------------- */
  function $(sel, root) { return (root || document).querySelector(sel); }
  function $all(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }
  function byNumber(a, b) {
    var na = parseInt(a.number, 10), nb = parseInt(b.number, 10);
    if (isNaN(na)) na = 999; if (isNaN(nb)) nb = 999;
    return na - nb || String(a.display_name).localeCompare(String(b.display_name), 'pt');
  }
  function groupName(id) {
    if (id === 'geral') return 'Geral';
    var g = S.groups.filter(function (x) { return x.id === id; })[0];
    return g ? g.name : '';
  }
  function groupById(id) { return S.groups.filter(function (x) { return x.id === id; })[0] || null; }
  function pad(n) { return (n < 10 ? '0' : '') + n; }
  function hhmm(d) { return pad(d.getHours()) + ':' + pad(d.getMinutes()); }
  function ago(iso) {
    if (!iso) return '';
    var s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
    if (s < 60) return 'há ' + s + ' s';
    if (s < 3600) return 'há ' + Math.round(s / 60) + ' min';
    return 'às ' + hhmm(new Date(iso));
  }
  function plural(n, one, many) { return n + ' ' + (n === 1 ? one : many); }
  function since(iso) {
    var m = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000));
    if (m < 1) return 'agora mesmo';
    if (m < 60) return 'há ' + m + ' min';
    if (m < 1440) return 'há ' + Math.round(m / 60) + ' h';
    return 'há ' + plural(Math.round(m / 1440), 'dia', 'dias');
  }
  function syncSummary(c) {
    c = c || {};
    var parts = [];
    if (c.new_players) parts.push(plural(c.new_players, 'jogador novo', 'jogadores novos'));
    if (c.numbers) parts.push(plural(c.numbers, 'número novo', 'números novos'));
    if (c.photos) parts.push(plural(c.photos, 'foto nova', 'fotos novas'));
    if (c.returned) parts.push(plural(c.returned, 'jogador voltou', 'jogadores voltaram'));
    if (c.left) parts.push(plural(c.left, 'jogador saiu', 'jogadores saíram'));
    return parts.length ? parts.join(', ') : 'sem mudanças';
  }
  function clubProgress(c) {
    return c && c.total ? 'A importar ' + Math.min(c.done + 1, c.total) + ' de ' + c.total + ' equipas…' : 'A procurar as equipas do clube na FPB…';
  }
  var DOW = ['dom', 'seg', 'ter', 'qua', 'qui', 'sex', 'sáb'];
  var MON = ['jan', 'fev', 'mar', 'abr', 'mai', 'jun', 'jul', 'ago', 'set', 'out', 'nov', 'dez'];
  function fpbDate(dateStr, timeStr) {
    var p = String(dateStr || '').split('-');
    var t = String(timeStr || '').toUpperCase().split('H');
    var d = new Date(+p[0], (+p[1] || 1) - 1, +p[2] || 1, +t[0] || 0, +t[1] || 0);
    return d;
  }
  function isoLocal(d) {
    var off = -d.getTimezoneOffset(), sign = off >= 0 ? '+' : '-';
    off = Math.abs(off);
    return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + 'T' + pad(d.getHours()) + ':' +
      pad(d.getMinutes()) + ':00' + sign + pad(Math.floor(off / 60)) + ':' + pad(off % 60);
  }
  function toInputDT(iso) {
    if (!iso) return '';
    var d = new Date(iso);
    if (isNaN(d)) return '';
    return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + 'T' + hhmm(d);
  }
  function luminance(hex) {
    var m = /^#?([0-9a-f]{6})$/i.exec(hex || '');
    if (!m) return null;
    var n = parseInt(m[1], 16);
    return [n >> 16, (n >> 8) & 255, n & 255].map(function (v) {
      v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
    }).reduce(function (acc, v, i) { return acc + v * [0.2126, 0.7152, 0.0722][i]; }, 0);
  }
  function contrastWithWhite(hex) { var l = luminance(hex); return l == null ? null : 1.05 / (l + 0.05); }
  function initials(name) {
    var w = String(name || '').trim().split(/\s+/).filter(Boolean);
    if (!w.length) return '?';
    if (w.length === 1) return w[0].slice(0, 3).toUpperCase();
    return w.slice(0, 3).map(function (x) { return x[0]; }).join('').toUpperCase();
  }
  function thumb(url, name) {
    var w = String(name || '').trim().split(/\s+/).filter(Boolean);
    var ini = w.length > 1 ? w[0][0] + w[w.length - 1][0] : (w[0] || '').slice(0, 2);
    return '<span class="thumb" aria-hidden="true">' + esc(ini.toUpperCase()) + (url ? '<img src="' + esc(url) + '" alt="">' : '') + '</span>';
  }
  function genderLabel(g) {
    g = String(g || '').toLowerCase();
    if (g[0] === 'm') return 'Masculino';
    if (g[0] === 'f') return 'Feminino';
    return g;
  }

  /* ---------------- API ---------------- */
  var pinWait = null;
  function api(method, path, body) {
    var opts = { method: method, headers: {}, credentials: 'same-origin' };
    if (typeof FormData !== 'undefined' && body instanceof FormData) opts.body = body;
    else if (method !== 'GET') {
      // The server refuses writes without this type (a form on another site can't send it).
      opts.headers['Content-Type'] = 'application/json';
      if (body !== undefined) opts.body = JSON.stringify(body);
    }
    return fetch(path, opts).then(function (res) {
      return res.text().then(function (text) {
        var data = null;
        if (text) { try { data = JSON.parse(text); } catch (e) { data = null; } }
        if (res.status === 401) {
          return askPin(data && data.error).then(function () { return api(method, path, body); });
        }
        if (!res.ok) throw new Error((data && data.error) || ('O programa respondeu com erro ' + res.status + '.'));
        return data;
      });
    }, function () {
      throw new Error('Sem ligação ao programa. Confirme que o Hallstream está aberto.');
    });
  }

  function askPin(msg) {
    if (pinWait) return pinWait;
    var box = $('#pin');
    box.hidden = false;
    $('#pinMsg').textContent = document.cookie.indexOf('pin=') >= 0 ? (msg || 'PIN errado. Tente de novo.') : '';
    var input = $('#pinValue');
    input.value = '';
    setTimeout(function () { input.focus(); }, 30);
    pinWait = new Promise(function (resolve) {
      $('#pinForm').onsubmit = function (e) {
        e.preventDefault();
        if (!/^\d{4}$/.test(input.value)) { $('#pinMsg').textContent = 'O PIN tem 4 algarismos.'; return; }
        document.cookie = 'pin=' + input.value + '; path=/; SameSite=Strict';
        box.hidden = true;
        pinWait = null;
        resolve();
        connectEvents();
      };
    });
    return pinWait;
  }

  /* Disable a button while a request runs and say how it went next to it. */
  function busy(btn, promiseFn, okText) {
    var msg = btn && btn.parentElement ? btn.parentElement.querySelector('.msg') : null;
    if (btn) btn.disabled = true;
    if (msg) { msg.className = 'msg'; msg.textContent = 'A guardar…'; }
    return Promise.resolve().then(promiseFn).then(function (r) {
      if (msg) {
        msg.className = 'msg ok'; msg.textContent = okText || 'Guardado';
        setTimeout(function () { if (msg.textContent === (okText || 'Guardado')) msg.textContent = ''; }, 3000);
      }
      return r;
    }, function (err) {
      if (msg) { msg.className = 'msg err'; msg.textContent = err.message; }
      else showError(err);
      throw err;
    }).then(function (r) { if (btn) btn.disabled = false; return r; }, function () { if (btn) btn.disabled = false; });
  }
  function showError(err) { confirmBox('Não deu', esc(err.message), { yes: 'OK', noCancel: true }); }

  function confirmBox(title, html, o) {
    o = o || {};
    var m = $('#modal');
    $('#modalTitle').textContent = title;
    $('#modalBody').innerHTML = html;
    var yes = $('#modalYes'), no = $('#modalNo');
    yes.textContent = o.yes || 'Confirmar';
    yes.className = 'btn ' + (o.danger ? 'danger' : 'primary');
    no.hidden = !!o.noCancel;
    m.hidden = false;
    setTimeout(function () { (o.noCancel ? yes : no).focus(); }, 30);
    return new Promise(function (resolve) {
      function done(v) { m.hidden = true; yes.onclick = no.onclick = null; document.onkeydown = null; resolve(v); }
      yes.onclick = function () { done(true); };
      no.onclick = function () { done(false); };
      document.onkeydown = function (e) { if (e.key === 'Escape') done(false); };
    });
  }

  function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) return navigator.clipboard.writeText(text);
    var ta = document.createElement('textarea');
    ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0';
    document.body.appendChild(ta); ta.select();
    try { document.execCommand('copy'); } finally { document.body.removeChild(ta); }
    return Promise.resolve();
  }

  /* ---------------- loading ---------------- */
  function loadPlayers(gid) {
    if (!gid) return Promise.resolve([]);
    return api('GET', '/api/groups/' + encodeURIComponent(gid) + '/players').then(function (list) {
      S.players[gid] = (list || []).slice().sort(byNumber);
      return S.players[gid];
    });
  }
  function loadSponsors(gid) {
    return api('GET', '/api/groups/' + encodeURIComponent(gid) + '/sponsors').then(function (list) {
      S.sponsors[gid] = list || [];
      return S.sponsors[gid];
    });
  }
  function loadGroups() {
    return api('GET', '/api/groups').then(function (g) {
      S.groups = (g || []).filter(function (x) { return x.id !== 'geral'; });
    });
  }
  function emptyGame() {
    return { group_id: S.groups.length ? S.groups[0].id : '', fpb_game_id: null,
      away: { name: '', short: '', color: '', crest_url: '' },
      start: '', venue: '', competition: '', round: '', squad: [] };
  }
  function normGame(g) {
    var base = emptyGame();
    g = g || {};
    for (var k in base) if (g[k] === undefined || g[k] === null) g[k] = base[k];
    if (g.fpb_game_id === '') g.fpb_game_id = null;
    g.away = Object.assign({}, base.away, g.away || {});
    return g;
  }
  function setGame(g) {
    S.game = normGame(g);
    S.draft = clone(S.game);
    S.gameNewer = null;
  }
  function same(a, b) { return JSON.stringify(a) === JSON.stringify(b); }

  // Another device (or the server, with the FPB crest) changed the game: take
  // it, unless this panel has edits not yet saved; then offer it instead.
  function refreshGame() {
    return api('GET', '/api/game').then(function (g) {
      g = normGame(g);
      if (same(g, S.game)) return;
      if (!same(S.draft, S.game)) {
        S.gameNewer = g;
        if (S.tab === 'jogo') renderJogo();
        return;
      }
      takeGame(g);
    }, function () {});
  }
  // The Jogo tab has no squad card until there is an escalão.
  function refreshSquadCard() {
    var card = $('#jSquadCard');
    if (S.tab === 'jogo' && card) card.innerHTML = renderSquad();
  }
  function takeGame(g) {
    setGame(g);
    if (S.tab === 'jogo') renderJogo();
    if (S.tab === 'ecras') renderEcras();
    loadPlayers(S.game.group_id).then(function () {
      refreshSquadCard();
      renderLtPlayers(); updateLowerThird();
    });
  }
  function loadStatus() {
    return api('GET', '/api/status').then(function (s) {
      S.status = s; renderChips(); renderStream();
      var c = s && s.club_setup, was = S.clubRunning;
      S.clubRunning = !!(c && c.running);
      if (S.clubRunning) {
        var cm = $('#clubMsg'); if (cm) cm.textContent = clubProgress(c);
        if (!S.clubTimer) S.clubTimer = setTimeout(function () { S.clubTimer = null; loadStatus(); }, 1500);
      } else if (was) {
        loadGroups().then(function () { if (S.tab === 'plantel') renderPlantel(); });
      }
      var u = $('#dUrl'); if (u && s && s.overlay_url) u.value = s.overlay_url;
      if (S.tab === 'ajuda') renderAjudaDir();
      renderShortcutAsk();
      refreshInicio();
      // The PC card needs the status: draw it once it comes in.
      if (S.tab === 'definicoes' && s && s.shortcut && !$('#dShortcut')) renderDefinicoes();
    }, function () {});
  }

  /* ---------------- live state (SSE) ---------------- */
  var es = null;
  function connectEvents() {
    if (es) { try { es.close(); } catch (e) {} }
    es = new EventSource('/api/events');
    es.addEventListener('state', function (ev) {
      try { S.state = JSON.parse(ev.data); } catch (e) { return; }
      $('#offline').hidden = true;
      onLive();
    });
    es.onopen = function () { $('#offline').hidden = true; };
    es.onerror = function () { $('#offline').hidden = false; };
  }

  var liveGame = null, lastScore = null, lastSide = '';
  function onLive() {
    var st = S.state; if (!st) return;
    var lv = st.live || {}, game = st.game || {};
    var gk = JSON.stringify([game.group && game.group.id, game.away, game.start, game.venue, game.competition, game.round,
      (game.squad || []).map(function (p) { return p.number + ' ' + p.name; })]);
    if (liveGame !== null && gk !== liveGame && S.game) refreshGame();
    liveGame = gk;
    var hn = (game.home && game.home.short) || lv.home_name || 'Casa';
    var an = (game.away && game.away.short) || lv.away_name || 'Fora';
    // The side that scored last has its last digit lit, until the other side scores.
    var hs = lv.home == null ? '' : String(lv.home), as = lv.away == null ? '' : String(lv.away);
    if (lastScore && +hs > +lastScore[0]) lastSide = 'home';
    else if (lastScore && +as > +lastScore[1]) lastSide = 'away';
    lastScore = [hs, as];
    function num(v, lit) {
      return '<span class="n">' + (lit && v ? esc(v.slice(0, -1)) + '<span class="lit">' + esc(v.slice(-1)) + '</span>' : esc(v)) + '</span>';
    }
    function sig(name, side) {
      var c = side && side.color;
      return '<span class="sig"' + (/^#[0-9a-f]{3,8}$/i.test(c || '') ? ' style="--c:' + c + '"' : '') + '>' + esc(name) + '</span>';
    }
    $('#liveScore').innerHTML = sig(hn, game.home) + ' ' + num(hs, lastSide === 'home') + '<span class="sep">–</span>' + num(as, lastSide === 'away') + ' ' + sig(an, game.away);
    $('#liveClock').textContent = (lv.period ? lv.period + 'º · ' : '') + (lv.clock || '');
    var cur = st.screen && st.screen.current;
    var tag = $('#liveScreen');
    var label = st.demo ? 'Demonstração' : cur === 'promo' ? 'Promo' : (SCREENS.filter(function (s) { return s.id === cur; })[0] || {}).label;
    tag.hidden = !label;
    tag.textContent = label || '';
    tag.classList.toggle('demo-tag', !!st.demo);
    updateScreenButtons();
    updateLowerThird();
    renderPromoAir();
    renderChips();
    if (S.tab === 'caixa') renderCaixaStatus();
    refreshInicio();
    var demo = $('#setDemo');
    if (demo && !demo.disabled) demo.checked = !!st.demo;
  }

  /* ---------------- tabs ---------------- */
  function showTab(name) {
    if (TABS.indexOf(name) < 0) name = 'jogo';
    S.tab = name;
    TABS.forEach(function (t) {
      $('#tab-' + t).hidden = t !== name;
      var nb = $('.nav [data-tab="' + t + '"]');
      if (nb) nb.setAttribute('aria-selected', t === name ? 'true' : 'false');
    });
    try { localStorage.setItem('painel.tab', name); } catch (e) {}
    // On a phone the tab row scrolls sideways: keep the open tab in sight, clear of the fade at the right edge.
    var navBtn = $('.nav [data-tab="' + name + '"]'), nav = navBtn && navBtn.parentElement;
    if (nav) nav.scrollLeft = Math.max(0, Math.min(nav.scrollLeft, navBtn.offsetLeft - 8), navBtn.offsetLeft + navBtn.offsetWidth + 56 - nav.clientWidth);
    // The flow reuses the YouTube and OBS ids of Definições: only one is drawn at a time.
    if (name !== 'inicio') $('#inicio').innerHTML = '';
    if (name === 'inicio') { $('#definicoes').innerHTML = ''; openInicio(); }
    if (name === 'ecras') { renderEcras(); fitPreview(); }
    if (name === 'jogo') renderJogo();
    if (name === 'plantel') renderPlantel();
    if (name === 'patrocinadores') renderSponsors();
    if (name === 'promo') renderPromo();
    if (name === 'transmissoes') renderTransmissoes();
    if (name === 'definicoes') { renderDefinicoes(); loadYt(); }
    clearInterval(devTimer);
    if (name === 'caixa') { renderCaixa(); loadDevices(); devTimer = setInterval(loadDevices, 2000); }
    if (name === 'ajuda') renderAjudaDir();
    window.scrollTo(0, 0);
  }

  /* ================= 1. JOGO ================= */
  function renderJogo() {
    var root = $('#jogo'), d = S.draft;
    if (!d) { root.innerHTML = '<p class="hint">A carregar…</p>'; return; }
    if (!S.groups.length) {
      root.innerHTML = '<div class="card"><p>Ainda não há escalões. Crie um no separador <b>Plantel</b>.</p></div>';
      return;
    }
    var g = groupById(d.group_id);
    var opts = S.groups.map(function (x) {
      return '<option value="' + esc(x.id) + '"' + (x.id === d.group_id ? ' selected' : '') + '>' + esc(x.name) + '</option>';
    }).join('');
    var a = d.away;
    var color = a.color || '#5C6170';
    var ratio = contrastWithWhite(color);

    var html = '';
    if (S.gameNewer) html += '<div class="ask"><p class="txt"><b>O jogo foi mudado noutro dispositivo</b><span class="hint">Ainda não guardou o que mudou aqui. Se carregar em Guardar jogo, substitui o que o outro dispositivo escolheu.</span></p>' +
      '<div class="row"><button class="btn primary" type="button" data-act="game-newer">Usar o jogo guardado</button></div></div>';
    html += '<div class="card"><div class="row">' +
      '<label class="field grow"><span>Escalão</span><select id="jGroup">' + (d.group_id ? '' : '<option value="">Escolha…</option>') + opts + '</select></label>' +
      (fpbOffered() ? '<button class="btn dark" data-act="fpb-games"' + (g && g.fpb_team_id ? '' : ' disabled') + '>Escolher jogo da FPB</button>' : '') +
      '<button class="btn" data-act="manual">Jogo manual</button>' +
      '</div>';
    if (g && !g.fpb_team_id && fpbOffered()) html += '<p class="hint" style="margin-top:10px">Este escalão não está ligado à FPB. Ligue-o em <a href="#plantel" data-goto="plantel">Plantel</a>, ou preencha o jogo à mão.</p>';
    if (S.fpbGamesErr) html += '<p class="alert" style="margin-top:12px">' + esc(S.fpbGamesErr) + '</p>';
    if (S.fpbGames) html += renderFpbGames();
    html += '</div>';

    html += '<div class="card"><h2>Adversário' + (d.fpb_game_id ? ' <span class="badge">FPB</span>' : '') + '</h2>' +
      '<div class="opp">' +
      '<div><div class="crest-tile" id="jCrest" style="--opp:' + esc(color) + '">' +
        (a.crest_url ? '<img src="' + esc(a.crest_url) + '" alt="Emblema do adversário">' : esc(a.short || initials(a.name))) + '</div>' +
        '<div class="stack" style="margin-top:10px">' +
        '<label class="btn small filebtn" style="width:100%">Carregar emblema<input type="file" id="jCrestFile" accept="image/png,image/jpeg,image/svg+xml"></label>' +
        (a.crest_url ? '<button class="btn quiet small" data-act="crest-clear" style="width:100%">Retirar</button>' : '') +
        '<span class="msg"></span></div></div>' +
      '<div class="stack">' +
        '<div class="row row-top">' +
          '<label class="field grow"><span>Nome <small>(máx. 20)</small></span><input id="jName" maxlength="20" value="' + esc(a.name) + '" autocomplete="off"><div class="counter" id="jNameC">' + a.name.length + '/20</div></label>' +
          '<label class="field" style="width:120px"><span>Sigla <small>(máx. 4)</small></span><input id="jShort" maxlength="4" value="' + esc(a.short) + '" autocomplete="off"></label>' +
        '</div>' +
        '<div><span class="label">Cor</span><div class="swatches">' +
          SWATCHES.map(function (c) {
            return '<button type="button" class="swatch" data-color="' + c + '" style="background:' + c + '" aria-label="Cor ' + c + '" aria-pressed="' + (c.toLowerCase() === color.toLowerCase()) + '"></button>';
          }).join('') +
          '<input type="color" id="jColor" value="' + esc(/^#[0-9a-f]{6}$/i.test(color) ? color : '#5C6170') + '" aria-label="Outra cor">' +
        '</div>' +
        (ratio !== null && ratio < 4.5 ? '<p class="warn" style="margin-top:8px">Esta cor é clara de mais para texto branco. Escolha uma mais escura.</p>' : '') +
        (!a.color && d.fpb_game_id ? '<p class="hint" style="margin-top:6px">O programa escolhe a cor a partir do emblema.</p>' : '') +
        '</div>' +
      '</div></div></div>';

    html += '<div class="card"><h2>Detalhes</h2><div class="cols">' +
      '<label class="field"><span>Data e hora</span><input type="datetime-local" id="jStart" value="' + esc(toInputDT(d.start)) + '"></label>' +
      '<label class="field"><span>Pavilhão</span><input id="jVenue" value="' + esc(d.venue) + '"></label>' +
      '<label class="field"><span>Competição</span><input id="jComp" value="' + esc(d.competition) + '"></label>' +
      '<label class="field"><span>Jornada ou fase</span><input id="jRound" value="' + esc(d.round) + '"></label>' +
      '</div></div>';

    html += '<div class="card" id="jSquadCard">' + renderSquad() + '</div>';

    html += '<div class="savebar"><button class="btn primary" data-act="save-game">Guardar jogo</button><span class="msg"></span>' +
      '<span style="flex:1"></span><button class="btn danger" data-act="new-game">Novo jogo</button></div>';
    root.innerHTML = html;
  }

  function renderFpbGames() {
    var g = groupById(S.draft.group_id);
    var list = S.fpbGames.games || [];
    var teamId = S.fpbGames.team ? S.fpbGames.team.id : (g && g.fpb_team_id);
    if (!list.length) return '<p class="hint" style="margin-top:12px">A FPB não tem jogos deste escalão nos próximos 14 dias.</p>';
    return '<div class="games" role="list">' + list.map(function (x, i) {
      var home = String(x.home && x.home.id) === String(teamId);
      var opp = home ? x.away : x.home;
      var d = fpbDate(x.date, x.time);
      var sel = String(S.draft.fpb_game_id) === String(x.id);
      return '<button class="game-pick' + (sel ? ' sel' : '') + '" data-act="pick-game" data-i="' + i + '" role="listitem">' +
        '<span class="when">' + DOW[d.getDay()] + ', ' + d.getDate() + ' ' + MON[d.getMonth()] + '<small>' + hhmm(d) + '</small></span>' +
        '<span class="who">' + esc(opp ? opp.name : '?') + '<small>' + esc([x.competition_name, x.game_week ? 'Jornada ' + x.game_week : x.phase_name].filter(Boolean).join(' · ')) + '</small>' +
        '<span class="where">' + esc(x.location || '') + '</span></span>' +
        '<span class="ha">' + (home ? 'Casa' : 'Fora') + '</span></button>';
    }).join('') + '</div>';
  }

  function renderSquad() {
    var d = S.draft, list = S.players[d.group_id];
    if (!d.group_id) return '<h2>Plantel do dia</h2><p class="hint">Escolha primeiro o escalão.</p>';
    if (!list) return '<h2>Plantel do dia</h2><p class="hint">A carregar jogadores…</p>';
    list = list.filter(function (p) { return !p.inactive; });
    var picked = {};
    d.squad.forEach(function (s) { picked[s.player_id] = s; });
    var n = d.squad.length;
    var html = '<h2>Plantel do dia <span class="count" id="jCount">' + n + ' de ' + MAX_SQUAD + '</span></h2>';
    if (!list.length) return html + '<p class="hint">Este escalão não tem jogadores. Adicione-os em <a href="#plantel" data-goto="plantel">Plantel</a>.</p>';
    html += '<p class="hint" style="margin-bottom:10px">Marque quem joga hoje. O número só muda para este jogo.</p>';
    html += '<p class="warn" id="jOver" style="margin-bottom:10px"' + (n > MAX_SQUAD ? '' : ' hidden') + '>Há mais de 12 jogadores marcados. Num jogo oficial só podem ir 12.</p>';
    html += '<div class="squad">' + list.map(function (p) {
      var s = picked[p.id];
      var id = 'sq-' + p.id;
      return '<div class="squad-row' + (s ? ' on' : '') + '">' +
        '<input type="checkbox" id="' + esc(id) + '" data-sq="' + esc(p.id) + '"' + (s ? ' checked' : '') + '>' +
        '<label for="' + esc(id) + '">' + esc(p.display_name) + '</label>' +
        '<input type="text" inputmode="numeric" maxlength="2" data-sqn="' + esc(p.id) + '" value="' + esc(s ? s.number : p.number) + '" aria-label="Número de ' + esc(p.display_name) + ' neste jogo"' + (s ? '' : ' disabled') + '>' +
        '</div>';
    }).join('') + '</div>';
    return html;
  }

  function gameEvents() {
    var root = $('#jogo');
    root.addEventListener('change', function (e) {
      var t = e.target, d = S.draft;
      if (t.id === 'jGroup') {
        d.group_id = t.value; d.squad = []; S.fpbGames = null; S.fpbGamesErr = '';
        renderJogo();
        loadPlayers(d.group_id).then(refreshSquadCard, showError);
      } else if (t.id === 'jColor') {
        d.away.color = t.value.toUpperCase(); renderJogo();
      } else if (t.id === 'jStart') {
        d.start = t.value ? isoLocal(new Date(t.value)) : '';
      } else if (t.id === 'jCrestFile' && t.files && t.files[0]) {
        var fd = new FormData(); fd.append('file', t.files[0]);
        busy(t.closest('label'), function () {
          return api('POST', '/api/game/opponent-logo', fd).then(function (r) { d.away.crest_url = r.crest_url; renderJogo(); });
        }, 'Carregado');
      } else if (t.dataset.sq) {
        var pid = t.dataset.sq;
        var p = (S.players[d.group_id] || []).filter(function (x) { return String(x.id) === pid; })[0];
        var numInput = $('[data-sqn="' + pid + '"]', root);
        if (t.checked) d.squad.push({ player_id: p.id, number: numInput.value || p.number });
        else d.squad = d.squad.filter(function (s) { return String(s.player_id) !== pid; });
        t.parentElement.classList.toggle('on', t.checked);
        numInput.disabled = !t.checked;
        $('#jCount').textContent = d.squad.length + ' de ' + MAX_SQUAD;
        $('#jOver').hidden = d.squad.length <= MAX_SQUAD;
      }
    });
    root.addEventListener('input', function (e) {
      var t = e.target, d = S.draft;
      if (t.id === 'jName') { d.away.name = t.value; $('#jNameC').textContent = t.value.length + '/20'; updateCrestTile(); }
      else if (t.id === 'jShort') { d.away.short = t.value.toUpperCase(); t.value = d.away.short; updateCrestTile(); }
      else if (t.id === 'jVenue') d.venue = t.value;
      else if (t.id === 'jComp') d.competition = t.value;
      else if (t.id === 'jRound') d.round = t.value;
      else if (t.dataset.sqn) {
        t.value = t.value.replace(/\D/g, '');
        d.squad.forEach(function (s) { if (String(s.player_id) === t.dataset.sqn) s.number = t.value; });
      }
    });
    root.addEventListener('click', function (e) {
      var go = e.target.closest('[data-goto]');
      if (go) { e.preventDefault(); showTab(go.dataset.goto); return; }
      var sw = e.target.closest('.swatch');
      if (sw) { S.draft.away.color = sw.dataset.color; renderJogo(); return; }
      var b = e.target.closest('[data-act]'); if (!b) return;
      var act = b.dataset.act, d = S.draft;
      if (act === 'game-newer') { takeGame(S.gameNewer); return; }
      if (act === 'fpb-games') {
        var g = groupById(d.group_id);
        busy(b, function () {
          S.fpbGamesErr = '';
          return api('GET', '/api/fpb/teams/' + encodeURIComponent(g.fpb_team_id) + '/next-games').then(function (r) {
            S.fpbGames = r || { games: [] }; renderJogo();
          }, function (err) { S.fpbGames = null; S.fpbGamesErr = err.message; renderJogo(); });
        }, ' ');
      } else if (act === 'manual') {
        var fresh = emptyGame();
        fresh.group_id = d.group_id; fresh.squad = d.squad;
        S.draft = fresh;
        S.fpbGames = null;
        renderJogo();
        $('#jName').focus();
      } else if (act === 'pick-game') {
        pickGame(S.fpbGames.games[+b.dataset.i], b);
      } else if (act === 'crest-clear') {
        d.away.crest_url = ''; renderJogo();
      } else if (act === 'save-game') {
        busy(b, saveGame);
      } else if (act === 'new-game') {
        confirmBox('Começar um jogo novo?', '<p>Apaga os resultados dos períodos do jogo anterior. O adversário e o plantel ficam como estão.</p>', { yes: 'Novo jogo', danger: true }).then(function (ok) {
          if (ok) busy(b, function () { return api('POST', '/api/game/reset'); }, 'Períodos apagados');
        });
      }
    });
  }

  function updateCrestTile() {
    var a = S.draft.away, tile = $('#jCrest');
    if (tile && !a.crest_url) tile.textContent = a.short || initials(a.name);
  }

  function pickGame(x, btn) {
    var d = S.draft;
    var teamId = S.fpbGames.team ? S.fpbGames.team.id : groupById(d.group_id).fpb_team_id;
    var home = String(x.home && x.home.id) === String(teamId);
    var opp = (home ? x.away : x.home) || {};
    d.fpb_game_id = x.id;
    d.away = {
      name: '', // the server fills it from the FPB name (opponentName in game.go)
      short: String(opp.short_name && opp.short_name.length <= 4 ? opp.short_name : initials(opp.name)).toUpperCase().slice(0, 4),
      color: '', crest_url: ''
    };
    d.start = isoLocal(fpbDate(x.date, x.time));
    d.venue = (x.location || '').replace(/\s*,\s*/g, ', ').trim();
    d.competition = x.competition_name || '';
    d.round = x.game_week ? 'Jornada ' + x.game_week : (x.phase_name || '');
    // Save straight away so the server fetches the crest and picks the colour.
    busy(btn, function () { return saveGame(); }, 'Jogo escolhido');
  }

  function saveGame() {
    var d = S.draft;
    if (!d.group_id) return Promise.reject(new Error('Escolha o escalão.'));
    var body = {
      group_id: d.group_id, fpb_game_id: d.fpb_game_id || null,
      away: { name: d.away.name.trim().slice(0, 20), short: d.away.short.trim().slice(0, 4), color: d.away.color || '', crest_url: d.away.crest_url || '' },
      start: d.start || '', venue: d.venue, competition: d.competition, round: d.round,
      squad: d.squad.map(function (s) { return { player_id: s.player_id, number: String(s.number || '') }; })
    };
    return api('PUT', '/api/game', body).then(function (r) {
      setGame(r || body);
      if (S.tab === 'jogo') {
        var msgText = 'Guardado';
        renderJogo();
        var m = $('.savebar .msg'); if (m) { m.className = 'msg ok'; m.textContent = msgText; }
      }
      renderEcras();
    });
  }

  /* ================= 2. ECRÃS ================= */
  var ecrasBuilt = false;
  function renderEcras() {
    var root = $('#ecras');
    var html = '<div class="ecras-grid"><div>' +
      '<div class="card"><h2>No ecrã agora</h2>' +
      '<div class="screens" id="screens">' + SCREENS.map(function (s) {
        return '<button class="scr' + (s.id === 'nada' ? ' nada' : '') + '" data-screen="' + s.id + '" aria-pressed="false">' +
          '<span class="scr-pv" aria-hidden="true"><svg viewBox="0 0 160 90">' + PV_COURT + (PV[s.id] || '') + '</svg></span>' +
          '<span class="scr-nome">' + esc(s.label) + '</span><small>' + esc(s.hint) + '</small></button>';
      }).join('') + '</div>' +
      '<div class="row" style="margin-top:14px;align-items:center">' +
        '<label class="switch"><input type="checkbox" id="autoScr"><i></i>Automático</label>' +
        '<span class="hint grow">Muda sozinho para Intervalo e Final, e volta ao Marcador.</span>' +
        '<span class="msg" id="scrMsg"></span>' +
      '</div>' +
      '<form class="row" id="vjForm" style="margin-top:14px;align-items:flex-end">' +
        '<label class="field grow"><span>Frase do Voltamos já <small>(opcional)</small></span><input id="vjLine" maxlength="60" autocomplete="off" placeholder="Ex.: A imagem regressa dentro de momentos"></label>' +
        '<button class="btn" type="submit">Guardar frase</button><span class="msg" id="vjMsg"></span>' +
      '</form>' +
      '<div class="row" style="margin-top:14px;align-items:center">' +
        '<button class="btn" type="button" data-act="resultado">Guardar imagem do resultado</button>' +
        '<span class="hint grow">PNG quadrado para as redes sociais.</span>' +
        '<span class="msg" id="resMsg"></span>' +
      '</div>' +
      '<div class="row" style="margin-top:10px;align-items:center">' +
        '<button class="btn" type="button" data-act="miniatura">Miniatura YouTube</button>' +
        '<span class="hint grow">Imagem 1280x720 do jogo escolhido, para a capa do direto.</span>' +
        '<span class="msg" id="minMsg"></span>' +
      '</div></div>' +
      '<div class="card"><h2>Rodapé com nome</h2>' +
        '<div id="ltNow"></div>' +
        '<span class="label">Jogador de hoje <small class="hint">(aparece 8 s)</small></span>' +
        '<div class="lt-grid" id="ltPlayers"></div>' +
        '<p class="msg" id="ltMsg"></p>' +
        '<form id="ltCom" style="margin-top:16px"><span class="label">Comentador</span><div class="row">' +
          '<label class="field grow"><span>Nome</span><input id="ltFirst" autocomplete="off"></label>' +
          '<label class="field grow"><span>Apelido</span><input id="ltLast" autocomplete="off"></label>' +
          '<label class="field grow"><span>Função</span><input id="ltLabel" value="Comentador" autocomplete="off"></label>' +
          '<button class="btn dark" type="submit">Mostrar</button>' +
        '</div></form>' +
      '</div></div>' +
      '<div><div class="card"><h2>Pré-visualização</h2><div class="preview" id="preview"><iframe id="pvFrame" title="Pré-visualização da transmissão" tabindex="-1"></iframe></div>' +
      '<p class="hint" style="margin-top:8px">É isto que o OBS mostra por cima da câmara.</p></div></div>' +
      '</div>';
    if (!ecrasBuilt) {
      root.innerHTML = html;
      ecrasBuilt = true;
      $('#pvFrame').src = MOCK ? '/overlay/index.html?painel=1' : '/overlay?painel=1';
      ecrasEvents();
    }
    renderLtPlayers();
    updateScreenButtons();
    updateLowerThird();
    var vj = $('#vjLine');
    if (vj && document.activeElement !== vj) vj.value = (S.settings && S.settings.overlay && S.settings.overlay.back_line) || '';
  }

  function nextLabel(n) {
    var d = new Date(n.start), opp = (n.opponent && n.opponent.name) || '';
    if (isNaN(d)) return opp;
    var when = new Intl.DateTimeFormat('pt-PT', { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Lisbon' }).format(d);
    return opp + ' · ' + when.replace(/\./g, '');
  }

  // Miniatura: preview first, then the PNG goes to Transferências.
  function miniatura(btn) {
    var m = $('#minMsg');
    if (!S.state) { m.className = 'msg err'; m.textContent = 'Ainda sem dados do jogo.'; return; }
    var st = S.state;
    btn.disabled = true; m.className = 'msg'; m.textContent = '';
    window.Miniatura.render(st).then(function (c) {
      return confirmBox('Miniatura YouTube', '<img class="mini-pv" src="' + c.toDataURL('image/png') + '" alt="Miniatura do jogo">' +
        '<p class="hint" style="margin-top:8px">' + esc(window.Miniatura.fileName(st)) + '</p>', { yes: 'Descarregar' }).then(function (ok) {
        if (ok) return window.Miniatura.download(c, st);
      });
    }).then(null, function () { m.className = 'msg err'; m.textContent = 'Não foi possível criar a imagem.'; })
      .then(function () { btn.disabled = false; });
  }

  function squadPlayers() {
    var g = S.game; if (!g || !g.group_id) return [];
    var list = S.players[g.group_id] || [];
    return (g.squad || []).map(function (s) {
      var p = list.filter(function (x) { return String(x.id) === String(s.player_id); })[0];
      return p && !p.inactive ? { id: p.id, name: p.display_name, number: s.number || p.number } : null;
    }).filter(Boolean).sort(byNumber);
  }

  function renderLtPlayers() {
    var el = $('#ltPlayers'); if (!el) return;
    var list = squadPlayers();
    if (!list.length) {
      el.innerHTML = '<p class="hint">Ainda não há plantel do dia. Marque os jogadores no separador <a href="#jogo" data-goto="jogo">Jogo</a>.</p>';
      return;
    }
    el.innerHTML = list.map(function (p) {
      return '<button class="lt" data-lt="' + esc(p.id) + '" data-num="' + esc(p.number) + '" aria-pressed="false"><b>' + esc(p.number) + '</b>' + esc(p.name) + '</button>';
    }).join('');
  }

  // What each screen puts on air, drawn small. The classes take the club's colours
  // from the custom properties paintScreens sets on #screens (painel.css has the fallbacks).
  var PV_COURT = '<rect class="pv-chao" width="160" height="90"/><path class="pv-linha" d="M80 0V90M0 22H18V68H0M160 22H142V68H160"/><circle class="pv-linha" cx="80" cy="45" r="12"/>';
  function pvRect(cls, x, y, w, h, rx) {
    return '<rect class="' + cls + '" x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '"' + (rx ? ' rx="' + rx + '"' : '') + '/>';
  }
  function pvShield(cls, x, y) { return '<path class="' + cls + '" d="M' + x + ' ' + y + 'h18v12c0 7-9 12-9 12s-9-5-9-12z"/>'; }
  var PV = {
    marcador: pvRect('pv-casa', 8, 8, 44, 10) + pvRect('pv-fora', 8, 18, 44, 10) + pvRect('pv-tinta', 52, 8, 18, 20) +
      pvRect('pv-texto', 12, 11.5, 16, 3) + pvRect('pv-texto', 12, 21.5, 16, 3) + pvRect('pv-texto', 40, 11, 8, 4) + pvRect('pv-texto', 40, 21, 8, 4) +
      pvRect('pv-papel', 55, 16.5, 12, 3),
    abertura: pvShield('pv-casa pv-orla', 44, 24) + pvShield('pv-fora pv-orla', 98, 24) + pvRect('pv-papel', 76, 35, 8, 2) + pvRect('pv-papel', 54, 62, 52, 5),
    plantel: [34, 28, 38, 30, 24].map(function (w, i) {
      var y = 10 + i * 14;
      return pvRect('pv-casa', 52, y, 56, 12) + pvRect('pv-texto', 55, y + 4, 6, 4) + pvRect('pv-texto', 66, y + 4.5, w, 3);
    }).join(''),
    desconto: pvRect('pv-tinta', 0, 68, 160, 22) + pvRect('pv-casa', 0, 68, 34, 22) + pvRect('pv-texto', 8, 77, 18, 4) +
      pvRect('pv-papel', 42, 74, 62, 3) + pvRect('pv-papel pv-meio', 42, 80, 40, 3) + pvRect('pv-papel', 132, 76, 20, 6),
    intervalo: pvRect('pv-papel', 38, 26, 84, 38) + pvRect('pv-casa', 38, 26, 42, 28) + pvRect('pv-fora', 80, 26, 42, 28) +
      pvRect('pv-texto', 52, 34, 14, 12) + pvRect('pv-texto', 94, 34, 14, 12) + pvRect('pv-tinta', 66, 57, 28, 3),
    final: pvRect('pv-tinta pv-orla', 38, 26, 84, 38) + pvRect('pv-casa', 42, 30, 36, 22) + pvRect('pv-fora', 82, 30, 36, 22) +
      pvRect('pv-texto', 53, 35, 14, 12) + pvRect('pv-texto', 93, 35, 14, 12) + pvRect('pv-papel', 70, 56, 20, 3),
    apoiantes: pvRect('pv-papel', 0, 62, 160, 28) + pvRect('pv-tinta', 10, 68, 30, 16) + pvRect('pv-oco', 46, 68, 30, 16) +
      pvRect('pv-casa', 82, 68, 30, 16) + pvRect('pv-oco', 118, 68, 30, 16),
    proximo: pvRect('pv-casa', 22, 30, 44, 26, 13) + pvRect('pv-fora', 94, 30, 44, 26, 13) + pvRect('pv-papel', 68, 30, 24, 12) +
      pvRect('pv-tinta pv-orla', 68, 44, 24, 12) + pvRect('pv-texto', 36, 41, 16, 4) + pvRect('pv-texto', 108, 41, 16, 4),
    voltamos: pvRect('pv-papel', 34, 30, 92, 30) + pvRect('pv-tinta', 50, 43, 60, 4)
  };
  function paintScreens(el, game) {
    var t = (game && game.theme) || {};
    [['--t-casa', (game && game.home && game.home.color) || t.red], ['--t-fora', game && game.away && game.away.color], ['--t-texto', t['on-red']]].forEach(function (p) {
      if (/^#[0-9a-f]{3,8}$/i.test(p[1] || '')) el.style.setProperty(p[0], p[1]); else el.style.removeProperty(p[0]);
    });
  }

  function updateScreenButtons() {
    var st = S.state; if (!st || !$('#screens')) return;
    paintScreens($('#screens'), st.game);
    var cur = st.screen && st.screen.current;
    $all('#screens .scr').forEach(function (b) { b.setAttribute('aria-pressed', b.dataset.screen === cur ? 'true' : 'false'); });
    // Próximo jogo only when the program found the next game; else the reason shows on the button.
    var px = $('#screens [data-screen="proximo"]'), next = st.game && st.game.next;
    if (px && !S.screenBusy) {
      px.disabled = !next;
      px.querySelector('small').textContent = next ? nextLabel(next) : (st.game && st.game.next_note) || 'Sem próximo jogo';
    }
    var auto = $('#autoScr');
    if (auto && !auto.disabled) auto.checked = !!(st.screen && st.screen.auto);
  }

  var ltTimer = null;
  function updateLowerThird() {
    var el = $('#ltNow'); if (!el || !S.state) return;
    var lt = S.state.lower_third || {};
    clearInterval(ltTimer);
    $all('#ltPlayers .lt').forEach(function (b) {
      b.setAttribute('aria-pressed', lt.visible && lt.type === 'jogador' && String(lt.number) === b.dataset.num ? 'true' : 'false');
    });
    if (!lt.visible) { el.innerHTML = ''; return; }
    function paint() {
      var left = lt.until ? Math.max(0, Math.ceil((new Date(lt.until).getTime() - Date.now()) / 1000)) : null;
      el.innerHTML = '<div class="onscreen"><b>No ecrã</b><span>' + (lt.number ? '#' + esc(lt.number) + ' ' : '') + esc(lt.first_name) + ' ' + esc(lt.last_name) +
        (lt.label ? ' · ' + esc(lt.label) : '') + '</span>' + (left !== null ? '<span class="hint">' + left + ' s</span>' : '') +
        '<span style="flex:1"></span><button class="btn small" data-act="lt-hide">Esconder</button></div>';
    }
    paint();
    if (lt.until) ltTimer = setInterval(paint, 1000);
  }

  function ecrasEvents() {
    var root = $('#ecras');
    root.addEventListener('click', function (e) {
      var go = e.target.closest('[data-goto]');
      if (go) { e.preventDefault(); showTab(go.dataset.goto); return; }
      var b = e.target.closest('[data-screen]');
      if (b) {
        var msg = $('#scrMsg');
        $all('#screens .scr').forEach(function (x) { x.disabled = true; });
        S.screenBusy = true;
        msg.className = 'msg'; msg.textContent = '';
        api('POST', '/api/screen', { screen: b.dataset.screen, auto: $('#autoScr').checked }).then(function () {
          if (S.state) { S.state.screen = Object.assign({}, S.state.screen, { current: b.dataset.screen }); updateScreenButtons(); }
        }, function (err) { msg.className = 'msg err'; msg.textContent = err.message; })
          .then(function () { $all('#screens .scr').forEach(function (x) { x.disabled = false; }); S.screenBusy = false; updateScreenButtons(); });
        return;
      }
      var res = e.target.closest('[data-act="resultado"]');
      if (res) {
        var rm = $('#resMsg');
        if (!S.state) { rm.className = 'msg err'; rm.textContent = 'Ainda sem dados do jogo.'; return; }
        res.disabled = true; rm.className = 'msg'; rm.textContent = '';
        window.Resultado.save(S.state).then(null, function () { rm.className = 'msg err'; rm.textContent = 'Não foi possível criar a imagem.'; })
          .then(function () { res.disabled = false; });
        return;
      }
      var mini = e.target.closest('[data-act="miniatura"]');
      if (mini) { miniatura(mini); return; }
      var lt = e.target.closest('[data-lt]');
      if (lt) { ltCall(lt, { player_id: lt.dataset.lt }); return; }
      var h = e.target.closest('[data-act="lt-hide"]');
      if (h) { h.disabled = true; api('DELETE', '/api/lower-third').catch(function (err) { $('#ltMsg').className = 'msg err'; $('#ltMsg').textContent = err.message; h.disabled = false; }); }
    });
    root.addEventListener('change', function (e) {
      if (e.target.id !== 'autoScr') return;
      var t = e.target, cur = (S.state && S.state.screen && S.state.screen.current) || 'marcador';
      t.disabled = true;
      api('POST', '/api/screen', { screen: cur, auto: t.checked }).then(null, function (err) {
        t.checked = !t.checked; $('#scrMsg').className = 'msg err'; $('#scrMsg').textContent = err.message;
      }).then(function () { t.disabled = false; });
    });
    $('#ltCom').addEventListener('submit', function (e) {
      e.preventDefault();
      var first = $('#ltFirst').value.trim(), last = $('#ltLast').value.trim();
      if (!first && !last) { $('#ltMsg').className = 'msg err'; $('#ltMsg').textContent = 'Escreva o nome do comentador.'; return; }
      ltCall(e.target.querySelector('button'), { type: 'comentador', first_name: first, last_name: last, label: $('#ltLabel').value.trim() });
    });
    $('#vjForm').addEventListener('submit', function (e) {
      e.preventDefault();
      if (!S.settings) return;
      var s = clone(S.settings);
      s.overlay = Object.assign({}, s.overlay, { back_line: $('#vjLine').value.trim() });
      busy(e.target.querySelector('button'), function () {
        return api('PUT', '/api/settings', s).then(function (r) { S.settings = r || s; $('#vjLine').value = S.settings.overlay.back_line || ''; });
      }, 'Guardada');
    });
  }

  function ltCall(btn, body) {
    var msg = $('#ltMsg');
    btn.disabled = true; msg.textContent = '';
    api('POST', '/api/lower-third', body).then(null, function (err) { msg.className = 'msg err'; msg.textContent = err.message; })
      .then(function () { btn.disabled = false; });
  }

  function fitPreview() {
    var box = $('#preview'), fr = $('#pvFrame');
    if (!box || !fr) return;
    fr.style.transform = 'scale(' + (box.clientWidth / 1920) + ')';
  }

  function renderChips() {
    var el = $('#chips'); if (!el) return;
    var st = S.state || {}, box = st.box || (S.status && S.status.box) || {}, s = S.status || {};
    var html = '';
    if (st.demo) html += '<span class="chip"><span class="dot wait"></span><b>Demonstração</b> dados a fingir</span>';
    var bt = (box.scan && box.scan.bluetooth) || 'ok';
    html += '<a class="chip link" href="#caixa" data-goto="caixa">' + (box.connected
      ? '<span class="dot on"></span><b>Caixa</b> Ligado' + (box.last_frame_at ? ' · ' + esc(ago(box.last_frame_at)) : '')
      : bt !== 'ok'
        ? '<span class="dot off"></span><b>Caixa</b> Bluetooth desligado'
        : '<span class="dot wait"></span><b>Caixa</b> À procura' + (box.last_frame_at ? ' · último sinal ' + esc(ago(box.last_frame_at)) : '')) + '</a>';
    if (obsOn()) {
      var obs = s.obs || {}, h = obs.health || {};
      var oc = !obs.connected ? ['off', 'Sem ligação', obs.error || ''] : !h.checked ? ['wait', 'A verificar…', ''] :
        h.ready ? ['on', 'Pronto', ''] : ['wait', 'Precisa de reparação', (h.problems || []).join(' ')];
      html += '<a class="chip link" href="#definicoes" data-goto="definicoes" title="' + esc(oc[2]) + '"><span class="dot ' + oc[0] + '"></span><b>OBS</b> ' + oc[1] + '</a>';
    }
    var yt = s.youtube;
    if (yt && yt.connected) {
      html += '<a class="chip link" href="#definicoes" data-goto="definicoes"><span class="dot ' + (yt.expired ? 'off' : 'on') + '"></span><b>YouTube</b> ' +
        (yt.expired ? 'Ligação expirou' : 'Ligado') + '</a>';
    }
    if (s.fpb) {
      html += '<span class="chip"><span class="dot ' + (s.fpb.reachable ? 'on' : 'wait') + '"></span><b>FPB</b> ' +
        (s.fpb.reachable ? 'Disponível' : 'Sem internet' + (s.fpb.cached_at ? ' · dados de ' + esc(new Date(s.fpb.cached_at).toLocaleDateString('pt-PT')) : '')) + '</span>';
    }
    el.innerHTML = html;
  }

  /* ================= OBS E TRANSMISSÃO ================= */
  function obsOn() { return !!(S.settings && S.settings.obs && S.settings.obs.enabled); }
  function hms(ms) {
    var t = Math.max(0, Math.floor(ms / 1000));
    return Math.floor(t / 3600) + ':' + pad(Math.floor(t % 3600 / 60)) + ':' + pad(t % 60);
  }
  function airMs() { var st = S.stream; return st && st.active ? st.duration_ms + (Date.now() - S.streamAt) : 0; }
  function loadStream() {
    if (!obsOn()) { setStream(null); return Promise.resolve(); }
    return api('GET', '/api/obs/stream').then(setStream, function () {});
  }
  function setStream(r) {
    S.stream = (r && r.stream) || null; S.streamAt = Date.now();
    S.ytStream = (r && r.youtube) || null;
    renderStream(); renderYtCard();
  }
  function obsHealth() { var o = (S.status && S.status.obs) || {}; return o.connected ? (o.health || {}) : {}; }

  // The stream bar at the top of Ecrãs, and "No ar" in the top bar.
  function renderStream() {
    var st = S.stream, live = !!(st && st.active), air = $('#liveAir');
    air.hidden = !live;
    tickStream();
    var el = $('#streamBar'); if (!el) return;
    el.hidden = !obsOn();
    if (el.hidden || el.dataset.busy) return;
    var h = obsHealth(), problem = h.checked && !h.ready && h.problems && h.problems[0];
    var yt = S.ytStream || {}, ytLive = live && yt.live, expired = yt.connected && yt.expired;
    var test = !!(ytLive && ytLive.test), canTest = yt.connected && !expired;
    var missing = live || !st ? [] : readiness(h, problem, expired);
    var key = [live, st && st.reconnecting, !!st, problem, ytLive && ytLive.link, expired, test, canTest,
      missing.map(function (m) { return m[1]; }).join(';')].join('|');
    if (el.dataset.key === key) return;
    el.dataset.key = key;
    var dot, title, sub, btn;
    if (live) {
      dot = 'live'; title = st.reconnecting ? 'A religar ao YouTube…' : test ? 'Teste no ar' : 'No ar';
      sub = test ? 'O direto é privado. Só a conta do canal o vê, no YouTube Studio. Para acabar, carregue em Terminar teste.' :
        'Para acabar o direto, carregue em Terminar transmissão.';
      btn = '<button class="btn dark" type="button" data-act="stream-stop">' + (test ? 'Terminar teste' : 'Terminar transmissão') + '</button>';
    } else if (!st) {
      dot = 'off'; title = 'Sem ligação ao OBS';
      sub = 'Abra o OBS, ou carregue em Configurar OBS em Definições.';
      btn = '<button class="btn primary" type="button" disabled>Iniciar transmissão</button>';
    } else {
      dot = missing.length ? '' : 'ready'; title = missing.length ? 'Fora do ar' : 'Pronto para transmitir';
      sub = missing.length ? 'Antes de começar:' :
        yt.connected ? 'Ao carregar no botão, o Hallstream cria o direto no YouTube e começa a transmitir. Para experimentar sem público, use Transmissão de teste.' :
        'O direto só começa quando carregar no botão.';
      btn = (canTest ? '<button class="btn" type="button" data-act="stream-test">Transmissão de teste</button>' : '') +
        '<button class="btn primary" type="button" data-act="stream-start">Iniciar transmissão</button>';
    }
    el.innerHTML = '<span class="dot big ' + dot + '"></span>' +
      '<div class="grow"><h2>' + esc(title) + (live ? ' <span class="air-time" id="airTime"></span>' : '') + '</h2>' +
      '<p class="hint">' + esc(sub) + '</p>' +
      (missing.length ? '<ul class="ready">' + missing.map(function (m) {
        return '<li><span class="dot ' + m[0] + '"></span>' + (m[2] ? '<a href="#' + m[2] + '" data-goto="' + m[2] + '">' + esc(m[1]) + '</a>' : esc(m[1])) + '</li>';
      }).join('') + '</ul>' : '') +
      (ytLive ? '<div class="yt-link"><span>No YouTube</span><a href="' + esc(ytLive.link) + '" target="_blank" rel="noopener">' + esc(ytLive.link.replace(/^https:\/\//, '')) + '</a>' +
        '<button class="btn small" type="button" data-act="yt-copy" data-url="' + esc(ytLive.link) + '">Copiar</button><span class="msg" role="status"></span></div>' : '') +
      '</div>' +
      '<div class="row">' + (problem ? '<button class="btn" type="button" data-act="obs-repair">Reparar</button>' : '') + btn + '</div>' +
      '<p class="msg" id="streamMsg" role="status"></p>';
    tickStream();
  }
  // What is missing before a start, as [dot, text, tab to fix it]. The
  // buttons stay on: the start is the volunteer's call.
  function readiness(h, problem, expired) {
    var s = S.state || {}, box = s.box || (S.status && S.status.box) || {}, g = S.game || {}, out = [];
    if (!box.connected && !s.demo) out.push(['off', 'A caixa não está ligada.', 'caixa']);
    if (!g.group_id || !(g.away && g.away.name)) out.push(['wait', 'Falta escolher o jogo de hoje.', 'jogo']);
    if (problem) out.push(['off', 'OBS: ' + problem, '']);
    else if (!h.checked) out.push(['wait', 'A verificar o OBS…', '']);
    if (expired) out.push(['off', 'A ligação ao YouTube expirou. Ligue-o outra vez em Definições.', 'definicoes']);
    return out;
  }
  function tickStream() {
    var t = hms(airMs()), air = $('#liveAir'), at = $('#airTime');
    var yl = S.ytStream && S.ytStream.live;
    if (!air.hidden) air.textContent = (yl && yl.test ? 'Teste ' : 'No ar ') + t;
    if (at) at.textContent = t;
  }
  function streamCall(btn, path, body) {
    var el = $('#streamBar'), msg = $('#streamMsg');
    el.dataset.busy = '1'; btn.disabled = true;
    msg.className = 'msg'; msg.textContent = path.slice(-5) === 'start' ? 'A ligar ao YouTube…' : 'A terminar…';
    return api('POST', path, body || {}).then(function (r) {
      delete el.dataset.busy; el.dataset.key = ''; setStream(r);
      var live = r && r.youtube && r.youtube.live;
      if (live && !live.test && !live.thumbnail_set) sendThumbnail(live.broadcast_id);
    }, function (err) {
      delete el.dataset.busy; btn.disabled = false;
      msg.className = 'msg err'; msg.textContent = err.message;
    });
  }
  function streamEvents() {
    $('#streamBar').addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]'); if (!b) return;
      if (b.dataset.act === 'stream-start') streamCall(b, '/api/obs/stream/start');
      if (b.dataset.act === 'stream-test') streamCall(b, '/api/obs/stream/start', { test: true });
      if (b.dataset.act === 'stream-stop') {
        var yl = S.ytStream && S.ytStream.live;
        (yl && yl.test ? Promise.resolve(true) :
          confirmBox('Terminar a transmissão?', '<p>O direto no YouTube acaba para quem está a ver.</p>', { yes: 'Terminar', danger: true })).then(function (yes) {
          // The bar may have been drawn again while the question was open.
          var now = $('#streamBar [data-act="stream-stop"]');
          if (yes && now) streamCall(now, '/api/obs/stream/stop');
        });
      }
      if (b.dataset.act === 'obs-repair') repairOBS();
      if (b.dataset.act === 'yt-copy') busy(b, function () { return copyText(b.dataset.url); }, 'Copiado');
    });
    $('#ytCard').addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]'); if (!b) return;
      if (b.dataset.act === 'yt-copy') busy(b, function () { return copyText(b.dataset.url); }, 'Copiado');
      if (b.dataset.act === 'yt-unlist') {
        busy(b, function () {
          return api('POST', '/api/youtube/unlist', { broadcast_id: b.dataset.id }).then(loadStream);
        }, 'Feito');
      }
    });
  }

  // The thumbnail is the Miniatura YouTube image, drawn here and sent to the
  // program, which hands it to YouTube. The stream goes on if YouTube says no.
  function sendThumbnail(id) {
    if (!S.state || !window.Miniatura) return;
    var msg = $('#streamMsg');
    window.Miniatura.render(S.state).then(function (c) {
      return new Promise(function (resolve, reject) { c.toBlob(function (b) { b ? resolve(b) : reject(new Error('PNG vazio')); }, 'image/png'); });
    }).then(function (blob) {
      var fd = new FormData();
      fd.append('broadcast_id', id);
      fd.append('file', blob, 'miniatura.png');
      return api('POST', '/api/youtube/thumbnail', fd);
    }).then(function () {
      if (msg) { msg.className = 'msg ok'; msg.textContent = 'Miniatura enviada para o YouTube.'; }
    }, function (err) {
      if (msg) { msg.className = 'msg err'; msg.textContent = 'O direto está no ar, mas a miniatura falhou. ' + err.message; }
    });
  }

  // After Terminar: the video turns unlisted, then the link to share.
  function renderYtCard() {
    var el = $('#ytCard'); if (!el) return;
    var yt = S.ytStream || {}, e = yt.ended;
    var live = S.stream && S.stream.active;
    el.hidden = !e || live;
    if (el.hidden) { el.dataset.key = ''; return; }
    var key = [e.broadcast_id, e.privacy, e.privacy_error, yt.waiting].join('|');
    if (el.dataset.key === key) return;
    el.dataset.key = key;
    var share = (e.result || e.title) + (e.escalao ? ' · ' + e.escalao : '') + '\n' + e.link;
    var html;
    if (e.test) {
      html = '<h2>Teste terminado</h2><p>O vídeo ficou privado: só a conta do canal o vê, em YouTube Studio &gt; Conteúdo &gt; Diretos. Pode apagá-lo lá. Link: <a href="' + esc(e.link) + '" target="_blank" rel="noopener">' + esc(e.link) + '</a></p>';
    } else if (e.privacy === 'unlisted') {
      html = '<h2>Vídeo não listado</h2><p>Só quem tiver o link vê o jogo. Link: <a href="' + esc(e.link) + '" target="_blank" rel="noopener">' + esc(e.link) + '</a></p>' +
        '<div class="row"><button class="btn dark" type="button" data-act="yt-copy" data-url="' + esc(e.link) + '">Copiar</button>' +
        '<a class="btn" href="https://wa.me/?text=' + encodeURIComponent(share) + '" target="_blank" rel="noopener">Partilhar no WhatsApp</a><span class="msg" role="status"></span></div>';
    } else if (e.privacy_error) {
      html = '<h2>O vídeo ainda está público</h2><p class="alert">' + esc(e.privacy_error) + '</p>' +
        '<div class="row" style="margin-top:12px"><button class="btn primary" type="button" data-act="yt-unlist" data-id="' + esc(e.broadcast_id) + '">Tentar outra vez</button>' +
        '<button class="btn" type="button" data-act="yt-copy" data-url="' + esc(e.link) + '">Copiar link</button><span class="msg" role="status"></span></div>';
    } else {
      html = '<h2>A terminar no YouTube…</h2><p class="hint">O YouTube fecha o direto cerca de um minuto depois. Então o Hallstream torna o vídeo não listado e o link aparece aqui.</p>';
    }
    el.innerHTML = html;
  }

  // "Reparar" runs Configurar OBS again where its checklist shows.
  function repairOBS() {
    showTab('definicoes');
    var b = $('[data-act="obs-setup"]');
    if (b) { b.scrollIntoView({ block: 'center' }); runSetup(b, ''); }
  }

  var STEP_TAG = { ok: 'Certo', created: 'Criado', fixed: 'Corrigido', failed: 'Falhou', choose: 'Escolha', later: 'Depois' };
  function setupHtml(r) {
    if (!r) return '';
    var html = '<ul class="checklist">' + (r.steps || []).map(function (st) {
      return '<li class="ck ck-' + esc(st.result) + '"><span class="ck-tag">' + esc(STEP_TAG[st.result] || st.result) + '</span>' +
        '<div><b>' + esc(st.label) + '</b><span class="hint">' + esc(st.detail) + '</span></div></li>';
    }).join('') + '</ul>';
    if (r.choices && r.choices.length) {
      html += '<fieldset class="picks"><legend>Que câmara quer usar?</legend>' + r.choices.map(function (c, i) {
        var label = c.type === 'source' ? 'Usar a fonte ' + c.label : c.type === 'device' ? c.label + ' (juntar ao OBS)' : c.label;
        return '<label class="pick"><input type="radio" name="obsPick" value="' + esc(c.value) + '"' + (i === 0 ? ' checked' : '') + '>' + esc(label) + '</label>';
      }).join('') + '</fieldset>' +
        '<div class="row"><button class="btn primary" type="button" data-act="obs-pick">Usar esta câmara</button></div>';
    }
    html += r.ok ? '<p class="msg ok">O OBS está pronto.</p>' : r.choices && r.choices.length ? '' : '<p class="msg err">Falta resolver o que diz Falhou.</p>';
    return html;
  }
  // Saves the form first, so the camera picked there is the one set up.
  function runSetup(btn, camera) {
    var msg = $('#obsSetupMsg'), out = $('#obsResult');
    var sel = $('#dObsCam'), pick = sel && /^(device:|plugin$)/.test(sel.value) ? sel.value : '';
    btn.disabled = true; msg.className = 'msg'; msg.textContent = 'A configurar o OBS… Pode demorar um minuto, se tiver de o abrir.';
    var s = readSettingsForm(), old = (S.settings.obs && S.settings.obs.camera_source) || '';
    return api('PUT', '/api/settings', s).then(function (r) {
      S.settings = r || s;
      return api('POST', '/api/obs/setup', { camera: camera || pick, old: old });
    }).then(function (r) {
      S.obsSetup = r; msg.textContent = '';
      return api('GET', '/api/settings').then(function (set) { S.settings = set; });
    }).then(function () {
      S.obsCams = null;
      renderDefinicoes(); loadStatus(); loadStream();
    }, function (err) {
      msg.className = 'msg err'; msg.textContent = err.message;
      if (out) out.innerHTML = '';
    }).then(function () { var b = $('[data-act="obs-setup"]'); if (b) b.disabled = false; });
  }

  /* ================= 7. CAIXA ================= */
  var devTimer = null, caixaBuilt = false;
  function loadDevices() {
    return api('GET', '/api/ble/devices').then(function (list) {
      S.devices = list || []; S.devicesErr = '';
    }, function (err) { S.devicesErr = err.message; }).then(renderDevices);
  }

  function renderCaixa() {
    if (!caixaBuilt) {
      $('#caixa').innerHTML =
        '<div class="card" id="cxStatus"></div>' +
        '<div id="cxBanner"></div>' +
        '<p class="warn">Feche a app SL Live Stream no tablet. A caixa só aceita uma ligação.</p>' +
        '<div class="card"><h2>Dispositivos por perto <span class="count" id="cxCount"></span></h2>' +
        '<p class="hint">Se a caixa não ligar sozinha, carregue em <b>Experimentar</b> no dispositivo com mais sinal. A lista atualiza-se a cada 2 segundos.</p>' +
        '<ul class="devs" id="cxList"></ul></div>';
      caixaBuilt = true;
    }
    renderCaixaStatus();
    renderDevices();
  }

  function scanState() {
    var st = S.state || {}, box = st.box || {};
    return { box: box, scan: box.scan || {}, demo: !!st.demo };
  }

  function renderCaixaStatus() {
    var el = $('#cxStatus'); if (!el) return;
    var c = scanState(), sc = c.scan, bt = sc.bluetooth || 'ok';
    var dot, title;
    if (c.demo) { dot = 'wait'; title = 'Demonstração: a caixa não está a ser usada'; }
    else if (bt !== 'ok') { dot = 'off'; title = 'Bluetooth desligado'; }
    else if (c.box.connected) { dot = 'on'; title = 'Ligado a ' + (sc.name || 'caixa'); }
    else { dot = 'wait'; title = 'À procura'; }
    var noDevices = !c.box.connected && S.devices !== null && !S.devices.length;
    var want = c.demo ? '' : bt !== 'ok' ? 'off' : noDevices ? 'none' : '';
    var html = '<div class="cx-status"><span class="dot big ' + dot + '"></span><div class="grow">' +
      '<h2>' + esc(title) + (c.box.connected && sc.address ? ' <code>' + esc(sc.address) + '</code>' : '') + '</h2>' +
      (sc.message && !want ? '<p class="cx-msg">' + esc(sc.message) + '</p>' : '') +
      (sc.saved && !c.box.connected ? '<p class="hint">Caixa guardada: <code>' + esc(sc.saved) + '</code></p>' : '') +
      '</div></div>' +
      (sc.saved ? '<div class="row" style="margin-top:12px;align-items:center"><button class="btn" data-act="cx-forget">Esquecer caixa</button>' +
        '<span class="hint grow">Volta a procurar qualquer caixa, em vez desta.</span><span class="msg"></span></div>' : '');
    // SSE can fire many times a second: only touch the DOM when it changes, so button messages stay.
    if (el.dataset.h !== html) { el.dataset.h = html; el.innerHTML = html; }
    var banner = $('#cxBanner');
    if (banner.dataset.kind === want) return;
    banner.dataset.kind = want;
    banner.innerHTML = !want ? '' :
      '<div class="alert cx-banner"><p>' + (want === 'off' ? 'O Bluetooth do computador está desligado ou não existe.' : 'Não vejo nenhum dispositivo Bluetooth. O Bluetooth do PC está ligado?') + '</p>' +
      '<div class="row"><button class="btn dark" data-act="cx-settings">Abrir definições de Bluetooth</button><span class="msg"></span></div></div>';
  }

  function bars(rssi) {
    var n = rssi >= -60 ? 4 : rssi >= -70 ? 3 : rssi >= -80 ? 2 : rssi >= -90 ? 1 : 0;
    var word = ['muito fraco', 'fraco', 'médio', 'bom', 'forte'][n];
    var h = '<span class="bars" role="img" aria-label="Sinal ' + word + ' (' + rssi + ' dBm)">';
    for (var i = 1; i <= 4; i++) h += '<i' + (i <= n ? ' class="on"' : '') + '></i>';
    return h + '</span>';
  }

  var devKey = '';
  function renderDevices() {
    var el = $('#cxList'); if (!el) return;
    renderCaixaStatus();
    var list = S.devices, c = scanState();
    var key = JSON.stringify([list, S.devicesErr, S.tries, c.scan.address]);
    if (key === devKey) return;
    devKey = key;
    $('#cxCount').textContent = list && list.length ? '(' + list.length + ')' : '';
    if (S.devicesErr) { el.innerHTML = '<li class="alert">' + esc(S.devicesErr) + '</li>'; return; }
    if (!list) { el.innerHTML = '<li class="hint">A carregar…</li>'; return; }
    if (!list.length) { el.innerHTML = '<li class="hint">Ainda não ouvi nenhum dispositivo.</li>'; return; }
    el.innerHTML = list.map(function (d) {
      var t = S.tries[d.address] || {}, isBox = c.box.connected && c.scan.address === d.address;
      return '<li class="dev' + (d.likely ? ' likely' : '') + '">' +
        '<div class="who">' +
          '<b class="name">' + (d.name ? esc(d.name_quoted || d.name) : '<i>(sem nome)</i>') + '</b>' +
          (d.likely ? ' <span class="badge gold">Provável</span>' : '') +
          '<small><code>' + esc(d.address) + '</code>' + (d.services && d.services.length ? ' · serviços ' + esc(d.services.join(', ')) : '') + '</small>' +
        '</div>' +
        '<div class="sig">' + bars(d.rssi) + '<small>' + esc(d.rssi) + ' dBm</small></div>' +
        '<div class="act">' + (isBox ? '<span class="in-use"><span class="dot on"></span>Ligada</span>' :
          '<button class="btn' + (d.likely ? ' primary' : '') + '" data-try="' + esc(d.address) + '"' + (t.busy ? ' disabled' : '') + '>' +
            (t.busy ? '<span class="spin" aria-hidden="true"></span>A ligar…' : 'Experimentar') + '</button>') +
        '</div>' +
        (t.msg ? '<p class="msg ' + (t.ok ? 'ok' : 'err') + ' dev-msg" role="status">' + esc(t.msg) + '</p>' : '') +
        '</li>';
    }).join('');
  }

  function caixaEvents() {
    $('#caixa').addEventListener('click', function (e) {
      var tb = e.target.closest('[data-try]');
      if (tb) {
        var addr = tb.dataset.try;
        S.tries = {}; S.tries[addr] = { busy: true };
        renderDevices();
        api('POST', '/api/ble/try', { address: addr }).then(function (r) {
          S.tries[addr] = r && r.ok ? { ok: true, msg: 'É a caixa. Ligado e guardado.' } : { msg: (r && r.error) || 'Não consegui ligar.' };
        }, function (err) { S.tries[addr] = { msg: err.message }; }).then(renderDevices);
        return;
      }
      var b = e.target.closest('[data-act]'); if (!b) return;
      if (b.dataset.act === 'cx-settings') {
        busy(b, function () { return api('POST', '/api/system/bluetooth-settings'); }, 'Abri as definições no computador.');
      } else if (b.dataset.act === 'cx-forget') {
        confirmBox('Esquecer caixa', 'O programa deixa de procurar só esta caixa e liga à primeira que encontrar.', { yes: 'Esquecer' }).then(function (yes) {
          if (yes) busy(b, function () { S.tries = {}; return api('POST', '/api/ble/forget'); }, 'Esquecida. A procurar…');
        });
      }
    });
  }

  /* ================= 3. PLANTEL ================= */
  function groupSelect(id, cur, withGeral) {
    var opts = (withGeral ? '<option value="geral"' + (cur === 'geral' ? ' selected' : '') + '>Geral (todos os escalões)</option>' : '') +
      S.groups.map(function (g) { return '<option value="' + esc(g.id) + '"' + (g.id === cur ? ' selected' : '') + '>' + esc(g.name) + '</option>'; }).join('');
    return '<select id="' + id + '">' + opts + '</select>';
  }

  function renderPlantel() {
    var root = $('#plantel');
    if (!S.plantelGid || !groupById(S.plantelGid)) S.plantelGid = (S.draft && S.draft.group_id) || (S.groups[0] && S.groups[0].id) || null;
    var gid = S.plantelGid, g = groupById(gid);
    var html = '<div class="card"><div class="row">' +
      (S.groups.length ? '<label class="field grow"><span>Escalão</span>' + groupSelect('pGroup', gid) + '</label>' : '<p class="grow">Ainda não há escalões.</p>') +
      '<form class="row" id="pNewGroup"><label class="field"><span>Novo escalão</span><input id="pNewGroupName" placeholder="ex.: Sub 14 Feminino"></label><button class="btn" type="submit">Criar</button><span class="msg"></span></form>' +
      '</div></div>';
    var fpb = fpbOffered();
    if (fpb && !S.groups.some(function (x) { return x.fpb_team_id; })) {
      var c = S.status && S.status.club_setup, run = c && c.running;
      html += '<div class="card"><h2>Equipas do clube</h2><p class="hint">Cria um escalão para cada equipa do clube na FPB, com os jogadores e as fotografias. Pode demorar um minuto.</p>' +
        '<div class="row" style="margin-top:12px"><button class="btn dark" data-act="club-setup"' + (run ? ' disabled' : '') + '>Importar equipas do clube</button>' +
        '<span class="msg" id="clubMsg">' + (run ? esc(clubProgress(c)) : '') + '</span></div></div>';
    }
    if (!g) { root.innerHTML = html; return; }

    html += '<div class="cols">';
    html += '<div class="card"><h2>Escalão</h2><div class="stack">' +
      '<label class="field"><span>Nome</span><input id="pGName" value="' + esc(g.name) + '"></label>' +
      '<label class="field"><span>Treinador</span><input id="pCoach" value="' + esc(g.coach || '') + '" placeholder="Nome Apelido"></label>' +
      '<div class="row"><button class="btn primary" data-act="save-group">Guardar</button><span class="msg"></span><span style="flex:1"></span>' +
      '<button class="btn quiet" data-act="del-group">Apagar escalão</button></div></div></div>';

    if (fpb) {
      html += '<div class="card"><h2>FPB</h2>';
      if (g.fpb_team_id) {
        html += '<p><span class="badge">FPB</span> Ligado a <b>' + esc(g.fpb_team_name || ('equipa ' + g.fpb_team_id)) + '</b></p>' +
          (g.needs_team ? '<p class="warn" style="margin:8px 0">A FPB tem uma época nova: escolha a equipa deste escalão.</p>' : '') +
          (g.last_sync ? '<p class="hint" style="margin:8px 0">Atualizado da FPB ' + esc(since(g.last_sync)) + ': ' + esc(syncSummary(g.last_changes)) + '.</p>' : '') +
          '<div class="row"><button class="btn dark" data-act="import">Importar da FPB</button><span class="msg"></span>' +
          '<button class="btn quiet" data-act="fpb-change">' + (g.needs_team ? 'Escolher equipa' : 'Mudar equipa') + '</button></div>' +
          '<p class="hint" style="margin-top:8px">Traz os jogadores que a FPB conhece. Os nomes e números que escreveu aqui não mudam.</p>';
      }
      if (!g.fpb_team_id || S.fpbTeams) {
        html += (g.fpb_team_id ? '<hr style="border:0;border-top:1px solid var(--line-soft);margin:16px 0">' : '<p class="hint">Ligue este escalão à equipa da FPB para trazer jogos e jogadores. Só é preciso uma vez.</p>') +
          '<form class="row" id="pFpbSearch"><label class="field grow"><span>Procurar equipa na FPB</span><input type="search" id="pFpbName" value="Casa"></label>' +
          '<button class="btn dark" type="submit">Procurar</button><span class="msg"></span></form>';
        if (S.fpbTeams) {
          html += '<div class="stack" style="margin-top:12px">' + (S.fpbTeams.length ? S.fpbTeams.map(function (t, i) {
            return '<button class="team-pick" data-act="fpb-pick" data-i="' + i + '"><span><b>' + esc(t.name) + '</b><br><small>' +
              esc([t.age, genderLabel(t.gender), t.squad_label].filter(Boolean).join(' · ')) + '</small></span><span class="btn small dark" aria-hidden="true">Escolher</span></button>';
          }).join('') : '<p class="hint">Nenhuma equipa com esse nome.</p>') + '</div>';
        }
      }
      html += '</div>';
    }
    html += '</div>';

    if (S.importResult && S.importResult.gid === gid) html += renderImport();

    var all = S.players[gid], ph = (S.settings && S.settings.photos) || {};
    var list = all && all.filter(function (p) { return !p.inactive; });
    var gone = all ? all.filter(function (p) { return p.inactive; }) : [];
    html += '<div class="card"><h2>Jogadores' + (list ? ' <span class="count">' + list.length + '</span>' : '') + '</h2>';
    html += '<div class="row photo-sw"><label class="switch"><input type="checkbox" data-photo="squad"' + (ph.squad ? ' checked' : '') + '><i></i>Mostrar fotos no plantel</label>' +
      '<label class="switch"><input type="checkbox" data-photo="lower_third"' + (ph.lower_third ? ' checked' : '') + '><i></i>Mostrar foto no rodapé</label><span class="msg"></span></div>' +
      '<p class="hint" style="margin-bottom:12px">As fotografias vêm da FPB quando importa ou liga um jogador. Quem não tem fotografia aparece com as iniciais.</p>';
    if (!list) html += '<p class="hint">A carregar…</p>';
    else {
      html += '<table class="players"><thead><tr><th>Nº</th><th>Nome</th><th></th></tr></thead><tbody>' + list.map(function (p) {
        var differs = p.fpb_name && p.fpb_name.toLowerCase() !== String(p.display_name).toLowerCase();
        return '<tr data-pid="' + esc(p.id) + '">' +
          '<td class="n"><input type="text" inputmode="numeric" maxlength="2" data-f="number" value="' + esc(p.number) + '" aria-label="Número"></td>' +
          '<td><div class="pw">' + thumb(p.photo_url, p.display_name) + '<div><input data-f="display_name" value="' + esc(p.display_name) + '" aria-label="Nome">' +
            (p.fpb_player_id ? '<div class="fpb"><span class="badge">FPB</span>' + (differs ? '<span>' + esc(p.fpb_name) + '</span>' : '') + '</div>' : '') + '</div></div></td>' +
          '<td class="act"><span class="msg"></span> ' +
            (p.fpb_player_id ? '<button class="btn quiet small" data-act="unlink">Desligar</button>' : '') +
            '<button class="btn danger small" data-act="del-player">Apagar</button></td></tr>';
      }).join('') + '</tbody></table>';
      if (!list.length) html += '<p class="hint">Sem jogadores. Adicione abaixo' + (g.fpb_team_id ? ' ou importe da FPB.' : '.') + '</p>';
      if (gone.length) {
        html += '<details style="margin-top:16px"><summary>Já não estão na equipa <span class="count">' + gone.length + '</span></summary>' +
          '<p class="hint" style="margin:8px 0">Não estão no plantel da FPB desta época, por isso não aparecem nos jogos.</p>' +
          gone.map(function (p) {
            return '<div class="row" data-pid="' + esc(p.id) + '" style="margin-top:8px"><span class="grow">' + esc((p.number ? p.number + ' · ' : '') + p.display_name) + '</span>' +
              '<button class="btn small" data-act="unhide">Voltar a mostrar</button><span class="msg"></span></div>';
          }).join('') + '</details>';
      }
    }
    html += '<form class="row" id="pAdd" style="margin-top:16px;padding-top:16px;border-top:1px solid var(--line-soft)">' +
      '<label class="field" style="width:84px"><span>Nº</span><input id="pAddNum" inputmode="numeric" maxlength="2"></label>' +
      '<label class="field grow"><span>Nome do jogador</span><input id="pAddName" autocomplete="off"></label>' +
      '<button class="btn primary" type="submit">Adicionar jogador</button><span class="msg"></span></form>';
    html += '</div>';
    root.innerHTML = html;
    if (!list) loadPlayers(gid).then(function () { if (S.tab === 'plantel') renderPlantel(); }, showError);
  }

  function renderImport() {
    var r = S.importResult, list = S.players[r.gid] || [];
    var html = '<div class="card"><h2>Importação da FPB</h2>';
    html += '<p>' + (r.updated.length ? '<b>' + r.updated.length + '</b> ' + (r.updated.length === 1 ? 'jogador ligado foi atualizado' : 'jogadores ligados foram atualizados') + '.' : 'Nenhum jogador ligado precisou de mudança.') + '</p>';
    if (r.photoErrors) html += '<p class="hint">Não foi possível descarregar ' + (r.photoErrors === 1 ? '1 fotografia' : r.photoErrors + ' fotografias') + ' da FPB. Volte a importar mais tarde.</p>';
    if (!r.suggestions.length) return html + '<p class="hint">Não há jogadores novos na FPB.</p></div>';
    html += '<p class="hint">A FPB tem estes jogadores que ainda não estão ligados. Adicione-os como novos, ou ligue-os a um jogador que já existe.</p>';
    html += r.suggestions.map(function (s, i) {
      var linking = S.linking[s.fpb_player_id];
      var opts = list.filter(function (p) { return !p.fpb_player_id; });
      var best = linking ? linking.choice : s.best_match;
      var bestP = list.filter(function (p) { return String(p.id) === String(s.best_match); })[0];
      return '<div class="sugg" data-si="' + i + '"><div class="pw">' + thumb(s.photo_url, s.name) + '<div class="who"><b>' + esc(s.number || '–') + '</b>' + esc(s.name) +
        '<small>' + esc(s.games) + ' jogos' + (bestP ? ' · parece ser <b>' + esc(bestP.display_name) + '</b>' : '') + '</small></div></div>' +
        '<div class="row"><button class="btn small" data-act="sug-add">Adicionar</button>' +
        '<button class="btn small dark" data-act="sug-link"' + (opts.length ? '' : ' disabled') + '>Ligar a…</button><span class="msg"></span></div>' +
        (linking ? '<div class="linkbox"><label class="field"><span>Ligar a este jogador</span><select data-link-choice>' +
          '<option value="">Escolha…</option>' + opts.map(function (p) {
            return '<option value="' + esc(p.id) + '"' + (String(p.id) === String(best) ? ' selected' : '') + '>' + esc((p.number ? p.number + ' · ' : '') + p.display_name) + '</option>';
          }).join('') + '</select></label><button class="btn small primary" data-act="sug-confirm">Confirmar</button>' +
          '<button class="btn small quiet" data-act="sug-cancel">Cancelar</button><span class="msg"></span></div>' : '') +
        '</div>';
    }).join('');
    return html + '</div>';
  }

  function plantelEvents() {
    var root = $('#plantel');
    function gid() { return S.plantelGid; }
    function base() { return '/api/groups/' + encodeURIComponent(gid()); }
    function refresh() { return loadPlayers(gid()).then(function () { if (S.tab === 'plantel') renderPlantel(); }); }

    // A photo that does not load (offline, gone from the FPB) leaves the initials.
    root.addEventListener('error', function (e) { if (e.target.closest && e.target.closest('.thumb')) e.target.remove(); }, true);

    root.addEventListener('change', function (e) {
      var t = e.target;
      if (t.id === 'pGroup') { S.plantelGid = t.value; S.fpbTeams = null; renderPlantel(); return; }
      if (t.dataset.photo) {
        var s = clone(S.settings), pm = t.closest('.row').querySelector('.msg');
        s.photos = Object.assign({ squad: false, lower_third: false }, s.photos);
        s.photos[t.dataset.photo] = t.checked;
        t.disabled = true; pm.className = 'msg'; pm.textContent = '';
        api('PUT', '/api/settings', s).then(function (r) {
          S.settings = r || s; pm.className = 'msg ok'; pm.textContent = 'Guardado';
        }, function (err) { t.checked = !t.checked; pm.className = 'msg err'; pm.textContent = err.message; })
          .then(function () { t.disabled = false; });
        return;
      }
      if (t.hasAttribute('data-link-choice')) {
        var si = +t.closest('[data-si]').dataset.si;
        S.linking[S.importResult.suggestions[si].fpb_player_id].choice = t.value;
        return;
      }
      var tr = t.closest('tr[data-pid]');
      if (tr && t.dataset.f) {
        var pid = tr.dataset.pid;
        var body = { display_name: $('[data-f="display_name"]', tr).value.trim(), number: $('[data-f="number"]', tr).value.replace(/\D/g, '') };
        if (!body.display_name) { var m0 = $('.msg', tr); m0.className = 'msg err'; m0.textContent = 'O nome não pode ficar vazio.'; return; }
        var msg = $('.msg', tr);
        msg.className = 'msg'; msg.textContent = 'A guardar…';
        api('PUT', base() + '/players/' + encodeURIComponent(pid), body).then(function (r) {
          var list = S.players[gid()] || [];
          list.forEach(function (p, i) { if (String(p.id) === pid) list[i] = r || Object.assign(p, body); });
          msg.className = 'msg ok'; msg.textContent = 'Guardado';
          setTimeout(function () { if (msg.textContent === 'Guardado') msg.textContent = ''; }, 2500);
        }, function (err) { msg.className = 'msg err'; msg.textContent = err.message; });
      }
    });

    root.addEventListener('submit', function (e) {
      e.preventDefault();
      var f = e.target, btn = f.querySelector('button[type="submit"]');
      if (f.id === 'pNewGroup') {
        var name = $('#pNewGroupName').value.trim();
        if (!name) return;
        busy(btn, function () {
          return api('POST', '/api/groups', { name: name }).then(function (g) {
            return loadGroups().then(function () { S.plantelGid = g && g.id; if (S.draft && !S.draft.group_id) S.draft.group_id = g.id; renderPlantel(); });
          });
        }, 'Criado');
      } else if (f.id === 'pFpbSearch') {
        var q = $('#pFpbName').value.trim();
        busy(btn, function () {
          return api('GET', '/api/fpb/teams?name=' + encodeURIComponent(q)).then(function (r) { S.fpbTeams = r || []; renderPlantel(); });
        }, ' ');
      } else if (f.id === 'pAdd') {
        var body = { display_name: $('#pAddName').value.trim(), number: $('#pAddNum').value.replace(/\D/g, '') };
        if (!body.display_name) { $('#pAddName').focus(); return; }
        busy(btn, function () {
          return api('POST', base() + '/players', body).then(refresh).then(function () { $('#pAddName').focus(); });
        }, 'Adicionado');
      }
    });

    root.addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]'); if (!b) return;
      var act = b.dataset.act, g = groupById(gid());
      if (act === 'club-setup') {
        var cm = $('#clubMsg');
        b.disabled = true; cm.className = 'msg'; cm.textContent = clubProgress(null);
        setTimeout(loadStatus, 500);
        api('POST', '/api/club/setup').then(function (r) {
          return loadGroups().then(function () {
            S.plantelGid = null; renderPlantel();
            confirmBox('Equipas do clube', '<p>' + (r.created ? 'Foram criados ' + plural(r.created, 'escalão', 'escalões') : 'Não havia escalões novos para criar') +
              ' e importados ' + plural(r.imported_players, 'jogador', 'jogadores') + '.</p>' +
              (r.errors && r.errors.length ? '<p class="hint">Não correu tudo bem:</p><ul>' + r.errors.map(function (e) { return '<li>' + esc(e) + '</li>'; }).join('') + '</ul>' : '') +
              '<p class="hint">Confira os escalões aqui no Plantel e acrescente o que falta, como os treinadores.</p>', { yes: 'OK', noCancel: true });
          });
        }, function (err) { cm.className = 'msg err'; cm.textContent = err.message; b.disabled = false; });
        return;
      }
      if (act === 'unhide') {
        var row = b.closest('[data-pid]');
        busy(b, function () {
          return api('PUT', base() + '/players/' + encodeURIComponent(row.dataset.pid), { inactive: false }).then(refresh);
        });
        return;
      }
      if (act === 'save-group') {
        busy(b, function () {
          // Name and coach only: the FPB link may have moved since this page loaded.
          return api('PUT', base(), { name: $('#pGName').value.trim(), coach: $('#pCoach').value.trim() })
            .then(function () { return loadGroups(); });
        });
      } else if (act === 'del-group') {
        confirmBox('Apagar o escalão ' + g.name + '?', '<p>Apaga também os jogadores e patrocinadores deste escalão. Não dá para desfazer.</p>', { yes: 'Apagar', danger: true }).then(function (ok) {
          if (!ok) return;
          api('DELETE', base()).then(function () { S.plantelGid = null; return loadGroups(); }).then(renderPlantel, showError);
        });
      } else if (act === 'fpb-change') {
        S.fpbTeams = S.fpbTeams ? null : []; renderPlantel();
      } else if (act === 'fpb-pick') {
        var t = S.fpbTeams[+b.dataset.i];
        busy(b, function () {
          return api('PUT', base(), { fpb_team_id: t.id, fpb_team_name: t.name + (t.age ? ' · ' + t.age : ''), fpb_team: t })
            .then(function () { S.fpbTeams = null; return loadGroups(); }).then(renderPlantel);
        });
      } else if (act === 'import') {
        busy(b, function () {
          return api('POST', base() + '/players/import').then(function (r) {
            r = r || {};
            S.importResult = { gid: gid(), updated: r.updated || [], suggestions: r.suggestions || [], photoErrors: r.photo_errors || 0 };
            S.linking = {};
            return loadGroups().then(refresh);
          });
        }, 'Importado');
      } else if (act === 'unlink' || act === 'del-player') {
        var tr = b.closest('tr'), pid = tr.dataset.pid;
        var p = (S.players[gid()] || []).filter(function (x) { return String(x.id) === pid; })[0];
        if (act === 'unlink') {
          busy(b, function () { return api('DELETE', base() + '/players/' + encodeURIComponent(pid) + '/link').then(refresh); });
        } else {
          confirmBox('Apagar ' + p.display_name + '?', '<p>Sai da lista deste escalão.</p>', { yes: 'Apagar', danger: true }).then(function (ok) {
            if (ok) busy(b, function () { return api('DELETE', base() + '/players/' + encodeURIComponent(pid)).then(refresh); });
          });
        }
      } else if (act.indexOf('sug-') === 0) {
        var si = +b.closest('[data-si]').dataset.si, s = S.importResult.suggestions[si];
        var drop = function () { S.importResult.suggestions.splice(si, 1); delete S.linking[s.fpb_player_id]; return refresh(); };
        if (act === 'sug-add') {
          busy(b, function () { return api('POST', base() + '/players/from-fpb', { fpb_player_id: s.fpb_player_id, fpb_athlete_id: s.fpb_athlete_id || 0, name: s.name, number: s.number, photo_url: s.photo_url || '' }).then(drop); });
        } else if (act === 'sug-link') {
          S.linking[s.fpb_player_id] = { choice: s.best_match || '' }; renderPlantel();
        } else if (act === 'sug-cancel') {
          delete S.linking[s.fpb_player_id]; renderPlantel();
        } else if (act === 'sug-confirm') {
          var choice = S.linking[s.fpb_player_id].choice;
          var m = b.parentElement.querySelector('.msg');
          if (!choice) { m.className = 'msg err'; m.textContent = 'Escolha o jogador.'; return; }
          busy(b, function () {
            return api('POST', base() + '/players/' + encodeURIComponent(choice) + '/link', { fpb_player_id: s.fpb_player_id, fpb_athlete_id: s.fpb_athlete_id || 0, fpb_name: s.name, photo_url: s.photo_url || '' }).then(drop);
          });
        }
      }
    });
  }

  /* ================= 4. PATROCINADORES ================= */
  function renderSponsors() {
    var root = $('#patrocinadores'), gid = S.sponsorGid;
    var list = S.sponsors[gid];
    var html = '<div class="card"><label class="field" style="max-width:420px"><span>De quem</span>' + groupSelect('spGroup', gid, true) + '</label></div>';
    html += '<div class="card"><h2>Logótipos de ' + esc(groupName(gid)) + (list ? ' <span class="count">' + list.length + '</span>' : '') + '</h2>';
    if (!list) html += '<p class="hint">A carregar…</p>';
    else if (!list.length) html += '<p class="hint">Ainda não há logótipos aqui.</p>';
    else {
      html += '<p class="hint" style="margin-bottom:6px">O principal aparece maior. Os outros rodam pela ordem da lista.</p><ul class="spons">' + list.map(function (s, i) {
        return '<li data-sid="' + esc(s.id) + '"><span class="logo">' + (s.url ? '<img src="' + esc(s.url) + '" alt="">' : '') + '</span>' +
          '<span><b>' + esc(s.name) + '</b><br><label class="princ"><input type="radio" name="princ" data-act="princ"' + (s.principal ? ' checked' : '') + '>Principal</label></span>' +
          '<span class="acts"><button class="btn icon small" data-act="up" aria-label="Subir"' + (i === 0 ? ' disabled' : '') + '>↑</button>' +
          '<button class="btn icon small" data-act="down" aria-label="Descer"' + (i === list.length - 1 ? ' disabled' : '') + '>↓</button>' +
          '<button class="btn danger small" data-act="del">Apagar</button></span></li>';
      }).join('') + '</ul><p class="msg" id="spMsg"></p>';
    }
    html += '</div>';
    html += '<div class="card"><h2>Adicionar logótipo</h2><form class="row" id="spAdd">' +
      '<label class="field grow"><span>Nome do patrocinador</span><input id="spName" autocomplete="off"></label>' +
      '<div class="grow"><span class="label">Ficheiro <small>(PNG, JPG ou SVG)</small></span><div class="row" style="align-items:center">' +
        '<label class="btn filebtn">Escolher ficheiro<input type="file" id="spFile" accept="image/png,image/jpeg,image/svg+xml"></label>' +
        '<span class="filename" id="spFileName">Nenhum escolhido</span></div></div>' +
      '<button class="btn primary" type="submit">Adicionar</button><span class="msg"></span></form></div>';
    root.innerHTML = html;
    if (!list) loadSponsors(gid).then(function () { if (S.tab === 'patrocinadores') renderSponsors(); }, showError);
  }

  function sponsorEvents() {
    var root = $('#patrocinadores');
    function base() { return '/api/groups/' + encodeURIComponent(S.sponsorGid) + '/sponsors'; }
    function saveOrder(list) {
      var msg = $('#spMsg');
      $all('#patrocinadores .spons button, #patrocinadores .spons input').forEach(function (x) { x.disabled = true; });
      msg.className = 'msg'; msg.textContent = 'A guardar…';
      return api('PUT', base(), list.map(function (s) { return { id: s.id, principal: !!s.principal }; })).then(function (r) {
        S.sponsors[S.sponsorGid] = Array.isArray(r) ? r : list;
        renderSponsors();
        var m = $('#spMsg'); if (m) { m.className = 'msg ok'; m.textContent = 'Guardado'; }
      }, function (err) {
        renderSponsors();
        var m = $('#spMsg'); if (m) { m.className = 'msg err'; m.textContent = err.message; }
      });
    }
    root.addEventListener('change', function (e) {
      var t = e.target;
      if (t.id === 'spGroup') { S.sponsorGid = t.value; renderSponsors(); return; }
      if (t.id === 'spFile') { $('#spFileName').textContent = t.files[0] ? t.files[0].name : 'Nenhum escolhido'; return; }
      if (t.dataset.act === 'princ') {
        var sid = t.closest('li').dataset.sid;
        var list = S.sponsors[S.sponsorGid].map(function (s) { return Object.assign({}, s, { principal: String(s.id) === sid }); });
        saveOrder(list);
      }
    });
    root.addEventListener('click', function (e) {
      var b = e.target.closest('button[data-act]'); if (!b) return;
      var li = b.closest('li'), sid = li.dataset.sid;
      var list = S.sponsors[S.sponsorGid].slice();
      var i = list.map(function (s) { return String(s.id); }).indexOf(sid);
      if (b.dataset.act === 'up' && i > 0) { list.splice(i - 1, 0, list.splice(i, 1)[0]); saveOrder(list); }
      else if (b.dataset.act === 'down' && i < list.length - 1) { list.splice(i + 1, 0, list.splice(i, 1)[0]); saveOrder(list); }
      else if (b.dataset.act === 'del') {
        confirmBox('Apagar ' + list[i].name + '?', '<p>O logótipo deixa de aparecer na transmissão.</p>', { yes: 'Apagar', danger: true }).then(function (ok) {
          if (!ok) return;
          api('DELETE', base() + '/' + encodeURIComponent(sid)).then(function () { return loadSponsors(S.sponsorGid); }).then(renderSponsors, showError);
        });
      }
    });
    root.addEventListener('submit', function (e) {
      e.preventDefault();
      var file = $('#spFile').files[0], name = $('#spName').value.trim();
      var btn = e.target.querySelector('button[type="submit"]'), msg = btn.parentElement.querySelector('.msg');
      if (!file) { msg.className = 'msg err'; msg.textContent = 'Escolha o ficheiro do logótipo.'; return; }
      if (!/\.(png|jpe?g|svg)$/i.test(file.name)) { msg.className = 'msg err'; msg.textContent = 'O ficheiro tem de ser PNG, JPG ou SVG.'; return; }
      var fd = new FormData();
      fd.append('file', file);
      fd.append('name', name || file.name.replace(/\.[^.]+$/, ''));
      busy(btn, function () {
        return api('POST', base(), fd).then(function () { return loadSponsors(S.sponsorGid); }).then(renderSponsors);
      }, 'Adicionado');
    });
  }

  /* ================= 5. PROMO ================= */
  var PROMO_MAX = 200 * 1024 * 1024;
  function loadPromo() {
    return api('GET', '/api/promo').then(function (p) {
      S.promo = p || { clips: [], timeout_clip: '' };
      S.promo.clips = S.promo.clips || [];
      if (!clipById(S.promoSel)) S.promoSel = S.promo.clips.length ? S.promo.clips[0].id : null;
      S.promo.clips.forEach(function (c) { if (!c.orientation) measureClip(c); });
    });
  }
  var prTimer = null; // a text edit waiting to be saved
  // Put the program's copy of a clip in the list, found by id.
  function keepClip(r) {
    var i = S.promo.clips.indexOf(clipById(r.id));
    if (i >= 0) S.promo.clips[i] = r;
  }
  function clipById(id) { return S.promo ? S.promo.clips.filter(function (c) { return c.id === id; })[0] || null : null; }
  function mmss(s) { s = Math.max(0, Math.round(s || 0)); return Math.floor(s / 60) + ':' + pad(s % 60); }
  function promoOnAir() {
    var st = S.state || {};
    return st.screen && st.screen.current === 'promo' && st.promo && st.promo.clip ? st.promo.clip : null;
  }
  // The browser measures each new clip (size, length) and tells the program, which lays it out from that.
  var measuring = {};
  function measureClip(c) {
    if (measuring[c.id]) return;
    measuring[c.id] = true;
    var v = document.createElement('video');
    v.preload = 'metadata';
    v.muted = true;
    v.onloadedmetadata = function () {
      // Only the measured fields: the program keeps the rest as last saved.
      var body = { width: v.videoWidth, height: v.videoHeight, duration: v.duration,
        orientation: v.videoWidth > v.videoHeight ? 'horizontal' : 'vertical' };
      v.removeAttribute('src'); v.load();
      api('PUT', '/api/promo/' + encodeURIComponent(c.id), body).then(function (r) {
        keepClip(r);
        var a = document.activeElement;
        if (S.tab === 'promo' && !prTimer && !(a && a.closest && a.closest('#prForm'))) renderPromo();
      }, function () { measuring[c.id] = false; });
    };
    v.onerror = function () { measuring[c.id] = false; };
    v.src = '/promo/' + c.file;
  }

  function renderPromo() {
    var root = $('#promo'), p = S.promo;
    if (!p) { root.innerHTML = '<p class="hint">A carregar…</p>'; loadPromo().then(renderPromo, showError); return; }
    var c = clipById(S.promoSel);
    var html = '<div id="prAir"></div><div class="promo-grid"><div class="card"><h2>Vídeos <span class="count">' + p.clips.length + '</span></h2>';
    if (!p.clips.length) html += '<p class="hint">Ainda não há vídeos. Carregue um MP4 feito no telemóvel ou no computador.</p>';
    html += '<ul class="clips">' + p.clips.map(function (x) {
      var o = x.orientation === 'horizontal' ? 'HORIZONTAL' : x.orientation === 'vertical' ? 'VERTICAL' : '';
      return '<li class="clip' + (x.id === S.promoSel ? ' sel' : '') + '" data-clip="' + esc(x.id) + '">' +
        '<button class="pick" type="button" data-act="pr-pick" aria-pressed="' + (x.id === S.promoSel) + '">' +
        '<span class="vthumb"><video muted preload="metadata" src="/promo/' + esc(x.file) + '#t=1"></video></span>' +
        '<span class="who"><b>' + esc(x.name) + '</b><small>' + (x.duration ? mmss(x.duration) : '–:––') + (o ? ' · <span class="badge">' + o + '</span>' : '') + '</small></span></button>' +
        '<button class="btn icon" type="button" data-act="pr-show" aria-label="Mostrar promo: ' + esc(x.name) + '" title="Mostrar promo">▶</button></li>';
    }).join('') + '</ul>';
    html += '<div class="row" style="margin-top:14px;align-items:center">' +
      '<label class="btn dark filebtn">Carregar vídeo<input type="file" id="prFile" accept="video/mp4,.mp4"></label>' +
      '<span class="hint">MP4, até 200 MB</span><span class="msg" id="prUp" role="status"></span></div></div>';

    html += '<div>';
    if (!c) {
      html += '<div class="card"><p class="hint">Escolha um vídeo da lista para mudar o texto e o som.</p></div>';
    } else {
      var vert = c.orientation === 'vertical';
      html += '<form class="card" id="prForm" autocomplete="off"><h2>Texto no ecrã</h2><div class="stack">' +
        '<label class="field"><span>Nome na lista</span><input id="prName" maxlength="60" value="' + esc(c.name) + '"></label>' +
        '<div class="cols2"><label class="field"><span>Etiqueta</span><input id="prKicker" maxlength="40" value="' + esc(c.kicker) + '"></label>' +
        '<label class="field"><span>Título</span><input id="prHeadline" maxlength="60" value="' + esc(c.headline) + '"></label></div>' +
        '<label class="field"><span>Linha</span><input id="prLine" maxlength="120" value="' + esc(c.line) + '"></label>' +
        '<div class="row" style="align-items:flex-end"><label class="field grow"><span>Link do QR <small>(opcional)</small></span><input type="url" id="prLink" maxlength="300" placeholder="https://…" value="' + esc(c.link) + '"></label>' +
        '<label class="switch"><input type="checkbox" id="prQR"' + (c.show_qr ? ' checked' : '') + (c.link ? '' : ' disabled') + '><i></i>Mostrar QR</label></div>' +
        '</div></form>' +
        '<div class="card" id="prPlay"><h2>Reprodução</h2><div class="stack">' +
        '<div class="row" style="align-items:center"><label class="switch"><input type="checkbox" id="prSound"' + (c.sound ? ' checked' : '') + '><i></i>Som</label>' +
        '<input type="range" id="prVol" min="0" max="100" step="5" value="' + esc(c.volume) + '" aria-label="Volume" class="grow"' + (c.sound ? '' : ' disabled') + '><b class="vol" id="prVolTxt">' + esc(c.volume) + '%</b></div>' +
        '<p class="hint">Para o som entrar na transmissão, ligue "Controlar áudio via OBS" (Control audio via OBS) nas propriedades da fonte do overlay no OBS.</p>' +
        '<div><span class="label">No fim</span><div class="seg" role="group" aria-label="No fim">' +
          '<button type="button" data-end="back" aria-pressed="' + (c.end !== 'loop') + '">Voltar ao ecrã anterior</button>' +
          '<button type="button" data-end="loop" aria-pressed="' + (c.end === 'loop') + '">Repetir</button></div></div>' +
        '<label class="switch"><input type="checkbox" id="prMini"' + (p.timeout_clip === c.id ? ' checked' : '') + (vert ? '' : ' disabled') + '><i></i>Mostrar pequeno no desconto de tempo</label>' +
        (vert ? '' : '<p class="hint" style="margin-top:-6px">Só os vídeos na vertical cabem ao lado da câmara.</p>') +
        '<p class="msg" id="prMsg" role="status"></p></div></div>' +
        '<div class="row"><button class="btn primary big grow" type="button" data-act="pr-show" data-id="' + esc(c.id) + '">Mostrar promo <small>· ' + esc(c.name) + '</small></button></div>' +
        '<div class="row" style="margin-top:10px"><button class="btn danger small" type="button" data-act="pr-del">Apagar vídeo</button></div>';
    }
    html += '</div></div>';
    root.innerHTML = html;
    S.promoAir = undefined;
    renderPromoAir();
  }
  // "No ar" with the stop button, redrawn only when the clip on air changes, so typing is never cut.
  function renderPromoAir() {
    var el = $('#prAir'); if (!el) return;
    var air = promoOnAir(), key = air ? air.id : '';
    if (key === S.promoAir) return;
    S.promoAir = key;
    var ac = air && clipById(air.id);
    el.innerHTML = air ? '<div class="onscreen"><b>No ar</b><span>' + esc(ac ? ac.name : 'Vídeo') + '</span><span style="flex:1"></span>' +
      '<button class="btn small" type="button" data-act="pr-stop">Parar e voltar</button></div>' : '';
  }

  function promoEvents() {
    var root = $('#promo');
    function msg(cls, text) { var m = $('#prMsg'); if (m) { m.className = 'msg ' + cls; m.textContent = text; } }
    function save() {
      var c = clipById(S.promoSel); if (!c) return Promise.resolve();
      var body = Object.assign({}, c, {
        name: $('#prName').value, kicker: $('#prKicker').value, headline: $('#prHeadline').value, line: $('#prLine').value,
        link: $('#prLink').value, show_qr: $('#prQR').checked, sound: $('#prSound').checked, volume: +$('#prVol').value
      });
      msg('', 'A guardar…');
      return api('PUT', '/api/promo/' + encodeURIComponent(c.id), body).then(function (r) {
        keepClip(r);
        // Show what the program kept (a link gets https://), without losing the field being typed in.
        var link = $('#prLink'); if (link && document.activeElement !== link) link.value = r.link;
        var qr = $('#prQR'); if (qr) { qr.disabled = !r.link; qr.checked = r.show_qr; }
        var name = root.querySelector('.clip.sel .who b'); if (name) name.textContent = r.name;
        msg('ok', 'Guardado');
      }, function (err) { msg('err', err.message); });
    }
    function later() { clearTimeout(prTimer); prTimer = setTimeout(function () { prTimer = null; save(); }, 500); }
    // Save an edit still waiting, before anything that leaves this clip or puts one on air.
    function flush() {
      if (!prTimer) return Promise.resolve();
      clearTimeout(prTimer);
      prTimer = null;
      return save();
    }
    root.addEventListener('input', function (e) {
      var t = e.target;
      if (t.id === 'prVol') { $('#prVolTxt').textContent = t.value + '%'; later(); return; }
      if (t.id === 'prLink') { var qr = $('#prQR'); qr.disabled = !t.value.trim(); if (!t.value.trim()) qr.checked = false; }
      if (t.closest('#prForm')) later();
    });
    root.addEventListener('change', function (e) {
      var t = e.target;
      if (t.id === 'prFile') { upload(t.files[0]); t.value = ''; return; }
      if (t.id === 'prSound') { $('#prVol').disabled = !t.checked; save(); return; }
      if (t.id === 'prQR') { save(); return; }
      if (t.id === 'prMini') {
        t.disabled = true;
        api('PUT', '/api/promo', { timeout_clip: t.checked ? S.promoSel : '' }).then(function (p) {
          S.promo.timeout_clip = p.timeout_clip; msg('ok', 'Guardado');
        }, function (err) { t.checked = !t.checked; msg('err', err.message); }).then(function () { t.disabled = false; });
      }
    });
    root.addEventListener('submit', function (e) { e.preventDefault(); save(); });
    root.addEventListener('click', function (e) {
      var b = e.target.closest('button'); if (!b) return;
      var li = b.closest('[data-clip]');
      if (b.dataset.end) {
        var c = clipById(S.promoSel);
        $all('[data-end]', root).forEach(function (x) { x.setAttribute('aria-pressed', x === b ? 'true' : 'false'); });
        c.end = b.dataset.end;
        save();
        return;
      }
      var act = b.dataset.act;
      if (act === 'pr-pick') { var pick = li.dataset.clip; flush().then(function () { S.promoSel = pick; renderPromo(); }); return; }
      if (act === 'pr-show') {
        var id = b.dataset.id || li.dataset.clip;
        b.disabled = true;
        // Save pending edits first, so the clip goes on air with the text on screen here.
        (id === S.promoSel && $('#prForm') ? (clearTimeout(prTimer), prTimer = null, save()) : flush()).then(function () {
          return api('POST', '/api/promo/' + encodeURIComponent(id) + '/show');
        }).then(null, showError).then(function () { b.disabled = false; });
        return;
      }
      if (act === 'pr-stop') { b.disabled = true; api('POST', '/api/promo/stop').then(null, showError); return; }
      if (act === 'pr-del') {
        var d = clipById(S.promoSel);
        confirmBox('Apagar ' + d.name + '?', '<p>O vídeo sai da lista e do computador. Se estiver no ar, a transmissão volta ao ecrã anterior.</p>', { yes: 'Apagar', danger: true }).then(function (ok) {
          if (!ok) return;
          api('DELETE', '/api/promo/' + encodeURIComponent(d.id)).then(function () { S.promoSel = null; return loadPromo(); }).then(renderPromo, showError);
        });
      }
    });
  }

  // XMLHttpRequest, not fetch, so the volunteer sees how far a big file has gone.
  function upload(file) {
    var out = $('#prUp');
    function say(cls, t) { out.className = 'msg ' + cls; out.textContent = t; }
    if (!file) return;
    if (!/\.mp4$/i.test(file.name)) return say('err', 'O vídeo tem de ser MP4.');
    if (file.size > PROMO_MAX) return say('err', 'O vídeo passa dos 200 MB.');
    var fd = new FormData();
    fd.append('file', file);
    if (MOCK) {
      return api('POST', '/api/promo', fd).then(function (c) { S.promoSel = c.id; return loadPromo(); }).then(renderPromo, function (err) { say('err', err.message); });
    }
    var x = new XMLHttpRequest();
    x.open('POST', '/api/promo');
    x.upload.onprogress = function (e) { if (e.lengthComputable) say('', 'A carregar… ' + Math.round(e.loaded / e.total * 100) + '%'); };
    x.onload = function () {
      var data = null; try { data = JSON.parse(x.responseText); } catch (e) {}
      if (x.status === 401) { askPin(data && data.error).then(function () { upload(file); }); return; }
      if (x.status !== 201) { say('err', (data && data.error) || 'O programa respondeu com erro ' + x.status + '.'); return; }
      S.promoSel = data.id;
      loadPromo().then(function () { if (S.tab === 'promo') renderPromo(); });
    };
    x.onerror = function () { say('err', 'Sem ligação ao programa. Confirme que o Hallstream está aberto.'); };
    say('', 'A carregar… 0%');
    x.send(fd);
  }

  /* ================= 6. DEFINIÇÕES ================= */
  function renderDefinicoes() {
    var root = $('#definicoes'), s = S.settings;
    if (!s) { root.innerHTML = '<p class="hint">A carregar…</p>'; return; }
    var o = s.obs || {};
    var url = (S.status && S.status.overlay_url) || (location.origin + '/overlay');
    var html = '';
    var fs = S.setup;
    if (fs) {
      html += '<div class="card"><h2>Configuração inicial</h2>' + (fs.done
        ? '<div class="row" style="align-items:center"><p class="grow">Configuração inicial concluída em <b>' + esc(longDate(fs.done)) + '</b>.</p>' +
          '<button class="btn" type="button" data-act="setup-review">Rever</button></div>'
        : '<div class="row" style="align-items:center"><p class="hint grow">Cinco passos para deixar prontos a caixa, o OBS e o YouTube.</p>' +
          '<button class="btn primary" type="button" data-act="setup-open">' + (Object.keys(fs.steps || {}).length ? 'Continuar' : 'Começar') + '</button></div>') + '</div>';
    }
    html += '<div class="card"><h2>Controlo do OBS</h2>' +
      '<p class="hint">Carregue em <b>Configurar OBS</b> uma vez, com o tablet ligado. O Hallstream prepara o OBS sozinho: liga o servidor WebSocket, cria a cena <b>Hallstream</b> com o grafismo por cima da câmara e põe a resolução em 1920x1080. Pode carregar outra vez quando quiser. O que já está certo fica como está.</p>' +
      '<div class="row" style="margin-top:12px;align-items:center"><button class="btn primary" type="button" data-act="obs-setup">Configurar OBS</button><span class="msg" id="obsSetupMsg" role="status"></span></div>' +
      '<div id="obsResult">' + setupHtml(S.obsSetup) + '</div>' +
      '<div class="cols" style="margin-top:12px">' +
      '<label class="field"><span>Câmara</span><select id="dObsCam">' + camOptions(o.camera_source || '') + '</select></label>' +
      '<label class="field"><span>Enquadramento</span><select id="dObsFit">' +
      '<option value="contain"' + (o.camera_fit !== 'fill' ? ' selected' : '') + '>Imagem inteira</option>' +
      '<option value="fill"' + (o.camera_fit === 'fill' ? ' selected' : '') + '>Encher o ecrã (corta as margens)</option></select></label>' +
      '</div><p class="hint">Câmara nova? Escolha-a aqui e carregue em <b>Configurar OBS</b>. A antiga fica escondida no OBS. Com "Imagem inteira", um tablet ao alto ou em 4:3 fica com margens. No desconto, as margens ficam azuis.</p>' +
      '<div class="row" style="margin-top:12px"><button class="btn" data-act="obs-fit">Ajustar câmara</button><span class="msg" id="dObsMsg"></span></div>' +
      (S.status && S.status.obs_app ?
        '<label class="switch" style="margin-top:12px"><input type="checkbox" id="dObsLaunch"' + (o.launch ? ' checked' : '') + '><i></i>Abrir o OBS com o Hallstream</label>' +
        '<p class="hint">O OBS abre escondido ao pé do relógio. Liga-se sozinho depois de Configurar OBS correr bem.</p>' +
        '<label class="switch" style="margin-top:8px"><input type="checkbox" id="dObsClose"' + (o.close_on_exit ? ' checked' : '') + '><i></i>Fechar o OBS ao sair</label>' +
        '<p class="hint">Ao escolher Sair no ícone do Hallstream, o OBS também fecha.</p>' : '') +
      '<details style="margin-top:12px"><summary class="hint">Ligação ao OBS feita à mão</summary>' +
      '<p class="hint">Só se o OBS estiver noutro computador, ou se Configurar OBS não conseguir ligar o servidor. No OBS: Ferramentas → Definições do servidor WebSocket.</p>' +
      '<label class="switch"><input type="checkbox" id="dObsOn"' + (o.enabled ? ' checked' : '') + '><i></i>Ligar ao OBS</label>' +
      '<div class="cols" style="margin-top:8px">' +
      '<label class="field"><span>Endereço</span><input id="dObsHost" value="' + esc(o.host || '127.0.0.1') + '"></label>' +
      '<label class="field"><span>Porta</span><input id="dObsPort" inputmode="numeric" value="' + esc(o.port || 4455) + '"></label>' +
      '<label class="field"><span>Palavra-passe</span><input type="password" id="dObsPass" value="' + esc(o.password || '') + '" autocomplete="off"></label>' +
      '</div>' +
      '<div class="row" style="margin-top:12px"><button class="btn" data-act="obs-test">Testar ligação</button><span class="msg" id="dObsTestMsg"></span></div>' +
      '<span class="label" style="margin-top:12px">Endereço do grafismo</span><div class="urlbox"><input id="dUrl" readonly value="' + esc(url) + '" aria-label="Endereço do grafismo">' +
      '<button class="btn dark" data-act="copy">Copiar</button><span class="msg" style="align-self:center"></span></div>' +
      '<p class="hint">Para pôr o grafismo à mão: no OBS, Fontes → + → Navegador, cole este endereço, largura 1920, altura 1080, ligue "Controlar áudio via OBS" e ponha a fonte por cima da câmara.</p>' +
      '</details>' +
      '<details style="margin-top:12px"><summary class="hint">Modo antigo, com uma cena só para o desconto</summary>' +
      '<p class="hint">Só se já tiver uma cena de desconto no OBS. Deixe em branco para a câmara encolher sozinha.</p><div class="cols">' +
      '<label class="field"><span>Cena do jogo</span><input id="dObsGame" value="' + esc(o.game_scene || 'Jogo') + '"></label>' +
      '<label class="field"><span>Cena do desconto de tempo</span><input id="dObsTimeout" value="' + esc(o.timeout_scene || '') + '" placeholder="em branco"></label>' +
      '</div></details></div>';

    html += '<div class="card" id="ytSetup">' + ytSetupHtml() + '</div>';

    html += '<div class="card"><h2>Telemóvel</h2>' +
      '<p class="hint">Abre este painel num telemóvel ligado à mesma rede Wi-Fi, com PIN.</p>' +
      '<label class="switch"><input type="checkbox" id="dPhone"' + (s.phone_access ? ' checked' : '') + '><i></i>Permitir telemóvel</label>' +
      '<p class="hint">A mudança só conta quando voltar a abrir o programa.</p>' +
      '<div class="row" style="margin-top:8px"><label class="field" style="width:160px"><span>PIN <small>(4 algarismos)</small></span><input id="dPin" inputmode="numeric" maxlength="4" value="' + esc(s.pin || '') + '" autocomplete="off"></label>' +
      (S.status && S.status.lan_url && s.phone_access ? '<p class="grow">No telemóvel abra <code>' + esc(S.status.lan_url) + '</code></p>' : '') + '</div></div>';

    html += clubCard(s);

    var bug = (s.overlay && s.overlay.bug_position) || 'top-left';
    html += '<div class="card"><h2>Grafismo</h2>' +
      '<label class="field" style="max-width:340px"><span>Posição do marcador</span><select id="dBug">' +
      '<option value="top-left"' + (bug !== 'bottom-left' ? ' selected' : '') + '>Canto superior esquerdo</option>' +
      '<option value="bottom-left"' + (bug === 'bottom-left' ? ' selected' : '') + '>Canto inferior esquerdo</option></select></label>' +
      '<p class="hint">O rodapé com o nome aparece no outro canto de baixo. Pode mudar durante o jogo, e o marcador desliza para o sítio novo.</p>' +
      sponsorCornerFields(s.overlay || {}) +
      '<label class="switch" style="margin-top:16px"><input type="checkbox" id="dCredit"' + (s.credit !== false ? ' checked' : '') + '><i></i>Mostrar «feito com Hallstream» na transmissão</label>' +
      '<p class="hint">Ponho o logótipo pequeno do Hallstream no canto de baixo à direita, quando está livre. Ajuda outros clubes a conhecer o programa.</p></div>';

    html += '<div class="savebar"><button class="btn primary" data-act="save-settings">Guardar definições</button><span class="msg"></span></div>';

    html += '<div class="card" style="margin-top:16px"><h2>Demonstração</h2>' +
      '<p class="hint">Mostra um jogo a fingir na transmissão, para testar sem a caixa do marcador. Desligue antes do jogo.</p>' +
      '<div class="row" style="align-items:center"><label class="switch"><input type="checkbox" id="setDemo"' + (S.state && S.state.demo ? ' checked' : '') + '><i></i>Modo demonstração</label><span class="msg"></span></div>' +
      (S.status && S.status.version ? '<p class="hint" style="margin-top:12px">Versão ' + esc(S.status.version) + ' · porta ' + esc(S.status.port) + '</p>' : '') +
      (S.status && S.status.data_dir ? '<p class="hint">Os dados ficam em <code>' + esc(S.status.data_dir) + '</code>. O ficheiro marcador.log guarda as mensagens do programa.</p>' : '') + '</div>';

    if (S.setup && S.setup.pc) {
      html += '<div class="card"><h2>Cópia de segurança</h2>' +
        '<p class="hint">Guarda num ficheiro .zip os escalões, os jogadores, as fotografias, os patrocinadores, os vídeos, as definições e a ligação ao YouTube. Guarde uma cópia depois de mudar alguma coisa, numa pen ou numa pasta só do clube. Não a envie por email nem por WhatsApp: com ela, qualquer pessoa transmite no canal do clube.</p>' +
        '<div class="row" style="align-items:center;margin-top:12px"><a class="btn" id="dBackup" href="/api/system/backup" download>Guardar cópia</a>' +
        '<button class="btn quiet" type="button" data-help="ajuda-pc-novo">Passar para outro computador</button></div></div>';
    }

    if (S.status && S.status.shortcut) {
      html += '<div class="card"><h2>Neste computador</h2>' +
        '<p class="hint">Para fechar o Hallstream, carregue no ícone dele ao pé do relógio e escolha <b>Sair</b>.</p>' +
        '<div class="row" style="align-items:center;margin-top:12px"><button class="btn" id="dShortcut" data-act="shortcut">Criar atalho no ambiente de trabalho</button><span class="msg"></span></div>' +
        (s.desktop_shortcut === 'created' ? '<p class="hint" style="margin-top:8px">O atalho já foi criado. Se o apagou, pode criá-lo outra vez.</p>' : '') +
        (S.status.autostart ? '<div class="row" style="align-items:center;margin-top:12px"><label class="switch"><input type="checkbox" id="dAutostart"' + (s.start_with_windows ? ' checked' : '') + '><i></i>Abrir com o Windows</label><span class="msg"></span></div>' +
          '<p class="hint">O Hallstream abre sozinho quando liga o PC. Com "Abrir o OBS com o Hallstream", o OBS abre também.</p>' : '') + '</div>';
    }
    root.innerHTML = html;
    if (o.enabled && !S.obsCams) loadObsCameras();
  }

  /* Club profile (settings.club): what the overlay shows for the home side, in the club's colours.
     SPORTS follows club.Sports in internal/club; only basketball works today. */
  var SPORTS = [['basketball', 'Basquetebol', true], ['futsal', 'Futsal'], ['handball', 'Andebol'], ['volleyball', 'Voleibol'], ['soccer', 'Futebol']];
  var COUNTRIES = [['PT', 'Portugal'], ['ES', 'Espanha'], ['FR', 'França'], ['LU', 'Luxemburgo'], ['CH', 'Suíça'], ['BR', 'Brasil'], ['AO', 'Angola'], ['CV', 'Cabo Verde'], ['MZ', 'Moçambique']];
  // The FPB import is for Portuguese basketball clubs only.
  function fpbFor(c) { c = c || {}; return (c.country || 'PT') === 'PT' && (c.sport || 'basketball') === 'basketball'; }
  function fpbOffered() { return fpbFor(S.settings && S.settings.club); }
  function clubCard(s) {
    var c = s.club || {}, crest = c.crest || '/assets/crest.svg', country = c.country || 'PT';
    var countries = COUNTRIES.some(function (x) { return x[0] === country; }) ? COUNTRIES : COUNTRIES.concat([[country, country]]);
    function colour(id, label, v) {
      v = /^#[0-9a-f]{6}$/i.test(v || '') ? v.toLowerCase() : '#2f4b7c';
      return '<label class="field"><span>' + label + '</span><span class="colour"><input type="color" id="' + id + '" value="' + esc(v) + '">' +
        '<code id="' + id + 'Hex">' + esc(v.toUpperCase()) + '</code></span></label>';
    }
    return '<div class="card"><h2>Clube</h2><div class="club">' +
      '<div><div class="crest-tile club-crest"><img id="dCrestImg" src="' + esc(crest) + '" alt="Emblema do clube"></div>' +
        '<div class="stack" style="margin-top:10px">' +
        '<label class="btn small filebtn" style="width:100%">Carregar emblema<input type="file" id="dCrestFile" accept="image/png,image/jpeg,image/svg+xml"></label>' +
        '<button class="btn quiet small" type="button" data-act="crest-remove" style="width:100%"' + (crest.indexOf('/media/') === 0 ? '' : ' hidden') + '>Remover emblema</button>' +
        '<span class="msg" id="dCrestMsg" role="status"></span></div></div>' +
      '<div class="cols">' +
        '<label class="field"><span>Nome <small>(máx. 12)</small></span><input id="dHome" maxlength="12" value="' + esc(c.name || '') + '"></label>' +
        '<label class="field"><span>Sigla <small>(máx. 4)</small></span><input id="dHomeShort" maxlength="4" value="' + esc(c.short || '') + '"></label>' +
        colour('dPrimary', 'Cor principal', c.primary) +
        colour('dSecondary', 'Cor secundária', c.secondary || '#1c1a17') +
        '<label class="field"><span>Modalidade</span><select id="dSport">' + SPORTS.map(function (x) {
          return '<option value="' + x[0] + '"' + (x[2] ? '' : ' disabled') + ((c.sport || 'basketball') === x[0] ? ' selected' : '') + '>' + x[1] + (x[2] ? '' : ' (em breve)') + '</option>';
        }).join('') + '</select></label>' +
        '<label class="field"><span>País</span><select id="dCountry">' + countries.map(function (x) {
          return '<option value="' + esc(x[0]) + '"' + (x[0] === country ? ' selected' : '') + '>' + esc(x[1]) + '</option>';
        }).join('') + '</select></label>' +
      '</div></div>' +
      '<p class="hint" style="margin-top:12px">A cor principal pinta o marcador e os ecrãs inteiros, e a secundária as faixas escuras. Ponho o texto por cima a branco ou a escuro, conforme se lê melhor.</p>' +
      '<div class="cols" style="margin-top:8px">' +
        '<label class="field"><span>Instagram</span><input id="dInsta" value="' + esc(s.instagram || '') + '" placeholder="@clube"></label>' +
        '<label class="field" data-fpb' + (fpbFor(c) ? '' : ' hidden') + '><span>Nome do clube na FPB</span><input id="dClubSearch" value="' + esc(s.club_search || '') + '" placeholder="Clube Exemplo"></label>' +
        '<label class="field" data-fpb' + (fpbFor(c) ? '' : ' hidden') + '><span>Endereço da FPB</span><input type="url" id="dFpb" value="' + esc(s.fpb_base_url || '') + '"></label>' +
      '</div><label class="switch" style="margin-top:8px"><input type="checkbox" id="dAuto"' + (s.auto_screens ? ' checked' : '') + '><i></i>Ecrãs automáticos ao abrir o programa</label></div>';
  }
  function setCrest(c) {
    S.settings.club = Object.assign({}, S.settings.club, { crest: c.crest });
    $('#dCrestImg').src = c.crest;
    $('[data-act="crest-remove"]').hidden = c.crest.indexOf('/media/') !== 0;
  }

  /* YouTube: the program's own login to the club's channel (set up once, on the PC). */
  function ytSetupHtml(bare) {
    var y = S.yt || (S.status && S.status.youtube);
    var html = bare ? '' : '<h2>YouTube</h2>';
    if (!y) return html + '<p class="hint">A carregar…</p>';
    if (y.connected) {
      var ch = y.channel || {};
      html += '<div class="yt-channel">' + (ch.thumbnail ? '<img src="' + esc(ch.thumbnail) + '" alt="" width="48" height="48" onerror="this.remove()">' : '') +
        '<p><span class="hint">Ligado como</span><b>' + esc(ch.title || 'canal do YouTube') + '</b></p></div>';
      html += y.expired ? '<p class="alert">A ligação ao YouTube expirou. Carregue em Ligar ao YouTube para entrar outra vez.</p>' :
        '<p class="hint">Ao carregar em Iniciar transmissão, o Hallstream cria o direto público deste jogo, com o título e a miniatura. Ao terminar, o vídeo fica não listado e o link aparece em Ecrãs.</p>';
      if (y.error && !y.expired) html += '<p class="msg err">' + esc(y.error) + '</p>';
      if (!y.setup) return html + '<p class="hint">Só se liga ou desliga no computador do clube.</p>';
      return html + '<div class="row" style="margin-top:12px;align-items:center">' +
        (y.expired ? '<button class="btn primary" type="button" data-act="yt-connect">Ligar ao YouTube</button>' : '') +
        '<button class="btn danger" type="button" data-act="yt-disconnect">Desligar</button><span class="msg" id="ytMsg" role="status"></span></div>';
    }
    if (!y.setup) return html + '<p class="hint">O YouTube ainda não está ligado. Liga-se uma vez, no computador do clube, na Configuração inicial.</p>';
    html += (bare ? '' : '<p class="hint">Cole o ID e o segredo do cliente OAuth, criados na Google Cloud com o guia <b>LIGAR-YOUTUBE.txt</b>, na pasta YouTube ao lado do programa.</p>') +
      '<div class="cols" style="margin-top:12px">' +
      '<label class="field"><span>ID do cliente</span><input id="ytId" autocomplete="off" spellcheck="false" value="' + esc(y.client_id || '') + '" placeholder="…apps.googleusercontent.com"></label>' +
      '<label class="field"><span>Segredo do cliente</span><input type="password" id="ytSecret" autocomplete="off" value="" placeholder="' + (y.has_secret ? 'Guardado. Deixe em branco para manter' : '') + '"></label>' +
      '</div><div class="row" style="margin-top:12px;align-items:center">' +
      '<button class="btn" type="button" data-act="yt-save">Guardar</button>' +
      '<button class="btn primary" type="button" data-act="yt-connect"' + (y.configured && !y.connecting ? '' : ' disabled') + '>Ligar ao YouTube</button>' +
      '<span class="msg" id="ytMsg" role="status"></span></div>';
    if (y.connecting) html += '<p class="hint" style="margin-top:8px">À espera do login na página do Google. Entre com a conta que gere o canal do clube, escolha o canal do clube e carregue em Continuar.</p>';
    else if (y.error) html += '<p class="msg err" style="margin-top:8px">' + esc(y.error) + '</p>';
    return html;
  }
  function renderYtSetup() { var el = $('#ytSetup'); if (el) el.innerHTML = ytSetupHtml(!!el.dataset.bare); }
  function loadYt() {
    return api('GET', '/api/youtube').then(function (y) {
      S.yt = y; if (S.status) S.status.youtube = y;
      renderYtSetup(); renderChips(); refreshInicio();
      clearTimeout(S.ytTimer);
      // After the login the program still reads the channel and sets up OBS.
      if (y && (y.connecting || S.ytWas && y.connected && !y.stream_ready && !y.error && (S.ytTries = (S.ytTries || 0) + 1) < 15)) S.ytTimer = setTimeout(loadYt, 2000);
      else if (S.ytWas && y && y.connected) { S.ytWas = false; S.ytTries = 0; loadStream(); }
      return y;
    }, function () {});
  }
  function ytEvents(root) {
    root.addEventListener('click', function (e) {
      var b = e.target.closest('[data-act^="yt-"]'); if (!b) return;
      var msg = $('#ytMsg');
      function say(cls, t) { if (msg) { msg.className = 'msg ' + cls; msg.textContent = t; } }
      if (b.dataset.act === 'yt-save') {
        b.disabled = true; say('', 'A guardar…');
        api('PUT', '/api/youtube/client', { client_id: $('#ytId').value.trim(), client_secret: $('#ytSecret').value.trim() })
          .then(function (y) {
            S.yt = y; renderYtSetup();
            var m = $('#ytMsg'); if (m) { m.className = 'msg ok'; m.textContent = 'Guardado'; }
          }, function (err) { b.disabled = false; say('err', err.message); });
      } else if (b.dataset.act === 'yt-connect') {
        // Opened now, while the click counts, so the browser doesn't block it.
        var win = window.open('', '_blank');
        b.disabled = true; say('', 'A abrir a página do Google…');
        api('POST', '/api/youtube/connect', {}).then(function (r) {
          if (win) win.location.href = r.auth_url;
          else say('', '');
          S.ytWas = true;
          return loadYt().then(function () {
            if (!win) { var m = $('#ytMsg'); if (m) m.innerHTML = '<a href="' + esc(r.auth_url) + '" target="_blank" rel="noopener">Abrir a página do Google</a>'; }
          });
        }, function (err) { if (win) win.close(); b.disabled = false; say('err', err.message); });
      } else if (b.dataset.act === 'yt-disconnect') {
        confirmBox('Desligar o YouTube?', '<p>O Hallstream deixa de criar os diretos e de tornar os vídeos não listados. Os vídeos ficam no canal como estão.</p>' +
          '<p>O OBS continua a enviar para a chave do Hallstream. Para usar outra vez a conta ligada no OBS, ligue-a em Definições > Transmissão do OBS.</p>',
        { yes: 'Desligar', danger: true }).then(function (yes) {
          if (!yes) return;
          b.disabled = true;
          api('POST', '/api/youtube/disconnect', {}).then(function (y) { S.yt = y; renderYtSetup(); renderChips(); loadStream(); },
            function (err) { b.disabled = false; say('err', err.message); });
        });
      }
    });
  }

  // Sponsor corner on the scoreboard: on by default, every 4 minutes.
  function sponsorCornerFields(ov) {
    return '<label class="switch" style="margin-top:16px"><input type="checkbox" id="dCorner"' + (ov.sponsor_corner !== false ? ' checked' : '') + '><i></i>Patrocinador no canto durante o jogo</label>' +
      '<div class="row" style="margin-top:8px"><label class="field" style="width:280px;max-width:100%"><span>De quantos em quantos minutos</span>' +
      '<input id="dCornerEvery" type="number" inputmode="numeric" min="1" max="30" value="' + esc(ov.sponsor_every || 4) + '"></label></div>' +
      '<p class="hint">Um patrocinador de cada vez, 10 segundos, no canto de cima à direita, pela ordem da faixa. Não aparece nos descontos de tempo nem nos ecrãs inteiros.</p>';
  }

  // The cameras OBS has, and the devices it could add; "" lets the program pick (DroidCam first).
  function camOptions(cur) {
    var list = S.obsCams || [], found = !cur, adds = '';
    var html = '<option value="">Automática</option>';
    list.forEach(function (c) {
      if (c.type === 'source') {
        var name = c.value.slice(7);
        if (name === cur) found = true;
        html += '<option value="' + esc(name) + '"' + (name === cur ? ' selected' : '') + '>' + esc(name) + '</option>';
      } else {
        adds += '<option value="' + esc(c.value) + '">' + esc(c.label) + '</option>';
      }
    });
    if (!found) html += '<option value="' + esc(cur) + '" selected>' + esc(cur) + (S.obsCams ? ' (não está no OBS)' : '') + '</option>';
    if (adds) html += '<optgroup label="Juntar ao OBS">' + adds + '</optgroup>';
    return html;
  }
  function loadObsCameras() {
    if (S.obsCamsLoading) return;
    S.obsCamsLoading = true;
    return api('POST', '/api/obs/cameras', {}).then(function (r) { S.obsCams = (r && r.cameras) || []; }, function () { S.obsCams = S.obsCams || []; }).then(function () {
      S.obsCamsLoading = false;
      var sel = $('#dObsCam');
      if (sel) { var v = sel.value; sel.innerHTML = camOptions(S.settings.obs.camera_source || ''); if (v) sel.value = v; }
    });
  }

  function readSettingsForm() {
    var s = clone(S.settings);
    s.obs = Object.assign({}, s.obs, {
      enabled: $('#dObsOn').checked, host: $('#dObsHost').value.trim(), port: parseInt($('#dObsPort').value, 10) || 4455,
      password: $('#dObsPass').value, game_scene: $('#dObsGame').value.trim(), timeout_scene: $('#dObsTimeout').value.trim(),
      camera_fit: $('#dObsFit').value
    });
    // A device to add becomes the camera only through Configurar OBS.
    var cam = $('#dObsCam').value;
    if (!/^(device:|plugin$)/.test(cam)) s.obs.camera_source = cam;
    if ($('#dObsLaunch')) { s.obs.launch = $('#dObsLaunch').checked; s.obs.close_on_exit = $('#dObsClose').checked; }
    s.phone_access = $('#dPhone').checked;
    s.pin = $('#dPin').value.trim();
    s.club = Object.assign({}, s.club, {
      name: $('#dHome').value.trim(), short: $('#dHomeShort').value.trim().toUpperCase(),
      primary: $('#dPrimary').value.toUpperCase(), secondary: $('#dSecondary').value.toUpperCase(),
      sport: $('#dSport').value, country: $('#dCountry').value
    });
    s.instagram = $('#dInsta').value.trim();
    s.club_search = $('#dClubSearch').value.trim();
    s.fpb_base_url = $('#dFpb').value.trim();
    s.auto_screens = $('#dAuto').checked;
    s.overlay = Object.assign({}, s.overlay, { bug_position: $('#dBug').value });
    s.overlay.sponsor_corner = $('#dCorner').checked;
    s.overlay.sponsor_every = Math.min(30, Math.max(1, parseInt($('#dCornerEvery').value, 10) || 4));
    s.credit = $('#dCredit').checked;
    return s;
  }

  function defsEvents() {
    var root = $('#definicoes');
    root.addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]'); if (!b) return;
      if (b.dataset.act === 'setup-open' || b.dataset.act === 'setup-review') {
        if (b.dataset.act === 'setup-review') S.setupStep = 0;
        showTab('inicio');
      } else if (b.dataset.act === 'copy') {
        busy(b, function () { $('#dUrl').select(); return copyText($('#dUrl').value); }, 'Copiado');
      } else if (b.dataset.act === 'crest-remove') {
        busy(b, function () { return api('DELETE', '/api/club/crest').then(setCrest); }, 'Emblema removido');
      } else if (b.dataset.act === 'save-settings') {
        var s = readSettingsForm();
        if (s.phone_access && !/^\d{4}$/.test(s.pin)) {
          var m = b.parentElement.querySelector('.msg'); m.className = 'msg err'; m.textContent = 'Para permitir o telemóvel, escreva um PIN de 4 algarismos.'; return;
        }
        busy(b, function () {
          return api('PUT', '/api/settings', s).then(function (r) { S.settings = r || s; renderChips(); loadStatus(); });
        });
      } else if (b.dataset.act === 'shortcut') {
        busy(b, function () { return createShortcut(true); }, 'Atalho criado');
      } else if (b.dataset.act === 'obs-test') {
        var s2 = readSettingsForm();
        var msg = $('#dObsTestMsg');
        b.disabled = true; msg.className = 'msg'; msg.textContent = 'A testar…';
        api('POST', '/api/obs/test', s2.obs).then(function (r) {
          if (r && r.connected) { msg.className = 'msg ok'; msg.textContent = 'Ligação ao OBS funciona.'; S.obsCams = null; loadObsCameras(); }
          else { msg.className = 'msg err'; msg.textContent = (r && r.error) || 'O OBS não respondeu.'; }
        }, function (err) { msg.className = 'msg err'; msg.textContent = err.message; }).then(function () { b.disabled = false; });
      } else if (b.dataset.act === 'obs-setup') {
        runSetup(b, '');
      } else if (b.dataset.act === 'obs-pick') {
        var pk = root.querySelector('input[name="obsPick"]:checked');
        if (pk) runSetup($('[data-act="obs-setup"]'), pk.value);
      } else if (b.dataset.act === 'obs-fit') {
        // Saves first, so the camera and fit chosen here are the ones used.
        var s3 = readSettingsForm(), fm = $('#dObsMsg');
        b.disabled = true; fm.className = 'msg'; fm.textContent = 'A ajustar…';
        api('PUT', '/api/settings', s3).then(function (r) {
          S.settings = r || s3;
          return api('POST', '/api/obs/camera/fit', {});
        }).then(function (r) {
          var f = (r && r.fit) || {};
          fm.className = r && r.warning ? 'msg err' : 'msg ok';
          fm.textContent = r && r.warning ? r.warning : 'Câmara ajustada: ' + (f.source || '') + ', ' + f.source_width + 'x' + f.source_height + '.';
        }, function (err) { fm.className = 'msg err'; fm.textContent = err.message; }).then(function () { b.disabled = false; });
      }
    });
    root.addEventListener('change', function (e) {
      var t = e.target;
      if (t.id === 'dPin') t.value = t.value.replace(/\D/g, '').slice(0, 4);
      if (t.id === 'dSport' || t.id === 'dCountry') {
        var on = fpbFor({ sport: $('#dSport').value, country: $('#dCountry').value });
        $all('[data-fpb]', root).forEach(function (el) { el.hidden = !on; });
        return;
      }
      if (t.id === 'dPrimary' || t.id === 'dSecondary') { $('#' + t.id + 'Hex').textContent = t.value.toUpperCase(); return; }
      if (t.id === 'dCrestFile' && t.files && t.files[0]) {
        var fd = new FormData(); fd.append('file', t.files[0]);
        busy(t.closest('label'), function () { return api('POST', '/api/club/crest', fd).then(setCrest); }, 'Emblema guardado');
        t.value = '';
        return;
      }
      if (t.id === 'dAutostart') {
        var am = t.closest('.row').querySelector('.msg');
        t.disabled = true; am.className = 'msg'; am.textContent = '';
        api('POST', '/api/system/autostart', { on: t.checked }).then(function () {
          S.settings.start_with_windows = t.checked;
          am.className = 'msg ok'; am.textContent = t.checked ? 'Abre com o Windows' : 'Já não abre com o Windows';
        }, function (err) { t.checked = !t.checked; am.className = 'msg err'; am.textContent = err.message; })
          .then(function () { t.disabled = false; });
        return;
      }
      if (t.id !== 'setDemo') return;
      var msg = t.closest('.row').querySelector('.msg');
      t.disabled = true; msg.textContent = '';
      api('POST', '/api/demo', { on: t.checked }).then(function () {
        msg.className = 'msg ok'; msg.textContent = t.checked ? 'Demonstração ligada' : 'Demonstração desligada';
      }, function (err) { t.checked = !t.checked; msg.className = 'msg err'; msg.textContent = err.message; })
        .then(function () { t.disabled = false; });
    });
  }

  /* ================= TRANSMISSÕES ================= */
  var PRIVACY = { 'public': 'Público', unlisted: 'Não listado', 'private': 'Privado' };
  function loadHistory(refresh) {
    var f = S.histF || {}, q = [];
    if (f.escalao) q.push('escalao=' + encodeURIComponent(f.escalao));
    if (f.season) q.push('season=' + encodeURIComponent(f.season));
    if (refresh) q.push('refresh=1');
    return api('GET', '/api/youtube/history' + (q.length ? '?' + q.join('&') : '')).then(function (h) {
      S.hist = h; S.histErr = '';
    }, function (err) { S.histErr = err.message; }).then(function () { if (S.tab === 'transmissoes') renderHistory(); });
  }
  function renderTransmissoes() {
    $('#transmissoes').innerHTML = '<div class="card hist-bar"><div class="row">' +
      '<label class="field"><span>Escalão</span><select id="hEscalao"></select></label>' +
      '<label class="field"><span>Época</span><select id="hSeason"></select></label>' +
      '<button class="btn" type="button" data-act="hist-refresh">Atualizar</button></div></div><div id="histList"></div>';
    renderHistory();
    loadHistory();
  }
  function renderHistory() {
    var list = $('#histList'); if (!list) return;
    // The lists come from every stream, whatever the filters.
    var f = S.histF || {}, h = S.hist, all = h || { escaloes: [], seasons: [] };
    function opts(vals, cur, none) {
      return '<option value="">' + none + '</option>' + vals.map(function (v) {
        return '<option value="' + esc(v) + '"' + (v === cur ? ' selected' : '') + '>' + esc(v) + '</option>';
      }).join('');
    }
    $('#hEscalao').innerHTML = opts(all.escaloes, f.escalao, 'Todos');
    $('#hSeason').innerHTML = opts(all.seasons, f.season, 'Todas');
    if (S.histErr) { list.innerHTML = '<p class="alert">' + esc(S.histErr) + '</p>'; return; }
    if (!h) { list.innerHTML = '<p class="hint">A carregar…</p>'; return; }
    var html = '';
    if (!h.connected) html += '<p class="warn">Com o YouTube ligado em Definições, aparecem aqui também os diretos feitos fora do Hallstream.</p>';
    if (h.remote_error) html += '<p class="alert">Não consegui ler o canal: ' + esc(h.remote_error) + '</p>';
    if (!h.items.length) {
      html += '<div class="card"><p class="hint">' + (f.escalao || f.season ? 'Nenhuma transmissão com estes filtros.' : 'Ainda não há transmissões. Aparecem aqui depois do primeiro direto com o YouTube ligado.') + '</p></div>';
      list.innerHTML = html; return;
    }
    html += '<ul class="hist">' + h.items.map(function (it) {
      var d = new Date(it.date), when = isNaN(d) ? '' : d.toLocaleDateString('pt-PT', { day: 'numeric', month: 'short', year: 'numeric' });
      var tag = it.external ? '<span class="badge">Fora do Hallstream</span>' : it.test ? '<span class="badge">Teste</span>' : '';
      var priv = PRIVACY[it.privacy] || it.privacy || '';
      return '<li><div class="who"><b>' + esc(it.result || it.title) + '</b>' +
        '<small>' + esc([when, it.result ? it.title : it.escalao].filter(Boolean).join(' · ')) + '</small>' +
        '<span class="tags"><span class="badge priv-' + esc(it.privacy) + '">' + esc(priv) + '</span>' + tag + '</span></div>' +
        '<div class="acts"><button class="btn small" type="button" data-act="hist-copy" data-url="' + esc(it.link) + '">Copiar</button>' +
        '<a class="btn small" href="' + esc(it.link) + '" target="_blank" rel="noopener">Abrir</a>' +
        (it.privacy !== 'unlisted' && !it.test && h.connected ? '<button class="btn small dark" type="button" data-act="hist-unlist" data-id="' + esc(it.broadcast_id) + '">Tornar não listado</button>' : '') +
        '<span class="msg" role="status"></span></div></li>';
    }).join('') + '</ul>';
    list.innerHTML = html;
  }
  function transmissoesEvents() {
    var root = $('#transmissoes');
    root.addEventListener('change', function (e) {
      if (e.target.id !== 'hEscalao' && e.target.id !== 'hSeason') return;
      S.histF = { escalao: $('#hEscalao').value, season: $('#hSeason').value };
      loadHistory();
    });
    root.addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]'); if (!b) return;
      if (b.dataset.act === 'hist-refresh') busy(b, function () { return loadHistory(true); }, 'Atualizado');
      if (b.dataset.act === 'hist-copy') busy(b, function () { return copyText(b.dataset.url); }, 'Copiado');
      if (b.dataset.act === 'hist-unlist') {
        busy(b, function () { return api('POST', '/api/youtube/unlist', { broadcast_id: b.dataset.id }); }, 'Feito')
          .then(function (r) { if (r) loadHistory(); });
      }
    });
  }

  /* ================= CONFIGURAÇÃO INICIAL ================= */
  // Five steps, one card at a time. The status comes from what the panel
  // already reads (status, settings, YouTube); ticks and "Saltar" from /api/setup.
  var WZ = [
    { id: 'clube', title: 'Clube e caixa' }, { id: 'obs', title: 'OBS' }, { id: 'youtube', title: 'YouTube' },
    { id: 'canal', title: 'Canal do YouTube' }, { id: 'ensaio', title: 'Ensaio' }
  ];
  var WZ_TAG = { ok: ['on', 'Feito'], todo: ['wait', 'Falta'], bad: ['off', 'Com problema'], skip: ['', 'Saltado'], wait: ['wait', 'A verificar'] };
  var ENSAIO = [
    ['ensaio-demo', 'Modo demonstração', 'Em <a href="#definicoes" data-goto="definicoes">Definições</a> &gt; Demonstração, ligue o modo demonstração. O grafismo mostra um jogo a fingir, sem a caixa.'],
    ['ensaio-previa', 'Pré-visualização', 'Em <a href="#ecras" data-goto="ecras">Ecrãs</a>, carregue em Abertura e depois em Marcador. A pré-visualização mostra o mesmo que o OBS.'],
    ['ensaio-desconto', 'Desconto de tempo', 'Carregue em Desconto de tempo. No OBS, a câmara encolhe para a janela do grafismo. Depois volte ao Marcador.'],
    ['ensaio-promo', 'Promo com som', 'Em <a href="#promo" data-goto="promo">Promo</a>, mostre um vídeo com Som ligado e confirme que se ouve no OBS.'],
    ['ensaio-teste', 'Transmissão de teste', 'Em <a href="#ecras" data-goto="ecras">Ecrãs</a>, carregue em Transmissão de teste. O direto é privado. Veja-o no YouTube Studio com a conta do canal, depois carregue em Terminar teste.']
  ];

  function longDate(iso) {
    var d = new Date(iso);
    return isNaN(d) ? '' : d.toLocaleDateString('pt-PT', { day: 'numeric', month: 'long', year: 'numeric' });
  }
  function loadSetup() {
    return api('GET', '/api/setup').then(function (f) { S.setup = f; renderSetupAsk(); }, function () {});
  }
  function setupSaid(f) { S.setup = f; renderSetupAsk(); refreshInicio(); }
  function wzTicked(id) { return !!(S.setup && S.setup.steps && S.setup.steps[id] === 'done'); }

  // The banner on top of Ecrãs, on the PC, until the setup is done or put off.
  function renderSetupAsk() {
    var f = S.setup;
    $('#setupAsk').hidden = !(f && f.pc && !f.done && !f.later);
  }

  // A tick ("Já fiz") or what the channel check found: yes, no, blocked or "".
  function canalItem(k) {
    var c = S.chk || {};
    var v = k === 'diretos' ? { on: 'yes', off: 'no' }[c.live] : c.verified;
    return v || (wzTicked('canal-' + k) ? 'yes' : '');
  }
  // The box as it is, not the one the demo pretends to have.
  function realBox() {
    var st = S.state || {};
    return st.demo ? {} : st.box || (S.status && S.status.box) || {};
  }
  function wzStatus(id) {
    var st = S.status || {}, set = S.settings || {}, y = S.yt || st.youtube || {};
    if (id === 'clube') {
      var c = set.club_setup || '', box = realBox();
      if (st.club_setup && st.club_setup.running) return { s: 'wait', why: clubProgress(st.club_setup) };
      if (c === 'pending') return { s: 'bad', why: 'A FPB ainda não respondeu. Confirme a internet e carregue em Importar equipas do clube, no Plantel.' };
      if (!c && !S.groups.length) return { s: 'todo', why: 'Faltam os escalões do clube, com os jogadores da FPB.' };
      if (!set.box_address && !box.connected) return { s: 'todo', why: 'A caixa do marcador ainda não ligou a este computador.' };
      return { s: 'ok', why: 'Os escalões vieram da FPB e a caixa já ligou a este computador.' };
    }
    if (id === 'obs') {
      var o = st.obs || {}, h = o.health || {};
      if (!obsOn()) return { s: 'todo', why: 'O OBS ainda não foi configurado.' };
      if (!o.connected) return { s: 'bad', why: o.error || 'O OBS não responde.' };
      if (!h.checked) return { s: 'wait', why: 'A ver se o OBS está pronto…' };
      if (h.ready) return { s: 'ok', why: 'O OBS está pronto.' };
      return { s: 'bad', why: (h.problems || [])[0] || 'O OBS precisa de reparação.' };
    }
    if (id === 'youtube') {
      if (y.connected && y.expired) return { s: 'bad', why: 'A ligação ao YouTube expirou. Carregue em Ligar ao YouTube outra vez.' };
      if (y.connected) return { s: 'ok', why: 'Ligado ao canal ' + ((y.channel && y.channel.title) || 'do YouTube') + '.' };
      if (y.error) return { s: 'bad', why: y.error };
      if (S.setup && S.setup.steps.youtube === 'skipped') return { s: 'skip', why: 'Fica para depois. A transmissão funciona sem o YouTube ligado ao Marcador, como até agora.' };
      return { s: 'todo', why: 'Falta ligar o Hallstream ao canal do clube.' };
    }
    if (id === 'canal') {
      var l = canalItem('diretos'), v = canalItem('verificado');
      if (S.chkBusy) return { s: 'wait', why: 'A perguntar ao YouTube…' };
      if (v === 'blocked') return { s: 'bad', why: 'O YouTube diz que o canal tem avisos das regras da comunidade ou bloqueios de Content ID. Veja o YouTube Studio.' };
      if (l === 'yes' && v === 'yes') return { s: 'ok', why: 'Os diretos estão ligados e o canal está verificado.' };
      if (l === 'no') return { s: 'todo', why: 'O YouTube diz que os diretos ainda não estão ligados neste canal.' };
      if (v === 'no') return { s: 'todo', why: 'O YouTube diz que o canal ainda não foi verificado com o telemóvel.' };
      return { s: 'todo', why: 'Confirme as duas coisas abaixo no YouTube.' };
    }
    var n = ENSAIO.filter(function (e) { return wzTicked(e[0]); }).length;
    return n === ENSAIO.length ? { s: 'ok', why: 'Ensaio feito.' } : { s: 'todo', why: 'Faltam ' + (ENSAIO.length - n) + ' de ' + ENSAIO.length + '.' };
  }
  function wzOpen(i) { var s = wzStatus(WZ[i].id).s; return s !== 'ok' && s !== 'skip'; }

  // Opens on the first step not done yet, or on the done card.
  function firstStep() {
    if (S.setup.done) return 'fim';
    for (var i = 0; i < WZ.length; i++) if (wzOpen(i)) return i;
    return WZ.length - 1;
  }
  function openInicio() {
    if (S.setupStep == null && S.setup && S.settings) S.setupStep = firstStep();
    renderInicio();
    // Fresh settings: the box, the club import and OBS change them meanwhile.
    Promise.all([api('GET', '/api/settings').then(function (s) { S.settings = s; }), loadSetup(), loadYt()]).then(function () {
      if (S.setupStep == null && S.setup) S.setupStep = firstStep();
      renderInicio();
      if (S.setup && S.setup.pc && S.yt && S.yt.connected && !S.chk && !S.chkBusy) checkChannel();
    }, function () {});
  }

  function renderInicio() {
    var root = $('#inicio');
    if (S.tab !== 'inicio') return;
    if (!S.setup || !S.settings || S.setupStep == null) { root.innerHTML = '<p class="hint">A carregar…</p>'; return; }
    root.innerHTML = '<ol class="wz-steps" id="wzSteps" aria-label="Passos"></ol>' +
      (S.setup.pc ? '' : '<p class="warn">Estes passos fazem-se no computador do clube. Aqui só se vê como estão.</p>') +
      '<div class="card wz" id="wzCard"><div id="wzHead"></div><div id="wzBody"></div><div class="wz-nav" id="wzNav"></div></div>';
    renderWzBody();
    refreshInicio(true);
  }

  // The parts that follow the live status: the step list, the card head and its buttons.
  function refreshInicio(force) {
    var el = $('#wzSteps'); if (!el || S.tab !== 'inicio') return;
    var cur = S.setupStep;
    var steps = WZ.map(function (w, i) {
      var t = WZ_TAG[wzStatus(w.id).s];
      return '<li><button type="button" data-wz="' + i + '"' + (i === cur ? ' aria-current="step"' : '') + '>' +
        '<span class="dot ' + t[0] + '" aria-hidden="true"></span><span class="n">' + (i + 1) + '</span><span class="t">' + esc(w.title) + '</span>' +
        '<span class="sr">' + esc(t[1]) + '</span></button></li>';
    }).join('');
    if (force || el.dataset.h !== steps) { el.dataset.h = steps; el.innerHTML = steps; }
    var head = $('#wzHead'), nav = $('#wzNav'), h, n;
    if (cur === 'fim') {
      h = '<h2>Configuração concluída</h2>';
      n = '<a class="btn primary" href="#ecras" data-goto="ecras">Ir para Ecrãs</a><button class="btn" type="button" data-wz="0">Rever os passos</button>';
    } else {
      var w = WZ[cur], st = wzStatus(w.id), tag = WZ_TAG[st.s], y = S.yt || {};
      h = '<div class="wz-top"><span class="wz-count">Passo ' + (cur + 1) + ' de ' + WZ.length + '</span>' +
        '<span class="wz-state st-' + st.s + '"><span class="dot ' + tag[0] + '"></span>' + tag[1] + '</span></div>' +
        '<h2>' + esc(w.title) + '</h2><p class="wz-why' + (st.s === 'bad' ? ' bad' : '') + '">' + esc(st.why) + '</p>';
      n = (cur > 0 ? '<button class="btn" type="button" data-wz="' + (cur - 1) + '">Anterior</button>' : '') + '<span class="grow"></span>' +
        (w.id === 'youtube' && S.setup.pc && !y.connected && st.s !== 'skip' ? '<button class="btn quiet" type="button" data-wz-act="yt-skip">Saltar por agora</button>' : '') +
        (cur < WZ.length - 1 ? '<button class="btn primary" type="button" data-wz="' + (cur + 1) + '">Seguinte</button>' :
          S.setup.pc ? '<button class="btn primary" type="button" data-wz-act="done">Configuração concluída</button>' : '') +
        '<span class="msg" id="wzNavMsg" role="status"></span>';
    }
    if (force || head.dataset.h !== h) { head.dataset.h = h; head.innerHTML = h; }
    var yc = !!(S.yt && S.yt.connected);
    $all('#wzBody .wz-how').forEach(function (x) { x.hidden = yc; });
    if (force || nav.dataset.h !== n) { nav.dataset.h = n; nav.innerHTML = n; }
  }

  function wzTick(id, label) {
    return '<label class="switch wz-tick"><input type="checkbox" data-tick="' + id + '"' + (wzTicked(id) ? ' checked' : '') +
      (S.setup.pc ? '' : ' disabled') + '><i></i>' + (label || 'Já fiz') + '</label>';
  }
  function wzItem(dot, title, html) {
    return '<li><span class="dot ' + dot + '" aria-hidden="true"></span><div><b>' + title + '</b>' + html + '</div></li>';
  }

  function renderWzBody() {
    var el = $('#wzBody'); if (!el) return;
    var cur = S.setupStep, pc = S.setup.pc, html = '';
    if (cur === 'fim') {
      html = '<p>Concluída em <b>' + esc(longDate(S.setup.done)) + '</b>. Pode rever os passos quando quiser, em Definições &gt; Configuração inicial.</p>';
      var left = WZ.filter(function (w, i) { return wzOpen(i); }).map(function (w) { return w.title; });
      if (left.length) html += '<p class="hint">Ainda por fazer: ' + esc(left.join(', ')) + '.</p>';
      el.innerHTML = html;
      return;
    }
    var id = WZ[cur].id;
    if (id === 'clube') {
      var set = S.settings, box = realBox();
      var club = (set.club_setup && set.club_setup !== 'pending') || S.groups.length;
      html = '<ul class="wz-list">' +
        wzItem(club ? 'on' : 'wait', 'Escalões da FPB', '<p class="hint">' + (club ? plural(S.groups.length, 'escalão', 'escalões') + ' no Plantel. Confira-os e junte os treinadores.' :
          'O Hallstream cria-os sozinho ao abrir pela primeira vez, se houver internet.') + ' <a href="#plantel" data-goto="plantel">Abrir Plantel</a></p>') +
        wzItem(set.box_address || box.connected ? 'on' : 'wait', 'Caixa do marcador', '<p class="hint">' + (box.connected ? 'Ligada agora.' : set.box_address ? 'Já ligou a este computador.' :
          'Ligue a consola do marcador e feche a app SL Live Stream nos telemóveis.') + ' <a href="#caixa" data-goto="caixa">Abrir Caixa</a></p>') +
        '</ul><p class="hint">Sem a consola por perto, passe à frente. O passo fica feito quando a caixa ligar.</p>';
    } else if (id === 'obs') {
      html = '<p>Instale o OBS 31 ou mais recente e o DroidCam, e feche o OBS. O Hallstream liga o servidor WebSocket, abre o OBS e prepara a cena, o grafismo, a câmara e a resolução.</p>';
      if (pc) html += '<div class="row" style="margin-top:12px;align-items:center"><button class="btn primary" type="button" data-wz-act="obs-setup">Configurar OBS</button>' +
        '<span class="msg" id="wzObsMsg" role="status"></span></div>';
      html += '<div id="wzObsResult">' + setupHtml(S.obsSetup) + '</div>';
    } else if (id === 'youtube') {
      html = '<div class="wz-how"><p>Precisa do ID e do segredo do cliente OAuth, criados na Google Cloud com o guia <b>LIGAR-YOUTUBE.txt</b>, na pasta YouTube.</p>' +
        '<ol class="steps"><li>Cole os dois e carregue em <b>Guardar</b>.</li>' +
        '<li>Carregue em <b>Ligar ao YouTube</b>. Entre com a conta Google que gere o canal do clube e, se o Google perguntar, escolha o canal do clube.</li>' +
        '<li>O Google avisa que a app não foi verificada. Carregue em <b>Advanced</b> e depois em <b>Go to Hallstream</b>. A app é a do clube.</li></ol></div>' +
        '<div class="wz-sub" id="ytSetup" data-bare="1">' + ytSetupHtml(true) + '</div>' +
        '<div class="wz-how"><p class="hint">Tem de ser a conta dona do canal, ou dona ou gestora da conta de marca do canal. Um convite em Permissões, no YouTube Studio, não chega, porque o Google não deixa essas contas usar a API.</p>' +
        '<p class="hint">Sem o ID e o segredo, carregue em Saltar por agora. A transmissão funciona na mesma, com a conta do YouTube ligada no próprio OBS.</p></div>';
    } else if (id === 'canal') {
      var l = canalItem('diretos'), v = canalItem('verificado'), c = S.chk || {}, yc = (S.yt || {}).connected;
      var said = function (t) { return '<p class="wz-said">' + t + '</p>'; };
      html = '<p>Duas coisas no próprio canal, feitas uma vez com a conta do canal.</p><ul class="wz-list">' +
        wzItem(l === 'yes' ? 'on' : 'wait', 'Diretos ligados', '<p class="hint">Em <a href="https://www.youtube.com/features" target="_blank" rel="noopener">youtube.com/features</a>, ligue os diretos. Na primeira vez, o YouTube pode demorar até 24 horas.</p>' +
          (c.live ? said(c.live === 'on' ? 'O YouTube confirma que os diretos estão ligados.' : 'O YouTube diz que ainda não estão ligados.') : wzTick('canal-diretos'))) +
        wzItem(v === 'yes' ? 'on' : v === 'blocked' ? 'off' : 'wait', 'Canal verificado com o telemóvel', '<p class="hint">Em <a href="https://www.youtube.com/verify" target="_blank" rel="noopener">youtube.com/verify</a>, confirme o canal com um código por SMS. Sem isso, o direto começa na mesma, mas sem a miniatura do jogo.</p>' +
          (c.verified ? said({ yes: 'O YouTube confirma que o canal está verificado.', no: 'O YouTube diz que ainda não está verificado.', blocked: 'O YouTube não deixa verificar este canal, porque tem avisos ou bloqueios.' }[c.verified]) : wzTick('canal-verificado'))) + '</ul>';
      if (pc && yc) html += '<div class="row" style="align-items:center"><button class="btn" type="button" data-wz-act="chk">Perguntar outra vez ao YouTube</button><span class="msg" id="wzChkMsg" role="status">' + esc(S.chkErr || '') + '</span></div>';
      else html += '<p class="hint">Com o YouTube ligado no passo anterior, o Hallstream pergunta ao YouTube. Sem ele, marque <b>Já fiz</b>.</p>';
    } else {
      html = '<p>Um ensaio curto, antes do primeiro jogo. Marque cada um quando estiver feito.</p><ul class="wz-list">' +
        ENSAIO.map(function (e) { return wzItem(wzTicked(e[0]) ? 'on' : 'wait', esc(e[1]), '<p class="hint">' + e[2] + '</p>' + wzTick(e[0])); }).join('') + '</ul>';
    }
    el.innerHTML = html;
  }

  function wzGo(i) {
    S.setupStep = i === 'fim' ? 'fim' : +i;
    renderInicio();
    if (S.setupStep === 3 && S.setup.pc && S.yt && S.yt.connected && !S.chk && !S.chkBusy) checkChannel();
    var card = $('#wzCard');
    if (card.getBoundingClientRect().top < 0) card.scrollIntoView();
  }

  function checkChannel() {
    S.chkBusy = true; S.chkErr = '';
    refreshInicio();
    api('POST', '/api/setup/channel', {}).then(function (r) { S.chk = r; }, function (err) { S.chk = null; S.chkErr = err.message; })
      .then(function () {
        S.chkBusy = false; refreshInicio();
        // Other steps may hold typing (the YouTube fields): only these two are drawn again.
        if (S.tab === 'inicio' && (S.setupStep === 'fim' || S.setupStep === 3)) renderWzBody();
      });
  }

  function wzObs(btn, camera) {
    var msg = $('#wzObsMsg');
    btn.disabled = true; msg.className = 'msg'; msg.textContent = 'A configurar o OBS… Pode demorar um minuto, se tiver de o abrir.';
    api('POST', '/api/obs/setup', { camera: camera || '', old: (S.settings.obs && S.settings.obs.camera_source) || '' }).then(function (r) {
      S.obsSetup = r; S.obsCams = null;
      return api('GET', '/api/settings').then(function (s) { S.settings = s; });
    }).then(function () {
      loadStatus(); loadStream(); renderWzBody(); refreshInicio();
    }, function (err) {
      var m = $('#wzObsMsg'); if (m) { m.className = 'msg err'; m.textContent = err.message; }
      var b = $('[data-wz-act="obs-setup"]'); if (b) b.disabled = false;
    });
  }

  function setupEvents() {
    var root = $('#inicio');
    ytEvents(root);
    root.addEventListener('click', function (e) {
      var go = e.target.closest('[data-goto]');
      if (go) { e.preventDefault(); showTab(go.dataset.goto); return; }
      var w = e.target.closest('[data-wz]');
      if (w) { wzGo(w.dataset.wz); return; }
      var b = e.target.closest('[data-wz-act], [data-act="obs-pick"]'); if (!b) return;
      var act = b.dataset.wzAct || b.dataset.act, msg = $('#wzNavMsg');
      if (act === 'obs-setup') wzObs(b, '');
      if (act === 'obs-pick') {
        var pk = root.querySelector('input[name="obsPick"]:checked');
        if (pk) wzObs(b, pk.value);
      }
      if (act === 'chk') checkChannel();
      if (act === 'yt-skip' || act === 'done') {
        b.disabled = true;
        var req = act === 'done' ? api('POST', '/api/setup/done', {}) : api('PUT', '/api/setup/steps/youtube', { state: 'skipped' });
        req.then(function (f) {
          setupSaid(f);
          wzGo(act === 'done' ? 'fim' : S.setupStep + 1);
        }, function (err) { b.disabled = false; if (msg) { msg.className = 'msg err'; msg.textContent = err.message; } });
      }
    });
    root.addEventListener('change', function (e) {
      var t = e.target; if (!t.dataset || !t.dataset.tick) return;
      t.disabled = true;
      api('PUT', '/api/setup/steps/' + encodeURIComponent(t.dataset.tick), { state: t.checked ? 'done' : '' }).then(function (f) {
        setupSaid(f); renderWzBody();
      }, function (err) { t.checked = !t.checked; t.disabled = false; showError(err); });
    });
    $('#setupAsk').addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]'); if (!b) return;
      if (b.dataset.act === 'setup-go') { showTab('inicio'); return; }
      busy(b, function () { return api('POST', '/api/setup/later', {}).then(setupSaid); }, ' ');
    });
  }

  /* ================= 8. AJUDA ================= */
  // The text lives in index.html; only the data folder comes from the server.
  function renderAjudaDir() {
    var dir = S.status && S.status.data_dir, el = $('#ajudaDir');
    if (!dir || $('#aDir')) return;
    el.innerHTML = '<div class="urlbox"><input id="aDir" readonly value="' + esc(dir) + '" aria-label="Pasta dos dados">' +
      '<button class="btn dark" data-act="copy-dir">Copiar</button><span class="msg" style="align-self:center"></span></div>';
  }
  function showHelp(id) {
    showTab('ajuda');
    var el = document.getElementById(id);
    if (el) el.scrollIntoView();
  }
  function ajudaEvents() {
    $('main').addEventListener('click', function (e) {
      var h = e.target.closest('[data-help]');
      if (h) { showHelp(h.dataset.help); return; }
      if (!e.target.closest('#ajuda')) return;
      var go = e.target.closest('[data-goto]');
      if (go) { e.preventDefault(); showTab(go.dataset.goto); return; }
      var b = e.target.closest('[data-act="copy-dir"]');
      if (b) busy(b, function () { $('#aDir').select(); return copyText($('#aDir').value); }, 'Copiado');
    });
  }

  /* ---------------- desktop shortcut ---------------- */
  // Asked once, on the PC itself (the status says so), until answered.
  function renderShortcutAsk() {
    var box = $('#shortcutAsk');
    if (box.dataset.closing) return;
    box.hidden = !(S.status && S.status.shortcut && S.settings && !S.settings.desktop_shortcut);
  }
  function createShortcut(create) {
    return api('POST', '/api/system/desktop-shortcut', { create: create }).then(function (r) {
      S.settings.desktop_shortcut = create ? 'created' : 'declined';
      renderShortcutAsk();
      return r;
    });
  }
  function shortcutEvents() {
    var box = $('#shortcutAsk');
    box.addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]'); if (!b) return;
      if (b.dataset.act === 'sc-no') {
        box.hidden = true;
        createShortcut(false).catch(function () {});
        return;
      }
      box.dataset.closing = '1'; // keep it up long enough to read the answer
      busy(b, function () { return createShortcut(true); }, 'Atalho criado').then(function (r) {
        if (!r) { delete box.dataset.closing; return; }
        $all('button', box).forEach(function (x) { x.disabled = true; });
        setTimeout(function () { box.hidden = true; delete box.dataset.closing; }, 2500);
      });
    });
  }

  /* ---------------- boot ---------------- */
  function boot() {
    $all('.nav [data-tab]').forEach(function (b) { b.addEventListener('click', function () { showTab(b.dataset.tab); }); });
    gameEvents(); plantelEvents(); sponsorEvents(); promoEvents(); defsEvents(); caixaEvents(); shortcutEvents(); ajudaEvents();
    transmissoesEvents(); ytEvents($('#definicoes'));
    streamEvents(); setupEvents();
    $('#chips').addEventListener('click', function (e) {
      var go = e.target.closest('[data-goto]');
      if (go) { e.preventDefault(); showTab(go.dataset.goto); }
    });
    window.addEventListener('resize', fitPreview);
    window.addEventListener('hashchange', function () { showTab(location.hash.slice(1)); });

    var start = location.hash.slice(1);
    if (TABS.indexOf(start) < 0) { try { start = localStorage.getItem('painel.tab') || 'jogo'; } catch (e) { start = 'jogo'; } }

    connectEvents();
    Promise.all([
      api('GET', '/api/settings').then(function (s) { S.settings = s; renderShortcutAsk(); loadStream(); }),
      loadGroups(),
      api('GET', '/api/game').then(function (g) { return g; }, function () { return null; }),
      loadSetup()
    ]).then(function (r) {
      setGame(r[2]);
      showTab(start);
      return loadPlayers(S.game.group_id).then(function () {
        refreshSquadCard();
        renderLtPlayers(); updateLowerThird();
      });
    }).catch(function (err) {
      showTab(start);
      showError(err);
    });
    loadStatus();
    setInterval(loadStatus, 10000);
    setInterval(renderChips, 5000);
    loadStream();
    setInterval(loadStream, 3000);
    setInterval(tickStream, 1000);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
})();
