// Hallstream overlay. Reads the live state (docs/CONTRACT.md) and updates the DOM in place.
// Query: ?screen=<name> forces a screen, ?state=<url> loads a static JSON instead of SSE,
// ?preview=1|foto puts a background behind the transparent page, ?now=<iso> fixes the clock and
// ?anim=0 turns transitions and animations off, so everything draws in its final state (both for tests).
// Screenshots of the live touches: ?pop=home:3 (or away:1…3) shows the points plate and ?corner=<n> shows
// sponsor n in the sponsor corner. Promo: ?vt=<seconds> shows the clip paused at that second, and
// ?painel=1 (the panel's preview) keeps every clip silent and never reports a clip's end, and takes the setup the panel
// posts (see look()); ?rodape=1 adds a sample lower third. The setup itself: ?style= ?bug= ?lt= ?margin= ?scale= ?accent= ?hide=
(() => {
  'use strict';

  const q = new URLSearchParams(location.search);
  const forcedScreen = q.get('screen');
  const stateURL = q.get('state');
  const fixedNow = q.get('now') ? Date.parse(q.get('now')) : NaN;
  const now = () => (isNaN(fixedNow) ? Date.now() : fixedNow);
  if (q.get('preview')) document.documentElement.dataset.preview = q.get('preview');
  const ANIM = q.get('anim') !== '0';
  if (!ANIM) {
    document.documentElement.style.setProperty('--fade', '0ms');
    document.documentElement.classList.add('noanim');
  }
  const reduced = matchMedia('(prefers-reduced-motion: reduce)');
  const forcedPop = /^(home|away):([123])$/.exec(q.get('pop') || '');
  const forcedCorner = parseInt(q.get('corner'), 10) || 0;

  const SCREENS = ['marcador', 'abertura', 'plantel', 'desconto', 'intervalo', 'final', 'apoiantes', 'proximo', 'voltamos', 'promo', 'nada'];
  // Full screens sit on the pavilion backdrop; most of them also carry the sponsor strip.
  const BG_SCREENS = new Set(['abertura', 'plantel', 'apoiantes', 'intervalo', 'final', 'proximo', 'voltamos', 'promo']);
  const DIM_SCREENS = new Set(['intervalo', 'final']);
  const STRIP_SCREENS = new Set(['abertura', 'plantel', 'intervalo', 'final', 'proximo', 'voltamos']);
  const LT_SCREENS = new Set(['marcador', 'nada']);
  // The credit shows only where the bottom-right corner is free: the strip and the time-out window fill it elsewhere.
  const CREDIT_SCREENS = new Set(['marcador', 'apoiantes', 'promo']);
  const SQUAD_MAX = 12;
  const TZ = 'Europe/Lisbon';

  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];
  const root = document.documentElement;

  // ---------- small DOM helpers: write only when the value changes ----------
  function setText(el, v) {
    v = v == null ? '' : String(v);
    if (el._v === v) return;
    el._v = v;
    // A dotted pair ("Campeonato · Jornada 5") gets its dot in an <i class="sep">, so a style can draw it its own way.
    const parts = v.split(' · ');
    if (parts.length > 1) {
      el.replaceChildren(...parts.flatMap((p, i) => {
        if (!i) return [p];
        const sep = document.createElement('i');
        sep.className = 'sep';
        sep.textContent = ' · ';
        return [sep, p];
      }));
    } else el.textContent = v;
    if (el.dataset.fit) fit(el);
  }
  function setClass(el, cls, on) {
    if (el.classList.contains(cls) !== !!on) el.classList.toggle(cls, !!on);
  }
  // Shrink a single-line element's font until its text fits its box (names up to 20 characters).
  // With data-alt, a name that would go below data-fit-min px shows the alternative (the sigla).
  function fit(el) {
    const base = +el.dataset.fit;
    const shrink = () => {
      let size = base;
      el.style.fontSize = size + 'px';
      const min = Math.round(base * 0.55);
      while (el.scrollWidth > el.clientWidth + 0.5 && size > min) {
        size -= 1;
        el.style.fontSize = size + 'px';
      }
      return size;
    };
    if (el.dataset.alt) el.textContent = el._v || '';
    if (shrink() < +el.dataset.fitMin && el._alt) {
      el.textContent = el._alt;
      shrink();
    }
  }

  // ---------- one-time DOM setup ----------
  const binds = {};
  for (const el of $$('[data-b]')) (binds[el.dataset.b] ||= []).push(el);

  const tiles = $$('[data-tile]');
  for (const tx of tiles) {
    const img = document.createElement('img');
    img.alt = '';
    img.addEventListener('error', () => { tx.classList.remove('has-img'); tx.classList.add('no-img'); });
    img.addEventListener('load', () => { tx.classList.add('has-img'); tx.classList.remove('no-img'); });
    const ini = document.createElement('b');
    ini.className = 'ini';
    tx.append(img, ini);
  }

  const plantel = $('#s-plantel');
  const coachBox = $('.coach');
  const cards = [];
  for (let i = 0; i < SQUAD_MAX; i++) {
    const card = document.createElement('div');
    card.className = 'card';
    card.innerHTML = '<div class="ph"><b class="ini"></b><img alt=""></div><div class="no"></div><div class="nm"><small></small><b></b></div>';
    coachBox.before(card);
    const photo = card.firstChild;
    const img = photo.lastChild;
    img.addEventListener('load', () => photo.classList.add('has-img'));
    img.addEventListener('error', () => photo.classList.remove('has-img'));
    const [first, last] = card.lastChild.children;
    cards.push({ card, photo, num: card.children[1], first, last });
  }

  const screens = {};
  for (const s of $$('.screen')) screens[s.dataset.screen] = s;

  const bg = $('#bg');
  const strip = $('#strip');
  const sponsorBoxes = $$('[data-spons]');
  const bugPos = $('.sb-pos');
  const bugBox = $('.sb');

  const lt = $('#lt');
  const ltBody = $('.lt-body', lt);
  const ltTab = $('.lt-tab', lt);
  const ltNum = $('[data-lt="number"]');
  const ltSmall = $('[data-lt="small"]');
  const ltBig = $('[data-lt="big"]');
  ltBig.dataset.fit = '48';
  const ltLabel = $('[data-lt="label"]');
  const ltImg = $('.lt-photo img');
  ltImg.addEventListener('error', () => { ltImg._bad = ltImg.getAttribute('src'); setClass(lt, 'photo', false); });

  const periodTables = $$('[data-periods]');
  const bonusCells = { home: $('[data-bonus="home"]'), away: $('[data-bonus="away"]') };
  const bugRows = { home: $('.sb-row.home'), away: $('.sb-row.away') };
  const pops = { home: $('[data-pop="home"]'), away: $('[data-pop="away"]') };
  const spc = $('#spc');
  const spcLogo = $('.spc-lg', spc);
  const spcBar = $('.spc-prog b', spc);
  const winners = { home: $('[data-win="home"]'), away: $('[data-win="away"]') };
  const credit = $('#credit');

  // ---------- motion ----------
  // Every entrance runs from a hidden first frame to the element's plain CSS (fill: backwards), so a
  // cancelled or finished animation always leaves the final state. Each screen keeps its running
  // animations in a list; entering the screen again cancels the list first. Leaving a screen only
  // fades the section out, so nothing is left half-drawn. Timing comes from the tokens in overlay.css.
  const css = getComputedStyle(root);
  const tok = (n) => css.getPropertyValue(n).trim();
  const E = { out: tok('--ease-out'), in: tok('--ease-in'), inOut: tok('--ease-in-out'), pop: tok('--ease-pop') };
  const T = {};
  for (const k of ['exit', 'quick', 'enter', 'long', 'count', 'squeeze']) T[k] = parseFloat(tok('--t-' + k)) || 0;
  T.stagger = parseFloat(tok('--stagger')) || 0;
  const WIN = { x: parseFloat(tok('--win-x')), y: parseFloat(tok('--win-y')), w: parseFloat(tok('--win-w')), h: parseFloat(tok('--win-h')) };
  const CANVAS = { x: 0, y: 0, w: 1920, h: 1080 };
  const PHOTO = q.get('preview') === 'foto';
  const TAN = Math.tan((12 * Math.PI) / 180); // the kit diagonal

  // With reduced motion every move or wipe becomes a plain fade.
  function play(list, el, frames, { dur = T.enter, delay = 0, ease = E.out, exit = false } = {}) {
    if (!ANIM || !el) return null;
    if (reduced.matches && frames.some((f) => f.transform || f.clipPath)) {
      frames = exit ? [{ opacity: 1 }, { opacity: 0 }] : [{ opacity: 0 }, { opacity: 1 }];
    }
    const a = el.animate(frames, { duration: dur, delay, easing: ease, fill: exit ? 'forwards' : 'backwards' });
    list.push(a);
    return a;
  }
  const stop = (list) => { for (const a of list.splice(0)) a.cancel(); };

  const poly = (pts) => `polygon(${pts.map(([x, y]) => `${x}px ${y}px`).join(',')})`;
  const fade = () => [{ opacity: 0 }, { opacity: 1 }];
  const move = (x, y = 0) => [{ transform: `translate(${x}px,${y}px)` }, { transform: 'none' }];
  const slide = (x) => [{ opacity: 0, transform: `translateX(${x}px)` }, { opacity: 1, transform: 'none' }];
  const rise = (y, base = '') => [{ opacity: 0, transform: `${base} translateY(${y}px)` }, { opacity: 1, transform: base || 'none' }];
  const pop = (s, base = '') => [{ opacity: 0, transform: `${base} scale(${s})` }, { opacity: 1, transform: base || 'none' }];
  // Reveal a w x h box left to right behind a leading edge at the kit angle.
  const wipe = (w, h) => {
    const dx = h * TAN;
    return [{ clipPath: poly([[0, 0], [0, 0], [-dx, h], [0, h]]) }, { clipPath: poly([[0, 0], [w + dx, 0], [w, h], [0, h]]) }];
  };


  // Count the scores up from 0 on a screen's entrance. The target is read every frame from the
  // value render() stored, so a score that changes meanwhile still ends right.
  function countUp(list, els, delay) {
    if (!ANIM || reduced.matches) return;
    els = els.filter((el) => el._v != null && el._v !== '');
    let raf = 0, timer = 0;
    const t0 = performance.now() + delay;
    const done = () => { cancelAnimationFrame(raf); clearTimeout(timer); for (const el of els) el.textContent = el._v; };
    const tick = (tx) => {
      const k = Math.min(1, Math.max(0, (tx - t0) / T.count));
      if (k >= 1) return done();
      const e = 1 - Math.pow(1 - k, 3);
      for (const el of els) el.textContent = String(Math.round((+el._v || 0) * e));
      raf = requestAnimationFrame(tick);
    };
    tick(performance.now());
    timer = setTimeout(done, delay + T.count + 250); // in case frames are throttled
    list.push({ cancel: done });
  }

  // The time-out squeeze. When the program moves the camera in OBS (state.camera) or the preview
  // photo stands in for it, the window shrinks from where the picture sits at full screen into
  // the time-out window, on the same curve and time as the camera, and opens back out on exit.
  // A turn while it runs starts from wherever the window is.
  const hole = $('.to-hole');
  const toBg = $('.to-bg');
  const cam = $('#cam');
  let camera = null;
  const squeezing = () => !!camera || PHOTO;
  const box = (r) => ({ left: r.x + 'px', top: r.y + 'px', width: r.w + 'px', height: r.h + 'px' });
  // The background with a hole where the picture is (outer box, then inner box, even-odd).
  const holed = (r) => ({ clipPath: `polygon(evenodd, 0px 0px, 1920px 0px, 1920px 1080px, 0px 1080px, 0px 0px, ${r.x}px ${r.y}px, ${r.x + r.w}px ${r.y}px, ${r.x + r.w}px ${r.y + r.h}px, ${r.x}px ${r.y + r.h}px, ${r.x}px ${r.y}px)` });
  const squeezeAnims = [];
  let squeezeSeq = 0;
  function squeeze(into) {
    const inner = camera ? camera.window : WIN, outer = camera ? camera.full : CANVAS;
    const target = into ? inner : outer, other = into ? outer : inner;
    let from = other;
    if (squeezeAnims.length) {
      const c = getComputedStyle(hole);
      from = { x: parseFloat(c.left), y: parseFloat(c.top), w: parseFloat(c.width), h: parseFloat(c.height) };
    }
    stop(squeezeAnims);
    const span = Math.abs(target.w - other.w);
    const dur = T.squeeze * (span ? Math.min(1, Math.abs(target.w - from.w) / span) : 1);
    const seq = ++squeezeSeq;
    let a = null;
    for (const el of PHOTO ? [hole, cam] : [hole]) {
      a = play(squeezeAnims, el, [box(from), box(target)], { dur, ease: E.inOut, exit: true });
    }
    // With reduced motion play() turns this into a fade: right on the way out, a flash on the way in.
    if (!(reduced.matches && into)) play(squeezeAnims, toBg, [holed(from), holed(target)], { dur, ease: E.inOut, exit: true });
    // Into the window the CSS already holds the end state; the way out is cleared by leaveDesconto.
    if (a && into) a.onfinish = () => { if (seq === squeezeSeq) stop(squeezeAnims); };
    return dur;
  }
  function leaveDesconto() {
    const s = screens.desconto;
    const list = (screenAnims.desconto ||= []);
    stop(list);
    setClass(s, 'leaving', true);
    for (const el of $$('.to-head, .to-u, .to-time, .to-strip', s)) {
      play(list, el, [{ opacity: 1 }, { opacity: 0 }], { dur: T.exit, ease: E.in, exit: true });
    }
    const dur = squeeze(false);
    const seq = squeezeSeq;
    setTimeout(() => {
      if (seq !== squeezeSeq) return;
      // The window is open again, so the frame shows nothing: drop it at once, then the fills.
      s.style.transition = 'none';
      setClass(s, 'leaving', false);
      void s.offsetWidth;
      s.style.transition = '';
      stop(list);
      stop(squeezeAnims);
    }, dur + 30);
    return dur * 0.6;
  }

  // Each screen enters in beats, and each beat settles before the next one starts. The backdrop
  // and the sponsor strip come in once and stay while one full screen follows another.
  const STRIP_DELAY = { abertura: 1700, plantel: 1300, intervalo: 1500, final: 1700, proximo: 1700, voltamos: 1100 };
  const lockup = (s, p) => p($('.lockup', s), rise(-16), { dur: T.enter, delay: 100 });
  const CHOREO = {
    abertura(s, p) {
      lockup(s, p);
      p($('.ab-kick', s), pop(0.85), { dur: T.quick + 100, delay: 300, ease: E.pop });
      p($('.ab-slash', s), [{ clipPath: 'inset(0 0 100% 0)' }, { clipPath: 'inset(0)' }], { dur: T.enter, delay: 350, ease: E.inOut });
      $$('.ab-shield', s).forEach((el, i) => p(el, pop(0.82), { delay: 600 + i * 100 }));
      $$('.ab-name', s).forEach((el, i) => p(el, rise(40), { delay: 680 + i * 100 }));
      p($('.ab-vs', s), pop(0.5, 'skewX(-12deg)'), { dur: T.quick + 100, delay: 950, ease: E.pop });
      $$('.ab-sub', s).forEach((el, i) => p(el, rise(20), { dur: T.quick + 100, delay: 1050 + i * 100 }));
      p($('.ab-info', s), rise(40), { delay: 1300 });
    },
    proximo(s, p) {
      CHOREO.abertura(s, p);
      p($('.px-where', s), pop(0.8, 'skewX(-12deg)'), { dur: T.quick + 100, delay: 1050, ease: E.pop });
    },
    // Calm on purpose: a break is no moment for a show. The halo behind the crest breathes in CSS.
    voltamos(s, p) {
      lockup(s, p);
      p($('.vj-crest', s), fade(), { dur: T.enter, delay: 250 });
      p($('.vj-title', s), rise(30, 'skewX(-6deg)'), { dur: T.enter, delay: 400 });
      p($('.vj-sub', s), fade(), { dur: T.enter, delay: 700 });
      p($('.vj-score', s), rise(20), { dur: T.enter, delay: 850 });
    },
    plantel(s, p) {
      p($('.pl-head', s), slide(-40), { delay: 100 });
      const shown = cards.filter((c) => c.card.style.display !== 'none');
      const photos = !s.classList.contains('nophoto');
      shown.forEach((c, i) => {
        const d = 350 + i * T.stagger;
        p(c.card, rise(30), { dur: T.quick + 150, delay: d });
        if (photos) p(c.photo, fade(), { dur: T.quick, delay: d + 150 });
      });
      p(coachBox, rise(30), { dur: T.quick + 150, delay: 350 + shown.length * T.stagger });
    },
    apoiantes(s, p) {
      p($('.ap-crest', s), pop(0.8), { delay: 100 });
      lockup(s, p);
      p($('.t1', s), rise(40, 'skewX(-6deg)'), { delay: 300 });
      p($('.t2', s), rise(40, 'skewX(-6deg)'), { delay: 420 });
      p($('.rules', s), fade(), { dur: T.quick, delay: 650 });
      const apStrip = $('.ap-strip', s);
      p(apStrip, wipe(1420, 210), { dur: T.long, delay: 800 });
      $$('.sp', apStrip).forEach((el, i) => p(el, rise(24), { dur: T.quick + 100, delay: 1200 + i * T.stagger * 2 }));
    },
    desconto(s, p) {
      ++squeezeSeq; // stops a way out that is still running
      setClass(s, 'leaving', false);
      let b = 0;
      if (squeezing()) {
        // Beat 1: the camera and the frame together. The rest starts as the squeeze settles.
        b = squeeze(true) * 0.8;
      } else {
        // The camera does not move: the background comes in from the left and the top, then fills the right and bottom edges.
        const L = (l, tx) => poly([[0, 0], [1920, 0], [1920, tx], [l, tx], [l, 1080], [0, 1080]]);
        p($('.to-frame', s), [
          { clipPath: L(0, 0), easing: E.out },
          { clipPath: L(WIN.x, WIN.y), easing: E.inOut, offset: 0.6 },
          { clipPath: L(1920, 1080) },
        ], { dur: T.long - 100, ease: 'linear' });
        b = 300;
      }
      p($('.to-head', s), slide(-40), { dur: T.quick + 100, delay: b });
      $$('.to-u', s).forEach((el, i) => p(el, slide(-80), { delay: b + 150 + i * 120 }));
      p($('.to-time', s), rise(20), { dur: T.quick + 100, delay: b + 450 });
      p($('.to-strip', s), rise(40), { delay: b + 550 });
      p($('.to-mini', s), rise(30), { delay: b + 650 });
    },
    promo(s, p) {
      const h = s.classList.contains('h');
      p($('.pr-vid', s), h ? wipe(1280, 720) : rise(40), { dur: h ? T.long : T.enter, delay: 100 });
      p($('.pr-top', s), rise(-16), { delay: 200 });
      p($('.pr-in', s), rise(-16), { delay: 200 });
      p($('.pr-kick', s), pop(0.85, 'skewX(-14deg)'), { dur: T.quick + 100, delay: 350, ease: E.pop });
      p($('.pr-head', s), rise(40), { delay: 450 });
      p($('.pr-line', s), rise(20), { dur: T.quick + 100, delay: 600 });
      p($('.pr-qr', s), pop(0.9), { dur: T.quick + 100, delay: 750 });
      p($('.pr-bar', s), fade(), { dur: T.quick, delay: 800 });
      p($('.pr-left', s), fade(), { dur: T.quick, delay: 850 });
    },
    intervalo(s, p, list, base) {
      lockup(s, p);
      p($('.iv-tab', s), rise(28), { delay: 250 });
      p($('.iv-block', s), wipe(1093, 320), { dur: T.long, delay: 450 });
      countUp(list, $$('[data-b$="Score"]', s), base + 900);
      p($('.iv-ps', s), rise(16), { dur: T.quick + 100, delay: 1300 });
    },
    final(s, p, list, base) {
      lockup(s, p);
      p($('.fn-tab', s), rise(28), { delay: 250 });
      p($('.fn-block', s), wipe(1093, 320), { dur: T.long, delay: 450 });
      $$('.blk-nm > div', s).forEach((el, i) => p(el, rise(16), { delay: 900 + i * 120 }));
      countUp(list, $$('[data-b$="Score"]', s), base + 900);
      p($('.fn-ps', s), rise(16), { dur: T.quick + 100, delay: 1350 });
      p($('.fn-thanks', s), fade(), { dur: T.enter, delay: 1500 });
      for (const w of Object.values(winners)) p(w, pop(0.4), { dur: T.quick + 150, delay: 1900, ease: E.pop });
    },
  };
  const screenAnims = {};
  // base holds the entrance back while the previous screen is still on its way out.
  function enterScreen(name, base = 0) {
    const list = (screenAnims[name] ||= []);
    stop(list);
    const p = (el, f, o = {}) => play(list, el, f, { ...o, delay: (o.delay || 0) + base });
    if (ANIM && CHOREO[name]) CHOREO[name](screens[name], p, list, base);
  }
  const bgAnims = [];
  function enterBackdrop(delay) {
    stop(bgAnims);
    play(bgAnims, $('.ph', bg), [{ transform: 'scale(1.06)' }, { transform: 'none' }], { dur: T.long * 2, delay });
    play(bgAnims, $('.brush', bg), [{ clipPath: 'inset(0 0 0 100%)' }, { clipPath: 'inset(0)' }], { dur: T.long, delay: delay + 300, ease: E.inOut });
  }
  const stripAnims = [];
  function enterStrip(delay) {
    stop(stripAnims);
    play(stripAnims, strip, move(0, 140), { delay });
  }

  // Live updates on the scoreboard and the time-out screen.
  function rollScore(span, old) {
    if (!ANIM) return;
    const cell = span.parentElement;
    if (cell._fx) stop(cell._fx);
    const fx = (cell._fx = []);
    const flash = document.createElement('i');
    flash.className = 'fx-flash';
    cell.prepend(flash);
    const clean = () => flash.remove();
    const f = play(fx, flash, [{ opacity: 0 }, { opacity: 0.55, offset: 0.25 }, { opacity: 0 }], { dur: 420, ease: 'ease-out' });
    f.onfinish = f.oncancel = clean;
    if (reduced.matches) return;
    const ghost = document.createElement('span');
    ghost.className = 'fx-ghost';
    ghost.textContent = old;
    cell.append(ghost);
    const g = play(fx, ghost, [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'translateY(-70%)' }], { dur: T.quick, ease: E.in, exit: true });
    g.onfinish = g.oncancel = () => ghost.remove();
    play(fx, span, [{ opacity: 0, transform: 'translateY(70%)' }, { opacity: 1, transform: 'none' }], { dur: T.quick + 80, delay: 60 });
  }
  const pipFx = [];
  function popPip(pip) {
    if (pipFx.length > 16) pipFx.splice(0, 8);
    if (!reduced.matches) play(pipFx, pip, [{ transform: 'scaleY(0)' }, { transform: 'scaleY(1.25)', offset: 0.6 }, { transform: 'none' }], { dur: T.quick + 40, ease: E.out });
  }
  const liveFx = [];
  function nudge(el, frames, opts) {
    if (liveFx.length > 16) liveFx.splice(0, 8);
    play(liveFx, el, frames, opts);
  }
  // The points just scored: the plate pops in by the score cell, holds, then fades. A new basket
  // while it shows replaces it at once, whichever team scored.
  const popFx = [];
  function showPop(team, pts) {
    stop(popFx);
    const el = pops[team];
    setText(el.firstChild.firstChild, '+' + pts);
    const inn = 350, hold = 1200, total = inn + hold + T.exit;
    const a = inn / total, b = (inn + hold) / total;
    const frames = reduced.matches ? [
      { opacity: 0 }, { opacity: 1, offset: a }, { opacity: 1, offset: b, easing: E.in }, { opacity: 0 },
    ] : [
      { opacity: 0, transform: 'scale(0.4)', easing: E.out },
      { opacity: 1, transform: 'scale(1.12) rotate(-4deg)', offset: a * 0.55, easing: E.pop },
      { opacity: 1, transform: 'none', offset: a },
      { opacity: 1, transform: 'none', offset: b, easing: E.in },
      { opacity: 0, transform: 'none' },
    ];
    popFx.push(el.animate(frames, { duration: total, easing: 'linear' }));
  }

  // ---------- formatting ----------
  const fmtTime = new Intl.DateTimeFormat(t('data.locale'), { hour: '2-digit', minute: '2-digit', hour12: false, timeZone: TZ });
  const fmtWeekday = new Intl.DateTimeFormat(t('data.locale'), { weekday: 'long', timeZone: TZ });
  const fmtDay = new Intl.DateTimeFormat(t('data.locale'), { day: 'numeric', timeZone: TZ });
  const fmtMonth = new Intl.DateTimeFormat(t('data.locale'), { month: 'short', timeZone: TZ });
  const plain = (x) => String(x || '').normalize('NFD').replace(/[\u0300-\u036f]/g, '').toUpperCase().replace(/\s+/g, ' ').trim();

  function dateLine(d) {
    const wd = fmtWeekday.format(d);
    const month = fmtMonth.format(d).replace('.', '');
    return t('overlay.data_linha', { dia_semana: wd.charAt(0).toUpperCase() + wd.slice(1), dia: fmtDay.format(d), mes: month });
  }
  function countdown(ms) {
    const mins = Math.ceil(ms / 60000);
    if (mins < 60) return t('overlay.abertura.comeca_min', { min: mins });
    const h = Math.floor(mins / 60), m = mins % 60;
    return m ? t('overlay.abertura.comeca_h_min', { h: h, min: m }) : t('overlay.abertura.comeca_h', { h: h });
  }
  function periodNum(p) {
    const n = parseInt(p, 10);
    return isNaN(n) ? null : n;
  }
  const joinDot = (...parts) => parts.filter((x) => x != null && String(x).trim() !== '').join(' · ');
  // "Nome Apelido Outro" → first "Nome", last "Apelido Outro"; a single word is the last name.
  function splitName(name) {
    const w = String(name || '').trim().split(/\s+/).filter(Boolean);
    return { full: w.join(' '), first: w.length > 1 ? w[0] : '', last: w.length > 1 ? w.slice(1).join(' ') : (w[0] || '') };
  }

  // ---------- derive what every screen shows ----------
  function view(st) {
    const live = st.live || {};
    const game = st.game || {};
    const home = game.home || {};
    const away = game.away || {};
    const pn = periodNum(live.period);
    const v = {};
    v.homeName = home.name || live.home_name || '';
    v.homeShort = home.short || v.homeName;
    v.awayName = away.name || live.away_name || '';
    v.homeShort = home.short || '';
    v.awayShort = away.short || '';
    v.sport = game.sport_label || t('overlay.desporto_omissao');
    v.homeScore = live.home ?? 0;
    v.awayScore = live.away ?? 0;
    v.clock = live.clock || '';
    v.shot = live.shot ?? '';
    v.homeFouls = live.home_fouls ?? 0;
    v.awayFouls = live.away_fouls ?? 0;
    v.periodShort = pn == null ? (live.period || '') : pn <= 4 ? t('overlay.periodo.curto', { n: pn }) : pn === 5 ? t('overlay.periodo.prol') : t('overlay.periodo.prol_n', { n: pn - 4 });
    v.periodLong = pn == null ? (live.period || '') : pn <= 4 ? t('overlay.periodo.longo', { n: pn }) : pn === 5 ? t('overlay.periodo.prolongamento') : t('overlay.periodo.prolong_n', { n: pn - 4 });
    v.compRound = joinDot(game.competition, game.round);
    v.venue = game.venue || '';
    const coach = splitName((game.group && game.group.coach) || '');
    v.coach = coach.full;
    v.coachFirst = coach.first;
    v.coachLast = coach.last;
    v.groupName = (game.group && game.group.name) || '';

    const start = game.start ? new Date(game.start) : null;
    const startOK = start && !isNaN(start);
    v.startTime = startOK ? fmtTime.format(start) : '';
    if (startOK) {
      const left = start.getTime() - now();
      v.startLine = left > 0 && left < 12 * 3600e3 ? countdown(left) : dateLine(start);
    } else {
      v.startLine = '';
    }

    // Próximo jogo: weekday ("SEXTA"), day ("20 NOV") and time in Lisbon.
    const next = game.next || null;
    const ns = next && next.start ? new Date(next.start) : null;
    const nsOK = ns && !isNaN(ns);
    v.next = next;
    v.nextName = next ? (next.opponent && next.opponent.name) || '' : '';
    v.nextFull = next && plain(next.full_name) !== plain(v.nextName) ? next.full_name : '';
    v.nextWhere = next ? (next.at_home ? t('overlay.proximo.em_casa') : t('overlay.proximo.fora')) : '';
    v.nextWeekday = nsOK ? fmtWeekday.format(ns).replace(/-feira$/, '').toUpperCase() : '';
    v.nextDay = nsOK ? (fmtDay.format(ns) + ' ' + fmtMonth.format(ns).replace('.', '')).toUpperCase() : '';
    v.nextTime = nsOK ? fmtTime.format(ns) : '';
    v.nextComp = next ? joinDot(next.competition, next.round) : '';
    v.nextVenue = next ? next.venue || '' : '';
    v.backLine = ((game.overlay || {}).back_line || '').trim();
    // Voltamos já shows the score only while a game is going on.
    v.inPlay = !!(st.box && st.box.connected) && (v.homeScore > 0 || v.awayScore > 0 || (st.periods || []).length > 0);

    const periods = Array.isArray(st.periods) ? st.periods : [];
    const lastDone = periods.length ? periods[periods.length - 1].period : 2;
    v.halfLine = t('overlay.intervalo.a_seguir', { n: lastDone + 1 });
    v.periods = periods;
    v.home = home;
    v.away = away;
    return v;
  }

  // ---------- render ----------
  let state = null;
  let shownScreen = null;
  let periodsKey = null;
  let sponsorsKey = null;

  function renderTile(el, team) {
    const crest = team.crest_url || '';
    const ini = team.initials || (team.short || team.name || '').slice(0, 3).toUpperCase();
    const key = crest + '|' + ini;
    if (el._k === key) return;
    el._k = key;
    const img = el.firstChild;
    el.lastChild.textContent = ini;
    if (crest) {
      el.classList.remove('no-img');
      if (img.getAttribute('src') !== crest) img.src = crest;
      if (img.complete && img.naturalWidth) el.classList.add('has-img');
    } else {
      img.removeAttribute('src');
      el.classList.remove('has-img');
      el.classList.add('no-img');
    }
  }

  function renderPips(team, n) {
    for (const p of $$(`[data-pips="${team}"]`)) {
      const on = Math.min(n, 4);
      [...p.children].forEach((i, k) => setClass(i, 'on', k < on));
      setClass(p, 'hot', n >= 4);
    }
    for (const el of $$(`[data-hot="${team}"]`)) setClass(el, 'hot', n >= 4);
  }

  // The points of each completed period, from Go, the lower of the two dimmed; the total is the score now.
  function renderPeriods(periods, v) {
    const key = JSON.stringify([periods, v.homeName, v.awayName, v.homeScore, v.awayScore]);
    if (key === periodsKey) return;
    periodsKey = key;
    const n = periods.length;
    for (const tab of periodTables) {
      tab.textContent = '';
      setClass(tab, 'none', n === 0);
      tab.style.setProperty('--cols', n);
      // Overtime adds columns: they narrow so the names keep about 250px.
      tab.style.setProperty('--colw', Math.min(112, Math.floor(594 / Math.max(n, 1))) + 'px');
      const cell = (cls, text) => {
        const d = document.createElement('div');
        d.className = cls;
        d.textContent = text;
        tab.append(d);
        return d;
      };
      cell('hd', '');
      for (const p of periods) {
        if (p.period <= 4) cell('hd', t('overlay.periodos.n', { n: p.period }));
        else cell('hd ot', p.period === 5 ? t('overlay.periodo.prol') : t('overlay.periodo.prol_n', { n: p.period - 4 }));
      }
      cell('hd tot', t('overlay.periodos.total'));
      for (const [tx, o] of [['home', 'away'], ['away', 'home']]) {
        const name = cell('tn', '');
        name.dataset.fit = '26';
        setText(name, v[tx + 'Name']);
        for (const p of periods) {
          const pts = p[tx + '_points'] ?? 0;
          cell(pts < (p[o + '_points'] ?? 0) ? 'lo' : '', pts);
        }
        cell('tot', v[tx + 'Score']);
      }
    }
  }

  // Every sponsor of the game, in the club's order (the principal first), in each strip. A logo
  // that is missing or fails to load shows the name instead.
  function renderSponsors(sp) {
    const list = [];
    if (sp && sp.principal) list.push(sp.principal);
    if (sp && Array.isArray(sp.others)) list.push(...sp.others);
    cornerList = list;
    const key = JSON.stringify(list);
    if (key === sponsorsKey) return;
    sponsorsKey = key;
    for (const boxEl of sponsorBoxes) {
      boxEl.textContent = '';
      setClass(boxEl, 'none', list.length === 0);
      list.forEach((x, i) => {
        if (i) boxEl.append(Object.assign(document.createElement('i'), { className: 'dv' }));
        const slot = document.createElement('div');
        slot.className = 'sp';
        const name = Object.assign(document.createElement('b'), { textContent: x.name || '' });
        if (x.url) {
          const img = Object.assign(document.createElement('img'), { alt: x.name || '' });
          img.addEventListener('error', () => img.replaceWith(name));
          img.src = x.url;
          slot.append(img);
        } else {
          slot.append(name);
        }
        boxEl.append(slot);
      });
    }
  }

  const nameInitials = (name) => {
    const w = String(name || '').trim().split(/\s+/).filter(Boolean);
    return w.length > 1 ? (w[0][0] + w[w.length - 1][0]).toUpperCase() : (w[0] || '').slice(0, 2).toUpperCase();
  };
  // Squad photo: the photo when it loads, else the initials. Each address is asked for once.
  function setPhoto(cell, url, name) {
    setText(cell.firstChild, nameInitials(name));
    url = url || '';
    if (cell._u === url) return;
    cell._u = url;
    cell.classList.remove('has-img');
    if (url) cell.lastChild.src = url; else cell.lastChild.removeAttribute('src');
  }

  function renderSquad(game) {
    const squad = Array.isArray(game.squad) ? game.squad : [];
    const photos = !!(game.photos && game.photos.squad);
    const modeChanged = plantel.classList.contains('nophoto') === photos;
    setClass(plantel, 'nophoto', !photos);
    cards.forEach((c, i) => {
      const p = squad[i];
      c.card.style.display = p ? '' : 'none';
      if (!p) return;
      const n = splitName(p.name);
      c.last.dataset.fit = photos ? '32' : '36';
      setText(c.num, p.number);
      setText(c.first, n.first);
      setText(c.last, n.last);
      if (modeChanged) fit(c.last);
      if (photos) setPhoto(c.photo, p.photo_url, p.name);
    });
  }

  // Lower third. In: the number block wipes in along the diagonal, the name slides out from behind
  // it, then the photo. Out: the same wipe backwards, faster. A new person while one is showing
  // plays the exit, then fills in the new text and plays the entrance.
  const ltAnims = [];
  let ltPhase = 'hidden'; // hidden | shown | swapping
  let ltWho = '';
  let ltKey = '';
  let ltNext = null;
  let ltSeq = 0;
  function ltContent(st, l) {
    const com = l.type === 'comentador';
    const photos = st.game && st.game.photos && st.game.photos.lower_third;
    const c = {
      com,
      photo: !com && photos && l.photo_url && l.photo_url !== ltImg._bad ? l.photo_url : '',
      big: com ? [l.first_name, l.last_name].filter(Boolean).join(' ') : l.last_name,
      small: l.first_name,
      num: l.number,
      label: l.label || t('overlay.rodape.comentario'),
    };
    c.who = JSON.stringify([c.com, c.big, c.small, c.num, c.label]);
    c.key = c.who + c.photo;
    return c;
  }
  function ltFill(c) {
    setClass(lt, 'comentador', c.com);
    setClass(lt, 'jogador', !c.com);
    if (c.photo && ltImg.getAttribute('src') !== c.photo) ltImg.src = c.photo;
    setClass(lt, 'photo', !!c.photo);
    if (c.com) {
      setText(ltBig, c.big);
      setText(ltLabel, c.label);
    } else {
      setText(ltNum, c.num);
      setText(ltSmall, c.small);
      setText(ltBig, c.big);
    }
    ltWho = c.who;
    ltKey = c.key;
  }
  function ltShow(c) {
    ltSeq++;
    stop(ltAnims);
    ltFill(c);
    setClass(lt, 'on', true);
    ltPhase = 'shown';
    if (!ANIM) return;
    const w = ltBody.offsetWidth, h = ltBody.offsetHeight, dx = h * TAN;
    // The body's leading edge stops on the navy band's diagonal, then runs on to the end.
    play(ltAnims, ltBody, [
      { clipPath: poly([[0, 0], [0, 0], [-dx, h], [0, h]]), easing: E.out },
      { clipPath: poly([[0, 0], [203.2, 0], [203.2 - dx, h], [0, h]]), easing: E.out, offset: 0.4 },
      { clipPath: poly([[0, 0], [w + dx, 0], [w, h], [0, h]]) },
    ], { dur: T.long - 80, ease: 'linear' });
    play(ltAnims, ltNum, pop(1.15), { dur: T.quick, delay: 100 });
    play(ltAnims, ltTab, wipe(ltTab.offsetWidth, ltTab.offsetHeight), { dur: T.quick + 80, delay: 300 });
    play(ltAnims, $('.lt-text', lt), slide(-56), { dur: T.enter - 80, delay: 500 });
    play(ltAnims, $('.lt-photo', lt), slide(-20), { dur: T.quick + 20, delay: 800 });
  }
  function ltExit(dur) {
    play(ltAnims, $('.lt-text', lt), [{ opacity: 1 }, { opacity: 0 }], { dur: dur * 0.6, ease: E.in, exit: true });
    play(ltAnims, ltTab, wipe(ltTab.offsetWidth, ltTab.offsetHeight).reverse(), { dur: dur * 0.8, ease: E.in, exit: true });
    return play(ltAnims, ltBody, wipe(ltBody.offsetWidth, ltBody.offsetHeight).reverse(), { dur, ease: E.in, exit: true });
  }
  function ltHide() {
    const swapping = ltPhase === 'swapping';
    ltSeq++;
    ltPhase = 'hidden';
    setClass(lt, 'on', false);
    if (swapping) return; // already wiping out
    stop(ltAnims);
    ltExit(T.exit + 60);
  }
  function ltSwap(c) {
    ltNext = c;
    ltPhase = 'swapping';
    const seq = ++ltSeq;
    stop(ltAnims);
    const a = ltExit(T.exit);
    if (!a) return ltShow(ltNext);
    a.onfinish = () => { if (seq === ltSeq) ltShow(ltNext); };
  }
  function renderLowerThird(st, screen) {
    const l = st.lower_third || {};
    if (!l.visible || !LT_SCREENS.has(screen)) {
      if (ltPhase !== 'hidden') ltHide();
      return;
    }
    const c = ltContent(st, l);
    if (ltPhase === 'hidden') ltShow(c);
    else if (ltPhase === 'swapping') ltNext = c;
    else if (c.who !== ltWho) ltSwap(c);
    else if (c.key !== ltKey) ltFill(c); // same person, only the photo changed
  }

  // Sponsor corner. Every sponsor_every minutes one sponsor, in the strip's order, shows for 10 s on the
  // scoreboard, with a red line that fills meanwhile. A turn that finds the scoreboard off screen, a
  // time-out or a lower third in the corner is skipped; one of them coming up hides the corner.
  const CORNER_MS = 10000;
  const cornerT0 = performance.now();
  const cornerFx = [];
  let cornerList = [];
  let cornerNext = 0;
  let cornerTurn = -1;
  let cornerUntil = 0;
  let cornerForcedKey = null;
  function cornerFill(x) {
    spcLogo.textContent = '';
    const name = Object.assign(document.createElement('b'), { textContent: x.name || '' });
    if (!x.url) return spcLogo.append(name);
    const img = Object.assign(document.createElement('img'), { alt: x.name || '' });
    img.addEventListener('error', () => img.replaceWith(name));
    img.src = x.url;
    spcLogo.append(img);
  }
  function ltInCorner() {
    if (ltPhase === 'hidden') return false;
    const a = spc.getBoundingClientRect(), b = lt.getBoundingClientRect();
    return a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom;
  }
  function cornerAllowed() {
    const o = (state && state.game && state.game.overlay) || {};
    return o.sponsor_corner !== false && shownScreen === 'marcador' && cornerList.length > 0 && !ltInCorner();
  }
  function cornerShow(x) {
    stop(cornerFx);
    cornerFill(x);
    setClass(spc, 'on', true);
    cornerUntil = performance.now() + CORNER_MS;
    play(cornerFx, spc, [{ clipPath: 'inset(0 0 0 100%)' }, { clipPath: 'inset(0)' }], { dur: T.enter });
    play(cornerFx, spcLogo, fade(), { dur: T.quick, delay: 250 });
    if (!reduced.matches) play(cornerFx, spcBar, [{ transform: 'scaleX(0)' }, { transform: 'none' }], { dur: CORNER_MS, ease: 'linear' });
  }
  function cornerHide() {
    cornerUntil = 0;
    setClass(spc, 'on', false);
  }
  function cornerTick() {
    if (forcedCorner) {
      if (!cornerList.length) return;
      const x = cornerList[(forcedCorner - 1) % cornerList.length], key = JSON.stringify(x);
      if (key === cornerForcedKey) return;
      cornerForcedKey = key;
      cornerFill(x);
      spcBar.style.transform = 'scaleX(0.62)';
      return setClass(spc, 'on', true);
    }
    const tx = performance.now();
    if (cornerUntil && (tx >= cornerUntil || !cornerAllowed())) cornerHide();
    const o = (state && state.game && state.game.overlay) || {};
    const every = Math.min(30, Math.max(1, +o.sponsor_every || 4)) * 60000;
    const turn = Math.floor((tx - cornerT0) / every);
    if (turn === cornerTurn) return;
    cornerTurn = turn;
    if (turn > 0 && !cornerUntil && cornerAllowed()) cornerShow(cornerList[cornerNext++ % cornerList.length]);
  }
  setInterval(cornerTick, 500);

  // ---------- promo: club videos ----------
  // state.promo.clip plays on the promo screen and state.promo.mini small on desconto. When a clip
  // that goes back ends, the overlay tells the program, which returns to the screen before.
  const PANEL = q.get('painel') === '1';
  const fixedT = parseFloat(q.get('vt'));
  const promo = screens.promo;
  const pr = { video: $('video', promo), bar: $('.pr-bar b', promo), left: $('[data-pr="left"]', promo),
    kick: $('.pr-kick', promo), head: $('[data-pr="headline"]', promo), line: $('[data-pr="line"]', promo),
    qr: $('.pr-qr', promo), qrImg: $('.pr-qr img', promo), short: $('[data-pr="short"]', promo) };
  const mini = { box: $('.to-mini'), video: $('.to-mini video'), bar: $('.to-mini .pr-prog b'), tag: $('.to-mtag') };
  let prClip = null, prKey = '', prSeq = -1, miniClip = null, miniKey = '', miniPlaying = false;

  const shortLink = (u) => String(u || '').replace(/^https?:\/\//i, '').replace(/^www\./i, '').replace(/\/$/, '');
  const mmss = (tx) => { tx = Math.max(0, Math.ceil(tx)); return Math.floor(tx / 60) + ':' + String(tx % 60).padStart(2, '0'); };
  // Shrink a block's font until it fits in `lines` lines and no word sticks out.
  function fitLines(el, base, lines) {
    let size = base;
    el.style.fontSize = size + 'px';
    while ((el.scrollHeight > lines * size * 1.02 + 2 || el.scrollWidth > el.clientWidth + 0.5) && size > base * 0.45) {
      size -= 2;
      el.style.fontSize = size + 'px';
    }
  }
  function fitPromo() {
    if (prClip) fitLines(pr.head, promo.classList.contains('h') ? 60 : 116, promo.classList.contains('h') ? 3 : 2);
  }
  function loadClip(v, c) {
    if (v.getAttribute('src') !== c.url) v.src = c.url;
    v.loop = c.end === 'loop';
    v.muted = PANEL || !c.sound;
    v.volume = Math.min(1, Math.max(0, (c.volume ?? 100) / 100));
  }
  // From the start; a browser that refuses sound without a click still plays it silent.
  function playClip(v) {
    if (!isNaN(fixedT)) {
      v.pause();
      v.currentTime = fixedT;
      return;
    }
    v.currentTime = 0;
    const muted = v.muted;
    v.play().catch(() => { if (!muted) { v.muted = true; v.play().catch(() => {}); } });
  }
  function progress(v, bar, left) {
    const d = v.duration, ok = d > 0 && isFinite(d);
    bar.style.transform = `scaleX(${ok ? Math.min(1, v.currentTime / d) : 0})`;
    if (left) setText(left, ok ? mmss(d - v.currentTime) : '');
  }
  for (const [v, bar, left] of [[pr.video, pr.bar, pr.left], [mini.video, mini.bar, null]]) {
    const step = () => { progress(v, bar, left); if (!v.paused && !v.ended) requestAnimationFrame(step); };
    for (const ev of ['play', 'seeked', 'loadedmetadata', 'timeupdate']) v.addEventListener(ev, ev === 'play' ? step : () => progress(v, bar, left));
  }
  // A clip without a set orientation (the panel had not measured it yet) is laid out by its own size.
  pr.video.addEventListener('loadedmetadata', () => {
    if (prClip && !prClip.orientation && pr.video.videoWidth) {
      setClass(promo, 'h', pr.video.videoWidth > pr.video.videoHeight);
      fitPromo();
    }
  });
  function reportEnd() {
    if (PANEL || stateURL || forcedScreen || shownScreen !== 'promo' || !prClip || prClip.end === 'loop') return;
    fetch('/api/promo/ended', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ seq: prSeq }) }).catch(() => {});
  }
  // A file the browser can't play ends at once, so the screen never hangs on a black frame.
  pr.video.addEventListener('ended', reportEnd);
  pr.video.addEventListener('error', reportEnd);
  mini.video.addEventListener('ended', () => setClass(mini.box, 'done', true));

  function renderPromo(st) {
    const p = st.promo || {};
    const c = p.clip;
    if (c) {
      const key = JSON.stringify(c), seq = p.seq || 0;
      if (key !== prKey || seq !== prSeq) {
        const fresh = seq !== prSeq || !prClip || prClip.url !== c.url;
        prKey = key; prSeq = seq; prClip = c;
        setText(pr.kick.firstChild, c.kicker);
        setClass(pr.kick, 'none', !c.kicker);
        setText(pr.head, c.headline);
        setClass(pr.head, 'none', !c.headline);
        setText(pr.line, c.line);
        setClass(pr.line, 'none', !c.line);
        setClass(pr.qr, 'none', !c.qr_url);
        if (c.qr_url && pr.qrImg.getAttribute('src') !== c.qr_url) pr.qrImg.src = c.qr_url;
        setText(pr.short, shortLink(c.link));
        if (c.orientation) setClass(promo, 'h', c.orientation === 'horizontal');
        setClass(promo, 'loop', c.end === 'loop');
        loadClip(pr.video, c);
        fitPromo();
        if (fresh && shownScreen === 'promo') playClip(pr.video);
      }
    }
    const m = p.mini || null, mk = m ? JSON.stringify(m) : '';
    if (mk !== miniKey) {
      miniKey = mk;
      // Another clip starts over (syncMini plays it); a text change leaves it running.
      if (!m || !miniClip || m.url !== miniClip.url) miniPlaying = false;
      miniClip = m;
      if (m) { loadClip(mini.video, m); setText(mini.tag, m.kicker); } else mini.video.pause();
    }
  }
  // The card shows while desconto does. The column keeps its tight layout while desconto fades out.
  function syncMini(entering) {
    const d = screens.desconto, on = shownScreen === 'desconto';
    if (on && d.classList.contains('mini') !== !!miniClip) {
      setClass(d, 'mini', !!miniClip);
      for (const el of $$('[data-fit-mini]', d)) {
        el._fit0 ||= el.dataset.fit;
        el.dataset.fit = miniClip ? el.dataset.fitMini : el._fit0;
        fit(el);
      }
    }
    const want = on && !!miniClip;
    if (want && (entering || !miniPlaying)) {
      miniPlaying = true;
      setClass(mini.box, 'done', false);
      playClip(mini.video);
    } else if (!want && miniPlaying) {
      miniPlaying = false;
      setTimeout(() => { if (!miniPlaying) mini.video.pause(); }, T.exit + 50);
    }
  }

  function setScreen(name, animate = true) {
    if (!SCREENS.includes(name)) name = 'nada';
    if (name === shownScreen) return;
    const was = shownScreen;
    const bgBefore = BG_SCREENS.has(was), stripBefore = STRIP_SCREENS.has(was);
    shownScreen = name;
    for (const [k, el] of Object.entries(screens)) setClass(el, 'on', k === name);
    if (was === 'promo') setTimeout(() => { if (shownScreen !== 'promo') pr.video.pause(); }, T.exit + 50);
    syncMini(name === 'desconto');
    if (BG_SCREENS.has(name)) root.dataset.bg = DIM_SCREENS.has(name) ? 'dim' : ''; else delete root.dataset.bg;
    if (STRIP_SCREENS.has(name)) root.dataset.strip = ''; else delete root.dataset.strip;
    root.dataset.screen = name;
    if (!animate) return;
    // The new screen waits for the old one to fade, so two graphics never cross. Desconto never
    // waits: its squeeze runs in step with the camera in OBS.
    let base = 0;
    if (ANIM && was === 'desconto' && squeezing()) base = leaveDesconto();
    else if (ANIM && was && was !== 'nada' && name !== 'desconto') base = T.exit;
    root.style.setProperty('--enter-delay', base + 'ms');
    enterScreen(name, base);
    // The clip starts once the screen before has gone, so its sound never plays over another graphic.
    if (name === 'promo' && prClip) setTimeout(() => { if (shownScreen === 'promo') playClip(pr.video); }, base);
    if (BG_SCREENS.has(name) && !bgBefore) enterBackdrop(base);
    if (STRIP_SCREENS.has(name) && !stripBefore) enterStrip(STRIP_DELAY[name] + base);
  }

  // ---------- the club's setup: style, places, size, accent, what stays off air ----------
  // The query wins over the state, so the panel can preview a setup before saving it:
  // ?style= ?bug= ?lt= ?margin= ?scale= ?accent= ?away=neutral ?hide=fouls,shot
  const W = 1920, H = 1080;
  const ACCENTS = new Set(['gold', 'white', 'orange', 'lime', 'cyan']);
  const STYLES = new Set(['classico', 'lombada', 'cor', 'placa', 'pavilhao']);
  const BUGS = new Set(['top-left', 'top-center', 'top-right', 'bottom-left', 'bottom-center', 'bottom-right']);
  const SIDES = new Set(['left', 'center', 'right']);
  // The panel's preview (?painel=1) posts {look: '<query string>'} as the club changes the setup,
  // so the preview follows the form before anything is saved.
  let lq = q;
  if (q.get('painel') === '1') {
    addEventListener('message', (e) => {
      if (e.origin !== location.origin || !e.data || typeof e.data.look !== 'string') return;
      lq = new URLSearchParams(e.data.look);
      render();
    });
  }
  function look(o) {
    o = o || {};
    const pick = (k, v, ok, def) => { v = lq.get(k) || v; return ok.has(v) ? v : def; };
    const num = (k, v, lo, hi, def) => { v = +(lq.get(k) || v); return v >= lo && v <= hi ? v : def; };
    const hide = lq.has('hide') ? lq.get('hide').split(',') : (o.hide || []);
    return {
      style: pick('style', o.style, STYLES, 'classico'),
      bug: pick('bug', o.bug_position, BUGS, 'top-left'),
      lt: pick('lt', o.lower_third, SIDES, 'left'),
      margin: num('margin', o.margin, 32, 96, 64),
      scale: num('scale', o.scale, 85, 125, 100) / 100,
      accent: pick('accent', o.accent, ACCENTS, 'gold'),
      neutralAway: (lq.get('away') || o.away_color) === 'neutral',
      hide: ['fouls', 'shot', 'strip'].filter((h) => hide.includes(h)).join(' '),
      photo: typeof o.photo === 'string' && o.photo.startsWith('/media/') ? o.photo : '',
    };
  }
  function setLook(lk) {
    for (const k of ['style', 'accent', 'hide']) if (root.dataset[k] !== lk[k]) root.dataset[k] = lk[k];
    const hall = lk.photo ? `url("${lk.photo}")` : '';
    if (root.style.getPropertyValue('--hall') !== hall) {
      if (hall) root.style.setProperty('--hall', hall); else root.style.removeProperty('--hall');
    }
  }

  // Where the bug, the lower third, the sponsor corner and the credit sit on the scoreboard screen.
  // Each takes its place from the setup; the lower third, then the corner and the credit, step aside
  // for what is already there. A change after the first state slides the bug across (a fade with
  // reduced motion).
  const overlaps = (a, b) => a && b && a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
  const xAt = (side, w, m) => (side === 'left' ? m : side === 'right' ? W - m - w : Math.round((W - w) / 2));
  let rects = {};
  function layout(lk, ltUp) {
    const m = lk.margin, s = lk.scale;
    const [vy, vx] = lk.bug.split('-');
    const bw = bugBox.offsetWidth * s, bh = bugBox.offsetHeight * s;
    const bug = { x: xAt(vx, bw, m), y: vy === 'top' ? m : H - m - bh, w: bw, h: bh };

    const lw = ltBody.offsetWidth * s, lh = lt.offsetHeight * s;
    let ltR = null;
    for (const side of [lk.lt, 'right', 'left', 'center']) {
      const r = { x: xAt(side, lw, m), y: H - m - lh, w: lw, h: lh };
      if (!overlaps(r, bug)) { ltR = r; break; }
    }
    if (!ltR) ltR = { x: xAt(lk.lt, lw, m), y: bug.y - 24 - lh, w: lw, h: lh }; // above a bottom bug

    const cw = spc.offsetWidth || 252, ch = spc.offsetHeight || 86;
    const corner = [W - m - cw, m].map((x) => ({ x, y: m, w: cw, h: ch })).find((r) => !overlaps(r, bug)) || null;
    const kw = credit.offsetWidth || 160, kh = credit.offsetHeight || 36;
    const busy = [bug, ltUp ? ltR : null];
    const cred = [W - m - kw, m].map((x) => ({ x, y: H - m - kh, w: kw, h: kh })).find((r) => !busy.some((b) => overlaps(r, b))) || null;

    const key = JSON.stringify([bug, ltR, corner, cred, s]);
    if (key === rects.key) return;
    const first = !rects.key;
    if (first) bugPos.style.transition = 'none';
    bugPos.style.transform = `translate(${bug.x}px, ${bug.y}px) scale(${s})`;
    root.dataset.bugX = vx;
    root.dataset.bugY = vy;
    // Off screen through the nearest edge, so it slides in from its own corner.
    root.style.setProperty('--bug-in', vx === 'left' ? 'translateX(-900px)' : vx === 'right' ? 'translateX(900px)'
      : `translateY(${vy === 'top' ? -(bug.y + bug.h + 40) / s : (H - bug.y + 40) / s}px)`);
    Object.assign(lt.style, { left: ltR.x + 'px', top: ltR.y + 'px', transform: `scale(${s})` });
    spc.style.visibility = corner ? '' : 'hidden';
    if (corner) Object.assign(spc.style, { left: corner.x + 'px', top: corner.y + 'px', right: 'auto' });
    credit.dataset.room = cred ? '1' : '';
    if (cred) Object.assign(credit.style, { left: cred.x + 'px', top: cred.y + 'px', right: 'auto', bottom: 'auto' });
    if (first) { void bugPos.offsetWidth; bugPos.style.transition = ''; }
    else if (reduced.matches && rects.bug && (rects.bug.x !== bug.x || rects.bug.y !== bug.y)) nudge(bugPos, fade(), { dur: T.enter });
    rects = { key, bug, lt: ltR, corner, cred };
  }

  // Score, fouls, period and shot clock changes animate only on the scoreboard and the time-out
  // screen while they show, and never on the first state or the first one after a reconnect.
  let last = null;
  let quiet = true;
  // True once the program has gone LOST_MS without answering: the numbers on screen are old.
  let lost = false;
  const LOST_MS = 7000;
  function liveEffects(v) {
    const cur = {
      home: +v.homeScore || 0, away: +v.awayScore || 0,
      homeF: Math.min(+v.homeFouls || 0, 4), awayF: Math.min(+v.awayFouls || 0, 4),
      per: v.periodShort, urgent: shotUrgent(v.shot),
    };
    const was = last;
    last = cur;
    if (!was || quiet || !ANIM) return;
    if (shownScreen !== 'marcador' && shownScreen !== 'desconto') return;
    const scr = screens[shownScreen];
    for (const tx of ['home', 'away']) {
      // Up to 3 is a basket. A correction downwards, or a jump of more than 3 (a resync), just changes.
      const d = cur[tx] - was[tx];
      if (d >= 1 && d <= 3) {
        for (const el of $$(`[data-b="${tx}Score"]`, scr)) rollScore(el, String(was[tx]));
        if (shownScreen === 'marcador') showPop(tx, d);
      }
      if (cur[tx + 'F'] > was[tx + 'F']) {
        for (const p of $$(`[data-pips="${tx}"]`, scr)) for (let k = was[tx + 'F']; k < cur[tx + 'F']; k++) popPip(p.children[k]);
      }
    }
    if (cur.per !== was.per) {
      for (const el of $$('[data-b="periodShort"], [data-b="periodLong"]', scr)) nudge(el, rise(10), { dur: T.quick + 50 });
    }
    // One gentle pulse as the 24 s drops under 5; the gold stays while it is under.
    if (cur.urgent && !was.urgent && shownScreen === 'marcador') {
      nudge(binds.shot[0], [{ transform: 'none' }, { transform: 'scale(1.14)', offset: 0.4 }, { transform: 'none' }], { dur: 420, ease: E.inOut });
    }
  }
  function shotUrgent(s) {
    const n = parseFloat(s);
    return s !== '' && s != null && !isNaN(n) && n < 5;
  }

  // The club's colours (game.theme, keyed by custom property name) over the defaults in overlay.css.
  // A key the new theme lacks goes back to its default; a value only changes when the club does.
  let themeKeys = [];
  let themeKey = '';
  function setTheme(theme) {
    theme = theme && typeof theme === 'object' ? theme : {};
    const key = JSON.stringify(theme);
    if (key === themeKey) return;
    themeKey = key;
    for (const k of themeKeys) if (!(k in theme)) root.style.removeProperty('--' + k);
    for (const [k, val] of Object.entries(theme)) if (typeof val === 'string') root.style.setProperty('--' + k, val);
    themeKeys = Object.keys(theme);
  }

  function render() {
    if (!state) return;
    const st = state;
    const v = view(st);
    for (const [k, els] of Object.entries(binds)) {
      if (!(k in v)) continue;
      for (const el of els) {
        if (el.dataset.alt && el._alt !== v[el.dataset.alt]) {
          el._alt = v[el.dataset.alt];
          if (el._v === v[k]) fit(el);
        }
        setText(el, v[k]);
      }
    }

    setTheme((st.game || {}).theme);
    const lk = look((st.game || {}).overlay);
    setLook(lk);
    root.style.setProperty('--away', (!lk.neutralAway && v.away.color) || '#5C6170');
    root.style.setProperty('--next', (v.next && v.next.opponent && v.next.opponent.color) || '#5C6170');
    setClass(screens.voltamos, 'no-line', !v.backLine);
    setClass(screens.voltamos, 'no-score', !v.inPlay);
    camera = st.camera && st.camera.full && st.camera.window ? st.camera : null;
    for (const k of ['x', 'y', 'w', 'h']) {
      if (camera) root.style.setProperty('--hole-' + k, camera.window[k] + 'px');
      else root.style.removeProperty('--hole-' + k);
    }
    setClass(root, 'off', lost || (st.box && st.box.connected === false));

    for (const tx of tiles) {
      const k = tx.dataset.tile;
      renderTile(tx, k === 'home' ? { ...v.home, name: v.homeName } : k === 'next' ? (v.next && v.next.opponent) || {} : { ...v.away, name: v.awayName });
    }

    const hf = +v.homeFouls || 0, af = +v.awayFouls || 0;
    renderPips('home', hf);
    renderPips('away', af);
    // FIBA: from the 5th team foul of the period the other side shoots free throws.
    for (const [tx, n] of [['home', hf], ['away', af]]) {
      setClass(bonusCells[tx], 'bonus', n >= 5);
      setClass(bugRows[tx], 'bonus', n >= 5);
    }

    const shotEl = binds.shot[0];
    setClass(shotEl, 'hot', String(v.shot).includes('.') || shotUrgent(v.shot));
    const noShot = v.shot === '' || v.shot == null;
    setClass(shotEl.parentElement, 'blank', noShot);
    setClass(shotEl.parentElement.parentElement, 'solo', noShot || root.dataset.hide.includes('shot'));

    const hs = +v.homeScore, as = +v.awayScore;
    setClass(winners.home, 'on', hs > as);
    setClass(winners.away, 'on', as > hs);
    setClass(coachBox, 'none', !v.coach);

    renderPeriods(v.periods, v);
    renderSquad(st.game || {});
    renderSponsors((st.game || {}).sponsors);

    renderPromo(st);
    let screen = forcedScreen || (st.screen && st.screen.current) || 'nada';
    if (screen === 'proximo' && !v.next) screen = 'nada'; // never an empty card
    setScreen(screen);
    syncMini(false);
    renderLowerThird(st, screen);
    layout(lk, ltPhase !== 'hidden');
    // The credit shows only where it has a free bottom corner.
    setClass(credit, 'on', (st.game || {}).credit !== false && CREDIT_SCREENS.has(screen) && credit.dataset.room === '1');
    liveEffects(v);
    cornerTick();
    quiet = false;
  }

  let queued = false;
  // ?rodape=1 (the panel's setup preview) always shows a player's lower third: the squad's first
  // player, or the club's name with a shirt number when there is no squad.
  function withSampleLT(st) {
    const p = ((st.game || {}).squad || [])[0] || {};
    const name = String(p.name || (st.game && st.game.home && st.game.home.name) || '').trim().split(/\s+/);
    return { ...st, lower_third: { visible: true, type: 'jogador', number: p.number || '7',
      first_name: name.length > 1 ? name[0] : '', last_name: name.slice(name.length > 1 ? 1 : 0).join(' ') } };
  }
  function apply(st) {
    if (st && q.get('rodape') === '1') st = withSampleLT(st);
    state = st;
    if (queued) return;
    queued = true;
    setTimeout(() => { queued = false; render(); }, 0);
  }

  // Names were fitted with fallback fonts if the web fonts were still loading: refit once they land.
  document.fonts.ready.then(() => { for (const el of $$('[data-fit]')) if (el._v) fit(el); fitPromo(); });
  // The opener countdown moves with the wall clock, not with state changes.
  setInterval(render, 15000);
  // A forced screen (previews, tests) shows at once, without its entrance.
  if (forcedScreen) setScreen(forcedScreen, false);
  if (forcedPop) {
    setText(pops[forcedPop[1]].firstChild.firstChild, '+' + forcedPop[2]);
    setClass(pops[forcedPop[1]], 'show', true);
  }

  // ---------- state sources ----------
  if (stateURL) {
    fetch(stateURL, { cache: 'no-store' }).then((r) => r.json()).then(apply).catch((e) => console.error('state', e));
    return;
  }

  // OBS keeps this page when the program stops. The grey dot of a lost box then shows too, from
  // LOST_MS after the event stream fails until a state arrives again, by the stream or by polling.
  let lostTimer = null;
  const fresh = (st) => {
    clearTimeout(lostTimer);
    lostTimer = null;
    lost = false;
    apply(st);
  };
  const stale = () => {
    lostTimer ||= setTimeout(() => { lost = true; setClass(root, 'off', true); }, LOST_MS);
  };

  let pollTimer = null;
  const pull = () => fetch('/api/state', { cache: 'no-store' })
    .then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
    .then(fresh)
    .catch(() => {});
  const startPolling = () => { if (!pollTimer) pollTimer = setInterval(pull, 2000); };
  const stopPolling = () => { clearInterval(pollTimer); pollTimer = null; };

  function connect() {
    let es;
    try { es = new EventSource('/api/events'); } catch (e) { startPolling(); setTimeout(connect, 5000); return; }
    // After a reconnect, rebuild the sponsor strip so a logo that failed to load gets another try.
    es.addEventListener('open', () => { quiet = true; sponsorsKey = null; });
    es.addEventListener('state', (e) => {
      stopPolling();
      try { fresh(JSON.parse(e.data)); } catch (err) { console.error('state', err); }
    });
    es.onerror = () => {
      quiet = true;
      stale();
      startPolling();
      // EventSource retries by itself unless the server refused the stream; then reopen it ourselves.
      if (es.readyState === EventSource.CLOSED) setTimeout(connect, 5000);
    };
  }
  pull();
  connect();
})();
