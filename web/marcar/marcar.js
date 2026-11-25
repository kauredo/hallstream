// Hallstream scorer's page (/marcar): a phone held sideways at the hall's table keeps the score by hand.
// It shows the live state (/api/events) and sends each tap to POST /api/marcar; the clock runs in the
// program, so a phone that sleeps or reloads loses nothing. Until "Marcar daqui" the console stays on
// air and every other control waits (docs/CONTRACT.md, "Scorer's page").
// A long press on any + takes one away instead: points, a foul, a time-out.
(() => {
  'use strict';
  const t = window.t;
  const $ = (s) => document.querySelector(s);
  const $$ = (s) => [...document.querySelectorAll(s)];
  window.applyI18N();

  let st = null;     // the live state
  let score = null;  // the manual score (running, clock, length…) from /api/marcar
  let input = '';
  let undo = null;   // the action undo takes back next, from the program
  let pinWait = null;

  // ---------- talking to the program ----------
  async function api(method, body) {
    for (;;) {
      let res;
      try {
        res = await fetch('/api/marcar', {
          method, credentials: 'same-origin', cache: 'no-store',
          headers: body ? { 'Content-Type': 'application/json' } : {},
          body: body ? JSON.stringify(body) : undefined,
        });
      } catch {
        offline(true);
        throw new Error(t('marcar.sem_ligacao'));
      }
      if (res.status === 401) { await askPin(); continue; }
      offline(false);
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || t('marcar.sem_ligacao'));
      return data;
    }
  }
  function took(data) {
    input = data.input;
    score = data.score;
    undo = data.undo || null;
    render();
  }
  function askPin() {
    if (pinWait) return pinWait;
    const box = $('#pin'), field = $('#pinValue');
    $('#pinMsg').textContent = document.cookie.includes('pin=') ? t('marcar.pin.errado') : '';
    field.value = '';
    box.showModal();
    pinWait = new Promise((resolve) => {
      const done = () => {
        if (!/^\d{4}$/.test(field.value)) { $('#pinMsg').textContent = t('marcar.pin.quatro'); return; }
        document.cookie = `pin=${field.value}; path=/; SameSite=Strict`;
        box.close();
        pinWait = null;
        resolve();
        listen();
      };
      $('#pinForm').onsubmit = (e) => { e.preventDefault(); done(); };
      field.oninput = () => { if (field.value.length === 4) done(); };
    });
    return pinWait;
  }
  // The page is useless without the PIN, so the back gesture never closes its dialog.
  $('#pin').addEventListener('cancel', (e) => e.preventDefault());

  let es = null, lostTimer = 0;
  function listen() {
    if (es) es.close();
    es = new EventSource('/api/events');
    es.addEventListener('state', (e) => {
      clearTimeout(lostTimer);
      offline(false);
      st = JSON.parse(e.data);
      input = st.input || input;
      render();
    });
    es.onerror = () => {
      clearTimeout(lostTimer);
      lostTimer = setTimeout(() => offline(true), 3000);
    };
  }
  // Offline, the message stays until the connection is back; an error from a tap shows for a while.
  let isOffline = false, errTimer = 0;
  function offline(on) {
    if (on === isOffline) return;
    isOffline = on;
    document.body.classList.toggle('offline', on);
    banner();
  }
  let errMsg = '';
  function say(msg) {
    errMsg = msg;
    clearTimeout(errTimer);
    errTimer = setTimeout(() => { errMsg = ''; banner(); }, 5000);
    banner();
  }
  function banner() {
    const f = $('#faixa'), msg = errMsg || (isOffline ? t('marcar.sem_ligacao') : '');
    f.textContent = msg;
    f.hidden = !msg;
  }

  // ---------- drawing ----------
  const rgb = (hex) => { const m = /^#?([0-9a-f]{6})$/i.exec(hex || ''); return m ? [0, 2, 4].map((i) => parseInt(m[1].slice(i, i + 2), 16)) : null; };
  // Text on a team colour: whichever of Soalho claro and Marcador reads better on it.
  function ink(hex) {
    const c = rgb(hex);
    if (!c) return '#1C1A17';
    const lin = (v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; };
    const l = 0.2126 * lin(c[0]) + 0.7152 * lin(c[1]) + 0.0722 * lin(c[2]);
    return (l + 0.05) / 0.0607 > 1.05 / (l + 0.05) ? '#1C1A17' : '#F8EFDD';
  }
  const PAPER = '#F8EFDD';
  // The two halves must never look alike: a visitor with no colour, or one close to the club's, gets paper.
  function awayColour(home, away) {
    const h = rgb(home), a = rgb(away);
    if (!a || (h && Math.abs(h[0] - a[0]) + Math.abs(h[1] - a[1]) + Math.abs(h[2] - a[2]) < 120)) return PAPER;
    return away;
  }
  const setV = (k, v) => { for (const el of $$(`[data-v="${k}"]`)) if (el.textContent !== String(v)) el.textContent = v; };
  function marks(el, n, max) {
    const count = Math.max(max, n);
    if (el.childElementCount !== count) el.replaceChildren(...Array.from({ length: count }, () => document.createElement('i')));
    [...el.children].forEach((e, i) => e.classList.toggle('on', i < n));
    el.setAttribute('aria-hidden', 'true');
  }
  const clockText = (tenths) => {
    if (tenths < 600) return `${Math.floor(tenths / 10)}.${tenths % 10}`;
    const s = Math.ceil(tenths / 10);
    return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
  };
  // The sport's rules from game.rules; a missing field reads as basketball's.
  function rulesOf() {
    const r = (st && st.game && st.game.rules) || {};
    return {
      periods: r.periods ?? 4, half: r.half ?? 2, timeouts: r.timeouts ?? [2, 3, 1], bonus: r.foul_bonus ?? 5,
      foulMarks: r.foul_marks ?? 5, freeKick: r.foul_penalty === 'free_kick', word: r.period_word ?? 'quarter', overtimeLength: r.overtime_length ?? 0,
      sets: r.scoring === 'sets', setsToWin: r.sets_to_win ?? 3, setPoints: r.set_points ?? 25,
      lastSetPoints: r.last_set_points ?? 15, winBy: r.win_by ?? 2,
    };
  }
  // The same rule as sport.SetWon: the side that has the set's points with the lead, else ''.
  function setWon(r, n, home, away) {
    const target = n === 2 * r.setsToWin - 1 ? r.lastSetPoints : r.setPoints;
    if (home >= target && home - away >= r.winBy) return 'home';
    if (away >= target && away - home >= r.winBy) return 'away';
    return '';
  }
  // A regulation period in the sport's word, by rules.period_word.
  const regulationName = (word, n) => (word === 'half' ? t('marcar.parte', { n }) : t('marcar.periodo', { n }));
  function periodName(p, periods, word) {
    return p <= periods ? regulationName(word, `<b>${p}</b>`) : p === periods + 1 ? t('marcar.prolongamento') : t('marcar.prolongamento_n', { n: `<b>${p - periods}</b>` });
  }
  // A set's score as "25–20", or a dash when it has no points (a console only sends who won).
  const setScoreText = (h, a) => (h === 0 && a === 0 ? '–' : `${h}–${a}`);
  const sigla = (team) => ((st && st.game && st.game[team]) || {}).short || '';
  // What undo takes back, in the words of the tap.
  function undoLabel(a) {
    if (!a) return '';
    const s = sigla(a.team);
    switch (a.do) {
      case 'point': return `${a.n > 0 ? '+' : '−'}${Math.abs(a.n)} ${s}`;
      case 'foul': return a.n > 0 ? t('marcar.ultimo.falta', { sigla: s }) : t('marcar.ultimo.falta_menos', { sigla: s });
      case 'timeout': return a.n > 0 ? t('marcar.ultimo.desconto', { sigla: s }) : t('marcar.ultimo.desconto_menos', { sigla: s });
      case 'serve': return t('marcar.ultimo.servico', { sigla: s });
      case 'set_next': return t('marcar.ultimo.set');
      case 'period_next': case 'period_back': return t('marcar.ultimo.periodo');
      case 'clock_set': case 'clock_add': return t('marcar.ultimo.relogio');
      default: return '';
    }
  }
  const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]);

  let zeroSince = 0;
  function render() {
    if (!st) return;
    $('#mesa').hidden = false;
    const g = st.game || {}, l = st.live || {}, home = g.home || {}, away = g.away || {}, th = g.theme || {};
    const root = document.documentElement.style;
    const hc = th.red || home.color || '#2F4B7C', ac = awayColour(hc, away.color);
    root.setProperty('--casa', hc);
    root.setProperty('--sobre-casa', ink(hc));
    root.setProperty('--fora', ac);
    root.setProperty('--sobre-fora', ink(ac));
    for (const [k, tm] of [['home', home], ['away', away]]) {
      const short = tm.short || tm.initials || '', name = tm.name || '';
      setV(k + 'Short', short);
      // "CAS CASA" says the same thing twice: the name only shows when it says more.
      setV(k + 'Name', name.toUpperCase().startsWith(short.toUpperCase()) && name.length <= short.length + 2 ? '' : name);
    }
    setV('home', l.home ?? 0);
    setV('away', l.away ?? 0);
    const period = +l.period || (score && score.period) || 1;
    const rules = rulesOf();
    const { periods, half, timeouts, bonus, sets, foulMarks, word, freeKick } = rules;
    const penaltyWord = freeKick ? t('marcar.livre') : t('marcar.bonus');
    document.body.classList.toggle('sets', sets);
    document.body.classList.toggle('nofouls', foulMarks === 0);
    // A sets sport has the same timeouts in every set.
    const maxTO = sets || period <= half ? timeouts[0] : period <= periods ? timeouts[1] : timeouts[2];
    for (const tx of ['home', 'away']) {
      const f = +l[tx + '_fouls'] || 0, to = +l[tx + '_timeouts'] || 0;
      marks($(`[data-marks="${tx}Fouls"]`), f, foulMarks);
      marks($(`[data-marks="${tx}Timeouts"]`), to, maxTO);
      setV(tx + 'Fouls', f);
      setV(tx + 'Timeouts', to);
      setV(tx + 'Sets', +l[tx + '_sets'] || 0);
      const serving = sets && l.serve === tx, ring = $(`.equipa[data-team="${tx}"] .servico`);
      ring.classList.toggle('on', serving);
      const serveText = serving ? t('marcar.a_servir') : t('marcar.servico');
      if (ring.textContent !== serveText) ring.textContent = serveText;
      const fouls = $(`.equipa[data-team="${tx}"] .faltas`);
      fouls.toggleAttribute('data-bonus', bonus > 0 && f >= bonus);
      const warning = fouls.querySelector('.aviso');
      if (warning.textContent !== penaltyWord) warning.textContent = penaltyWord;
    }
    // The finished sets, as small scores in the middle strip.
    const finished = $('#feitos'), done = sets && Array.isArray(st.sets) ? st.sets : [];
    finished.hidden = !sets;
    // Heads for the two columns, in the order the scores are written (home first).
    const head = document.createElement('span');
    head.className = 'cab';
    head.textContent = `${sigla('home')} – ${sigla('away')}`;
    finished.replaceChildren(...(done.length ? [head] : []), ...done.map((x, i) => {
      const row = document.createElement('span'), k = document.createElement('i');
      k.textContent = x.set ?? i + 1;
      row.append(k, setScoreText(x.home, x.away));
      return row;
    }));

    const manual = input === 'manual';
    document.body.classList.toggle('consola', !manual);
    $('#estado').textContent = manual ? t('marcar.estado.manual') : t('marcar.estado.consola');
    $('#periodo').innerHTML = sets ? t('marcar.set', { n: `<b>${period}</b>` }) : periodName(period, periods, word);
    const running = !!(manual && score && score.running);
    document.body.classList.toggle('corre', running);
    $('#relogio').textContent = l.clock || (score ? clockText(score.clock) : '');
    // At 0:00 the button waits two seconds before it offers the next period, so a late Parar does nothing.
    const atZero = manual && score && !score.running && score.clock === 0;
    if (atZero && !zeroSince) { zeroSince = Date.now(); setTimeout(render, 2100); }
    if (!atZero) zeroSince = 0;
    const go = $('#arrancar');
    go.classList.toggle('seguinte', !!atZero && !sets);
    // The set can end once a side has the points with the lead; the button lights up to say so.
    const setOver = sets && !!setWon(rules, period, +l.home || 0, +l.away || 0) && (+l.home_sets || 0) < rules.setsToWin && (+l.away_sets || 0) < rules.setsToWin;
    go.classList.toggle('pronto', manual && setOver);
    // Why "Set seguinte" waits: the points the leading side still needs, with the lead the set takes.
    const why = $('#falta'), won = (+l.home_sets || 0) >= rules.setsToWin || (+l.away_sets || 0) >= rules.setsToWin;
    why.hidden = !(sets && manual);
    if (sets && manual) {
      const h = +l.home || 0, a = +l.away || 0;
      const target = period === 2 * rules.setsToWin - 1 ? rules.lastSetPoints : rules.setPoints;
      const need = Math.max(target - Math.max(h, a), rules.winBy - Math.abs(h - a), 1);
      why.textContent = won ? t('marcar.jogo_acabado') : setOver ? t('marcar.set_pronto') : need === 1 ? t('marcar.set_falta_um') : t('marcar.set_falta', { n: need });
    }
    if (!manual) { go.textContent = t('marcar.marcar_daqui'); go.dataset.do = 'take'; delete go.dataset.ask; go.disabled = false; }
    else if (sets) { go.textContent = t('marcar.set_seguinte'); go.dataset.do = ''; go.dataset.ask = 'set_next'; go.disabled = !setOver; }
    else if (atZero) { go.textContent = t('marcar.periodo_seguinte_botao'); go.dataset.do = ''; go.dataset.ask = 'period_next'; go.disabled = Date.now() - zeroSince < 2000; }
    else { go.textContent = running ? t('marcar.parar') : t('marcar.iniciar'); go.dataset.do = running ? 'clock_stop' : 'clock_start'; delete go.dataset.ask; go.disabled = false; }
    if (!atZero && !sets) delete go.dataset.ask;
    for (const b of $$('.equipa button, .seta, .acerto button, #relogio')) b.disabled = !manual;
    const u = $('#anular'), label = undoLabel(undo);
    u.disabled = !manual || !undo;
    u.innerHTML = label ? t('marcar.anular_isto', { o: `<b>${esc(label)}</b>` }) : t('marcar.anular');
    if ($('#acertar').open && score) $('#acertarClock').textContent = l.clock || clockText(score.clock);
  }

  // ---------- taps ----------
  const pending = new Set();
  let tagTimer = 0;
  function showTag(team, n) {
    for (const tx of ['home', 'away']) {
      const tag = $(`[data-last="${tx}"]`);
      tag.hidden = tx !== team;
      if (tx === team) tag.textContent = (n > 0 ? '+' : '−') + Math.abs(n);
    }
    clearTimeout(tagTimer);
    tagTimer = setTimeout(() => { for (const tag of $$('[data-last]')) tag.hidden = true; }, 4000);
  }
  async function send(a, what) {
    const key = JSON.stringify(a);
    if (pending.has(key)) return; // the same tap again while the first is on its way
    pending.add(key);
    if (a.do === 'point') showTag(a.team, a.n); // at once, so a slow network never looks like a missed tap
    try {
      took(await api('POST', a));
      if (navigator.vibrate) navigator.vibrate(12);
    } catch (err) {
      if (navigator.vibrate) navigator.vibrate([40, 60, 40]);
      if (a.do === 'point') for (const tag of $$('[data-last]')) tag.hidden = true;
      say(what ? t('marcar.nao_enviado', { o: what, erro: err.message }) : err.message);
    } finally {
      setTimeout(() => pending.delete(key), 350);
    }
  }
  function actionOf(b, negative) {
    const team = (b.closest('.equipa') || {}).dataset?.team;
    const a = { do: b.dataset.do };
    if (team) { a.team = team; a.n = (+b.dataset.n) * (negative ? -1 : 1); }
    if (b.dataset.tenths) a.tenths = +b.dataset.tenths;
    return a;
  }
  // A long press on a + takes one away; the click that follows the press is swallowed.
  let pressTimer = 0, pressed = null;
  document.addEventListener('pointerdown', (e) => {
    const b = e.target.closest('button[data-long]');
    if (!b || b.disabled) return;
    pressed = null;
    clearTimeout(pressTimer);
    pressTimer = setTimeout(() => {
      pressed = b;
      const a = actionOf(b, true);
      send(a, undoLabel(a));
    }, 550);
  });
  for (const ev of ['pointerup', 'pointercancel', 'pointerleave']) document.addEventListener(ev, () => clearTimeout(pressTimer));
  document.addEventListener('contextmenu', (e) => { if (e.target.closest('button[data-long]')) e.preventDefault(); });
  document.addEventListener('click', (e) => {
    const b = e.target.closest('button[data-do], button[data-ask]');
    if (!b || b.disabled) return;
    if (pressed === b) { pressed = null; return; }
    if (b.dataset.ask) return ask(b.dataset.ask);
    if (!b.dataset.do) return;
    const a = actionOf(b, false);
    send(a, undoLabel(a));
  });

  // Changing the period zeroes the team fouls, and a set can only be reopened with undo, so both ask first.
  function ask(what) {
    const box = $('#confirmar'), period = (st && +st.live?.period) || (score && score.period) || 1;
    if (what === 'set_next') {
      const l = (st && st.live) || {};
      $('#confirmarTitulo').textContent = t('marcar.confirmar.set', { n: period, casa: l.home ?? 0, fora: l.away ?? 0 });
      $('#confirmarDica').textContent = t('marcar.confirmar.set_dica');
      $('#confirmarSim').textContent = t('marcar.confirmar.acabar_set');
      $('#confirmarSim').onclick = () => { box.close(); send({ do: what }, t('marcar.ultimo.set')); };
      box.showModal();
      return;
    }
    const next = what === 'period_next';
    const { periods, word } = rulesOf();
    if (!next && period <= 1) return;
    $('#confirmarTitulo').innerHTML = next ? t('marcar.confirmar.seguinte', { periodo: periodName(period + 1, periods, word) }) : t('marcar.confirmar.anterior', { periodo: periodName(period - 1, periods, word) });
    $('#confirmarDica').textContent = next ? t('marcar.confirmar.seguinte_dica') : t('marcar.confirmar.anterior_dica');
    $('#confirmarSim').textContent = next ? t('marcar.confirmar.passar') : t('marcar.confirmar.voltar');
    $('#confirmarSim').onclick = () => { box.close(); send({ do: what }, t('marcar.ultimo.periodo')); };
    box.showModal();
  }
  $('#confirmarNao').addEventListener('click', () => $('#confirmar').close());

  // The length of the period on now: an overtime runs the sport's overtime length when it sets one.
  function periodLength() {
    const { periods, overtimeLength } = rulesOf();
    return score.period > periods && overtimeLength > 0 ? overtimeLength : score.length;
  }

  // A tap on the clock opens the steps; each one applies at once.
  $('#relogio').addEventListener('click', () => {
    if (!score) return;
    $('#acertarRepor').textContent = t('marcar.acertar.repor', { tempo: clockText(periodLength()) });
    $('#acertar').showModal();
    render();
  });
  $('#acertarRepor').addEventListener('click', () => score && send({ do: 'clock_set', tenths: periodLength() }, t('marcar.ultimo.relogio')));
  $('#acertarFechar').addEventListener('click', () => $('#acertar').close());

  // ---------- start ----------
  async function start() {
    try { took(await api('GET')); } catch (err) { say(err.message); }
    listen();
    // An open page keeps the manual input "connected" (Heartbeat on the server).
    setInterval(() => api('GET').then(took, () => {}), 15000);
  }
  start();
})();
