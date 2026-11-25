/* Imagem do resultado (1080x1080 PNG) para as redes sociais, desenhada num <canvas>.
   Segue o ecrã Final do overlay: casa na cor do clube, visitante a branco, a diagonal do equipamento entre os dois.
   Só usa imagens do mesmo servidor (/media, /assets), para o canvas poder ser exportado. */
(function () {
  'use strict';
  var W = 1080;
  var GOLD = '#D6BD2F', GREY = '#ECEAE6', MUTED = 'rgba(255,255,255,0.72)';
  // The club's colours come from state.game.theme (club.Theme in Go), as on the overlay; these
  // defaults are the default club's, the same as overlay.css. draw() sets C before drawing.
  var THEME = { red: '#2F4B7C', 'on-red': '#FFFFFF', kit: '#223659', 'on-kit': '#FFFFFF', navy: '#1C1A17' };
  var C = THEME;
  function colours(st) {
    var tx = (st.game && st.game.theme) || {}, c = {};
    Object.keys(THEME).forEach(function (k) { c[k] = typeof tx[k] === 'string' ? tx[k] : THEME[k]; });
    return c;
  }
  var COND = '"Barlow Condensed", sans-serif', TEXT = 'Barlow, sans-serif', DISPLAY = 'Graduate, serif';
  var TZ = window.HS_TZ || 'Europe/Lisbon'; // the club's zone, from /i18n.js
  var TAN12 = Math.tan(12 * Math.PI / 180);
  // Faixa principal: nomes de T a M, resultados de M a B.
  var T = 236, M = 566, B = 806;
  var HOME_X = 268, AWAY_X = 830, NAME_MAX = 420;

  function joinDot() {
    return Array.prototype.filter.call(arguments, function (s) { return s != null && String(s).trim() !== ''; }).join(' · ');
  }
  function periodLabel(n) { return n <= 4 ? t('painel.resultado.periodo', { n: n }) : n === 5 ? t('painel.resultado.prol') : t('painel.resultado.prol_n', { n: n - 4 }); }

  // Final = o 4º período (ou um prolongamento) já registado e nenhum período novo a decorrer.
  function heading(st) {
    var ps = st.periods || [], last = ps[ps.length - 1], now = parseInt((st.live || {}).period, 10);
    if (last && last.period >= 4 && !(now > last.period)) return t('painel.resultado.final');
    return st.screen && st.screen.current === 'intervalo' ? t('painel.resultado.intervalo') : t('painel.resultado.em_curso');
  }

  function gameDate(st) {
    var d = st.game && st.game.start ? new Date(st.game.start) : new Date();
    return isNaN(d) ? new Date() : d;
  }
  function dateLine(d) {
    var wd = new Intl.DateTimeFormat(t('data.locale'), { weekday: 'long', timeZone: TZ }).format(d);
    var day = new Intl.DateTimeFormat(t('data.locale'), { day: 'numeric', timeZone: TZ }).format(d);
    var month = new Intl.DateTimeFormat(t('data.locale'), { month: 'short', timeZone: TZ }).format(d).replace('.', '');
    var year = new Intl.DateTimeFormat(t('data.locale'), { year: 'numeric', timeZone: TZ }).format(d);
    return t('painel.resultado.data', { dia_semana: wd.charAt(0).toUpperCase() + wd.slice(1), dia: day, mes: month, ano: year });
  }
  function fileName(st) {
    var away = (st.game && st.game.away) || {};
    var day = new Intl.DateTimeFormat('en-CA', { year: 'numeric', month: '2-digit', day: '2-digit', timeZone: TZ }).format(gameDate(st));
    var who = String(away.short || away.name || (st.live || {}).away_name || t('painel.resultado.jogo')).normalize('NFD').replace(/[̀-ͯ]/g, '')
      .toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || t('painel.resultado.jogo');
    return t('painel.resultado.ficheiro', { dia: day, adversario: who });
  }

  function loadImage(url) {
    return new Promise(function (resolve) {
      if (!url) return resolve(null);
      var img = new Image();
      img.onload = function () { resolve(img.naturalWidth ? img : null); };
      img.onerror = function () { resolve(null); };
      img.src = url;
    });
  }
  // fonts.ready só espera pelas fontes já pedidas; o painel não usa Graduate, por isso pedimo-las aqui.
  function loadFonts() {
    return Promise.all(['400 58px Graduate', '500 28px Barlow', '600 30px "Barlow Condensed"', '700 30px "Barlow Condensed"', '800 30px "Barlow Condensed"']
      .map(function (f) { return document.fonts.load(f, 'AÉÇ0º'); })).then(function () { return document.fonts.ready; });
  }

  function text(x, s, px, py, font, color, align, spacing) {
    x.font = font; x.fillStyle = color; x.textAlign = align || 'center'; x.letterSpacing = (spacing || 0) + 'px';
    x.fillText(s, px, py);
  }
  // Encolhe a letra até o texto caber em max.
  function fit(x, s, size, family, weight, max) {
    for (; size > 20; size -= 2) { x.font = weight + ' ' + size + 'px ' + family; if (x.measureText(s).width <= max) break; }
    return x.font;
  }
  function contain(x, img, cx, cy, w, h) {
    var k = Math.min(w / img.naturalWidth, h / img.naturalHeight), iw = img.naturalWidth * k, ih = img.naturalHeight * k;
    x.drawImage(img, cx - iw / 2, cy - ih / 2, iw, ih);
  }
  // Polígono da linha diagonal (12°) até à direita do quadro, entre y0 e y1, deslocado off px.
  function rightOf(x, off, y0, y1, color) {
    function edge(y) { return 514 + TAN12 * ((T + B) / 2 - y) + off; }
    x.fillStyle = color; x.beginPath();
    x.moveTo(edge(y0), y0); x.lineTo(W, y0); x.lineTo(W, y1); x.lineTo(edge(y1), y1); x.closePath(); x.fill();
  }

  function tile(x, team, img, cx, bg, strip) {
    var y = T + 26, s = 150;
    x.fillStyle = bg; x.fillRect(cx - s / 2, y, s, s);
    x.fillStyle = strip; x.fillRect(cx - s / 2, y + s - 6, s, 6);
    if (img) contain(x, img, cx, y + (s - 6) / 2, 110, 114);
    else {
      x.textBaseline = 'middle';
      text(x, team.initials || String(team.short || team.name || '').slice(0, 3).toUpperCase(), cx, y + (s - 6) / 2 + 3, '800 52px ' + COND, C.navy, 'center', 0.5);
      x.textBaseline = 'alphabetic';
    }
  }
  function side(x, team, name, img, score, cx, won, onRed) {
    tile(x, team, img, cx, onRed ? '#FFFFFF' : GREY, onRed ? C.red : (team.color || '#5C6170'));
    if (won) {
      x.font = '800 24px ' + COND; x.letterSpacing = '1.5px';
      var w = x.measureText(t('painel.resultado.vencedor')).width + 22;
      x.fillStyle = C.navy; x.fillRect(cx - w / 2, 432, w, 34);
      text(x, t('painel.resultado.vencedor'), cx + 0.75, 458, x.font, GOLD, 'center', 1.5);
    }
    x.letterSpacing = '0px';
    text(x, name, cx, 540, fit(x, name, 58, DISPLAY, 400, NAME_MAX), onRed ? C['on-red'] : C.navy, 'center');
    x.font = '800 220px ' + COND;
    var m = x.measureText(String(score));
    text(x, String(score), cx, (M + B) / 2 + (m.actualBoundingBoxAscent - m.actualBoundingBoxDescent) / 2, x.font, onRed ? C['on-kit'] : '#FFFFFF', 'center');
  }

  function periodsLine(x, periods, y) {
    if (!periods.length) return;
    var parts = [];
    periods.forEach(function (p, i) {
      if (i) parts.push(['·', '700 30px ' + COND, MUTED, 18]);
      parts.push([periodLabel(p.period), '700 28px ' + COND, MUTED, 10]);
      parts.push([p.home + '–' + p.away, '800 38px ' + COND, '#FFFFFF', 18]);
    });
    x.letterSpacing = '1px';
    var total = parts.reduce(function (sum, p, i) { x.font = p[1]; return sum + x.measureText(p[0]).width + (i < parts.length - 1 ? p[3] : 0); }, 0);
    var px = (W - total) / 2;
    parts.forEach(function (p) { text(x, p[0], px, y, p[1], p[2], 'left', 1); px += x.measureText(p[0]).width + p[3]; });
  }

  function footer(x, handle, sp, logo) {
    x.fillStyle = C.red; x.fillRect(0, 958, W, 2);
    var cy = 1020;
    var igX = 64;
    if (handle) {
      if (!sp) { x.font = '700 30px ' + COND; igX = (W - x.measureText(handle).width - 44) / 2; }
      x.strokeStyle = '#FFFFFF'; x.lineWidth = 2.5;
      x.beginPath(); x.roundRect(igX, cy - 15, 30, 30, 8); x.stroke();
      x.beginPath(); x.arc(igX + 15, cy, 6.5, 0, Math.PI * 2); x.stroke();
      x.fillStyle = '#FFFFFF'; x.beginPath(); x.arc(igX + 23.5, cy - 7.5, 1.6, 0, Math.PI * 2); x.fill();
      x.textBaseline = 'middle';
      text(x, handle, igX + 44, cy + 1, '700 30px ' + COND, '#FFFFFF', 'left');
      x.textBaseline = 'alphabetic';
    }
    if (!sp) return;
    var sw = 280, sh = 90, sx = W - 64 - sw;
    x.textBaseline = 'middle';
    text(x, t('painel.resultado.apoio'), sx - 20, cy + 1, '700 20px ' + COND, MUTED, 'right', 1.5);
    x.fillStyle = '#FFFFFF'; x.fillRect(sx, cy - sh / 2, sw, sh);
    if (logo) contain(x, logo, sx + sw / 2, cy, sw - 28, sh - 20);
    else { x.letterSpacing = '0px'; text(x, sp.name || '', sx + sw / 2, cy + 1, fit(x, sp.name || '', 30, COND, 700, sw - 28), C.navy, 'center'); }
    x.textBaseline = 'alphabetic';
  }

  function draw(st, homeImg, awayImg, logo) {
    var g = st.game || {}, live = st.live || {}, home = g.home || {}, away = g.away || {};
    var hs = +live.home || 0, as = +live.away || 0;
    var head = heading(st), final = head === t('painel.resultado.final');
    var sp = g.sponsors && g.sponsors.principal;
    var c = document.createElement('canvas');
    c.width = c.height = W;
    var x = c.getContext('2d');
    C = colours(st);

    x.fillStyle = C.navy; x.fillRect(0, 0, W, W);
    text(x, head, W / 2, 124, '800 92px ' + COND, GOLD, 'center', 3);
    var sub = joinDot(g.competition, g.round);
    if (sub) text(x, sub, W / 2, 184, fit(x, sub, 34, COND, 600, 960), MUTED, 'center', 1);

    // Casa à esquerda, visitante à direita; a faixa branca e azul a 12° faz a costura, como no equipamento.
    x.fillStyle = C.red; x.fillRect(0, T, W, M - T);
    x.fillStyle = C.kit; x.fillRect(0, M, W, B - M);
    rightOf(x, 0, T, B, '#FFFFFF');
    rightOf(x, 8, T, B, C.navy);
    rightOf(x, 52, T, M, '#FFFFFF');
    rightOf(x, 52, M, B, away.color || '#5C6170');

    side(x, home, home.name || live.home_name || '', homeImg, hs, HOME_X, final && hs > as, true);
    side(x, away, away.name || live.away_name || '', awayImg, as, AWAY_X, final && as > hs, false);

    periodsLine(x, st.periods || [], 862);
    x.letterSpacing = '0px';
    var info = joinDot(dateLine(gameDate(st)), g.venue);
    text(x, info, W / 2, 926, fit(x, info, 28, TEXT, 500, 960), MUTED, 'center');

    footer(x, g.instagram || '', sp, logo);
    return c;
  }

  function render(st) {
    var g = st.game || {}, sp = g.sponsors && g.sponsors.principal;
    return Promise.all([loadFonts(), loadImage(g.home && g.home.crest_url), loadImage(g.away && g.away.crest_url), loadImage(sp && sp.url)])
      .then(function (r) { return draw(st, r[1], r[2], r[3]); });
  }

  function save(st) {
    return render(st).then(function (c) {
      return new Promise(function (resolve, reject) { c.toBlob(function (b) { b ? resolve(b) : reject(new Error(t('painel.imagem.vazia'))); }, 'image/png'); });
    }).then(function (blob) {
      var a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = fileName(st);
      document.body.appendChild(a); a.click(); a.remove();
      setTimeout(function () { URL.revokeObjectURL(a.href); }, 1000);
    });
  }

  window.Resultado = { render: render, save: save, fileName: fileName };
})();
