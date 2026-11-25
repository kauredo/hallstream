/* Painel do Hallstream. Sem build, sem bibliotecas. O servidor guarda tudo;
   o browser só guarda o último separador aberto. */
'use strict';

(function () {
  var MOCK = /[?&]mock=1\b/.test(location.search);
  // "inicio" is the Configuração inicial: a tab with no button in the nav.
  var TABS = ['jogo', 'ecras', 'plantel', 'patrocinadores', 'promo', 'transmissoes', 'definicoes', 'caixa', 'ajuda', 'inicio'];
  var SCREENS = [
    { id: 'marcador', label: t('painel.ecra.marcador'), hint: t('painel.ecra.marcador_dica') },
    { id: 'abertura', label: t('painel.ecra.abertura'), hint: t('painel.ecra.abertura_dica') },
    { id: 'plantel', label: t('painel.ecra.plantel'), hint: t('painel.ecra.plantel_dica') },
    { id: 'desconto', label: t('painel.ecra.desconto'), hint: t('painel.ecra.desconto_dica') },
    { id: 'intervalo', label: t('painel.ecra.intervalo'), hint: t('painel.ecra.intervalo_dica') },
    { id: 'final', label: t('painel.ecra.final'), hint: t('painel.ecra.final_dica') },
    { id: 'apoiantes', label: t('painel.ecra.apoiantes'), hint: t('painel.ecra.apoiantes_dica') },
    { id: 'proximo', label: t('painel.ecra.proximo'), hint: t('painel.ecra.proximo_dica') },
    { id: 'voltamos', label: t('painel.ecra.voltamos'), hint: t('painel.ecra.voltamos_dica') },
    { id: 'nada', label: t('painel.ecra.nada'), hint: t('painel.ecra.nada_dica') }
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
    return na - nb || String(a.display_name).localeCompare(String(b.display_name), t('data.locale'));
  }
  function groupName(id) {
    if (id === 'geral') return t('painel.geral');
    var g = S.groups.filter(function (x) { return x.id === id; })[0];
    return g ? g.name : '';
  }
  function groupById(id) { return S.groups.filter(function (x) { return x.id === id; })[0] || null; }
  function pad(n) { return (n < 10 ? '0' : '') + n; }
  function hhmm(d) { return pad(d.getHours()) + ':' + pad(d.getMinutes()); }
  function ago(iso) {
    if (!iso) return '';
    var s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
    if (s < 60) return t('painel.tempo.ha_s', { n: s });
    if (s < 3600) return t('painel.tempo.ha_min', { n: Math.round(s / 60) });
    return t('painel.tempo.as', { hora: hhmm(new Date(iso)) });
  }
  // one and many come already filled with n, from a pair of keys ending in .um and .outros.
  function plural(n, one, many) { return n === 1 ? one : many; }
  function since(iso) {
    var m = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000));
    if (m < 1) return t('painel.tempo.agora');
    if (m < 60) return t('painel.tempo.ha_min', { n: m });
    if (m < 1440) return t('painel.tempo.ha_h', { n: Math.round(m / 60) });
    var days = Math.round(m / 1440);
    return plural(days, t('painel.tempo.ha_dias.um', { n: days }), t('painel.tempo.ha_dias.outros', { n: days }));
  }
  function syncSummary(c) {
    c = c || {};
    var parts = [];
    if (c.new_players) parts.push(plural(c.new_players, t('painel.sync.novos.um', { n: c.new_players }), t('painel.sync.novos.outros', { n: c.new_players })));
    if (c.numbers) parts.push(plural(c.numbers, t('painel.sync.numeros.um', { n: c.numbers }), t('painel.sync.numeros.outros', { n: c.numbers })));
    if (c.photos) parts.push(plural(c.photos, t('painel.sync.fotos.um', { n: c.photos }), t('painel.sync.fotos.outros', { n: c.photos })));
    if (c.returned) parts.push(plural(c.returned, t('painel.sync.voltaram.um', { n: c.returned }), t('painel.sync.voltaram.outros', { n: c.returned })));
    if (c.left) parts.push(plural(c.left, t('painel.sync.sairam.um', { n: c.left }), t('painel.sync.sairam.outros', { n: c.left })));
    return parts.length ? parts.join(', ') : t('painel.sync.sem_mudancas');
  }
  function clubProgress(c) {
    return c && c.total ? t('painel.clube.a_importar', { n: Math.min(c.done + 1, c.total), total: c.total }) : t('painel.clube.a_procurar');
  }
  var DOW = [t('painel.data.dia_curto.0'), t('painel.data.dia_curto.1'), t('painel.data.dia_curto.2'), t('painel.data.dia_curto.3'),
    t('painel.data.dia_curto.4'), t('painel.data.dia_curto.5'), t('painel.data.dia_curto.6')];
  var MON = [t('painel.data.mes_curto.1'), t('painel.data.mes_curto.2'), t('painel.data.mes_curto.3'), t('painel.data.mes_curto.4'),
    t('painel.data.mes_curto.5'), t('painel.data.mes_curto.6'), t('painel.data.mes_curto.7'), t('painel.data.mes_curto.8'),
    t('painel.data.mes_curto.9'), t('painel.data.mes_curto.10'), t('painel.data.mes_curto.11'), t('painel.data.mes_curto.12')];
  function fpbDate(dateStr, timeStr) {
    var p = String(dateStr || '').split('-');
    var tx = String(timeStr || '').toUpperCase().split('H');
    var d = new Date(+p[0], (+p[1] || 1) - 1, +p[2] || 1, +tx[0] || 0, +tx[1] || 0);
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
    if (g[0] === 'm') return t('painel.genero.masculino');
    if (g[0] === 'f') return t('painel.genero.feminino');
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
        if (!res.ok) throw new Error((data && data.error) || t('painel.api.erro', { codigo: res.status }));
        return data;
      });
    }, function () {
      throw new Error(t('painel.api.sem_ligacao'));
    });
  }

  function askPin(msg) {
    if (pinWait) return pinWait;
    var box = $('#pin');
    box.hidden = false;
    $('#pinMsg').textContent = document.cookie.indexOf('pin=') >= 0 ? (msg || t('painel.pin.errado')) : '';
    var input = $('#pinValue');
    input.value = '';
    setTimeout(function () { input.focus(); }, 30);
    pinWait = new Promise(function (resolve) {
      $('#pinForm').onsubmit = function (e) {
        e.preventDefault();
        if (!/^\d{4}$/.test(input.value)) { $('#pinMsg').textContent = t('painel.pin.quatro'); return; }
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
    if (msg) { msg.className = 'msg'; msg.textContent = t('painel.a_guardar'); }
    return Promise.resolve().then(promiseFn).then(function (r) {
      if (msg) {
        msg.className = 'msg ok'; msg.textContent = okText || t('painel.guardado');
        setTimeout(function () { if (msg.textContent === (okText || t('painel.guardado'))) msg.textContent = ''; }, 3000);
      }
      return r;
    }, function (err) {
      if (msg) { msg.className = 'msg err'; msg.textContent = err.message; }
      else showError(err);
      throw err;
    }).then(function (r) { if (btn) btn.disabled = false; return r; }, function () { if (btn) btn.disabled = false; });
  }
  function showError(err) { confirmBox(t('painel.nao_deu'), esc(err.message), { yes: t('painel.ok'), noCancel: true }); }

  function confirmBox(title, html, o) {
    o = o || {};
    var m = $('#modal');
    $('#modalTitle').textContent = title;
    $('#modalBody').innerHTML = html;
    var yes = $('#modalYes'), no = $('#modalNo');
    yes.textContent = o.yes || t('painel.confirmar');
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
      renderUpdateAsk();
      refreshInicio();
      var dl = $('#dLang');
      if (dl && s && s.langs && dl.options.length !== s.langs.length) dl.innerHTML = langOptions(s.langs);
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
    var hn = (game.home && game.home.short) || lv.home_name || t('painel.topo.casa');
    var an = (game.away && game.away.short) || lv.away_name || t('painel.topo.fora');
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
    $('#liveClock').textContent = lv.period ? t('painel.topo.periodo', { periodo: lv.period, relogio: lv.clock || '' }) : (lv.clock || '');
    var cur = st.screen && st.screen.current;
    var tag = $('#liveScreen');
    var label = st.demo ? t('painel.topo.demo') : cur === 'promo' ? t('painel.nav.promo') : (SCREENS.filter(function (s) { return s.id === cur; })[0] || {}).label;
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
    TABS.forEach(function (tx) {
      $('#tab-' + tx).hidden = tx !== name;
      var nb = $('.nav [data-tab="' + tx + '"]');
      if (nb) nb.setAttribute('aria-selected', tx === name ? 'true' : 'false');
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
    if (!d) { root.innerHTML = '<p class="hint">' + t('painel.a_carregar') + '</p>'; return; }
    if (!S.groups.length) {
      root.innerHTML = '<div class="card"><p>' + t('painel.jogo.sem_escaloes') + '</p></div>';
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
    if (S.gameNewer) html += '<div class="ask"><p class="txt"><b>' + t('painel.jogo.mudado') + '</b><span class="hint">' + t('painel.jogo.mudado_dica') + '</span></p>' +
      '<div class="row"><button class="btn primary" type="button" data-act="game-newer">' + t('painel.jogo.usar_guardado') + '</button></div></div>';
    html += '<div class="card"><div class="row">' +
      '<label class="field grow"><span>' + t('painel.jogo.escalao') + '</span><select id="jGroup">' + (d.group_id ? '' : '<option value="">' + t('painel.escolha') + '</option>') + opts + '</select></label>' +
      (fpbOffered() ? '<button class="btn dark" data-act="fpb-games"' + (g && g.fpb_team_id ? '' : ' disabled') + '>' + t('painel.jogo.escolher_fpb') + '</button>' : '') +
      '<button class="btn" data-act="manual">' + t('painel.jogo.manual') + '</button>' +
      '</div>';
    if (g && !g.fpb_team_id && fpbOffered()) html += '<p class="hint" style="margin-top:10px">' + t('painel.jogo.sem_fpb') + '</p>';
    if (S.fpbGamesErr) html += '<p class="alert" style="margin-top:12px">' + esc(S.fpbGamesErr) + '</p>';
    if (S.fpbGames) html += renderFpbGames();
    html += '</div>';
    if (g) html += fixturesCard(g);

    html += '<div class="card"><h2>' + t('painel.jogo.adversario') + (d.fpb_game_id ? ' <span class="badge">FPB</span>' : '') + '</h2>' +
      '<div class="opp">' +
      '<div><div class="crest-tile" id="jCrest" style="--opp:' + esc(color) + '">' +
        (a.crest_url ? '<img src="' + esc(a.crest_url) + '" alt="' + t('painel.jogo.emblema_alt') + '">' : esc(a.short || initials(a.name))) + '</div>' +
        '<div class="stack" style="margin-top:10px">' +
        '<label class="btn small filebtn" style="width:100%">' + t('painel.jogo.carregar_emblema') + '<input type="file" id="jCrestFile" accept="image/png,image/jpeg,image/svg+xml"></label>' +
        (a.crest_url ? '<button class="btn quiet small" data-act="crest-clear" style="width:100%">' + t('painel.retirar') + '</button>' : '') +
        '<span class="msg"></span></div></div>' +
      '<div class="stack">' +
        '<div class="row row-top">' +
          '<label class="field grow"><span>' + t('painel.jogo.nome') + '</span><input id="jName" maxlength="20" value="' + esc(a.name) + '" autocomplete="off"><div class="counter" id="jNameC">' + a.name.length + '/20</div></label>' +
          '<label class="field" style="width:120px"><span>' + t('painel.jogo.sigla') + '</span><input id="jShort" maxlength="4" value="' + esc(a.short) + '" autocomplete="off"></label>' +
        '</div>' +
        '<div><span class="label">' + t('painel.jogo.cor') + '</span><div class="swatches">' +
          SWATCHES.map(function (c) {
            return '<button type="button" class="swatch" data-color="' + c + '" style="background:' + c + '" aria-label="' + t('painel.jogo.cor_n', { cor: c }) + '" aria-pressed="' + (c.toLowerCase() === color.toLowerCase()) + '"></button>';
          }).join('') +
          '<input type="color" id="jColor" value="' + esc(/^#[0-9a-f]{6}$/i.test(color) ? color : '#5C6170') + '" aria-label="' + t('painel.jogo.outra_cor') + '">' +
        '</div>' +
        (ratio !== null && ratio < 4.5 ? '<p class="warn" style="margin-top:8px">' + t('painel.jogo.cor_clara') + '</p>' : '') +
        (!a.color && d.fpb_game_id ? '<p class="hint" style="margin-top:6px">' + t('painel.jogo.cor_do_emblema') + '</p>' : '') +
        '</div>' +
      '</div></div></div>';

    html += '<div class="card"><h2>' + t('painel.jogo.detalhes') + '</h2><div class="cols">' +
      '<label class="field"><span>' + t('painel.jogo.data_hora') + '</span><input type="datetime-local" id="jStart" value="' + esc(toInputDT(d.start)) + '"></label>' +
      '<label class="field"><span>' + t('painel.jogo.pavilhao') + '</span><input id="jVenue" value="' + esc(d.venue) + '"></label>' +
      '<label class="field"><span>' + t('painel.jogo.competicao') + '</span><input id="jComp" value="' + esc(d.competition) + '"></label>' +
      '<label class="field"><span>' + t('painel.jogo.jornada_fase') + '</span><input id="jRound" value="' + esc(d.round) + '"></label>' +
      '</div></div>';

    html += '<div class="card" id="jSquadCard">' + renderSquad() + '</div>';

    html += '<div class="savebar"><button class="btn primary" data-act="save-game">' + t('painel.jogo.guardar') + '</button><span class="msg"></span>' +
      '<span style="flex:1"></span><button class="btn danger" data-act="new-game">' + t('painel.jogo.novo') + '</button></div>';
    root.innerHTML = html;
  }

  // Calendário: the escalão's games typed or imported by the club (any club, with or without the FPB).
  // A game picked here fills the game of the day, as one picked from the FPB does.
  function loadFixtures(gid) {
    return api('GET', '/api/groups/' + encodeURIComponent(gid) + '/fixtures').then(function (list) {
      S.fixtures = S.fixtures || {};
      S.fixtures[gid] = list || [];
      if (S.tab === 'jogo') renderJogo();
    }, function (err) { S.fixtures = S.fixtures || {}; S.fixtures[gid] = []; showError(err); });
  }
  function fixturesCard(g) {
    S.fixtures = S.fixtures || {};
    var all = S.fixtures[g.id];
    var html = '<div class="card" id="jFixtures"><h2>' + t('painel.calendario.titulo') + '</h2>';
    if (!all) { loadFixtures(g.id); return html + '<p class="hint">' + t('painel.a_carregar') + '</p></div>'; }
    var soon = Date.now() - 3 * 3600e3;
    var next = all.filter(function (f) { return new Date(f.start).getTime() >= soon; });
    if (!next.length) html += '<p class="hint">' + (fpbOffered() && g.fpb_team_id ? t('painel.calendario.vazio_fpb') : t('painel.calendario.vazio')) + '</p>';
    else html += '<div class="games" role="list">' + next.map(function (f) {
      var d = new Date(f.start), o = f.opponent || {};
      var sel = !S.draft.fpb_game_id && S.draft.start && new Date(S.draft.start).getTime() === d.getTime() && S.draft.away.name === o.name;
      return '<div class="fx-row" role="listitem"><button class="game-pick' + (sel ? ' sel' : '') + '" data-act="pick-fixture" data-fid="' + esc(f.id) + '">' +
        '<span class="when">' + t('painel.jogo.fpb_data', { dia_semana: DOW[d.getDay()], dia: d.getDate(), mes: MON[d.getMonth()] }) + '<small>' + hhmm(d) + '</small></span>' +
        '<span class="who">' + esc(o.name) + '<small>' + esc([f.competition, /^\d{1,3}$/.test(f.round || '') ? t('jogo.jornada', { n: f.round }) : f.round].filter(Boolean).join(', ')) + '</small>' +
        '<span class="where">' + esc(f.venue || '') + '</span></span>' +
        '<span class="ha">' + (f.home ? t('painel.topo.casa') : t('painel.topo.fora')) + '</span></button>' +
        '<button class="btn quiet small" data-act="del-fixture" data-fid="' + esc(f.id) + '" aria-label="' + t('painel.calendario.apagar_jogo', { adversario: esc(o.name) }) + '">' + t('painel.apagar') + '</button></div>';
    }).join('') + '</div>';
    html += '<details class="fx-add"' + (all.length ? '' : ' open') + '><summary>' + t('painel.calendario.juntar') + '</summary>' +
      '<form id="fxForm" class="stack" style="margin-top:12px"><div class="cols">' +
        '<label class="field"><span>' + t('painel.jogo.data_hora') + '</span><input type="datetime-local" id="fxStart" required></label>' +
        '<label class="field"><span>' + t('painel.calendario.onde') + '</span><select id="fxHome"><option value="1">' + t('painel.calendario.em_casa') + '</option><option value="0">' + t('painel.calendario.fora') + '</option></select></label>' +
        '<label class="field"><span>' + t('painel.jogo.nome') + '</span><input id="fxName" maxlength="20" required autocomplete="off"></label>' +
        '<label class="field"><span>' + t('painel.jogo.sigla') + '</span><input id="fxShort" maxlength="4" autocomplete="off"></label>' +
        '<label class="field"><span>' + t('painel.jogo.pavilhao') + '</span><input id="fxVenue"></label>' +
        '<label class="field"><span>' + t('painel.jogo.competicao') + '</span><input id="fxComp"></label>' +
        '<label class="field"><span>' + t('painel.jogo.jornada_fase') + '</span><input id="fxRound"></label>' +
        '<label class="field"><span>' + t('painel.calendario.emblema') + '</span><input type="file" id="fxCrest" accept="image/png,image/jpeg,image/svg+xml"></label>' +
      '</div><div class="row"><button class="btn primary" type="submit">' + t('painel.calendario.guardar_jogo') + '</button><span class="msg"></span></div></form></details>';
    html += csvRow('fxCsv', 'jogos.csv', t('painel.csv.modelo_jogos'), t('painel.csv.dica_jogos')) + csvResult('fixtures', g.id);
    return html + '</div>';
  }
  function pickFixture(f, btn) {
    var d = S.draft, o = f.opponent || {};
    d.fpb_game_id = null;
    d.away = { name: o.name || '', short: (o.short || initials(o.name)).toUpperCase().slice(0, 4), color: o.color || '', crest_url: o.crest_url || '' };
    d.start = f.start;
    d.venue = f.venue || '';
    d.competition = f.competition || '';
    d.round = /^\d{1,3}$/.test(f.round || '') ? t('jogo.jornada', { n: f.round }) : (f.round || '');
    busy(btn, function () { return saveGame(); }, t('painel.jogo.escolhido'));
  }
  // An import button for a CSV, with a template to download.
  function csvRow(id, file, template, hint) {
    return '<div class="csv-row"><label class="btn small filebtn">' + t('painel.csv.importar') + '<input type="file" id="' + id + '" accept=".csv,text/csv,text/plain"></label>' +
      '<a class="btn quiet small" download="' + file + '" href="data:text/csv;charset=utf-8,' + encodeURIComponent(template) + '">' + t('painel.csv.modelo') + '</a>' +
      '<span class="msg" role="status"></span><p class="hint">' + hint + '</p></div>';
  }
  function csvResult(kind, gid) {
    var r = S.csvResult;
    if (!r || r.kind !== kind || r.gid !== gid) return '';
    var html = '<div class="csv-result" role="status"><p><b>' + (kind === 'players'
      ? t('painel.csv.resultado_plantel', { novos: r.added, atualizados: r.updated || 0 })
      : t('painel.csv.resultado_jogos', { novos: r.added })) + '</b></p>';
    if (r.skipped && r.skipped.length) html += '<p>' + t('painel.csv.ignoradas') + '</p><ul>' + r.skipped.map(function (x) {
      return '<li>' + t('painel.csv.linha', { n: x.line, motivo: esc(x.reason) }) + '</li>';
    }).join('') + '</ul>';
    return html + '</div>';
  }
  function postCSV(path, input) {
    var fd = new FormData(); fd.append('file', input.files[0]);
    input.value = '';
    return api('POST', path, fd);
  }
  function manualEvents() {
    $('#plantel').addEventListener('change', function (e) {
      var tx = e.target, gid = S.plantelGid;
      if (tx.id === 'pCsv' && tx.files && tx.files[0]) {
        busy(tx.closest('label'), function () {
          return postCSV('/api/groups/' + encodeURIComponent(gid) + '/players/csv', tx).then(function (r) {
            S.players[gid] = r.players;
            S.csvResult = { kind: 'players', gid: gid, added: r.added, updated: r.updated, skipped: r.skipped };
            renderPlantel();
            // Photo addresses download in the background; read the players again shortly.
            setTimeout(function () { loadPlayers(gid).then(function () { if (S.tab === 'plantel') renderPlantel(); }); }, 4000);
          });
        });
      } else if (tx.matches('[data-photo-file]') && tx.files && tx.files[0]) {
        var pid = tx.closest('[data-pid]').dataset.pid, fd = new FormData();
        fd.append('file', tx.files[0]);
        tx.value = '';
        busy(tx.closest('label'), function () {
          return api('POST', '/api/groups/' + encodeURIComponent(gid) + '/players/' + encodeURIComponent(pid) + '/photo', fd).then(function () {
            return loadPlayers(gid).then(renderPlantel);
          });
        });
      }
    });
    $('#plantel').addEventListener('click', function (e) {
      var b = e.target.closest('[data-act="photo-remove"]'); if (!b) return;
      var gid = S.plantelGid, pid = b.closest('[data-pid]').dataset.pid;
      busy(b, function () {
        return api('DELETE', '/api/groups/' + encodeURIComponent(gid) + '/players/' + encodeURIComponent(pid) + '/photo').then(function () {
          return loadPlayers(gid).then(renderPlantel);
        });
      });
    });
    var jogo = $('#jogo');
    jogo.addEventListener('change', function (e) {
      var tx = e.target, gid = S.draft && S.draft.group_id;
      if (tx.id !== 'fxCsv' || !tx.files || !tx.files[0] || !gid) return;
      busy(tx.closest('label'), function () {
        return postCSV('/api/groups/' + encodeURIComponent(gid) + '/fixtures/csv', tx).then(function (r) {
          S.fixtures[gid] = r.fixtures;
          S.csvResult = { kind: 'fixtures', gid: gid, added: r.added, skipped: r.skipped };
          renderJogo();
        });
      });
    });
    jogo.addEventListener('submit', function (e) {
      if (e.target.id !== 'fxForm') return;
      e.preventDefault();
      var gid = S.draft.group_id, btn = e.target.querySelector('[type="submit"]'), crest = $('#fxCrest').files[0];
      var body = {
        start: $('#fxStart').value ? isoLocal(new Date($('#fxStart').value)) : '',
        home: $('#fxHome').value === '1',
        opponent: { name: $('#fxName').value.trim(), short: $('#fxShort').value.trim() },
        venue: $('#fxVenue').value.trim(), competition: $('#fxComp').value.trim(), round: $('#fxRound').value.trim()
      };
      busy(btn, function () {
        return api('POST', '/api/groups/' + encodeURIComponent(gid) + '/fixtures', body).then(function (f) {
          if (!crest) return;
          var fd = new FormData(); fd.append('file', crest);
          return api('POST', '/api/groups/' + encodeURIComponent(gid) + '/fixtures/' + encodeURIComponent(f.id) + '/crest', fd);
        }).then(function () { return loadFixtures(gid); });
      });
    });
    jogo.addEventListener('click', function (e) {
      var b = e.target.closest('[data-act="pick-fixture"], [data-act="del-fixture"]'); if (!b) return;
      var gid = S.draft.group_id, list = (S.fixtures && S.fixtures[gid]) || [];
      var f = list.filter(function (x) { return x.id === b.dataset.fid; })[0]; if (!f) return;
      if (b.dataset.act === 'pick-fixture') { pickFixture(f, b); return; }
      confirmBox(t('painel.calendario.apagar_pergunta', { adversario: esc(f.opponent.name) }), '', { yes: t('painel.apagar'), danger: true }).then(function (ok) {
        if (ok) busy(b, function () { return api('DELETE', '/api/groups/' + encodeURIComponent(gid) + '/fixtures/' + encodeURIComponent(f.id)).then(function () { return loadFixtures(gid); }); });
      });
    });
  }

  function renderFpbGames() {
    var g = groupById(S.draft.group_id);
    var list = S.fpbGames.games || [];
    var teamId = S.fpbGames.team ? S.fpbGames.team.id : (g && g.fpb_team_id);
    if (!list.length) return '<p class="hint" style="margin-top:12px">' + t('painel.jogo.fpb_sem_jogos') + '</p>';
    return '<div class="games" role="list">' + list.map(function (x, i) {
      var home = String(x.home && x.home.id) === String(teamId);
      var opp = home ? x.away : x.home;
      var d = fpbDate(x.date, x.time);
      var sel = String(S.draft.fpb_game_id) === String(x.id);
      return '<button class="game-pick' + (sel ? ' sel' : '') + '" data-act="pick-game" data-i="' + i + '" role="listitem">' +
        '<span class="when">' + t('painel.jogo.fpb_data', { dia_semana: DOW[d.getDay()], dia: d.getDate(), mes: MON[d.getMonth()] }) + '<small>' + hhmm(d) + '</small></span>' +
        '<span class="who">' + esc(opp ? opp.name : '?') + '<small>' + esc([x.competition_name, x.game_week ? t('jogo.jornada', { n: x.game_week }) : x.phase_name].filter(Boolean).join(' · ')) + '</small>' +
        '<span class="where">' + esc(x.location || '') + '</span></span>' +
        '<span class="ha">' + (home ? t('painel.topo.casa') : t('painel.topo.fora')) + '</span></button>';
    }).join('') + '</div>';
  }

  function renderSquad() {
    var d = S.draft, list = S.players[d.group_id];
    if (!d.group_id) return '<h2>' + t('painel.jogo.plantel_do_dia') + '</h2><p class="hint">' + t('painel.jogo.escolha_escalao') + '</p>';
    if (!list) return '<h2>' + t('painel.jogo.plantel_do_dia') + '</h2><p class="hint">' + t('painel.jogo.a_carregar_jogadores') + '</p>';
    list = list.filter(function (p) { return !p.inactive; });
    var picked = {};
    d.squad.forEach(function (s) { picked[s.player_id] = s; });
    var n = d.squad.length;
    var html = '<h2>' + t('painel.jogo.plantel_do_dia') + ' <span class="count" id="jCount">' + t('painel.jogo.contagem', { n: n, max: MAX_SQUAD }) + '</span></h2>';
    if (!list.length) return html + '<p class="hint">' + t('painel.jogo.sem_jogadores') + '</p>';
    html += '<p class="hint" style="margin-bottom:10px">' + t('painel.jogo.marque') + '</p>';
    html += '<p class="warn" id="jOver" style="margin-bottom:10px"' + (n > MAX_SQUAD ? '' : ' hidden') + '>' + t('painel.jogo.mais_de_12') + '</p>';
    html += '<div class="squad">' + list.map(function (p) {
      var s = picked[p.id];
      var id = 'sq-' + p.id;
      return '<div class="squad-row' + (s ? ' on' : '') + '">' +
        '<input type="checkbox" id="' + esc(id) + '" data-sq="' + esc(p.id) + '"' + (s ? ' checked' : '') + '>' +
        '<label for="' + esc(id) + '">' + esc(p.display_name) + '</label>' +
        '<input type="text" inputmode="numeric" maxlength="2" data-sqn="' + esc(p.id) + '" value="' + esc(s ? s.number : p.number) + '" aria-label="' + t('painel.jogo.numero_de', { nome: esc(p.display_name) }) + '"' + (s ? '' : ' disabled') + '>' +
        '</div>';
    }).join('') + '</div>';
    return html;
  }

  function gameEvents() {
    var root = $('#jogo');
    root.addEventListener('change', function (e) {
      var tx = e.target, d = S.draft;
      if (tx.id === 'jGroup') {
        d.group_id = tx.value; d.squad = []; S.fpbGames = null; S.fpbGamesErr = '';
        renderJogo();
        loadPlayers(d.group_id).then(refreshSquadCard, showError);
      } else if (tx.id === 'jColor') {
        d.away.color = tx.value.toUpperCase(); renderJogo();
      } else if (tx.id === 'jStart') {
        d.start = tx.value ? isoLocal(new Date(tx.value)) : '';
      } else if (tx.id === 'jCrestFile' && tx.files && tx.files[0]) {
        var fd = new FormData(); fd.append('file', tx.files[0]);
        busy(tx.closest('label'), function () {
          return api('POST', '/api/game/opponent-logo', fd).then(function (r) { d.away.crest_url = r.crest_url; renderJogo(); });
        }, t('painel.carregado'));
      } else if (tx.dataset.sq) {
        var pid = tx.dataset.sq;
        var p = (S.players[d.group_id] || []).filter(function (x) { return String(x.id) === pid; })[0];
        var numInput = $('[data-sqn="' + pid + '"]', root);
        if (tx.checked) d.squad.push({ player_id: p.id, number: numInput.value || p.number });
        else d.squad = d.squad.filter(function (s) { return String(s.player_id) !== pid; });
        tx.parentElement.classList.toggle('on', tx.checked);
        numInput.disabled = !tx.checked;
        $('#jCount').textContent = t('painel.jogo.contagem', { n: d.squad.length, max: MAX_SQUAD });
        $('#jOver').hidden = d.squad.length <= MAX_SQUAD;
      }
    });
    root.addEventListener('input', function (e) {
      var tx = e.target, d = S.draft;
      if (tx.id === 'jName') { d.away.name = tx.value; $('#jNameC').textContent = tx.value.length + '/20'; updateCrestTile(); }
      else if (tx.id === 'jShort') { d.away.short = tx.value.toUpperCase(); tx.value = d.away.short; updateCrestTile(); }
      else if (tx.id === 'jVenue') d.venue = tx.value;
      else if (tx.id === 'jComp') d.competition = tx.value;
      else if (tx.id === 'jRound') d.round = tx.value;
      else if (tx.dataset.sqn) {
        tx.value = tx.value.replace(/\D/g, '');
        d.squad.forEach(function (s) { if (String(s.player_id) === tx.dataset.sqn) s.number = tx.value; });
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
        confirmBox(t('painel.jogo.novo_pergunta'), '<p>' + t('painel.jogo.novo_texto') + '</p>', { yes: t('painel.jogo.novo'), danger: true }).then(function (ok) {
          if (ok) busy(b, function () { return api('POST', '/api/game/reset'); }, t('painel.jogo.periodos_apagados'));
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
    d.round = x.game_week ? t('jogo.jornada', { n: x.game_week }) : (x.phase_name || '');
    // Save straight away so the server fetches the crest and picks the colour.
    busy(btn, function () { return saveGame(); }, t('painel.jogo.escolhido'));
  }

  function saveGame() {
    var d = S.draft;
    if (!d.group_id) return Promise.reject(new Error(t('painel.jogo.escolha_o_escalao')));
    var body = {
      group_id: d.group_id, fpb_game_id: d.fpb_game_id || null,
      away: { name: d.away.name.trim().slice(0, 20), short: d.away.short.trim().slice(0, 4), color: d.away.color || '', crest_url: d.away.crest_url || '' },
      start: d.start || '', venue: d.venue, competition: d.competition, round: d.round,
      squad: d.squad.map(function (s) { return { player_id: s.player_id, number: String(s.number || '') }; })
    };
    return api('PUT', '/api/game', body).then(function (r) {
      setGame(r || body);
      if (S.tab === 'jogo') {
        var msgText = t('painel.guardado');
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
      '<div class="card"><h2>' + t('painel.ecras.no_ecra') + '</h2>' +
      '<div class="screens" id="screens">' + SCREENS.map(function (s) {
        return '<button class="scr' + (s.id === 'nada' ? ' nada' : '') + '" data-screen="' + s.id + '" aria-pressed="false">' +
          '<span class="scr-pv" aria-hidden="true"><svg viewBox="0 0 160 90">' + PV_COURT + (PV[s.id] || '') + '</svg></span>' +
          '<span class="scr-nome">' + esc(s.label) + '</span><small>' + esc(s.hint) + '</small></button>';
      }).join('') + '</div>' +
      '<div class="row" style="margin-top:14px;align-items:center">' +
        '<label class="switch"><input type="checkbox" id="autoScr"><i></i>' + t('painel.ecras.automatico') + '</label>' +
        '<span class="hint grow">' + t('painel.ecras.automatico_dica') + '</span>' +
        '<span class="msg" id="scrMsg"></span>' +
      '</div>' +
      '<form class="row" id="vjForm" style="margin-top:14px;align-items:flex-end">' +
        '<label class="field grow"><span>' + t('painel.ecras.frase_voltamos') + '</span><input id="vjLine" maxlength="60" autocomplete="off" placeholder="' + t('painel.ecras.frase_voltamos_ex') + '"></label>' +
        '<button class="btn" type="submit">' + t('painel.ecras.guardar_frase') + '</button><span class="msg" id="vjMsg"></span>' +
      '</form>' +
      '<div class="row" style="margin-top:14px;align-items:center">' +
        '<button class="btn" type="button" data-act="resultado">' + t('painel.ecras.guardar_resultado') + '</button>' +
        '<span class="hint grow">' + t('painel.ecras.guardar_resultado_dica') + '</span>' +
        '<span class="msg" id="resMsg"></span>' +
      '</div>' +
      '<div class="row" style="margin-top:10px;align-items:center">' +
        '<button class="btn" type="button" data-act="miniatura">' + t('painel.ecras.miniatura') + '</button>' +
        '<span class="hint grow">' + t('painel.ecras.miniatura_dica') + '</span>' +
        '<span class="msg" id="minMsg"></span>' +
      '</div></div>' +
      '<div class="card"><h2>' + t('painel.ecras.rodape') + '</h2>' +
        '<div id="ltNow"></div>' +
        '<span class="label">' + t('painel.ecras.jogador_hoje') + '</span>' +
        '<div class="lt-grid" id="ltPlayers"></div>' +
        '<p class="msg" id="ltMsg"></p>' +
        '<form id="ltCom" style="margin-top:16px"><span class="label">' + t('painel.ecras.comentador') + '</span><div class="row">' +
          '<label class="field grow"><span>' + t('painel.nome') + '</span><input id="ltFirst" autocomplete="off"></label>' +
          '<label class="field grow"><span>' + t('painel.apelido') + '</span><input id="ltLast" autocomplete="off"></label>' +
          '<label class="field grow"><span>' + t('painel.ecras.funcao') + '</span><input id="ltLabel" value="' + t('painel.ecras.comentador') + '" autocomplete="off"></label>' +
          '<button class="btn dark" type="submit">' + t('painel.mostrar') + '</button>' +
        '</div></form>' +
      '</div></div>' +
      '<div><div class="card"><h2>' + t('painel.ecras.previsualizacao') + '</h2><div class="preview" id="preview"><iframe id="pvFrame" title="' + t('painel.ecras.previsualizacao_titulo') + '" tabindex="-1"></iframe></div>' +
      '<p class="hint" style="margin-top:8px">' + t('painel.ecras.previsualizacao_dica') + '</p></div></div>' +
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
    var when = new Intl.DateTimeFormat(t('data.locale'), { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Lisbon' }).format(d);
    return opp + ' · ' + when.replace(/\./g, '');
  }

  // Miniatura: preview first, then the PNG goes to Transferências.
  function miniatura(btn) {
    var m = $('#minMsg');
    if (!S.state) { m.className = 'msg err'; m.textContent = t('painel.ecras.sem_dados'); return; }
    var st = S.state;
    btn.disabled = true; m.className = 'msg'; m.textContent = '';
    window.Miniatura.render(st).then(function (c) {
      return confirmBox(t('painel.ecras.miniatura'), '<img class="mini-pv" src="' + c.toDataURL('image/png') + '" alt="' + t('painel.ecras.miniatura_alt') + '">' +
        '<p class="hint" style="margin-top:8px">' + esc(window.Miniatura.fileName(st)) + '</p>', { yes: t('painel.descarregar') }).then(function (ok) {
        if (ok) return window.Miniatura.download(c, st);
      });
    }).then(null, function () { m.className = 'msg err'; m.textContent = t('painel.imagem.falhou'); })
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
      el.innerHTML = '<p class="hint">' + t('painel.ecras.sem_plantel') + '</p>';
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
    var tx = (game && game.theme) || {};
    [['--t-casa', (game && game.home && game.home.color) || tx.red], ['--t-fora', game && game.away && game.away.color], ['--t-texto', tx['on-red']]].forEach(function (p) {
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
      px.querySelector('small').textContent = next ? nextLabel(next) : (st.game && st.game.next_note) || t('painel.ecras.sem_proximo');
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
      el.innerHTML = '<div class="onscreen"><b>' + t('painel.ecras.no_ecra_curto') + '</b><span>' + (lt.number ? '#' + esc(lt.number) + ' ' : '') + esc(lt.first_name) + ' ' + esc(lt.last_name) +
        (lt.label ? ' · ' + esc(lt.label) : '') + '</span>' + (left !== null ? '<span class="hint">' + t('painel.segundos', { n: left }) + '</span>' : '') +
        '<span style="flex:1"></span><button class="btn small" data-act="lt-hide">' + t('painel.esconder') + '</button></div>';
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
        if (!S.state) { rm.className = 'msg err'; rm.textContent = t('painel.ecras.sem_dados'); return; }
        res.disabled = true; rm.className = 'msg'; rm.textContent = '';
        window.Resultado.save(S.state).then(null, function () { rm.className = 'msg err'; rm.textContent = t('painel.imagem.falhou'); })
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
      var tx = e.target, cur = (S.state && S.state.screen && S.state.screen.current) || 'marcador';
      tx.disabled = true;
      api('POST', '/api/screen', { screen: cur, auto: tx.checked }).then(null, function (err) {
        tx.checked = !tx.checked; $('#scrMsg').className = 'msg err'; $('#scrMsg').textContent = err.message;
      }).then(function () { tx.disabled = false; });
    });
    $('#ltCom').addEventListener('submit', function (e) {
      e.preventDefault();
      var first = $('#ltFirst').value.trim(), last = $('#ltLast').value.trim();
      if (!first && !last) { $('#ltMsg').className = 'msg err'; $('#ltMsg').textContent = t('painel.ecras.escreva_comentador'); return; }
      ltCall(e.target.querySelector('button'), { type: 'comentador', first_name: first, last_name: last, label: $('#ltLabel').value.trim() });
    });
    $('#vjForm').addEventListener('submit', function (e) {
      e.preventDefault();
      if (!S.settings) return;
      var s = clone(S.settings);
      s.overlay = Object.assign({}, s.overlay, { back_line: $('#vjLine').value.trim() });
      busy(e.target.querySelector('button'), function () {
        return api('PUT', '/api/settings', s).then(function (r) { S.settings = r || s; $('#vjLine').value = S.settings.overlay.back_line || ''; });
      }, t('painel.guardada'));
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
    if (st.demo) html += '<span class="chip"><span class="dot wait"></span>' + t('painel.chip.demo') + '</span>';
    var bt = (box.scan && box.scan.bluetooth) || 'ok';
    if (st.input === 'manual') html += '<span class="chip">' + (box.connected
      ? '<span class="dot on"></span>' + t('painel.chip.manual_ligada')
      : '<span class="dot wait"></span>' + t('painel.chip.manual_espera')) + '</span>';
    else html += '<a class="chip link" href="#caixa" data-goto="caixa">' + (box.connected
      ? '<span class="dot on"></span>' + t('painel.chip.caixa_ligada') + (box.last_frame_at ? ' · ' + esc(ago(box.last_frame_at)) : '')
      : bt !== 'ok'
        ? '<span class="dot off"></span>' + t('painel.chip.caixa_bt_desligado')
        : '<span class="dot wait"></span>' + t('painel.chip.caixa_a_procura') + (box.last_frame_at ? ' · ' + t('painel.chip.ultimo_sinal', { quando: esc(ago(box.last_frame_at)) }) : '')) + '</a>';
    if (obsOn()) {
      var obs = s.obs || {}, h = obs.health || {};
      var oc = !obs.connected ? ['off', t('painel.chip.sem_ligacao'), obs.error || ''] : !h.checked ? ['wait', t('painel.chip.a_verificar'), ''] :
        h.ready ? ['on', t('painel.chip.pronto'), ''] : ['wait', t('painel.chip.reparacao'), (h.problems || []).join(' ')];
      html += '<a class="chip link" href="#definicoes" data-goto="definicoes" title="' + esc(oc[2]) + '"><span class="dot ' + oc[0] + '"></span><b>OBS</b> ' + oc[1] + '</a>';
    }
    var yt = s.youtube;
    if (yt && yt.connected) {
      html += '<a class="chip link" href="#definicoes" data-goto="definicoes"><span class="dot ' + (yt.expired ? 'off' : 'on') + '"></span><b>YouTube</b> ' +
        (yt.expired ? t('painel.chip.expirou') : t('painel.chip.ligado')) + '</a>';
    }
    if (s.fpb) {
      html += '<span class="chip"><span class="dot ' + (s.fpb.reachable ? 'on' : 'wait') + '"></span><b>FPB</b> ' +
        (s.fpb.reachable ? t('painel.chip.disponivel') : t('painel.chip.sem_internet') + (s.fpb.cached_at ? ' · ' + t('painel.chip.dados_de', { data: esc(new Date(s.fpb.cached_at).toLocaleDateString(t('data.locale'))) }) : '')) + '</span>';
    }
    el.innerHTML = html;
  }

  /* ================= OBS E TRANSMISSÃO ================= */
  function obsOn() { return !!(S.settings && S.settings.obs && S.settings.obs.enabled); }
  function hms(ms) {
    var tx = Math.max(0, Math.floor(ms / 1000));
    return Math.floor(tx / 3600) + ':' + pad(Math.floor(tx % 3600 / 60)) + ':' + pad(tx % 60);
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
      dot = 'live'; title = st.reconnecting ? t('painel.stream.a_religar') : test ? t('painel.stream.teste_no_ar') : t('painel.stream.no_ar');
      sub = test ? t('painel.stream.teste_dica') :
        t('painel.stream.no_ar_dica');
      btn = '<button class="btn dark" type="button" data-act="stream-stop">' + (test ? t('painel.stream.terminar_teste') : t('painel.stream.terminar')) + '</button>';
    } else if (!st) {
      dot = 'off'; title = t('painel.stream.sem_obs');
      sub = t('painel.stream.sem_obs_dica');
      btn = '<button class="btn primary" type="button" disabled>' + t('painel.stream.iniciar') + '</button>';
    } else {
      dot = missing.length ? '' : 'ready'; title = missing.length ? t('painel.stream.fora_do_ar') : t('painel.stream.pronto');
      sub = missing.length ? t('painel.stream.antes') :
        yt.connected ? t('painel.stream.pronto_yt') :
        t('painel.stream.pronto_dica');
      btn = (canTest ? '<button class="btn" type="button" data-act="stream-test">' + t('painel.stream.teste') + '</button>' : '') +
        '<button class="btn primary" type="button" data-act="stream-start">' + t('painel.stream.iniciar') + '</button>';
    }
    el.innerHTML = '<span class="dot big ' + dot + '"></span>' +
      '<div class="grow"><h2>' + esc(title) + (live ? ' <span class="air-time" id="airTime"></span>' : '') + '</h2>' +
      '<p class="hint">' + esc(sub) + '</p>' +
      (missing.length ? '<ul class="ready">' + missing.map(function (m) {
        return '<li><span class="dot ' + m[0] + '"></span>' + (m[2] ? '<a href="#' + m[2] + '" data-goto="' + m[2] + '">' + esc(m[1]) + '</a>' : esc(m[1])) + '</li>';
      }).join('') + '</ul>' : '') +
      (ytLive ? '<div class="yt-link"><span>' + t('painel.stream.no_youtube') + '</span><a href="' + esc(ytLive.link) + '" target="_blank" rel="noopener">' + esc(ytLive.link.replace(/^https:\/\//, '')) + '</a>' +
        '<button class="btn small" type="button" data-act="yt-copy" data-url="' + esc(ytLive.link) + '">' + t('painel.copiar') + '</button><span class="msg" role="status"></span></div>' : '') +
      '</div>' +
      '<div class="row">' + (problem ? '<button class="btn" type="button" data-act="obs-repair">' + t('painel.stream.reparar') + '</button>' : '') + btn + '</div>' +
      '<p class="msg" id="streamMsg" role="status"></p>';
    tickStream();
  }
  // What is missing before a start, as [dot, text, tab to fix it]. The
  // buttons stay on: the start is the volunteer's call.
  function readiness(h, problem, expired) {
    var s = S.state || {}, box = s.box || (S.status && S.status.box) || {}, g = S.game || {}, out = [];
    if (!box.connected && !s.demo) out.push(s.input === 'manual' ? ['off', t('painel.stream.falta_marcador'), ''] : ['off', t('painel.stream.falta_caixa'), 'caixa']);
    if (!g.group_id || !(g.away && g.away.name)) out.push(['wait', t('painel.stream.falta_jogo'), 'jogo']);
    if (problem) out.push(['off', t('painel.stream.falta_obs', { problema: problem }), '']);
    else if (!h.checked) out.push(['wait', t('painel.stream.a_verificar_obs'), '']);
    if (expired) out.push(['off', t('painel.stream.yt_expirou'), 'definicoes']);
    return out;
  }
  function tickStream() {
    var tx = hms(airMs()), air = $('#liveAir'), at = $('#airTime');
    var yl = S.ytStream && S.ytStream.live;
    if (!air.hidden) air.textContent = yl && yl.test ? t('painel.stream.topo_teste', { tempo: tx }) : t('painel.stream.topo_no_ar', { tempo: tx });
    if (at) at.textContent = tx;
  }
  function streamCall(btn, path, body) {
    var el = $('#streamBar'), msg = $('#streamMsg');
    el.dataset.busy = '1'; btn.disabled = true;
    msg.className = 'msg'; msg.textContent = path.slice(-5) === 'start' ? t('painel.stream.a_ligar_yt') : t('painel.stream.a_terminar');
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
          confirmBox(t('painel.stream.terminar_pergunta'), '<p>' + t('painel.stream.terminar_texto') + '</p>', { yes: t('painel.terminar'), danger: true })).then(function (yes) {
          // The bar may have been drawn again while the question was open.
          var now = $('#streamBar [data-act="stream-stop"]');
          if (yes && now) streamCall(now, '/api/obs/stream/stop');
        });
      }
      if (b.dataset.act === 'obs-repair') repairOBS();
      if (b.dataset.act === 'yt-copy') busy(b, function () { return copyText(b.dataset.url); }, t('painel.copiado'));
    });
    $('#ytCard').addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]'); if (!b) return;
      if (b.dataset.act === 'yt-copy') busy(b, function () { return copyText(b.dataset.url); }, t('painel.copiado'));
      if (b.dataset.act === 'yt-unlist') {
        busy(b, function () {
          return api('POST', '/api/youtube/unlist', { broadcast_id: b.dataset.id }).then(loadStream);
        }, t('painel.feito'));
      }
    });
  }

  // The thumbnail is the Miniatura YouTube image, drawn here and sent to the
  // program, which hands it to YouTube. The stream goes on if YouTube says no.
  function sendThumbnail(id) {
    if (!S.state || !window.Miniatura) return;
    var msg = $('#streamMsg');
    window.Miniatura.render(S.state).then(function (c) {
      return new Promise(function (resolve, reject) { c.toBlob(function (b) { b ? resolve(b) : reject(new Error(t('painel.imagem.vazia'))); }, 'image/png'); });
    }).then(function (blob) {
      var fd = new FormData();
      fd.append('broadcast_id', id);
      fd.append('file', blob, 'miniatura.png');
      return api('POST', '/api/youtube/thumbnail', fd);
    }).then(function () {
      if (msg) { msg.className = 'msg ok'; msg.textContent = t('painel.stream.miniatura_enviada'); }
    }, function (err) {
      if (msg) { msg.className = 'msg err'; msg.textContent = t('painel.stream.miniatura_falhou', { erro: err.message }); }
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
      html = t('painel.yt.teste_terminado', { link: '<a href="' + esc(e.link) + '" target="_blank" rel="noopener">' + esc(e.link) + '</a>' });
    } else if (e.privacy === 'unlisted') {
      html = t('painel.yt.nao_listado', { link: '<a href="' + esc(e.link) + '" target="_blank" rel="noopener">' + esc(e.link) + '</a>' }) +
        '<div class="row"><button class="btn dark" type="button" data-act="yt-copy" data-url="' + esc(e.link) + '">' + t('painel.copiar') + '</button>' +
        '<a class="btn" href="https://wa.me/?text=' + encodeURIComponent(share) + '" target="_blank" rel="noopener">' + t('painel.yt.whatsapp') + '</a><span class="msg" role="status"></span></div>';
    } else if (e.privacy_error) {
      html = '<h2>' + t('painel.yt.ainda_publico') + '</h2><p class="alert">' + esc(e.privacy_error) + '</p>' +
        '<div class="row" style="margin-top:12px"><button class="btn primary" type="button" data-act="yt-unlist" data-id="' + esc(e.broadcast_id) + '">' + t('painel.tentar_outra_vez') + '</button>' +
        '<button class="btn" type="button" data-act="yt-copy" data-url="' + esc(e.link) + '">' + t('painel.yt.copiar_link') + '</button><span class="msg" role="status"></span></div>';
    } else {
      html = '<h2>' + t('painel.yt.a_terminar') + '</h2><p class="hint">' + t('painel.yt.a_terminar_dica') + '</p>';
    }
    el.innerHTML = html;
  }

  // "Reparar" runs Configurar OBS again where its checklist shows.
  function repairOBS() {
    showTab('definicoes');
    var b = $('[data-act="obs-setup"]');
    if (b) { b.scrollIntoView({ block: 'center' }); runSetup(b, ''); }
  }

  var STEP_TAG = { ok: t('painel.obs.passo.ok'), created: t('painel.obs.passo.criado'), fixed: t('painel.obs.passo.corrigido'), failed: t('painel.obs.passo.falhou'),
    choose: t('painel.obs.passo.escolha'), later: t('painel.obs.passo.depois') };
  function setupHtml(r) {
    if (!r) return '';
    var html = '<ul class="checklist">' + (r.steps || []).map(function (st) {
      return '<li class="ck ck-' + esc(st.result) + '"><span class="ck-tag">' + esc(STEP_TAG[st.result] || st.result) + '</span>' +
        '<div><b>' + esc(st.label) + '</b><span class="hint">' + esc(st.detail) + '</span></div></li>';
    }).join('') + '</ul>';
    if (r.choices && r.choices.length) {
      html += '<fieldset class="picks"><legend>' + t('painel.obs.que_camara') + '</legend>' + r.choices.map(function (c, i) {
        var label = c.type === 'source' ? t('painel.obs.usar_fonte', { fonte: c.label }) : c.type === 'device' ? t('painel.obs.juntar', { camara: c.label }) : c.label;
        return '<label class="pick"><input type="radio" name="obsPick" value="' + esc(c.value) + '"' + (i === 0 ? ' checked' : '') + '>' + esc(label) + '</label>';
      }).join('') + '</fieldset>' +
        '<div class="row"><button class="btn primary" type="button" data-act="obs-pick">' + t('painel.obs.usar_camara') + '</button></div>';
    }
    html += r.ok ? '<p class="msg ok">' + t('painel.obs.pronto') + '</p>' : r.choices && r.choices.length ? '' : '<p class="msg err">' + t('painel.obs.falta_resolver') + '</p>';
    return html;
  }
  // Saves the form first, so the camera picked there is the one set up.
  function runSetup(btn, camera) {
    var msg = $('#obsSetupMsg'), out = $('#obsResult');
    var sel = $('#dObsCam'), pick = sel && /^(device:|plugin$)/.test(sel.value) ? sel.value : '';
    btn.disabled = true; msg.className = 'msg'; msg.textContent = t('painel.obs.a_configurar');
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
        '<div class="card" id="cxInput"></div>' +
        '<div class="card" id="cxStatus"></div>' +
        '<div id="cxBanner"></div>' +
        '<p class="warn">' + t('painel.caixa.feche_app') + '</p>' +
        '<div class="card"><h2>' + t('painel.caixa.dispositivos') + ' <span class="count" id="cxCount"></span></h2>' +
        '<p class="hint">' + t('painel.caixa.dispositivos_dica') + '</p>' +
        '<ul class="devs" id="cxList"></ul></div>';
      caixaBuilt = true;
      $('#cxInput').addEventListener('change', function (e) {
        if (e.target.name !== 'cxIn' || !S.settings) return;
        var s = Object.assign({}, S.settings, { input: e.target.value }), msg = $('#cxInMsg');
        msg.className = 'msg'; msg.textContent = '';
        api('PUT', '/api/settings', s).then(function (got) {
          S.settings = got; msg.className = 'msg ok'; msg.textContent = t('painel.marcar.guardado');
        }, function (err) { msg.className = 'msg err'; msg.textContent = err.message; renderCaixaInput(); });
      });
    }
    renderCaixaInput();
    renderCaixaStatus();
    renderDevices();
  }

  // Where the score comes from: the console's Stream Box, or a phone on /marcar (with its QR code).
  function renderCaixaInput() {
    var el = $('#cxInput'); if (!el || !S.settings) return;
    var s = S.settings, manual = s.input === 'manual';
    var lan = S.status && S.status.lan_url ? S.status.lan_url.replace(/\/painel$/, '/marcar') : '';
    var opt = function (v, on, label, hint) {
      return '<label><input type="radio" name="cxIn" value="' + v + '"' + (on ? ' checked' : '') + '><span><b>' + label + '</b><small>' + hint + '</small></span></label>';
    };
    el.innerHTML = '<h2>' + t('painel.marcar.titulo') + '</h2>' +
      '<div class="cx-in">' +
        opt('console', !manual, t('painel.marcar.consola'), t('painel.marcar.consola_dica')) +
        opt('manual', manual, t('painel.marcar.telemovel'), t('painel.marcar.telemovel_dica')) +
      '</div><span class="msg" id="cxInMsg" role="status"></span>' +
      (lan
        ? '<div class="cx-qr"><img src="/marcar/qr.svg" width="148" height="148" alt="' + t('painel.marcar.qr_alt') + '">' +
          '<p>' + t('painel.marcar.abra', { url: '<code>' + esc(lan) + '</code>' }) + '</p></div>'
        : '<p class="hint">' + t('painel.marcar.sem_telemovel') + '</p>');
  }

  function scanState() {
    var st = S.state || {}, box = st.box || {};
    // With manual scoring box.connected is the phone; the box is linked when the reader has an address.
    if (st.input === 'manual') box = Object.assign({}, box, { connected: !!(box.scan && box.scan.address) });
    return { box: box, scan: box.scan || {}, demo: !!st.demo };
  }

  function renderCaixaStatus() {
    var el = $('#cxStatus'); if (!el) return;
    var c = scanState(), sc = c.scan, bt = sc.bluetooth || 'ok';
    var dot, title;
    if (c.demo) { dot = 'wait'; title = t('painel.caixa.demo'); }
    else if (bt !== 'ok') { dot = 'off'; title = t('painel.caixa.bt_desligado'); }
    else if (c.box.connected) { dot = 'on'; title = t('painel.caixa.ligado_a', { nome: sc.name || t('painel.caixa.caixa') }); }
    else { dot = 'wait'; title = t('painel.caixa.a_procura'); }
    var noDevices = !c.box.connected && S.devices !== null && !S.devices.length;
    var want = c.demo ? '' : bt !== 'ok' ? 'off' : noDevices ? 'none' : '';
    var html = '<div class="cx-status"><span class="dot big ' + dot + '"></span><div class="grow">' +
      '<h2>' + esc(title) + (c.box.connected && sc.address ? ' <code>' + esc(sc.address) + '</code>' : '') + '</h2>' +
      (sc.message && !want ? '<p class="cx-msg">' + esc(sc.message) + '</p>' : '') +
      (sc.saved && !c.box.connected ? '<p class="hint">' + t('painel.caixa.guardada', { endereco: '<code>' + esc(sc.saved) + '</code>' }) + '</p>' : '') +
      '</div></div>' +
      (sc.saved ? '<div class="row" style="margin-top:12px;align-items:center"><button class="btn" data-act="cx-forget">' + t('painel.caixa.esquecer') + '</button>' +
        '<span class="hint grow">' + t('painel.caixa.esquecer_dica') + '</span><span class="msg"></span></div>' : '');
    // SSE can fire many times a second: only touch the DOM when it changes, so button messages stay.
    if (el.dataset.h !== html) { el.dataset.h = html; el.innerHTML = html; }
    var banner = $('#cxBanner');
    if (banner.dataset.kind === want) return;
    banner.dataset.kind = want;
    banner.innerHTML = !want ? '' :
      '<div class="alert cx-banner"><p>' + (want === 'off' ? t('ble.desligado') : t('ble.nenhum_dispositivo')) + '</p>' +
      '<div class="row"><button class="btn dark" data-act="cx-settings">' + t('painel.caixa.abrir_definicoes_bt') + '</button><span class="msg"></span></div></div>';
  }

  function bars(rssi) {
    var n = rssi >= -60 ? 4 : rssi >= -70 ? 3 : rssi >= -80 ? 2 : rssi >= -90 ? 1 : 0;
    var word = [t('painel.caixa.sinal.0'), t('painel.caixa.sinal.1'), t('painel.caixa.sinal.2'), t('painel.caixa.sinal.3'), t('painel.caixa.sinal.4')][n];
    var h = '<span class="bars" role="img" aria-label="' + t('painel.caixa.sinal', { forca: word, rssi: rssi }) + '">';
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
    if (!list) { el.innerHTML = '<li class="hint">' + t('painel.a_carregar') + '</li>'; return; }
    if (!list.length) { el.innerHTML = '<li class="hint">' + t('painel.caixa.nenhum_ouvido') + '</li>'; return; }
    el.innerHTML = list.map(function (d) {
      var tx = S.tries[d.address] || {}, isBox = c.box.connected && c.scan.address === d.address;
      return '<li class="dev' + (d.likely ? ' likely' : '') + '">' +
        '<div class="who">' +
          '<b class="name">' + (d.name ? esc(d.name_quoted || d.name) : '<i>' + t('painel.caixa.sem_nome') + '</i>') + '</b>' +
          (d.likely ? ' <span class="badge gold">' + t('painel.caixa.provavel') + '</span>' : '') +
          '<small><code>' + esc(d.address) + '</code>' + (d.services && d.services.length ? ' · ' + t('painel.caixa.servicos', { lista: esc(d.services.join(', ')) }) : '') + '</small>' +
        '</div>' +
        '<div class="sig">' + bars(d.rssi) + '<small>' + esc(d.rssi) + ' dBm</small></div>' +
        '<div class="act">' + (isBox ? '<span class="in-use"><span class="dot on"></span>' + t('painel.caixa.ligada') + '</span>' :
          '<button class="btn' + (d.likely ? ' primary' : '') + '" data-try="' + esc(d.address) + '"' + (tx.busy ? ' disabled' : '') + '>' +
            (tx.busy ? '<span class="spin" aria-hidden="true"></span>' + t('painel.caixa.a_ligar') : t('painel.caixa.experimentar')) + '</button>') +
        '</div>' +
        (tx.msg ? '<p class="msg ' + (tx.ok ? 'ok' : 'err') + ' dev-msg" role="status">' + esc(tx.msg) + '</p>' : '') +
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
          S.tries[addr] = r && r.ok ? { ok: true, msg: t('painel.caixa.e_a_caixa') } : { msg: (r && r.error) || t('painel.caixa.nao_liguei') };
        }, function (err) { S.tries[addr] = { msg: err.message }; }).then(renderDevices);
        return;
      }
      var b = e.target.closest('[data-act]'); if (!b) return;
      if (b.dataset.act === 'cx-settings') {
        busy(b, function () { return api('POST', '/api/system/bluetooth-settings'); }, t('painel.caixa.abri_definicoes'));
      } else if (b.dataset.act === 'cx-forget') {
        confirmBox(t('painel.caixa.esquecer'), t('painel.caixa.esquecer_texto'), { yes: t('painel.esquecer') }).then(function (yes) {
          if (yes) busy(b, function () { S.tries = {}; return api('POST', '/api/ble/forget'); }, t('painel.caixa.esquecida'));
        });
      }
    });
  }

  /* ================= 3. PLANTEL ================= */
  function groupSelect(id, cur, withGeral) {
    var opts = (withGeral ? '<option value="geral"' + (cur === 'geral' ? ' selected' : '') + '>' + t('painel.geral_todos') + '</option>' : '') +
      S.groups.map(function (g) { return '<option value="' + esc(g.id) + '"' + (g.id === cur ? ' selected' : '') + '>' + esc(g.name) + '</option>'; }).join('');
    return '<select id="' + id + '">' + opts + '</select>';
  }

  function renderPlantel() {
    var root = $('#plantel');
    if (!S.plantelGid || !groupById(S.plantelGid)) S.plantelGid = (S.draft && S.draft.group_id) || (S.groups[0] && S.groups[0].id) || null;
    var gid = S.plantelGid, g = groupById(gid);
    var html = '<div class="card"><div class="row">' +
      (S.groups.length ? '<label class="field grow"><span>' + t('painel.jogo.escalao') + '</span>' + groupSelect('pGroup', gid) + '</label>' : '<p class="grow">' + t('painel.plantel.sem_escaloes') + '</p>') +
      '<form class="row" id="pNewGroup"><label class="field"><span>' + t('painel.plantel.novo_escalao') + '</span><input id="pNewGroupName" placeholder="' + t('painel.plantel.novo_escalao_ex') + '"></label><button class="btn" type="submit">' + t('painel.atalho.criar') + '</button><span class="msg"></span></form>' +
      '</div></div>';
    var fpb = fpbOffered();
    if (fpb && !S.groups.some(function (x) { return x.fpb_team_id; })) {
      var c = S.status && S.status.club_setup, run = c && c.running;
      html += '<div class="card"><h2>' + t('painel.plantel.equipas_clube') + '</h2><p class="hint">' + t('painel.plantel.equipas_clube_dica') + '</p>' +
        '<div class="row" style="margin-top:12px"><button class="btn dark" data-act="club-setup"' + (run ? ' disabled' : '') + '>' + t('painel.plantel.importar_equipas') + '</button>' +
        '<span class="msg" id="clubMsg">' + (run ? esc(clubProgress(c)) : '') + '</span></div></div>';
    }
    if (!g) { root.innerHTML = html; return; }

    html += '<div class="cols">';
    html += '<div class="card"><h2>' + t('painel.jogo.escalao') + '</h2><div class="stack">' +
      '<label class="field"><span>' + t('painel.nome') + '</span><input id="pGName" value="' + esc(g.name) + '"></label>' +
      '<label class="field"><span>' + t('painel.plantel.treinador') + '</span><input id="pCoach" value="' + esc(g.coach || '') + '" placeholder="' + t('painel.plantel.treinador_ex') + '"></label>' +
      '<div class="row"><button class="btn primary" data-act="save-group">' + t('painel.guardar') + '</button><span class="msg"></span><span style="flex:1"></span>' +
      '<button class="btn quiet" data-act="del-group">' + t('painel.plantel.apagar_escalao') + '</button></div></div></div>';

    if (fpb) {
      html += '<div class="card"><h2>FPB</h2>';
      if (g.fpb_team_id) {
        html += '<p>' + t('painel.plantel.ligado_a', { equipa: '<b>' + esc(g.fpb_team_name || t('painel.plantel.equipa_n', { id: g.fpb_team_id })) + '</b>' }) + '</p>' +
          (g.needs_team ? '<p class="warn" style="margin:8px 0">' + t('painel.plantel.epoca_nova') + '</p>' : '') +
          (g.last_sync ? '<p class="hint" style="margin:8px 0">' + t('painel.plantel.atualizado', { quando: esc(since(g.last_sync)), mudancas: esc(syncSummary(g.last_changes)) }) + '</p>' : '') +
          '<div class="row"><button class="btn dark" data-act="import">' + t('painel.plantel.importar_fpb') + '</button><span class="msg"></span>' +
          '<button class="btn quiet" data-act="fpb-change">' + (g.needs_team ? t('painel.plantel.escolher_equipa') : t('painel.plantel.mudar_equipa')) + '</button></div>' +
          '<p class="hint" style="margin-top:8px">' + t('painel.plantel.importar_dica') + '</p>';
      }
      if (!g.fpb_team_id || S.fpbTeams) {
        html += (g.fpb_team_id ? '<hr style="border:0;border-top:1px solid var(--line-soft);margin:16px 0">' : '<p class="hint">' + t('painel.plantel.ligue_fpb') + '</p>') +
          '<form class="row" id="pFpbSearch"><label class="field grow"><span>' + t('painel.plantel.procurar_equipa') + '</span><input type="search" id="pFpbName" value="Casa"></label>' +
          '<button class="btn dark" type="submit">' + t('painel.procurar') + '</button><span class="msg"></span></form>';
        if (S.fpbTeams) {
          html += '<div class="stack" style="margin-top:12px">' + (S.fpbTeams.length ? S.fpbTeams.map(function (tx, i) {
            return '<button class="team-pick" data-act="fpb-pick" data-i="' + i + '"><span><b>' + esc(tx.name) + '</b><br><small>' +
              esc([tx.age, genderLabel(tx.gender), tx.squad_label].filter(Boolean).join(' · ')) + '</small></span><span class="btn small dark" aria-hidden="true">' + t('painel.escolher') + '</span></button>';
          }).join('') : '<p class="hint">' + t('painel.plantel.nenhuma_equipa') + '</p>') + '</div>';
        }
      }
      html += '</div>';
    }
    html += '</div>';

    if (S.importResult && S.importResult.gid === gid) html += renderImport();

    var all = S.players[gid], ph = (S.settings && S.settings.photos) || {};
    var list = all && all.filter(function (p) { return !p.inactive; });
    var gone = all ? all.filter(function (p) { return p.inactive; }) : [];
    html += '<div class="card"><h2>' + t('painel.plantel.jogadores') + (list ? ' <span class="count">' + list.length + '</span>' : '') + '</h2>';
    html += '<div class="row photo-sw"><label class="switch"><input type="checkbox" data-photo="squad"' + (ph.squad ? ' checked' : '') + '><i></i>' + t('painel.plantel.fotos_plantel') + '</label>' +
      '<label class="switch"><input type="checkbox" data-photo="lower_third"' + (ph.lower_third ? ' checked' : '') + '><i></i>' + t('painel.plantel.foto_rodape') + '</label><span class="msg"></span></div>' +
      '<p class="hint" style="margin-bottom:12px">' + (fpb ? t('painel.plantel.fotos_dica') : t('painel.plantel.fotos_dica_sem_fpb')) + '</p>';
    if (!list) html += '<p class="hint">' + t('painel.a_carregar') + '</p>';
    else {
      html += '<table class="players"><thead><tr><th>' + t('painel.plantel.numero_curto') + '</th><th>' + t('painel.nome') + '</th><th></th></tr></thead><tbody>' + list.map(function (p) {
        var differs = p.fpb_name && p.fpb_name.toLowerCase() !== String(p.display_name).toLowerCase();
        return '<tr data-pid="' + esc(p.id) + '">' +
          '<td class="n"><input type="text" inputmode="numeric" maxlength="2" data-f="number" value="' + esc(p.number) + '" aria-label="' + t('painel.plantel.numero') + '"></td>' +
          '<td><div class="pw">' + thumb(p.photo_url, p.display_name) + '<div><input data-f="display_name" value="' + esc(p.display_name) + '" aria-label="' + t('painel.nome') + '">' +
            (p.fpb_player_id ? '<div class="fpb"><span class="badge">FPB</span>' + (differs ? '<span>' + esc(p.fpb_name) + '</span>' : '') + '</div>' : '') + '</div></div></td>' +
          '<td class="act"><span class="msg"></span> ' +
            '<label class="btn quiet small filebtn">' + t('painel.plantel.foto') + '<input type="file" data-photo-file accept="image/png,image/jpeg"></label>' +
            (p.photo_by_hand ? '<button class="btn quiet small" data-act="photo-remove">' + t('painel.plantel.tirar_foto') + '</button>' : '') +
            (p.fpb_player_id ? '<button class="btn quiet small" data-act="unlink">' + t('painel.desligar') + '</button>' : '') +
            '<button class="btn danger small" data-act="del-player">' + t('painel.apagar') + '</button></td></tr>';
      }).join('') + '</tbody></table>';
      if (!list.length) html += '<p class="hint">' + (g.fpb_team_id ? t('painel.plantel.sem_jogadores_fpb') : t('painel.plantel.sem_jogadores')) + '</p>';
      if (gone.length) {
        html += '<details style="margin-top:16px"><summary>' + t('painel.plantel.ja_nao_estao') + ' <span class="count">' + gone.length + '</span></summary>' +
          '<p class="hint" style="margin:8px 0">' + t('painel.plantel.ja_nao_estao_dica') + '</p>' +
          gone.map(function (p) {
            return '<div class="row" data-pid="' + esc(p.id) + '" style="margin-top:8px"><span class="grow">' + esc((p.number ? p.number + ' · ' : '') + p.display_name) + '</span>' +
              '<button class="btn small" data-act="unhide">' + t('painel.plantel.voltar_mostrar') + '</button><span class="msg"></span></div>';
          }).join('') + '</details>';
      }
    }
    html += '<form class="row" id="pAdd" style="margin-top:16px;padding-top:16px;border-top:1px solid var(--line-soft)">' +
      '<label class="field" style="width:84px"><span>' + t('painel.plantel.numero_curto') + '</span><input id="pAddNum" inputmode="numeric" maxlength="2"></label>' +
      '<label class="field grow"><span>' + t('painel.plantel.nome_jogador') + '</span><input id="pAddName" autocomplete="off"></label>' +
      '<button class="btn primary" type="submit">' + t('painel.plantel.adicionar') + '</button><span class="msg"></span></form>';
    html += csvRow('pCsv', 'plantel.csv', t('painel.csv.modelo_plantel'), t('painel.csv.dica_plantel')) + csvResult('players', gid);
    html += '</div>';
    root.innerHTML = html;
    if (!list) loadPlayers(gid).then(function () { if (S.tab === 'plantel') renderPlantel(); }, showError);
  }

  function renderImport() {
    var r = S.importResult, list = S.players[r.gid] || [];
    var html = '<div class="card"><h2>' + t('painel.importar.titulo') + '</h2>';
    var nu = r.updated.length;
    html += '<p>' + (nu ? plural(nu, t('painel.importar.atualizados.um', { n: nu }), t('painel.importar.atualizados.outros', { n: nu })) : t('painel.importar.nenhum')) + '</p>';
    if (r.photoErrors) html += '<p class="hint">' + plural(r.photoErrors, t('painel.importar.fotos_falharam.um', { n: r.photoErrors }), t('painel.importar.fotos_falharam.outros', { n: r.photoErrors })) + '</p>';
    if (!r.suggestions.length) return html + '<p class="hint">' + t('painel.importar.sem_novos') + '</p></div>';
    html += '<p class="hint">' + t('painel.importar.sugestoes') + '</p>';
    html += r.suggestions.map(function (s, i) {
      var linking = S.linking[s.fpb_player_id];
      var opts = list.filter(function (p) { return !p.fpb_player_id; });
      var best = linking ? linking.choice : s.best_match;
      var bestP = list.filter(function (p) { return String(p.id) === String(s.best_match); })[0];
      return '<div class="sugg" data-si="' + i + '"><div class="pw">' + thumb(s.photo_url, s.name) + '<div class="who"><b>' + esc(s.number || '–') + '</b>' + esc(s.name) +
        '<small>' + t('painel.importar.jogos', { n: esc(s.games) }) + (bestP ? ' · ' + t('painel.importar.parece', { nome: '<b>' + esc(bestP.display_name) + '</b>' }) : '') + '</small></div></div>' +
        '<div class="row"><button class="btn small" data-act="sug-add">' + t('painel.adicionar') + '</button>' +
        '<button class="btn small dark" data-act="sug-link"' + (opts.length ? '' : ' disabled') + '>' + t('painel.importar.ligar_a') + '</button><span class="msg"></span></div>' +
        (linking ? '<div class="linkbox"><label class="field"><span>' + t('painel.importar.ligar_jogador') + '</span><select data-link-choice>' +
          '<option value="">' + t('painel.escolha') + '</option>' + opts.map(function (p) {
            return '<option value="' + esc(p.id) + '"' + (String(p.id) === String(best) ? ' selected' : '') + '>' + esc((p.number ? p.number + ' · ' : '') + p.display_name) + '</option>';
          }).join('') + '</select></label><button class="btn small primary" data-act="sug-confirm">' + t('painel.confirmar') + '</button>' +
          '<button class="btn small quiet" data-act="sug-cancel">' + t('painel.cancelar') + '</button><span class="msg"></span></div>' : '') +
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
      var tx = e.target;
      if (tx.id === 'pGroup') { S.plantelGid = tx.value; S.fpbTeams = null; renderPlantel(); return; }
      if (tx.dataset.photo) {
        var s = clone(S.settings), pm = tx.closest('.row').querySelector('.msg');
        s.photos = Object.assign({ squad: false, lower_third: false }, s.photos);
        s.photos[tx.dataset.photo] = tx.checked;
        tx.disabled = true; pm.className = 'msg'; pm.textContent = '';
        api('PUT', '/api/settings', s).then(function (r) {
          S.settings = r || s; pm.className = 'msg ok'; pm.textContent = t('painel.guardado');
        }, function (err) { tx.checked = !tx.checked; pm.className = 'msg err'; pm.textContent = err.message; })
          .then(function () { tx.disabled = false; });
        return;
      }
      if (tx.hasAttribute('data-link-choice')) {
        var si = +tx.closest('[data-si]').dataset.si;
        S.linking[S.importResult.suggestions[si].fpb_player_id].choice = tx.value;
        return;
      }
      var tr = tx.closest('tr[data-pid]');
      if (tr && tx.dataset.f) {
        var pid = tr.dataset.pid;
        var body = { display_name: $('[data-f="display_name"]', tr).value.trim(), number: $('[data-f="number"]', tr).value.replace(/\D/g, '') };
        if (!body.display_name) { var m0 = $('.msg', tr); m0.className = 'msg err'; m0.textContent = t('api.nome_vazio'); return; }
        var msg = $('.msg', tr);
        msg.className = 'msg'; msg.textContent = t('painel.a_guardar');
        api('PUT', base() + '/players/' + encodeURIComponent(pid), body).then(function (r) {
          var list = S.players[gid()] || [];
          list.forEach(function (p, i) { if (String(p.id) === pid) list[i] = r || Object.assign(p, body); });
          msg.className = 'msg ok'; msg.textContent = t('painel.guardado');
          setTimeout(function () { if (msg.textContent === t('painel.guardado')) msg.textContent = ''; }, 2500);
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
        }, t('painel.criado'));
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
        }, t('painel.adicionado'));
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
            var nj = plural(r.imported_players, t('painel.clube.jogadores.um', { n: r.imported_players }), t('painel.clube.jogadores.outros', { n: r.imported_players }));
            confirmBox(t('painel.plantel.equipas_clube'), '<p>' + (r.created
              ? t('painel.clube.criados', { escaloes: plural(r.created, t('painel.clube.escaloes.um', { n: r.created }), t('painel.clube.escaloes.outros', { n: r.created })), jogadores: nj })
              : t('painel.clube.nada_novo', { jogadores: nj })) + '</p>' +
              (r.errors && r.errors.length ? '<p class="hint">' + t('painel.clube.nao_correu_bem') + '</p><ul>' + r.errors.map(function (e) { return '<li>' + esc(e) + '</li>'; }).join('') + '</ul>' : '') +
              '<p class="hint">' + t('painel.clube.confira') + '</p>', { yes: t('painel.ok'), noCancel: true });
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
        confirmBox(t('painel.plantel.apagar_escalao_pergunta', { nome: g.name }), '<p>' + t('painel.plantel.apagar_escalao_texto') + '</p>', { yes: t('painel.apagar'), danger: true }).then(function (ok) {
          if (!ok) return;
          api('DELETE', base()).then(function () { S.plantelGid = null; return loadGroups(); }).then(renderPlantel, showError);
        });
      } else if (act === 'fpb-change') {
        S.fpbTeams = S.fpbTeams ? null : []; renderPlantel();
      } else if (act === 'fpb-pick') {
        var tx = S.fpbTeams[+b.dataset.i];
        busy(b, function () {
          return api('PUT', base(), { fpb_team_id: tx.id, fpb_team_name: tx.name + (tx.age ? ' · ' + tx.age : ''), fpb_team: tx })
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
        }, t('painel.importado'));
      } else if (act === 'unlink' || act === 'del-player') {
        var tr = b.closest('tr'), pid = tr.dataset.pid;
        var p = (S.players[gid()] || []).filter(function (x) { return String(x.id) === pid; })[0];
        if (act === 'unlink') {
          busy(b, function () { return api('DELETE', base() + '/players/' + encodeURIComponent(pid) + '/link').then(refresh); });
        } else {
          confirmBox(t('painel.apagar_pergunta', { nome: p.display_name }), '<p>' + t('painel.plantel.apagar_jogador_texto') + '</p>', { yes: t('painel.apagar'), danger: true }).then(function (ok) {
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
          if (!choice) { m.className = 'msg err'; m.textContent = t('painel.importar.escolha_jogador'); return; }
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
    var html = '<div class="card"><label class="field" style="max-width:420px"><span>' + t('painel.patrocinadores.de_quem') + '</span>' + groupSelect('spGroup', gid, true) + '</label></div>';
    html += '<div class="card"><h2>' + t('painel.patrocinadores.logotipos_de', { nome: esc(groupName(gid)) }) + (list ? ' <span class="count">' + list.length + '</span>' : '') + '</h2>';
    if (!list) html += '<p class="hint">' + t('painel.a_carregar') + '</p>';
    else if (!list.length) html += '<p class="hint">' + t('painel.patrocinadores.vazio') + '</p>';
    else {
      html += '<p class="hint" style="margin-bottom:6px">' + t('painel.patrocinadores.ordem') + '</p><ul class="spons">' + list.map(function (s, i) {
        return '<li data-sid="' + esc(s.id) + '"><span class="logo">' + (s.url ? '<img src="' + esc(s.url) + '" alt="">' : '') + '</span>' +
          '<span><b>' + esc(s.name) + '</b><br><label class="princ"><input type="radio" name="princ" data-act="princ"' + (s.principal ? ' checked' : '') + '>' + t('painel.patrocinadores.principal') + '</label></span>' +
          '<span class="acts"><button class="btn icon small" data-act="up" aria-label="' + t('painel.subir') + '"' + (i === 0 ? ' disabled' : '') + '>↑</button>' +
          '<button class="btn icon small" data-act="down" aria-label="' + t('painel.descer') + '"' + (i === list.length - 1 ? ' disabled' : '') + '>↓</button>' +
          '<button class="btn danger small" data-act="del">' + t('painel.apagar') + '</button></span></li>';
      }).join('') + '</ul><p class="msg" id="spMsg"></p>';
    }
    html += '</div>';
    html += '<div class="card"><h2>' + t('painel.patrocinadores.adicionar') + '</h2><form class="row" id="spAdd">' +
      '<label class="field grow"><span>' + t('painel.patrocinadores.nome') + '</span><input id="spName" autocomplete="off"></label>' +
      '<div class="grow"><span class="label">' + t('painel.patrocinadores.ficheiro') + '</span><div class="row" style="align-items:center">' +
        '<label class="btn filebtn">' + t('painel.escolher_ficheiro') + '<input type="file" id="spFile" accept="image/png,image/jpeg,image/svg+xml"></label>' +
        '<span class="filename" id="spFileName">' + t('painel.nenhum_escolhido') + '</span></div></div>' +
      '<button class="btn primary" type="submit">' + t('painel.adicionar') + '</button><span class="msg"></span></form></div>';
    root.innerHTML = html;
    if (!list) loadSponsors(gid).then(function () { if (S.tab === 'patrocinadores') renderSponsors(); }, showError);
  }

  function sponsorEvents() {
    var root = $('#patrocinadores');
    function base() { return '/api/groups/' + encodeURIComponent(S.sponsorGid) + '/sponsors'; }
    function saveOrder(list) {
      var msg = $('#spMsg');
      $all('#patrocinadores .spons button, #patrocinadores .spons input').forEach(function (x) { x.disabled = true; });
      msg.className = 'msg'; msg.textContent = t('painel.a_guardar');
      return api('PUT', base(), list.map(function (s) { return { id: s.id, principal: !!s.principal }; })).then(function (r) {
        S.sponsors[S.sponsorGid] = Array.isArray(r) ? r : list;
        renderSponsors();
        var m = $('#spMsg'); if (m) { m.className = 'msg ok'; m.textContent = t('painel.guardado'); }
      }, function (err) {
        renderSponsors();
        var m = $('#spMsg'); if (m) { m.className = 'msg err'; m.textContent = err.message; }
      });
    }
    root.addEventListener('change', function (e) {
      var tx = e.target;
      if (tx.id === 'spGroup') { S.sponsorGid = tx.value; renderSponsors(); return; }
      if (tx.id === 'spFile') { $('#spFileName').textContent = tx.files[0] ? tx.files[0].name : t('painel.nenhum_escolhido'); return; }
      if (tx.dataset.act === 'princ') {
        var sid = tx.closest('li').dataset.sid;
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
        confirmBox(t('painel.apagar_pergunta', { nome: list[i].name }), '<p>' + t('painel.patrocinadores.apagar_texto') + '</p>', { yes: t('painel.apagar'), danger: true }).then(function (ok) {
          if (!ok) return;
          api('DELETE', base() + '/' + encodeURIComponent(sid)).then(function () { return loadSponsors(S.sponsorGid); }).then(renderSponsors, showError);
        });
      }
    });
    root.addEventListener('submit', function (e) {
      e.preventDefault();
      var file = $('#spFile').files[0], name = $('#spName').value.trim();
      var btn = e.target.querySelector('button[type="submit"]'), msg = btn.parentElement.querySelector('.msg');
      if (!file) { msg.className = 'msg err'; msg.textContent = t('painel.patrocinadores.escolha_ficheiro'); return; }
      if (!/\.(png|jpe?g|svg)$/i.test(file.name)) { msg.className = 'msg err'; msg.textContent = t('painel.patrocinadores.tipo'); return; }
      var fd = new FormData();
      fd.append('file', file);
      fd.append('name', name || file.name.replace(/\.[^.]+$/, ''));
      busy(btn, function () {
        return api('POST', base(), fd).then(function () { return loadSponsors(S.sponsorGid); }).then(renderSponsors);
      }, t('painel.adicionado'));
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
    if (!p) { root.innerHTML = '<p class="hint">' + t('painel.a_carregar') + '</p>'; loadPromo().then(renderPromo, showError); return; }
    var c = clipById(S.promoSel);
    var html = '<div id="prAir"></div><div class="promo-grid"><div class="card"><h2>' + t('painel.promo.videos') + ' <span class="count">' + p.clips.length + '</span></h2>';
    if (!p.clips.length) html += '<p class="hint">' + t('painel.promo.vazio') + '</p>';
    html += '<ul class="clips">' + p.clips.map(function (x) {
      var o = x.orientation === 'horizontal' ? t('painel.promo.horizontal') : x.orientation === 'vertical' ? t('painel.promo.vertical') : '';
      return '<li class="clip' + (x.id === S.promoSel ? ' sel' : '') + '" data-clip="' + esc(x.id) + '">' +
        '<button class="pick" type="button" data-act="pr-pick" aria-pressed="' + (x.id === S.promoSel) + '">' +
        '<span class="vthumb"><video muted preload="metadata" src="/promo/' + esc(x.file) + '#t=1"></video></span>' +
        '<span class="who"><b>' + esc(x.name) + '</b><small>' + (x.duration ? mmss(x.duration) : '–:––') + (o ? ' · <span class="badge">' + o + '</span>' : '') + '</small></span></button>' +
        '<button class="btn icon" type="button" data-act="pr-show" aria-label="' + t('painel.promo.mostrar_nome', { nome: esc(x.name) }) + '" title="' + t('painel.promo.mostrar') + '">▶</button></li>';
    }).join('') + '</ul>';
    html += '<div class="row" style="margin-top:14px;align-items:center">' +
      '<label class="btn dark filebtn">' + t('painel.promo.carregar') + '<input type="file" id="prFile" accept="video/mp4,.mp4"></label>' +
      '<span class="hint">' + t('painel.promo.limite') + '</span><span class="msg" id="prUp" role="status"></span></div></div>';

    html += '<div>';
    if (!c) {
      html += '<div class="card"><p class="hint">' + t('painel.promo.escolha') + '</p></div>';
    } else {
      var vert = c.orientation === 'vertical';
      html += '<form class="card" id="prForm" autocomplete="off"><h2>' + t('painel.promo.texto') + '</h2><div class="stack">' +
        '<label class="field"><span>' + t('painel.promo.nome_lista') + '</span><input id="prName" maxlength="60" value="' + esc(c.name) + '"></label>' +
        '<div class="cols2"><label class="field"><span>' + t('painel.promo.etiqueta') + '</span><input id="prKicker" maxlength="40" value="' + esc(c.kicker) + '"></label>' +
        '<label class="field"><span>' + t('painel.promo.titulo') + '</span><input id="prHeadline" maxlength="60" value="' + esc(c.headline) + '"></label></div>' +
        '<label class="field"><span>' + t('painel.promo.linha') + '</span><input id="prLine" maxlength="120" value="' + esc(c.line) + '"></label>' +
        '<div class="row" style="align-items:flex-end"><label class="field grow"><span>' + t('painel.promo.link_qr') + '</span><input type="url" id="prLink" maxlength="300" placeholder="https://…" value="' + esc(c.link) + '"></label>' +
        '<label class="switch"><input type="checkbox" id="prQR"' + (c.show_qr ? ' checked' : '') + (c.link ? '' : ' disabled') + '><i></i>' + t('painel.promo.mostrar_qr') + '</label></div>' +
        '</div></form>' +
        '<div class="card" id="prPlay"><h2>' + t('painel.promo.reproducao') + '</h2><div class="stack">' +
        '<div class="row" style="align-items:center"><label class="switch"><input type="checkbox" id="prSound"' + (c.sound ? ' checked' : '') + '><i></i>' + t('painel.promo.som') + '</label>' +
        '<input type="range" id="prVol" min="0" max="100" step="5" value="' + esc(c.volume) + '" aria-label="' + t('painel.promo.volume') + '" class="grow"' + (c.sound ? '' : ' disabled') + '><b class="vol" id="prVolTxt">' + esc(c.volume) + '%</b></div>' +
        '<p class="hint">' + t('painel.promo.som_dica') + '</p>' +
        '<div><span class="label">' + t('painel.promo.no_fim') + '</span><div class="seg" role="group" aria-label="' + t('painel.promo.no_fim') + '">' +
          '<button type="button" data-end="back" aria-pressed="' + (c.end !== 'loop') + '">' + t('painel.promo.voltar') + '</button>' +
          '<button type="button" data-end="loop" aria-pressed="' + (c.end === 'loop') + '">' + t('painel.promo.repetir') + '</button></div></div>' +
        '<label class="switch"><input type="checkbox" id="prMini"' + (p.timeout_clip === c.id ? ' checked' : '') + (vert ? '' : ' disabled') + '><i></i>' + t('painel.promo.pequeno') + '</label>' +
        (vert ? '' : '<p class="hint" style="margin-top:-6px">' + t('painel.promo.so_vertical') + '</p>') +
        '<p class="msg" id="prMsg" role="status"></p></div></div>' +
        '<div class="row"><button class="btn primary big grow" type="button" data-act="pr-show" data-id="' + esc(c.id) + '">' + t('painel.promo.mostrar_botao', { nome: esc(c.name) }) + '</button></div>' +
        '<div class="row" style="margin-top:10px"><button class="btn danger small" type="button" data-act="pr-del">' + t('painel.promo.apagar') + '</button></div>';
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
    el.innerHTML = air ? '<div class="onscreen"><b>' + t('painel.stream.no_ar') + '</b><span>' + esc(ac ? ac.name : t('promo.nome_omissao')) + '</span><span style="flex:1"></span>' +
      '<button class="btn small" type="button" data-act="pr-stop">' + t('painel.promo.parar') + '</button></div>' : '';
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
      msg('', t('painel.a_guardar'));
      return api('PUT', '/api/promo/' + encodeURIComponent(c.id), body).then(function (r) {
        keepClip(r);
        // Show what the program kept (a link gets https://), without losing the field being typed in.
        var link = $('#prLink'); if (link && document.activeElement !== link) link.value = r.link;
        var qr = $('#prQR'); if (qr) { qr.disabled = !r.link; qr.checked = r.show_qr; }
        var name = root.querySelector('.clip.sel .who b'); if (name) name.textContent = r.name;
        msg('ok', t('painel.guardado'));
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
      var tx = e.target;
      if (tx.id === 'prVol') { $('#prVolTxt').textContent = tx.value + '%'; later(); return; }
      if (tx.id === 'prLink') { var qr = $('#prQR'); qr.disabled = !tx.value.trim(); if (!tx.value.trim()) qr.checked = false; }
      if (tx.closest('#prForm')) later();
    });
    root.addEventListener('change', function (e) {
      var tx = e.target;
      if (tx.id === 'prFile') { upload(tx.files[0]); tx.value = ''; return; }
      if (tx.id === 'prSound') { $('#prVol').disabled = !tx.checked; save(); return; }
      if (tx.id === 'prQR') { save(); return; }
      if (tx.id === 'prMini') {
        tx.disabled = true;
        api('PUT', '/api/promo', { timeout_clip: tx.checked ? S.promoSel : '' }).then(function (p) {
          S.promo.timeout_clip = p.timeout_clip; msg('ok', t('painel.guardado'));
        }, function (err) { tx.checked = !tx.checked; msg('err', err.message); }).then(function () { tx.disabled = false; });
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
        confirmBox(t('painel.apagar_pergunta', { nome: d.name }), '<p>' + t('painel.promo.apagar_texto') + '</p>', { yes: t('painel.apagar'), danger: true }).then(function (ok) {
          if (!ok) return;
          api('DELETE', '/api/promo/' + encodeURIComponent(d.id)).then(function () { S.promoSel = null; return loadPromo(); }).then(renderPromo, showError);
        });
      }
    });
  }

  // XMLHttpRequest, not fetch, so the volunteer sees how far a big file has gone.
  function upload(file) {
    var out = $('#prUp');
    function say(cls, tx) { out.className = 'msg ' + cls; out.textContent = tx; }
    if (!file) return;
    if (!/\.mp4$/i.test(file.name)) return say('err', t('api.promo.tem_de_ser_mp4'));
    if (file.size > PROMO_MAX) return say('err', t('api.promo.grande'));
    var fd = new FormData();
    fd.append('file', file);
    if (MOCK) {
      return api('POST', '/api/promo', fd).then(function (c) { S.promoSel = c.id; return loadPromo(); }).then(renderPromo, function (err) { say('err', err.message); });
    }
    var x = new XMLHttpRequest();
    x.open('POST', '/api/promo');
    x.upload.onprogress = function (e) { if (e.lengthComputable) say('', t('painel.promo.a_carregar_pct', { pct: Math.round(e.loaded / e.total * 100) })); };
    x.onload = function () {
      var data = null; try { data = JSON.parse(x.responseText); } catch (e) {}
      if (x.status === 401) { askPin(data && data.error).then(function () { upload(file); }); return; }
      if (x.status !== 201) { say('err', (data && data.error) || t('painel.api.erro', { codigo: x.status })); return; }
      S.promoSel = data.id;
      loadPromo().then(function () { if (S.tab === 'promo') renderPromo(); });
    };
    x.onerror = function () { say('err', t('painel.api.sem_ligacao')); };
    say('', t('painel.promo.a_carregar_pct', { pct: 0 }));
    x.send(fd);
  }

  /* ================= 6. DEFINIÇÕES ================= */
  function langOptions(langs) {
    var cur = S.settings.lang || 'pt';
    return langs.map(function (l) { return '<option value="' + esc(l.code) + '"' + (l.code === cur ? ' selected' : '') + '>' + esc(l.name) + '</option>'; }).join('');
  }

  function renderDefinicoes() {
    var root = $('#definicoes'), s = S.settings;
    if (!s) { root.innerHTML = '<p class="hint">' + t('painel.a_carregar') + '</p>'; return; }
    var o = s.obs || {};
    var url = (S.status && S.status.overlay_url) || (location.origin + '/overlay');
    var html = '';
    var fs = S.setup;
    if (fs) {
      html += '<div class="card"><h2>' + t('painel.inicio.titulo') + '</h2>' + (fs.done
        ? '<div class="row" style="align-items:center"><p class="grow">' + t('painel.definicoes.setup_concluida', { data: '<b>' + esc(longDate(fs.done)) + '</b>' }) + '</p>' +
          '<button class="btn" type="button" data-act="setup-review">' + t('painel.rever') + '</button></div>'
        : '<div class="row" style="align-items:center"><p class="hint grow">' + t('painel.definicoes.setup_dica') + '</p>' +
          '<button class="btn primary" type="button" data-act="setup-open">' + (Object.keys(fs.steps || {}).length ? t('painel.continuar') : t('painel.comecar')) + '</button></div>') + '</div>';
    }
    var langs = (S.status && S.status.langs) || [{ code: s.lang || 'pt', name: t('data.nome') }];
    html += '<div class="card"><h2>' + t('painel.definicoes.idioma') + '</h2>' +
      '<p class="hint">' + t('painel.definicoes.idioma_dica') + '</p>' +
      '<div class="row" style="align-items:center"><select id="dLang" style="max-width:420px" aria-label="' + t('painel.definicoes.idioma') + '">' +
      langOptions(langs) +
      '</select><span class="msg" role="status"></span></div></div>';
    html += '<div class="card"><h2>' + t('painel.atualizacao.titulo') + '</h2>' +
      '<div class="row" style="align-items:center"><label class="switch"><input type="checkbox" id="dUpdates"' + (s.update_check ? ' checked' : '') + '><i></i>' + t('painel.atualizacao.avisar') + '</label><span class="msg" role="status"></span></div>' +
      '<p class="hint">' + t('painel.atualizacao.avisar_dica') + '</p></div>';
    html += '<div class="card"><h2>' + t('painel.definicoes.obs') + '</h2>' +
      '<p class="hint">' + t('painel.definicoes.obs_dica') + '</p>' +
      '<div class="row" style="margin-top:12px;align-items:center"><button class="btn primary" type="button" data-act="obs-setup">' + t('painel.configurar_obs') + '</button><span class="msg" id="obsSetupMsg" role="status"></span></div>' +
      '<div id="obsResult">' + setupHtml(S.obsSetup) + '</div>' +
      '<div class="cols" style="margin-top:12px">' +
      '<label class="field"><span>' + t('painel.definicoes.camara') + '</span><select id="dObsCam">' + camOptions(o.camera_source || '') + '</select></label>' +
      '<label class="field"><span>' + t('painel.definicoes.enquadramento') + '</span><select id="dObsFit">' +
      '<option value="contain"' + (o.camera_fit !== 'fill' ? ' selected' : '') + '>' + t('painel.definicoes.imagem_inteira') + '</option>' +
      '<option value="fill"' + (o.camera_fit === 'fill' ? ' selected' : '') + '>' + t('painel.definicoes.encher') + '</option></select></label>' +
      '</div><p class="hint">' + t('painel.definicoes.camara_dica') + '</p>' +
      '<div class="row" style="margin-top:12px"><button class="btn" data-act="obs-fit">' + t('painel.definicoes.ajustar') + '</button><span class="msg" id="dObsMsg"></span></div>' +
      (S.status && S.status.obs_app ?
        '<label class="switch" style="margin-top:12px"><input type="checkbox" id="dObsLaunch"' + (o.launch ? ' checked' : '') + '><i></i>' + t('painel.definicoes.abrir_obs') + '</label>' +
        '<p class="hint">' + t('painel.definicoes.abrir_obs_dica') + '</p>' +
        '<label class="switch" style="margin-top:8px"><input type="checkbox" id="dObsClose"' + (o.close_on_exit ? ' checked' : '') + '><i></i>' + t('painel.definicoes.fechar_obs') + '</label>' +
        '<p class="hint">' + t('painel.definicoes.fechar_obs_dica') + '</p>' : '') +
      '<details style="margin-top:12px"><summary class="hint">' + t('painel.definicoes.obs_mao') + '</summary>' +
      '<p class="hint">' + t('painel.definicoes.obs_mao_dica') + '</p>' +
      '<label class="switch"><input type="checkbox" id="dObsOn"' + (o.enabled ? ' checked' : '') + '><i></i>' + t('painel.definicoes.ligar_obs') + '</label>' +
      '<div class="cols" style="margin-top:8px">' +
      '<label class="field"><span>' + t('painel.definicoes.endereco') + '</span><input id="dObsHost" value="' + esc(o.host || '127.0.0.1') + '"></label>' +
      '<label class="field"><span>' + t('painel.definicoes.porta') + '</span><input id="dObsPort" inputmode="numeric" value="' + esc(o.port || 4455) + '"></label>' +
      '<label class="field"><span>' + t('painel.definicoes.palavra_passe') + '</span><input type="password" id="dObsPass" value="' + esc(o.password || '') + '" autocomplete="off"></label>' +
      '</div>' +
      '<div class="row" style="margin-top:12px"><button class="btn" data-act="obs-test">' + t('painel.definicoes.testar') + '</button><span class="msg" id="dObsTestMsg"></span></div>' +
      '<span class="label" style="margin-top:12px">' + t('painel.definicoes.endereco_grafismo') + '</span><div class="urlbox"><input id="dUrl" readonly value="' + esc(url) + '" aria-label="' + t('painel.definicoes.endereco_grafismo') + '">' +
      '<button class="btn dark" data-act="copy">' + t('painel.copiar') + '</button><span class="msg" style="align-self:center"></span></div>' +
      '<p class="hint">' + t('painel.definicoes.grafismo_mao') + '</p>' +
      '</details>' +
      '<details style="margin-top:12px"><summary class="hint">' + t('painel.definicoes.modo_antigo') + '</summary>' +
      '<p class="hint">' + t('painel.definicoes.modo_antigo_dica') + '</p><div class="cols">' +
      '<label class="field"><span>' + t('painel.definicoes.cena_jogo') + '</span><input id="dObsGame" value="' + esc(o.game_scene || 'Jogo') + '"></label>' +
      '<label class="field"><span>' + t('painel.definicoes.cena_desconto') + '</span><input id="dObsTimeout" value="' + esc(o.timeout_scene || '') + '" placeholder="' + t('painel.definicoes.em_branco') + '"></label>' +
      '</div></details></div>';

    html += '<div class="card"><h2>' + t('painel.destino.titulo') + '</h2>' + destHtml() + '</div>';
    html += '<div class="card" id="ytSetup">' + ytSetupHtml() + '</div>';

    html += '<div class="card"><h2>' + t('painel.definicoes.telemovel') + '</h2>' +
      '<p class="hint">' + t('painel.definicoes.telemovel_dica') + '</p>' +
      '<label class="switch"><input type="checkbox" id="dPhone"' + (s.phone_access ? ' checked' : '') + '><i></i>' + t('painel.definicoes.permitir_telemovel') + '</label>' +
      '<p class="hint">' + t('painel.definicoes.telemovel_depois') + '</p>' +
      '<div class="row" style="margin-top:8px"><label class="field" style="width:160px"><span>' + t('painel.definicoes.pin') + '</span><input id="dPin" inputmode="numeric" maxlength="4" value="' + esc(s.pin || '') + '" autocomplete="off"></label>' +
      (S.status && S.status.lan_url && s.phone_access ? '<p class="grow">' + t('painel.definicoes.telemovel_abra', { url: '<code>' + esc(S.status.lan_url) + '</code>' }) + '</p>' : '') + '</div></div>';

    html += clubCard(s);

    html += lookCard(s.overlay || {}) +
      '<div class="card"><h2>' + t('painel.definicoes.grafismo') + '</h2>' +
      sponsorCornerFields(s.overlay || {}) +
      '<label class="switch" style="margin-top:16px"><input type="checkbox" id="dCredit"' + (s.credit !== false ? ' checked' : '') + '><i></i>' + t('painel.definicoes.credito') + '</label>' +
      '<p class="hint">' + t('painel.definicoes.credito_dica') + '</p></div>';

    html += '<div class="savebar"><button class="btn primary" data-act="save-settings">' + t('painel.definicoes.guardar') + '</button><span class="msg"></span></div>';

    html += '<div class="card" style="margin-top:16px"><h2>' + t('painel.topo.demo') + '</h2>' +
      '<p class="hint">' + t('painel.definicoes.demo_dica') + '</p>' +
      '<div class="row" style="align-items:center"><label class="switch"><input type="checkbox" id="setDemo"' + (S.state && S.state.demo ? ' checked' : '') + '><i></i>' + t('app.bandeja.demo') + '</label><span class="msg"></span></div>' +
      (S.status && S.status.version ? '<p class="hint" style="margin-top:12px">' + t('painel.definicoes.versao', { versao: esc(S.status.version), porta: esc(S.status.port) }) + '</p>' : '') +
      (S.status && S.status.data_dir ? '<p class="hint">' + t('painel.definicoes.dados_em', { pasta: '<code>' + esc(S.status.data_dir) + '</code>' }) + '</p>' : '') + '</div>';

    if (S.setup && S.setup.pc) {
      html += '<div class="card"><h2>' + t('painel.definicoes.copia') + '</h2>' +
        '<p class="hint">' + t('painel.definicoes.copia_dica') + '</p>' +
        '<div class="row" style="align-items:center;margin-top:12px"><a class="btn" id="dBackup" href="/api/system/backup" download>' + t('painel.definicoes.guardar_copia') + '</a>' +
        '<button class="btn quiet" type="button" data-help="ajuda-pc-novo">' + t('painel.ajuda.pc.titulo') + '</button></div></div>';
    }

    if (S.status && S.status.shortcut) {
      html += '<div class="card"><h2>' + t('painel.definicoes.neste_pc') + '</h2>' +
        '<p class="hint">' + t('painel.definicoes.fechar_dica') + '</p>' +
        '<div class="row" style="align-items:center;margin-top:12px"><button class="btn" id="dShortcut" data-act="shortcut">' + t('painel.definicoes.criar_atalho') + '</button><span class="msg"></span></div>' +
        (s.desktop_shortcut === 'created' ? '<p class="hint" style="margin-top:8px">' + t('painel.definicoes.atalho_criado') + '</p>' : '') +
        (S.status.autostart ? '<div class="row" style="align-items:center;margin-top:12px"><label class="switch"><input type="checkbox" id="dAutostart"' + (s.start_with_windows ? ' checked' : '') + '><i></i>' + t('painel.definicoes.abrir_windows') + '</label><span class="msg"></span></div>' +
          '<p class="hint">' + t('painel.definicoes.abrir_windows_dica') + '</p>' : '') + '</div>';
    }
    root.innerHTML = html;
    startLook();
    if (o.enabled && !S.obsCams) loadObsCameras();
  }

  /* Club profile (settings.club): what the overlay shows for the home side, in the club's colours.
     SPORTS follows club.Sports in internal/club; only basketball works today. */
  var SPORTS = [['basketball', t('painel.desporto.basketball'), true], ['futsal', t('painel.desporto.futsal')], ['handball', t('painel.desporto.handball')],
    ['volleyball', t('painel.desporto.volleyball')], ['soccer', t('painel.desporto.soccer')]];
  var COUNTRIES = [['PT', t('painel.pais.pt')], ['ES', t('painel.pais.es')], ['FR', t('painel.pais.fr')], ['LU', t('painel.pais.lu')], ['CH', t('painel.pais.ch')],
    ['BR', t('painel.pais.br')], ['AO', t('painel.pais.ao')], ['CV', t('painel.pais.cv')], ['MZ', t('painel.pais.mz')]];
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
    return '<div class="card"><h2>' + t('painel.clube.titulo') + '</h2><div class="club">' +
      '<div><div class="crest-tile club-crest"><img id="dCrestImg" src="' + esc(crest) + '" alt="' + t('painel.clube.emblema_alt') + '"></div>' +
        '<div class="stack" style="margin-top:10px">' +
        '<label class="btn small filebtn" style="width:100%">' + t('painel.jogo.carregar_emblema') + '<input type="file" id="dCrestFile" accept="image/png,image/jpeg,image/svg+xml"></label>' +
        '<button class="btn quiet small" type="button" data-act="crest-remove" style="width:100%"' + (crest.indexOf('/media/') === 0 ? '' : ' hidden') + '>' + t('painel.clube.remover_emblema') + '</button>' +
        '<span class="msg" id="dCrestMsg" role="status"></span></div></div>' +
      '<div class="cols">' +
        '<label class="field"><span>' + t('painel.clube.nome') + '</span><input id="dHome" maxlength="12" value="' + esc(c.name || '') + '"></label>' +
        '<label class="field"><span>' + t('painel.jogo.sigla') + '</span><input id="dHomeShort" maxlength="4" value="' + esc(c.short || '') + '"></label>' +
        colour('dPrimary', t('painel.clube.cor_principal'), c.primary) +
        colour('dSecondary', t('painel.clube.cor_secundaria'), c.secondary || '#1c1a17') +
        '<label class="field"><span>' + t('painel.clube.modalidade') + '</span><select id="dSport">' + SPORTS.map(function (x) {
          return '<option value="' + x[0] + '"' + (x[2] ? '' : ' disabled') + ((c.sport || 'basketball') === x[0] ? ' selected' : '') + '>' + (x[2] ? x[1] : t('painel.clube.em_breve', { modalidade: x[1] })) + '</option>';
        }).join('') + '</select></label>' +
        '<label class="field"><span>' + t('painel.clube.pais') + '</span><select id="dCountry">' + countries.map(function (x) {
          return '<option value="' + esc(x[0]) + '"' + (x[0] === country ? ' selected' : '') + '>' + esc(x[1]) + '</option>';
        }).join('') + '</select></label>' +
      '</div></div>' +
      '<p class="hint" style="margin-top:12px">' + t('painel.clube.cores_dica') + '</p>' +
      '<div class="cols" style="margin-top:8px">' +
        '<label class="field"><span>Instagram</span><input id="dInsta" value="' + esc(s.instagram || '') + '" placeholder="@clube"></label>' +
        '<label class="field" data-fpb' + (fpbFor(c) ? '' : ' hidden') + '><span>' + t('painel.clube.nome_fpb') + '</span><input id="dClubSearch" value="' + esc(s.club_search || '') + '" placeholder="' + t('painel.clube.nome_fpb_ex') + '"></label>' +
        '<label class="field" data-fpb' + (fpbFor(c) ? '' : ' hidden') + '><span>' + t('painel.clube.endereco_fpb') + '</span><input type="url" id="dFpb" value="' + esc(s.fpb_base_url || '') + '"></label>' +
      '</div><label class="switch" style="margin-top:8px"><input type="checkbox" id="dAuto"' + (s.auto_screens ? ' checked' : '') + '><i></i>' + t('painel.clube.ecras_auto') + '</label></div>';
  }
  function setCrest(c) {
    S.settings.club = Object.assign({}, S.settings.club, { crest: c.crest });
    $('#dCrestImg').src = c.crest;
    $('[data-act="crest-remove"]').hidden = c.crest.indexOf('/media/') !== 0;
  }

  /* YouTube: the program's own login to the club's channel (set up once, on the PC). */
  /* Where OBS sends the picture: the YouTube stream the program manages, or a custom RTMP server.
     Drawn in Definições and in the setup's destination step; PUT /api/stream saves it. */
  function streamDest() { return (S.settings && S.settings.stream) || {}; }
  function destHtml() {
    var d = streamDest(), rt = d.service === 'rtmp', off = S.setup && !S.setup.pc ? ' disabled' : '';
    var opt = function (v, on, label, hint) {
      return '<label><input type="radio" name="dest" value="' + v + '"' + (on ? ' checked' : '') + off + '><span><b>' + label + '</b><small>' + hint + '</small></span></label>';
    };
    return '<div data-dest><p class="hint">' + t('painel.destino.texto') + '</p><div class="cx-in">' +
      opt('youtube', !rt, 'YouTube', t('painel.destino.youtube_dica')) + opt('rtmp', rt, t('painel.destino.rtmp'), t('painel.destino.rtmp_dica')) + '</div>' +
      '<div class="cols" data-rtmp' + (rt ? '' : ' hidden') + '>' +
        '<label class="field"><span>' + t('painel.destino.servidor') + '</span><input data-f="server" placeholder="rtmp://" value="' + esc(d.server || '') + '"' + off + '></label>' +
        '<label class="field"><span>' + t('painel.destino.chave') + '</span><input type="password" data-f="key" autocomplete="off" value="' + esc(d.key || '') + '"' + off + '></label></div>' +
      (off ? '' : '<div class="row" style="margin-top:12px;align-items:center"><button class="btn" type="button" data-act="dest-save">' + t('painel.destino.guardar') + '</button><span class="msg" role="status"></span></div>') +
      '</div>';
  }
  function destEvents() {
    var main = $('main');
    main.addEventListener('change', function (e) {
      var box = e.target.closest('[data-dest]');
      if (box && e.target.name === 'dest') box.querySelector('[data-rtmp]').hidden = e.target.value !== 'rtmp';
    });
    main.addEventListener('click', function (e) {
      var b = e.target.closest('[data-act="dest-save"]'); if (!b) return;
      var box = b.closest('[data-dest]'), msg = b.parentNode.querySelector('.msg'), pick = box.querySelector('input[name="dest"]:checked');
      var body = { service: pick ? pick.value : 'youtube', server: box.querySelector('[data-f="server"]').value, key: box.querySelector('[data-f="key"]').value };
      b.disabled = true; msg.className = 'msg'; msg.textContent = '';
      api('PUT', '/api/stream', body).then(function (r) {
        S.settings.stream = r.stream;
        var o = r.obs;
        msg.className = 'msg ' + (o && o.result === 'failed' ? 'err' : 'ok');
        msg.textContent = o ? o.detail : t('painel.marcar.guardado');
        if (S.tab === 'inicio') { renderWzBody(); refreshInicio(true); }
      }, function (err) { msg.className = 'msg err'; msg.textContent = err.message; })
        .then(function () { b.disabled = false; });
    });
  }

  function ytSetupHtml(bare) {
    var y = S.yt || (S.status && S.status.youtube);
    var html = bare ? '' : '<h2>YouTube</h2>';
    if (!y) return html + '<p class="hint">' + t('painel.a_carregar') + '</p>';
    if (y.connected) {
      var ch = y.channel || {};
      html += '<div class="yt-channel">' + (ch.thumbnail ? '<img src="' + esc(ch.thumbnail) + '" alt="" width="48" height="48" onerror="this.remove()">' : '') +
        '<p><span class="hint">' + t('painel.yt.ligado_como') + '</span><b>' + esc(ch.title || t('painel.yt.canal')) + '</b></p></div>';
      html += y.expired ? '<p class="alert">' + t('painel.yt.expirou') + '</p>' :
        '<p class="hint">' + t('painel.yt.ligado_dica') + '</p>';
      if (y.error && !y.expired) html += '<p class="msg err">' + esc(y.error) + '</p>';
      if (!y.setup) return html + '<p class="hint">' + t('painel.yt.so_no_pc') + '</p>';
      return html + '<div class="row" style="margin-top:12px;align-items:center">' +
        (y.expired ? '<button class="btn primary" type="button" data-act="yt-connect">' + t('painel.definicoes.ligar_yt') + '</button>' : '') +
        '<button class="btn danger" type="button" data-act="yt-disconnect">' + t('painel.desligar') + '</button><span class="msg" id="ytMsg" role="status"></span></div>';
    }
    if (!y.setup) return html + '<p class="hint">' + t('painel.yt.ainda_nao') + '</p>';
    html += (bare ? '' : '<p class="hint">' + t('painel.yt.cole') + '</p>') +
      '<div class="cols" style="margin-top:12px">' +
      '<label class="field"><span>' + t('painel.yt.id') + '</span><input id="ytId" autocomplete="off" spellcheck="false" value="' + esc(y.client_id || '') + '" placeholder="…apps.googleusercontent.com"></label>' +
      '<label class="field"><span>' + t('painel.yt.segredo') + '</span><input type="password" id="ytSecret" autocomplete="off" value="" placeholder="' + (y.has_secret ? t('painel.yt.segredo_guardado') : '') + '"></label>' +
      '</div><div class="row" style="margin-top:12px;align-items:center">' +
      '<button class="btn" type="button" data-act="yt-save">' + t('painel.guardar') + '</button>' +
      '<button class="btn primary" type="button" data-act="yt-connect"' + (y.configured && !y.connecting ? '' : ' disabled') + '>' + t('painel.definicoes.ligar_yt') + '</button>' +
      '<span class="msg" id="ytMsg" role="status"></span></div>';
    if (y.connecting) html += '<p class="hint" style="margin-top:8px">' + t('painel.yt.a_espera') + '</p>';
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
      function say(cls, tx) { if (msg) { msg.className = 'msg ' + cls; msg.textContent = tx; } }
      if (b.dataset.act === 'yt-save') {
        b.disabled = true; say('', t('painel.a_guardar'));
        api('PUT', '/api/youtube/client', { client_id: $('#ytId').value.trim(), client_secret: $('#ytSecret').value.trim() })
          .then(function (y) {
            S.yt = y; renderYtSetup();
            var m = $('#ytMsg'); if (m) { m.className = 'msg ok'; m.textContent = t('painel.guardado'); }
          }, function (err) { b.disabled = false; say('err', err.message); });
      } else if (b.dataset.act === 'yt-connect') {
        // Opened now, while the click counts, so the browser doesn't block it.
        var win = window.open('', '_blank');
        b.disabled = true; say('', t('painel.yt.a_abrir'));
        api('POST', '/api/youtube/connect', {}).then(function (r) {
          if (win) win.location.href = r.auth_url;
          else say('', '');
          S.ytWas = true;
          return loadYt().then(function () {
            if (!win) { var m = $('#ytMsg'); if (m) m.innerHTML = '<a href="' + esc(r.auth_url) + '" target="_blank" rel="noopener">' + t('painel.yt.abrir') + '</a>'; }
          });
        }, function (err) { if (win) win.close(); b.disabled = false; say('err', err.message); });
      } else if (b.dataset.act === 'yt-disconnect') {
        confirmBox(t('painel.yt.desligar_pergunta'), '<p>' + t('painel.yt.desligar_texto') + '</p>' +
          '<p>' + t('painel.yt.desligar_texto2') + '</p>',
        { yes: t('painel.desligar'), danger: true }).then(function (yes) {
          if (!yes) return;
          b.disabled = true;
          api('POST', '/api/youtube/disconnect', {}).then(function (y) { S.yt = y; renderYtSetup(); renderChips(); loadStream(); },
            function (err) { b.disabled = false; say('err', err.message); });
        });
      }
    });
  }

  // Aparência no ar: the on-air style and setup, with a live preview. The preview gets each change
  // straight away (overlay.js look()); the air gets it on Guardar.
  var LOOK = {
    styles: ['classico', 'lombada', 'cor', 'placa', 'pavilhao'],
    bugs: ['top-left', 'top-center', 'top-right', 'bottom-left', 'bottom-center', 'bottom-right'],
    sides: ['left', 'center', 'right'],
    accents: { gold: '#D6BD2F', white: '#F2F1EC', orange: '#F5A23A', lime: '#C3D93B', cyan: '#62CDE3' },
    hide: ['fouls', 'shot', 'strip']
  };
  function lookCard(ov) {
    var style = LOOK.styles.indexOf(ov.style) >= 0 ? ov.style : 'classico';
    var bug = LOOK.bugs.indexOf(ov.bug_position) >= 0 ? ov.bug_position : 'top-left';
    var side = LOOK.sides.indexOf(ov.lower_third) >= 0 ? ov.lower_third : 'left';
    var accent = LOOK.accents[ov.accent] ? ov.accent : 'gold';
    var hide = ov.hide || [];
    var L = {
      ver_marcador: t('painel.aparencia.ver.marcador'), ver_rodape: t('painel.aparencia.ver.rodape'), ver_final: t('painel.aparencia.ver.final'),
      classico: t('painel.aparencia.estilo.classico'), classico_dica: t('painel.aparencia.estilo.classico_dica'),
      lombada: t('painel.aparencia.estilo.lombada'), lombada_dica: t('painel.aparencia.estilo.lombada_dica'),
      cor: t('painel.aparencia.estilo.cor'), cor_dica: t('painel.aparencia.estilo.cor_dica'),
      placa: t('painel.aparencia.estilo.placa'), placa_dica: t('painel.aparencia.estilo.placa_dica'),
      pavilhao: t('painel.aparencia.estilo.pavilhao'), pavilhao_dica: t('painel.aparencia.estilo.pavilhao_dica'),
      'top-left': t('painel.aparencia.canto.cima_esquerda'), 'top-center': t('painel.aparencia.canto.cima_centro'),
      'top-right': t('painel.aparencia.canto.cima_direita'), 'bottom-left': t('painel.aparencia.canto.baixo_esquerda'),
      'bottom-center': t('painel.aparencia.canto.baixo_centro'), 'bottom-right': t('painel.aparencia.canto.baixo_direita'),
      left: t('painel.aparencia.lado.esquerda'), center: t('painel.aparencia.lado.centro'), right: t('painel.aparencia.lado.direita'),
      gold: t('painel.aparencia.destaque.ouro'), white: t('painel.aparencia.destaque.branco'), orange: t('painel.aparencia.destaque.laranja'),
      lime: t('painel.aparencia.destaque.lima'), cyan: t('painel.aparencia.destaque.ciano'),
      fouls: t('painel.aparencia.mostrar.faltas'), shot: t('painel.aparencia.mostrar.relogio_24'), strip: t('painel.aparencia.mostrar.faixa')
    };
    var radio = function (name, value, on, label, extra) {
      return '<label class="opt"><input type="radio" name="' + name + '" value="' + value + '"' + (on ? ' checked' : '') + '>' + label + (extra || '') + '</label>';
    };
    var html = '<div class="card aparencia"><h2>' + t('painel.aparencia.titulo') + '</h2>' +
      '<p class="hint">' + t('painel.aparencia.dica') + '</p>' +
      '<div class="ap-grid"><div class="ap-ver">' +
        '<div class="preview" id="lookPreview"><iframe id="lookFrame" title="' + t('painel.ecras.previsualizacao_titulo') + '" tabindex="-1"></iframe></div>' +
        '<div class="seg" role="group" aria-label="' + t('painel.aparencia.ver') + '" id="lookView">' +
          ['marcador', 'rodape', 'final'].map(function (v, i) {
            return '<button type="button" data-view="' + v + '" aria-pressed="' + (i === 1) + '">' + L['ver_' + v] + '</button>';
          }).join('') + '</div></div>' +
      '<div class="ap-form">' +
        '<fieldset class="ap-estilos"><legend>' + t('painel.aparencia.estilo') + '</legend>' +
          LOOK.styles.map(function (k) {
            return radio('dStyle', k, k === style, '<b>' + L[k] + '</b>', '<small>' + L[k + '_dica'] + '</small>');
          }).join('') + '</fieldset>' +
        '<fieldset><legend>' + t('painel.definicoes.posicao') + '</legend>' +
          '<div class="cantos">' + LOOK.bugs.map(function (k) {
            return '<label><input type="radio" name="dBugPos" value="' + k + '"' + (k === bug ? ' checked' : '') + ' aria-label="' + L[k] + '"><i></i></label>';
          }).join('') + '</div></fieldset>' +
        '<fieldset><legend>' + t('painel.aparencia.rodape') + '</legend><div class="opts">' +
          LOOK.sides.map(function (k) { return radio('dLtSide', k, k === side, L[k]); }).join('') +
          '</div><p class="hint">' + t('painel.aparencia.rodape_dica') + '</p></fieldset>' +
        '<label class="field"><span>' + t('painel.aparencia.tamanho') + ' <output id="dScaleOut">' + (ov.scale || 100) + ' %</output></span>' +
          '<input type="range" id="dScale" min="85" max="125" step="5" value="' + (ov.scale || 100) + '"></label>' +
        '<label class="field"><span>' + t('painel.aparencia.margem') + ' <output id="dMarginOut">' + (ov.margin || 64) + ' px</output></span>' +
          '<input type="range" id="dMargin" min="32" max="96" step="8" value="' + (ov.margin || 64) + '"></label>' +
        '<p class="hint">' + t('painel.aparencia.margem_dica') + '</p>' +
        '<fieldset><legend>' + t('painel.aparencia.destaque') + '</legend><div class="opts amostras">' +
          Object.keys(LOOK.accents).map(function (k) {
            return radio('dAccent', k, k === accent, '<i style="background:' + LOOK.accents[k] + '"></i>' + L[k]);
          }).join('') + '</div><p class="hint">' + t('painel.aparencia.destaque_dica') + '</p></fieldset>' +
        '<fieldset><legend>' + t('painel.aparencia.adversario') + '</legend><div class="opts">' +
          radio('dAway', 'own', ov.away_color !== 'neutral', t('painel.aparencia.adversario.dele')) +
          radio('dAway', 'neutral', ov.away_color === 'neutral', t('painel.aparencia.adversario.cinzento')) + '</div></fieldset>' +
        '<fieldset><legend>' + t('painel.aparencia.mostrar') + '</legend>' +
          LOOK.hide.map(function (k) {
            return '<label class="switch"><input type="checkbox" data-show="' + k + '"' + (hide.indexOf(k) < 0 ? ' checked' : '') + '><i></i>' + L[k] + '</label>';
          }).join('') + '</fieldset>' +
        '<fieldset><legend>' + t('painel.aparencia.foto') + '</legend>' +
          '<p class="hint">' + t('painel.aparencia.foto_dica') + '</p>' +
          '<div class="row" style="align-items:center">' +
          '<label class="btn small filebtn">' + t('painel.aparencia.foto.escolher') + '<input type="file" id="dHallFile" accept="image/png,image/jpeg"></label>' +
          '<button class="btn quiet small" type="button" data-act="hall-remove"' + (ov.photo ? '' : ' hidden') + '>' + t('painel.aparencia.foto.tirar') + '</button>' +
          '<span class="msg" id="dHallMsg" role="status"></span></div></fieldset>' +
      '</div></div></div>';
    return html;
  }
  // The setup as the form has it now, in the overlay's query words.
  function lookForm() {
    var val = function (n) { var el = $('input[name="' + n + '"]:checked'); return el ? el.value : ''; };
    return {
      style: val('dStyle'), bug: val('dBugPos'), lt: val('dLtSide'), accent: val('dAccent'), away: val('dAway'),
      scale: $('#dScale').value, margin: $('#dMargin').value,
      hide: LOOK.hide.filter(function (k) { var c = $('[data-show="' + k + '"]'); return c && !c.checked; })
    };
  }
  function lookView() {
    var b = $('#lookView [aria-pressed="true"]');
    return b ? b.dataset.view : 'rodape';
  }
  function lookSrc() {
    var v = lookView();
    var base = MOCK ? '/overlay/index.html' : '/overlay';
    return base + '?painel=1&anim=0&screen=' + (v === 'final' ? 'final' : 'marcador') + (v === 'rodape' ? '&rodape=1' : '');
  }
  function postLook() {
    var fr = $('#lookFrame'); if (!fr || !fr.contentWindow) return;
    var f = lookForm();
    var qs = 'style=' + f.style + '&bug=' + f.bug + '&lt=' + f.lt + '&accent=' + f.accent + '&scale=' + f.scale + '&margin=' + f.margin + '&away=' + f.away + '&hide=' + f.hide.join(',');
    fr.contentWindow.postMessage({ look: qs }, location.origin);
    $('#dScaleOut').textContent = f.scale + ' %';
    $('#dMarginOut').textContent = f.margin + ' px';
  }
  function fitLook() {
    var box = $('#lookPreview'), fr = $('#lookFrame');
    if (box && fr) fr.style.transform = 'scale(' + (box.clientWidth / 1920) + ')';
  }
  function startLook() {
    var fr = $('#lookFrame'); if (!fr) return;
    fr.addEventListener('load', postLook);
    fr.src = lookSrc();
    fitLook();
  }
  function setHall(ov) {
    S.settings.overlay = Object.assign({}, S.settings.overlay, { photo: ov.photo });
    $('[data-act="hall-remove"]').hidden = !ov.photo;
    var fr = $('#lookFrame'); if (fr) fr.src = lookSrc(); // the photo comes with the state
  }

  // Sponsor corner on the scoreboard: on by default, every 4 minutes.
  function sponsorCornerFields(ov) {
    return '<label class="switch" style="margin-top:16px"><input type="checkbox" id="dCorner"' + (ov.sponsor_corner !== false ? ' checked' : '') + '><i></i>' + t('painel.canto.ligar') + '</label>' +
      '<div class="row" style="margin-top:8px"><label class="field" style="width:280px;max-width:100%"><span>' + t('painel.canto.minutos') + '</span>' +
      '<input id="dCornerEvery" type="number" inputmode="numeric" min="1" max="30" value="' + esc(ov.sponsor_every || 4) + '"></label></div>' +
      '<p class="hint">' + t('painel.canto.dica') + '</p>';
  }

  // The cameras OBS has, and the devices it could add; "" lets the program pick (DroidCam first).
  function camOptions(cur) {
    var list = S.obsCams || [], found = !cur, adds = '';
    var html = '<option value="">' + t('painel.camara.automatica') + '</option>';
    list.forEach(function (c) {
      if (c.type === 'source') {
        var name = c.value.slice(7);
        if (name === cur) found = true;
        html += '<option value="' + esc(name) + '"' + (name === cur ? ' selected' : '') + '>' + esc(name) + '</option>';
      } else {
        adds += '<option value="' + esc(c.value) + '">' + esc(c.label) + '</option>';
      }
    });
    if (!found) html += '<option value="' + esc(cur) + '" selected>' + (S.obsCams ? t('painel.camara.fora_obs', { camara: esc(cur) }) : esc(cur)) + '</option>';
    if (adds) html += '<optgroup label="' + t('painel.camara.juntar') + '">' + adds + '</optgroup>';
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
    var lk = lookForm();
    s.overlay = Object.assign({}, s.overlay, { style: lk.style, bug_position: lk.bug, lower_third: lk.lt, accent: lk.accent,
      away_color: lk.away, scale: +lk.scale, margin: +lk.margin, hide: lk.hide });
    s.overlay.sponsor_corner = $('#dCorner').checked;
    s.overlay.sponsor_every = Math.min(30, Math.max(1, parseInt($('#dCornerEvery').value, 10) || 4));
    s.credit = $('#dCredit').checked;
    return s;
  }

  function defsEvents() {
    var root = $('#definicoes');
    // Aparência no ar: every change goes to the preview at once.
    root.addEventListener('input', function (e) { if (e.target.closest('.aparencia')) postLook(); });
    root.addEventListener('click', function (e) {
      var v = e.target.closest('#lookView [data-view]');
      if (v) {
        $all('#lookView [data-view]').forEach(function (x) { x.setAttribute('aria-pressed', String(x === v)); });
        $('#lookFrame').src = lookSrc();
      }
    });
    root.addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]'); if (!b) return;
      if (b.dataset.act === 'hall-remove') {
        busy(b, function () { return api('DELETE', '/api/overlay/photo').then(setHall); }, t('painel.aparencia.foto.tirada'));
        return;
      }
      if (b.dataset.act === 'setup-open' || b.dataset.act === 'setup-review') {
        if (b.dataset.act === 'setup-review') S.setupStep = 0;
        showTab('inicio');
      } else if (b.dataset.act === 'copy') {
        busy(b, function () { $('#dUrl').select(); return copyText($('#dUrl').value); }, t('painel.copiado'));
      } else if (b.dataset.act === 'crest-remove') {
        busy(b, function () { return api('DELETE', '/api/club/crest').then(setCrest); }, t('painel.clube.emblema_removido'));
      } else if (b.dataset.act === 'save-settings') {
        var s = readSettingsForm();
        if (s.phone_access && !/^\d{4}$/.test(s.pin)) {
          var m = b.parentElement.querySelector('.msg'); m.className = 'msg err'; m.textContent = t('painel.definicoes.pin_telemovel'); return;
        }
        busy(b, function () {
          return api('PUT', '/api/settings', s).then(function (r) { S.settings = r || s; renderChips(); loadStatus(); });
        });
      } else if (b.dataset.act === 'shortcut') {
        busy(b, function () { return createShortcut(true); }, t('painel.definicoes.atalho_ok'));
      } else if (b.dataset.act === 'obs-test') {
        var s2 = readSettingsForm();
        var msg = $('#dObsTestMsg');
        b.disabled = true; msg.className = 'msg'; msg.textContent = t('painel.definicoes.a_testar');
        api('POST', '/api/obs/test', s2.obs).then(function (r) {
          if (r && r.connected) { msg.className = 'msg ok'; msg.textContent = t('painel.definicoes.obs_funciona'); S.obsCams = null; loadObsCameras(); }
          else { msg.className = 'msg err'; msg.textContent = (r && r.error) || t('painel.definicoes.obs_nao_respondeu'); }
        }, function (err) { msg.className = 'msg err'; msg.textContent = err.message; }).then(function () { b.disabled = false; });
      } else if (b.dataset.act === 'obs-setup') {
        runSetup(b, '');
      } else if (b.dataset.act === 'obs-pick') {
        var pk = root.querySelector('input[name="obsPick"]:checked');
        if (pk) runSetup($('[data-act="obs-setup"]'), pk.value);
      } else if (b.dataset.act === 'obs-fit') {
        // Saves first, so the camera and fit chosen here are the ones used.
        var s3 = readSettingsForm(), fm = $('#dObsMsg');
        b.disabled = true; fm.className = 'msg'; fm.textContent = t('painel.definicoes.a_ajustar');
        api('PUT', '/api/settings', s3).then(function (r) {
          S.settings = r || s3;
          return api('POST', '/api/obs/camera/fit', {});
        }).then(function (r) {
          var f = (r && r.fit) || {};
          fm.className = r && r.warning ? 'msg err' : 'msg ok';
          fm.textContent = r && r.warning ? r.warning : t('painel.definicoes.ajustada', { fonte: f.source || '', largura: f.source_width, altura: f.source_height });
        }, function (err) { fm.className = 'msg err'; fm.textContent = err.message; }).then(function () { b.disabled = false; });
      }
    });
    root.addEventListener('change', function (e) {
      var tx = e.target;
      if (tx.id === 'dPin') tx.value = tx.value.replace(/\D/g, '').slice(0, 4);
      if (tx.id === 'dUpdates') {
        var um = tx.closest('.row').querySelector('.msg');
        tx.disabled = true; um.className = 'msg'; um.textContent = '';
        api('PUT', '/api/settings', Object.assign({}, S.settings, { update_check: tx.checked })).then(function (saved) {
          S.settings = saved; um.className = 'msg ok'; um.textContent = t('painel.marcar.guardado');
          setTimeout(loadStatus, 3000);
        }, function (err) { tx.checked = !tx.checked; um.className = 'msg err'; um.textContent = err.message; })
          .then(function () { tx.disabled = false; });
        return;
      }
      if (tx.id === 'dLang') {
        // Every page reads its words once, from /i18n.js, so a new language means a reload.
        var lm = tx.closest('.row').querySelector('.msg');
        tx.disabled = true;
        api('PUT', '/api/settings', Object.assign({}, S.settings, { lang: tx.value })).then(function () { location.reload(); },
          function (err) { tx.disabled = false; tx.value = S.settings.lang || 'pt'; lm.className = 'msg err'; lm.textContent = err.message; });
        return;
      }
      if (tx.id === 'dSport' || tx.id === 'dCountry') {
        var on = fpbFor({ sport: $('#dSport').value, country: $('#dCountry').value });
        $all('[data-fpb]', root).forEach(function (el) { el.hidden = !on; });
        return;
      }
      if (tx.id === 'dPrimary' || tx.id === 'dSecondary') { $('#' + tx.id + 'Hex').textContent = tx.value.toUpperCase(); return; }
      if (tx.id === 'dHallFile' && tx.files && tx.files[0]) {
        var hf = new FormData(); hf.append('file', tx.files[0]);
        busy(tx.closest('label'), function () { return api('POST', '/api/overlay/photo', hf).then(setHall); }, t('painel.aparencia.foto.guardada'));
        tx.value = '';
        return;
      }
      if (tx.id === 'dCrestFile' && tx.files && tx.files[0]) {
        var fd = new FormData(); fd.append('file', tx.files[0]);
        busy(tx.closest('label'), function () { return api('POST', '/api/club/crest', fd).then(setCrest); }, t('painel.clube.emblema_guardado'));
        tx.value = '';
        return;
      }
      if (tx.id === 'dAutostart') {
        var am = tx.closest('.row').querySelector('.msg');
        tx.disabled = true; am.className = 'msg'; am.textContent = '';
        api('POST', '/api/system/autostart', { on: tx.checked }).then(function () {
          S.settings.start_with_windows = tx.checked;
          am.className = 'msg ok'; am.textContent = tx.checked ? t('painel.definicoes.abre_windows') : t('painel.definicoes.nao_abre_windows');
        }, function (err) { tx.checked = !tx.checked; am.className = 'msg err'; am.textContent = err.message; })
          .then(function () { tx.disabled = false; });
        return;
      }
      if (tx.id !== 'setDemo') return;
      var msg = tx.closest('.row').querySelector('.msg');
      tx.disabled = true; msg.textContent = '';
      api('POST', '/api/demo', { on: tx.checked }).then(function () {
        msg.className = 'msg ok'; msg.textContent = tx.checked ? t('painel.definicoes.demo_ligada') : t('painel.definicoes.demo_desligada');
      }, function (err) { tx.checked = !tx.checked; msg.className = 'msg err'; msg.textContent = err.message; })
        .then(function () { tx.disabled = false; });
    });
  }

  /* ================= TRANSMISSÕES ================= */
  var PRIVACY = { 'public': t('painel.hist.publico'), unlisted: t('painel.hist.nao_listado'), 'private': t('painel.hist.privado') };
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
      '<label class="field"><span>' + t('painel.jogo.escalao') + '</span><select id="hEscalao"></select></label>' +
      '<label class="field"><span>' + t('painel.hist.epoca') + '</span><select id="hSeason"></select></label>' +
      '<button class="btn" type="button" data-act="hist-refresh">' + t('painel.hist.atualizar') + '</button></div></div><div id="histList"></div>';
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
    $('#hEscalao').innerHTML = opts(all.escaloes, f.escalao, t('painel.hist.todos'));
    $('#hSeason').innerHTML = opts(all.seasons, f.season, t('painel.hist.todas'));
    if (S.histErr) { list.innerHTML = '<p class="alert">' + esc(S.histErr) + '</p>'; return; }
    if (!h) { list.innerHTML = '<p class="hint">' + t('painel.a_carregar') + '</p>'; return; }
    var html = '';
    if (!h.connected) html += '<p class="warn">' + t('painel.hist.sem_yt') + '</p>';
    if (h.remote_error) html += '<p class="alert">' + t('painel.hist.erro_canal', { erro: esc(h.remote_error) }) + '</p>';
    if (!h.items.length) {
      html += '<div class="card"><p class="hint">' + (f.escalao || f.season ? t('painel.hist.sem_filtro') : t('painel.hist.vazio')) + '</p></div>';
      list.innerHTML = html; return;
    }
    html += '<ul class="hist">' + h.items.map(function (it) {
      var d = new Date(it.date), when = isNaN(d) ? '' : d.toLocaleDateString(t('data.locale'), { day: 'numeric', month: 'short', year: 'numeric' });
      var tag = it.external ? '<span class="badge">' + t('yt.fora_hallstream') + '</span>' : it.test ? '<span class="badge">' + t('painel.hist.teste') + '</span>' : '';
      var priv = PRIVACY[it.privacy] || it.privacy || '';
      return '<li><div class="who"><b>' + esc(it.result || it.title) + '</b>' +
        '<small>' + esc([when, it.result ? it.title : it.escalao].filter(Boolean).join(' · ')) + '</small>' +
        '<span class="tags"><span class="badge priv-' + esc(it.privacy) + '">' + esc(priv) + '</span>' + tag + '</span></div>' +
        '<div class="acts"><button class="btn small" type="button" data-act="hist-copy" data-url="' + esc(it.link) + '">' + t('painel.copiar') + '</button>' +
        '<a class="btn small" href="' + esc(it.link) + '" target="_blank" rel="noopener">' + t('painel.abrir') + '</a>' +
        (it.privacy !== 'unlisted' && !it.test && h.connected ? '<button class="btn small dark" type="button" data-act="hist-unlist" data-id="' + esc(it.broadcast_id) + '">' + t('painel.hist.tornar_nao_listado') + '</button>' : '') +
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
      if (b.dataset.act === 'hist-refresh') busy(b, function () { return loadHistory(true); }, t('painel.hist.atualizado'));
      if (b.dataset.act === 'hist-copy') busy(b, function () { return copyText(b.dataset.url); }, t('painel.copiado'));
      if (b.dataset.act === 'hist-unlist') {
        busy(b, function () { return api('POST', '/api/youtube/unlist', { broadcast_id: b.dataset.id }); }, t('painel.feito'))
          .then(function (r) { if (r) loadHistory(); });
      }
    });
  }

  /* ================= CONFIGURAÇÃO INICIAL ================= */
  // Seven steps, one card at a time. The status comes from what the panel
  // already reads (status, settings, YouTube); ticks, choices and "Saltar" from /api/setup.
  var WZ = [
    { id: 'idioma', title: t('painel.wz.idioma') }, { id: 'clube', title: t('painel.wz.clube') }, { id: 'entrada', title: t('painel.nav.caixa') },
    { id: 'obs', title: t('painel.wz.obs') }, { id: 'youtube', title: t('painel.wz.destino') },
    { id: 'canal', title: t('painel.wz.canal') }, { id: 'ensaio', title: t('painel.wz.ensaio') }
  ];
  function wzIndex(id) { for (var i = 0; i < WZ.length; i++) if (WZ[i].id === id) return i; return -1; }
  var WZ_TAG = { ok: ['on', t('painel.feito')], todo: ['wait', t('painel.wz.falta')], bad: ['off', t('painel.wz.com_problema')], skip: ['', t('painel.wz.saltado')], wait: ['wait', t('painel.wz.a_verificar')] };
  var ENSAIO = [
    ['ensaio-demo', t('app.bandeja.demo'), t('painel.ensaio.demo')],
    ['ensaio-previa', t('painel.ecras.previsualizacao'), t('painel.ensaio.previa')],
    ['ensaio-desconto', t('painel.ecra.desconto'), t('painel.ensaio.desconto')],
    ['ensaio-promo', t('painel.ensaio.promo_titulo'), t('painel.ensaio.promo')],
    ['ensaio-teste', t('painel.stream.teste'), t('painel.ensaio.teste')]
  ];

  function longDate(iso) {
    var d = new Date(iso);
    return isNaN(d) ? '' : d.toLocaleDateString(t('data.locale'), { day: 'numeric', month: 'long', year: 'numeric' });
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
    if (id === 'idioma') {
      var lang = ((st.langs || []).filter(function (l) { return l.code === (set.lang || 'pt'); })[0] || {}).name || t('data.nome');
      return wzTicked('idioma') ? { s: 'ok', why: t('painel.wz.idioma_ok', { lingua: lang }) } : { s: 'todo', why: t('painel.wz.idioma_falta') };
    }
    if (id === 'clube') {
      if (st.club_setup && st.club_setup.running) return { s: 'wait', why: clubProgress(st.club_setup) };
      if (fpbOffered() && set.club_setup === 'pending') return { s: 'bad', why: t('painel.wz.clube_fpb_pendente') };
      if (!wzTicked('clube')) return { s: 'todo', why: t('painel.wz.clube_falta') };
      return { s: 'ok', why: t('painel.wz.clube_ok_nome', { clube: (set.club && set.club.name) || '' }) };
    }
    if (id === 'entrada') {
      if (!wzTicked('entrada')) return { s: 'todo', why: t('painel.wz.entrada_falta') };
      if (set.input === 'manual') return { s: 'ok', why: t('painel.wz.entrada_mao_ok') };
      var box = realBox();
      if (!set.box_address && !box.connected) return { s: 'todo', why: t('painel.wz.clube_sem_caixa') };
      return { s: 'ok', why: box.connected ? t('painel.wz.caixa_agora') : t('painel.wz.caixa_ja') };
    }
    if (id === 'obs') {
      var o = st.obs || {}, h = o.health || {};
      if (!obsOn()) return { s: 'todo', why: t('painel.wz.obs_por_configurar') };
      if (!o.connected) return { s: 'bad', why: o.error || t('painel.wz.obs_nao_responde') };
      if (!h.checked) return { s: 'wait', why: t('painel.wz.obs_a_ver') };
      if (h.ready) return { s: 'ok', why: t('painel.obs.pronto') };
      return { s: 'bad', why: (h.problems || [])[0] || t('painel.wz.obs_reparar') };
    }
    if (id === 'youtube' && streamDest().service === 'rtmp') return { s: 'ok', why: t('painel.wz.destino_rtmp', { servidor: streamDest().server }) };
    if (id === 'canal' && streamDest().service === 'rtmp') return { s: 'skip', why: t('painel.wz.canal_so_yt') };
    if (id === 'youtube') {
      if (y.connected && y.expired) return { s: 'bad', why: t('painel.wz.yt_expirou') };
      if (y.connected) return { s: 'ok', why: t('painel.wz.yt_ligado', { canal: (y.channel && y.channel.title) || t('painel.wz.yt_canal') }) };
      if (y.error) return { s: 'bad', why: y.error };
      if (S.setup && S.setup.steps.youtube === 'skipped') return { s: 'skip', why: t('painel.wz.yt_depois') };
      return { s: 'todo', why: t('painel.wz.yt_falta') };
    }
    if (id === 'canal') {
      var l = canalItem('diretos'), v = canalItem('verificado');
      if (S.chkBusy) return { s: 'wait', why: t('painel.wz.canal_a_perguntar') };
      if (v === 'blocked') return { s: 'bad', why: t('painel.wz.canal_bloqueado') };
      if (l === 'yes' && v === 'yes') return { s: 'ok', why: t('painel.wz.canal_ok') };
      if (l === 'no') return { s: 'todo', why: t('painel.wz.canal_sem_diretos') };
      if (v === 'no') return { s: 'todo', why: t('painel.wz.canal_por_verificar') };
      return { s: 'todo', why: t('painel.wz.canal_confirme') };
    }
    var n = ENSAIO.filter(function (e) { return wzTicked(e[0]); }).length;
    return n === ENSAIO.length ? { s: 'ok', why: t('painel.wz.ensaio_feito') } : { s: 'todo', why: t('painel.wz.ensaio_faltam', { n: ENSAIO.length - n, total: ENSAIO.length }) };
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
    if (!S.setup || !S.settings || S.setupStep == null) { root.innerHTML = '<p class="hint">' + t('painel.a_carregar') + '</p>'; return; }
    root.innerHTML = '<ol class="wz-steps" id="wzSteps" aria-label="' + t('painel.wz.passos') + '"></ol>' +
      (S.setup.pc ? '' : '<p class="warn">' + t('painel.wz.so_no_pc') + '</p>') +
      '<div class="card wz" id="wzCard"><div id="wzHead"></div><div id="wzBody"></div><div class="wz-nav" id="wzNav"></div></div>';
    renderWzBody();
    refreshInicio(true);
  }

  // The parts that follow the live status: the step list, the card head and its buttons.
  function refreshInicio(force) {
    var el = $('#wzSteps'); if (!el || S.tab !== 'inicio') return;
    var cur = S.setupStep;
    var steps = WZ.map(function (w, i) {
      var tx = WZ_TAG[wzStatus(w.id).s];
      return '<li><button type="button" data-wz="' + i + '"' + (i === cur ? ' aria-current="step"' : '') + '>' +
        '<span class="dot ' + tx[0] + '" aria-hidden="true"></span><span class="n">' + (i + 1) + '</span><span class="t">' + esc(w.title) + '</span>' +
        '<span class="sr">' + esc(tx[1]) + '</span></button></li>';
    }).join('');
    if (force || el.dataset.h !== steps) { el.dataset.h = steps; el.innerHTML = steps; }
    var head = $('#wzHead'), nav = $('#wzNav'), h, n;
    if (cur === 'fim') {
      h = '<h2>' + t('painel.wz.concluida') + '</h2>';
      n = '<a class="btn primary" href="#ecras" data-goto="ecras">' + t('painel.wz.ir_ecras') + '</a><button class="btn" type="button" data-wz="0">' + t('painel.wz.rever') + '</button>';
    } else {
      var w = WZ[cur], st = wzStatus(w.id), tag = WZ_TAG[st.s], y = S.yt || {};
      h = '<div class="wz-top"><span class="wz-count">' + t('painel.wz.passo_n', { n: cur + 1, total: WZ.length }) + '</span>' +
        '<span class="wz-state st-' + st.s + '"><span class="dot ' + tag[0] + '"></span>' + tag[1] + '</span></div>' +
        '<h2>' + esc(w.title) + '</h2><p class="wz-why' + (st.s === 'bad' ? ' bad' : '') + '">' + esc(st.why) + '</p>';
      n = (cur > 0 ? '<button class="btn" type="button" data-wz="' + (cur - 1) + '">' + t('painel.anterior') + '</button>' : '') + '<span class="grow"></span>' +
        (w.id === 'youtube' && S.setup.pc && !y.connected && streamDest().service !== 'rtmp' && st.s !== 'skip' ? '<button class="btn quiet" type="button" data-wz-act="yt-skip">' + t('painel.wz.saltar') + '</button>' : '') +
        (cur < WZ.length - 1 ? '<button class="btn primary" type="button" data-wz="' + (cur + 1) + '">' + t('painel.seguinte') + '</button>' :
          S.setup.pc ? '<button class="btn primary" type="button" data-wz-act="done">' + t('painel.wz.concluida') + '</button>' : '') +
        '<span class="msg" id="wzNavMsg" role="status"></span>';
    }
    if (force || head.dataset.h !== h) { head.dataset.h = h; head.innerHTML = h; }
    var yc = !!(S.yt && S.yt.connected);
    $all('#wzBody .wz-how').forEach(function (x) { x.hidden = yc; });
    if (force || nav.dataset.h !== n) { nav.dataset.h = n; nav.innerHTML = n; }
  }

  function wzTick(id, label) {
    return '<label class="switch wz-tick"><input type="checkbox" data-tick="' + id + '"' + (wzTicked(id) ? ' checked' : '') +
      (S.setup.pc ? '' : ' disabled') + '><i></i>' + (label || t('painel.wz.ja_fiz')) + '</label>';
  }
  function wzItem(dot, title, html) {
    return '<li><span class="dot ' + dot + '" aria-hidden="true"></span><div><b>' + title + '</b>' + html + '</div></li>';
  }

  function renderWzBody() {
    var el = $('#wzBody'); if (!el) return;
    var cur = S.setupStep, pc = S.setup.pc, html = '';
    if (cur === 'fim') {
      html = '<p>' + t('painel.wz.concluida_em', { data: '<b>' + esc(longDate(S.setup.done)) + '</b>' }) + '</p>';
      var left = WZ.filter(function (w, i) { return wzOpen(i); }).map(function (w) { return w.title; });
      if (left.length) html += '<p class="hint">' + t('painel.wz.por_fazer', { lista: esc(left.join(', ')) }) + '</p>';
      el.innerHTML = html;
      return;
    }
    var id = WZ[cur].id, set = S.settings, off = pc ? '' : ' disabled';
    var save = function (act, label) {
      return pc ? '<div class="row" style="margin-top:12px;align-items:center"><button class="btn primary" type="button" data-wz-act="' + act + '">' + label + '</button>' +
        '<span class="msg" id="wzSaveMsg" role="status"></span></div>' : '';
    };
    if (id === 'idioma') {
      var langs = (S.status && S.status.langs) || [{ code: set.lang || 'pt', name: t('data.nome') }];
      html = '<p>' + t('painel.wz.idioma_texto') + '</p><select id="wLang" style="max-width:420px" aria-label="' + t('painel.wz.idioma') + '"' + off + '>' + langOptions(langs) + '</select>' +
        save('lang', t('painel.wz.idioma_usar'));
    } else if (id === 'clube') {
      var c = set.club || {};
      var colour = function (cid, label, v) {
        v = /^#[0-9a-f]{6}$/i.test(v || '') ? v.toLowerCase() : '#2f4b7c';
        return '<label class="field"><span>' + label + '</span><span class="colour"><input type="color" id="' + cid + '" value="' + esc(v) + '"' + off + '></span></label>';
      };
      html = '<p>' + t('painel.wz.clube_texto') + '</p><div class="cols">' +
        '<label class="field"><span>' + t('painel.clube.nome') + '</span><input id="wHome" maxlength="12" value="' + esc(c.name || '') + '"' + off + '></label>' +
        '<label class="field"><span>' + t('painel.jogo.sigla') + '</span><input id="wHomeShort" maxlength="4" value="' + esc(c.short || '') + '"' + off + '></label>' +
        colour('wPrimary', t('painel.clube.cor_principal'), c.primary) +
        colour('wSecondary', t('painel.clube.cor_secundaria'), c.secondary || '#1c1a17') +
        '<label class="field"><span>' + t('painel.clube.modalidade') + '</span><select id="wSport"' + off + '>' + SPORTS.map(function (x) {
          return '<option value="' + x[0] + '"' + (x[2] ? '' : ' disabled') + ((c.sport || 'basketball') === x[0] ? ' selected' : '') + '>' + (x[2] ? x[1] : t('painel.clube.em_breve', { modalidade: x[1] })) + '</option>';
        }).join('') + '</select></label>' +
        '<label class="field"><span>' + t('painel.clube.pais') + '</span><select id="wCountry"' + off + '>' + COUNTRIES.map(function (x) {
          return '<option value="' + esc(x[0]) + '"' + (x[0] === (c.country || 'PT') ? ' selected' : '') + '>' + esc(x[1]) + '</option>';
        }).join('') + '</select></label></div>';
      if (fpbOffered()) {
        var club = (set.club_setup && set.club_setup !== 'pending') || S.groups.length;
        html += '<ul class="wz-list">' + wzItem(club ? 'on' : 'wait', t('painel.wz.escaloes_fpb'), '<p class="hint">' + (club ? plural(S.groups.length, t('painel.wz.escaloes_no_plantel.um', { n: S.groups.length }), t('painel.wz.escaloes_no_plantel.outros', { n: S.groups.length })) :
          t('painel.wz.escaloes_sozinho')) + ' <a href="#plantel" data-goto="plantel">' + t('painel.ajuda.problemas.abrir_plantel') + '</a></p>') + '</ul>';
      } else {
        html += '<p class="hint">' + t('painel.wz.plantel_csv') + ' <a href="#plantel" data-goto="plantel">' + t('painel.ajuda.problemas.abrir_plantel') + '</a></p>';
      }
      html += save('club', t('painel.wz.clube_guardar'));
    } else if (id === 'entrada') {
      var manual = set.input === 'manual', box = realBox();
      var opt = function (v, title, hint, on) {
        return '<label><input type="radio" name="wInput" value="' + v + '"' + (on ? ' checked' : '') + off + '><span><b>' + title + '</b><small>' + hint + '</small></span></label>';
      };
      html = '<p>' + t('painel.wz.entrada_texto') + '</p><div class="cx-in">' +
        opt('console', t('painel.marcar.consola'), t('painel.marcar.consola_dica'), !manual) +
        opt('manual', t('painel.marcar.telemovel'), t('painel.marcar.telemovel_dica'), manual) + '</div>' +
        save('input', t('painel.wz.entrada_usar'));
      if (wzTicked('entrada')) {
        html += manual
          ? '<p class="hint">' + t('painel.wz.entrada_qr') + ' <a href="#caixa" data-goto="caixa">' + t('painel.ajuda.problemas.abrir_caixa') + '</a></p>'
          : '<ul class="wz-list">' + wzItem(set.box_address || box.connected ? 'on' : 'wait', t('painel.wz.caixa'), '<p class="hint">' + (box.connected ? t('painel.wz.caixa_agora') : set.box_address ? t('painel.wz.caixa_ja') :
            t('painel.wz.caixa_ligue')) + ' <a href="#caixa" data-goto="caixa">' + t('painel.ajuda.problemas.abrir_caixa') + '</a></p>') + '</ul><p class="hint">' + t('painel.wz.caixa_depois') + '</p>';
      }
    } else if (id === 'obs') {
      html = '<p>' + t('painel.wz.obs_texto') + '</p>';
      if (pc) html += '<div class="row" style="margin-top:12px;align-items:center"><button class="btn primary" type="button" data-wz-act="obs-setup">' + t('painel.configurar_obs') + '</button>' +
        '<span class="msg" id="wzObsMsg" role="status"></span></div>';
      html += '<div id="wzObsResult">' + setupHtml(S.obsSetup) + '</div>';
    } else if (id === 'youtube' && streamDest().service === 'rtmp') {
      html = destHtml();
    } else if (id === 'youtube') {
      html = destHtml() + '<div class="wz-how"><p>' + t('painel.wz.yt_precisa') + '</p>' +
        '<ol class="steps"><li>' + t('painel.wz.yt_passo1') + '</li>' +
        '<li>' + t('painel.wz.yt_passo2') + '</li>' +
        '<li>' + t('painel.wz.yt_passo3') + '</li></ol></div>' +
        '<div class="wz-sub" id="ytSetup" data-bare="1">' + ytSetupHtml(true) + '</div>' +
        '<div class="wz-how"><p class="hint">' + t('painel.wz.yt_conta') + '</p>' +
        '<p class="hint">' + t('painel.wz.yt_sem_id') + '</p></div>';
    } else if (id === 'canal') {
      var l = canalItem('diretos'), v = canalItem('verificado'), c = S.chk || {}, yc = (S.yt || {}).connected;
      var said = function (tx) { return '<p class="wz-said">' + tx + '</p>'; };
      html = '<p>' + t('painel.wz.canal_texto') + '</p><ul class="wz-list">' +
        wzItem(l === 'yes' ? 'on' : 'wait', t('painel.wz.diretos'), '<p class="hint">' + t('painel.wz.diretos_dica') + '</p>' +
          (c.live ? said(c.live === 'on' ? t('painel.wz.diretos_sim') : t('painel.wz.diretos_nao')) : wzTick('canal-diretos'))) +
        wzItem(v === 'yes' ? 'on' : v === 'blocked' ? 'off' : 'wait', t('painel.wz.verificado'), '<p class="hint">' + t('painel.wz.verificado_dica') + '</p>' +
          (c.verified ? said({ yes: t('painel.wz.verificado_sim'), no: t('painel.wz.verificado_nao'), blocked: t('painel.wz.verificado_bloqueado') }[c.verified]) : wzTick('canal-verificado'))) + '</ul>';
      if (pc && yc) html += '<div class="row" style="align-items:center"><button class="btn" type="button" data-wz-act="chk">' + t('painel.wz.perguntar') + '</button><span class="msg" id="wzChkMsg" role="status">' + esc(S.chkErr || '') + '</span></div>';
      else html += '<p class="hint">' + t('painel.wz.sem_yt') + '</p>';
    } else {
      html = '<p>' + t('painel.wz.ensaio_texto') + '</p><ul class="wz-list">' +
        ENSAIO.map(function (e) { return wzItem(wzTicked(e[0]) ? 'on' : 'wait', esc(e[1]), '<p class="hint">' + e[2] + '</p>' + wzTick(e[0])); }).join('') + '</ul>';
    }
    el.innerHTML = html;
  }

  function wzGo(i) {
    S.setupStep = i === 'fim' ? 'fim' : +i;
    renderInicio();
    if (S.setupStep === wzIndex('canal') && S.setup.pc && S.yt && S.yt.connected && !S.chk && !S.chkBusy) checkChannel();
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
        if (S.tab === 'inicio' && (S.setupStep === 'fim' || S.setupStep === wzIndex('canal'))) renderWzBody();
      });
  }

  function wzObs(btn, camera) {
    var msg = $('#wzObsMsg');
    btn.disabled = true; msg.className = 'msg'; msg.textContent = t('painel.obs.a_configurar');
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

  // Saves what the Idioma, Clube and Marcação steps ask, ticks the step and moves on.
  function wzSave(btn, act) {
    var msg = $('#wzSaveMsg'), set = S.settings, next = {};
    if (act === 'lang') next.lang = $('#wLang').value;
    if (act === 'club') {
      next.club = Object.assign({}, set.club, {
        name: $('#wHome').value.trim(), short: $('#wHomeShort').value.trim().toUpperCase(),
        primary: $('#wPrimary').value.toUpperCase(), secondary: $('#wSecondary').value.toUpperCase(),
        sport: $('#wSport').value, country: $('#wCountry').value
      });
    }
    if (act === 'input') {
      var pick = $('#wzBody input[name="wInput"]:checked');
      next.input = pick ? pick.value : 'console';
    }
    var id = { lang: 'idioma', club: 'clube', input: 'entrada' }[act];
    btn.disabled = true; msg.className = 'msg'; msg.textContent = '';
    api('PUT', '/api/settings', Object.assign({}, set, next)).then(function (saved) {
      var reload = act === 'lang' && saved.lang !== (set.lang || 'pt');
      S.settings = saved;
      return api('PUT', '/api/setup/steps/' + id, { state: 'done' }).then(function (f) {
        // Every page reads its words once, so a new language means a reload.
        if (reload) { location.reload(); return; }
        setupSaid(f);
        wzGo(act === 'input' ? S.setupStep : S.setupStep + 1);
      });
    }).then(null, function (err) {
      var m = $('#wzSaveMsg'); if (m) { m.className = 'msg err'; m.textContent = err.message; }
      btn.disabled = false;
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
      if (act === 'lang' || act === 'club' || act === 'input') { wzSave(b, act); return; }
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
      var tx = e.target; if (!tx.dataset || !tx.dataset.tick) return;
      tx.disabled = true;
      api('PUT', '/api/setup/steps/' + encodeURIComponent(tx.dataset.tick), { state: tx.checked ? 'done' : '' }).then(function (f) {
        setupSaid(f); renderWzBody();
      }, function (err) { tx.checked = !tx.checked; tx.disabled = false; showError(err); });
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
    el.innerHTML = '<div class="urlbox"><input id="aDir" readonly value="' + esc(dir) + '" aria-label="' + t('painel.ajuda.pasta_dados') + '">' +
      '<button class="btn dark" data-act="copy-dir">' + t('painel.copiar') + '</button><span class="msg" style="align-self:center"></span></div>';
  }
  function showHelp(id) {
    showTab('ajuda');
    var el = document.getElementById(id);
    if (el) el.scrollIntoView();
  }
  // A new GitHub issue with what a report always needs already filled in; the person adds the rest there.
  function feedbackURL() {
    var st = S.status || {}, set = S.settings || {};
    var body = t('painel.feedback.corpo', {
      versao: st.version || '?', sistema: navigator.userAgent,
      marcacao: set.input === 'manual' ? t('painel.marcar.telemovel') : t('painel.marcar.consola'),
      desporto: (set.club && set.club.sport) || 'basketball'
    });
    return 'https://github.com/kauredo/hallstream/issues/new?body=' + encodeURIComponent(body);
  }
  function ajudaEvents() {
    $('main').addEventListener('click', function (e) {
      var h = e.target.closest('[data-help]');
      if (h) { showHelp(h.dataset.help); return; }
      if (!e.target.closest('#ajuda')) return;
      var go = e.target.closest('[data-goto]');
      if (go) { e.preventDefault(); showTab(go.dataset.goto); return; }
      if (e.target.closest('[data-act="feedback"]')) { window.open(feedbackURL(), '_blank', 'noopener'); return; }
      var b = e.target.closest('[data-act="copy-dir"]');
      if (b) busy(b, function () { $('#aDir').select(); return copyText($('#aDir').value); }, t('painel.copiado'));
    });
  }

  /* ---------------- a newer release ---------------- */
  // Shown while /api/status names a newer release, until "Agora não" for that version (this browser only).
  function laterVersion() { try { return localStorage.getItem('hs-update-later') || ''; } catch (e) { return ''; } }
  function renderUpdateAsk() {
    var u = S.status && S.status.update, box = $('#updateAsk');
    box.hidden = !u || laterVersion() === u.version;
    if (box.hidden) return;
    $('#updateTxt').textContent = t('painel.atualizacao.pergunta', { versao: u.version });
    $('#updateLink').href = u.url;
  }
  function updateEvents() {
    $('#updateAsk').addEventListener('click', function (e) {
      if (!e.target.closest('[data-act="up-later"]')) return;
      try { localStorage.setItem('hs-update-later', S.status.update.version); } catch (err) { /* hide for now only */ }
      $('#updateAsk').hidden = true;
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
      busy(b, function () { return createShortcut(true); }, t('painel.definicoes.atalho_ok')).then(function (r) {
        if (!r) { delete box.dataset.closing; return; }
        $all('button', box).forEach(function (x) { x.disabled = true; });
        setTimeout(function () { box.hidden = true; delete box.dataset.closing; }, 2500);
      });
    });
  }

  /* ---------------- boot ---------------- */
  function boot() {
    $all('.nav [data-tab]').forEach(function (b) { b.addEventListener('click', function () { showTab(b.dataset.tab); }); });
    gameEvents(); plantelEvents(); manualEvents(); sponsorEvents(); promoEvents(); defsEvents(); destEvents(); caixaEvents(); shortcutEvents(); updateEvents(); ajudaEvents();
    transmissoesEvents(); ytEvents($('#definicoes'));
    streamEvents(); setupEvents();
    $('#chips').addEventListener('click', function (e) {
      var go = e.target.closest('[data-goto]');
      if (go) { e.preventDefault(); showTab(go.dataset.goto); }
    });
    window.addEventListener('resize', function () { fitPreview(); fitLook(); });
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
