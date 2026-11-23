/* Miniatura do YouTube (1280x720 PNG) do jogo escolhido, desenhada num <canvas>.
   Segue a maqueta: pavilhão na cor do clube, escalão numa placa, os dois emblemas, os nomes e a data.
   Sem patrocinadores. Um emblema que falhe (ou que sujasse o canvas) dá lugar às iniciais. */
(function () {
  'use strict';
  var W = 1280, H = 720;
  var GREY = '#ECEAE6';
  // The club's colours come from state.game.theme (club.Theme in Go), as on the overlay; these
  // defaults are the default club's, the same as overlay.css. draw() sets C before drawing.
  var THEME = { red: '#2F4B7C', 'on-red': '#FFFFFF', 'red-hi': '#6688C5', 'red-text': '#2F4B7C', blood: '#152238',
    ink: '#070B12', 'ink-rgb': '7,11,18', tint: '#2F4B7C', navy: '#1C1A17' };
  var C = THEME;
  function colours(st) {
    var t = (st.game && st.game.theme) || {}, c = {};
    Object.keys(THEME).forEach(function (k) { c[k] = typeof t[k] === 'string' ? t[k] : THEME[k]; });
    return c;
  }
  function ink(a) { return 'rgba(' + C['ink-rgb'] + ',' + a + ')'; }
  var COND = '"Barlow Condensed", sans-serif', DISPLAY = 'Graduate, serif';
  var TZ = 'Europe/Lisbon';
  var SKEW = Math.tan(12 * Math.PI / 180);
  var HOME_X = 300, AWAY_X = 980;
  // The brush group of the overlay (web/overlay/index.html), in its 1920x1080 space, with its theme colour.
  var BRUSH = [
    ['M0,16 C41.6,-30 260,-16 520,-7 L490,30.4 C312,62 78,47 40,39 Z', 'red'],
    ['M50,77 C81.4,59 246,66 442,68.7 L412,82.2 C285.2,95 108.8,87.5 90,85.3 Z', 'red-hi'],
    ['M130,107 C157.9,99 304.2,103.2 478.4,103.8 L448.4,109 C339,115 182.3,110.5 170,110.2 Z', 'red']
  ];

  function fmt(o, d) { return new Intl.DateTimeFormat('pt-PT', Object.assign({ timeZone: TZ }, o)).format(d); }
  function gameDate(st) {
    var d = st.game && st.game.start ? new Date(st.game.start) : null;
    return d && !isNaN(d) ? d : null;
  }
  function slug(s) {
    return String(s || '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  }
  function names(st) {
    var g = st.game || {}, live = st.live || {};
    return { home: (g.home && g.home.name) || live.home_name || 'CASA', away: (g.away && g.away.name) || live.away_name || 'VISITANTE' };
  }
  function fileName(st) {
    var n = names(st), d = gameDate(st) || new Date();
    var day = new Intl.DateTimeFormat('en-CA', { year: 'numeric', month: '2-digit', day: '2-digit', timeZone: TZ }).format(d);
    return 'miniatura-' + (slug(n.home) || 'casa') + '-vs-' + (slug(n.away) || 'visitante') + '-' + day + '.png';
  }

  function loadImage(url) {
    return new Promise(function (resolve) {
      if (!url) return resolve(null);
      var img = new Image();
      img.crossOrigin = 'anonymous'; // an image without CORS fails here instead of tainting the canvas
      img.onload = function () { resolve(img.naturalWidth ? img : null); };
      img.onerror = function () { resolve(null); };
      img.src = url;
    });
  }
  // fonts.ready só espera pelas fontes já pedidas; o painel não usa Graduate, por isso pedimo-las aqui.
  function loadFonts() {
    return Promise.all(['400 84px Graduate', '600 20px "Barlow Condensed"', '700 20px "Barlow Condensed"', '800 40px "Barlow Condensed"']
      .map(function (f) { return document.fonts.load(f, 'AÉÇÁ0º'); })).then(function () { return document.fonts.ready; });
  }

  function text(x, s, px, py, font, color, spacing) {
    x.font = font; x.fillStyle = color; x.textAlign = 'center'; x.letterSpacing = (spacing || 0) + 'px';
    x.fillText(s, px, py);
  }
  function fit(x, s, size, family, weight, max) {
    for (; size > 24; size -= 2) { x.font = weight + ' ' + size + 'px ' + family; if (x.measureText(s).width <= max) break; }
    return x.font;
  }
  // A parallelogram leaning like the kit diagonal, centred on cx, from y to y + h.
  function lean(x, cx, y, w, h, color) {
    var dx = h * SKEW / 2;
    x.fillStyle = color; x.beginPath();
    x.moveTo(cx - w / 2 + dx, y); x.lineTo(cx + w / 2 + dx, y); x.lineTo(cx + w / 2 - dx, y + h); x.lineTo(cx - w / 2 - dx, y + h);
    x.closePath(); x.fill();
  }
  function contain(x, img, cx, cy, w, h) {
    var k = Math.min(w / img.naturalWidth, h / img.naturalHeight), iw = img.naturalWidth * k, ih = img.naturalHeight * k;
    x.drawImage(img, cx - iw / 2, cy - ih / 2, iw, ih);
  }

  function backdrop(x, photo) {
    x.fillStyle = C.ink; x.fillRect(0, 0, W, H);
    if (photo) {
      var k = Math.max((W + 26) / photo.naturalWidth, (H + 26) / photo.naturalHeight);
      var pw = photo.naturalWidth * k, ph = photo.naturalHeight * k;
      x.save();
      x.filter = 'grayscale(1) contrast(1.2) brightness(0.5) blur(2px)';
      x.drawImage(photo, (W - pw) / 2, (H - ph) * 0.4, pw, ph);
      x.restore();
    }
    x.save();
    x.globalCompositeOperation = 'multiply';
    x.fillStyle = C.tint; x.fillRect(0, 0, W, H);
    x.restore();
    var top = x.createLinearGradient(0, 0, 0, H * 0.26);
    top.addColorStop(0, ink(0.8)); top.addColorStop(1, ink(0));
    x.fillStyle = top; x.fillRect(0, 0, W, H);
    x.save();
    x.translate(W / 2, H / 2); x.scale(1, (0.7 * H) / (0.75 * W));
    var v = x.createRadialGradient(0, 0, 0, 0, 0, 0.75 * W);
    v.addColorStop(0, ink(0.15)); v.addColorStop(1, ink(0.82));
    x.fillStyle = v; x.fillRect(-W, -W, 2 * W, 2 * W);
    x.restore();
    // One quiet brush group, top right, as on the overlay.
    x.save();
    x.globalAlpha = 0.3;
    x.scale(W / 1920, H / 1080);
    x.translate(960, 540); x.rotate(Math.PI); x.translate(-960, -540);
    x.translate(-130, 1100); x.rotate(-61 * Math.PI / 180);
    BRUSH.forEach(function (b) { x.fillStyle = C[b[1]]; x.fill(new Path2D(b[0])); });
    x.restore();
  }

  function lockup(x, home, sport) {
    x.textBaseline = 'alphabetic';
    var a = home, sep = '  ·  ', b = sport;
    x.font = '600 20px ' + COND; x.letterSpacing = '8px';
    var wa = x.measureText(a + sep).width;
    x.font = '700 20px ' + COND;
    var wb = x.measureText(b).width, left = (W - wa - wb) / 2;
    x.textAlign = 'left';
    x.font = '600 20px ' + COND; x.fillStyle = 'rgba(255,255,255,0.9)'; x.fillText(a + sep, left, 36);
    x.font = '700 20px ' + COND; x.fillStyle = C['red-hi']; x.fillText(b, left + wa, 36);
  }

  function plate(x, label) {
    if (!label) return;
    x.font = '800 40px ' + COND; x.letterSpacing = '4px';
    var w = x.measureText(label).width + 64;
    lean(x, W / 2 + 6, 58, w, 60, C.blood);
    lean(x, W / 2, 52, w, 60, C.red);
    x.textBaseline = 'middle';
    text(x, label, W / 2 + 2, 84, x.font, C['on-red'], 4);
    x.textBaseline = 'alphabetic';
  }

  function shield(x, cx, team, img, bar, tileBg) {
    var w = 220, h = 284, l = cx - w / 2, t = 132;
    x.save();
    x.shadowColor = 'rgba(0,0,0,0.5)'; x.shadowBlur = 22; x.shadowOffsetY = 14;
    x.fillStyle = '#FFFFFF'; x.beginPath();
    x.moveTo(l, t); x.lineTo(l + w, t); x.lineTo(l + w, t + h * 0.66); x.lineTo(cx, t + h); x.lineTo(l, t + h * 0.66); x.closePath(); x.fill();
    x.restore();
    x.fillStyle = bar; x.fillRect(l, t, w, 10);
    if (img) return contain(x, img, cx, t + 28 + 106, 172, 212);
    // No crest: the initials, as the overlay's tiles do.
    x.fillStyle = tileBg; x.fillRect(cx - 60, t + 70, 120, 120);
    x.textBaseline = 'middle';
    text(x, team.initials || String(team.short || team.name || '').slice(0, 3).toUpperCase(), cx, t + 132, '400 52px ' + DISPLAY, C.navy, 0);
    x.textBaseline = 'alphabetic';
  }

  // Barlow Condensed 800, solid fill and a dark shadow: Graduate's thin slabs blur when YouTube shows the
  // thumbnail at 168x94, and a stroke draws the inner contours and ghosts the name.
  function name(x, s, cx) {
    x.save();
    x.shadowColor = 'rgba(0,0,0,0.7)'; x.shadowBlur = 18; x.shadowOffsetY = 6;
    x.letterSpacing = '1px';
    text(x, s.toUpperCase(), cx, 514, fit(x, s.toUpperCase(), 124, COND, 800, 500), '#FFFFFF', 1);
    x.restore();
  }

  function tag(x, s, cx) {
    x.font = '800 24px ' + COND; x.letterSpacing = '4px';
    var w = x.measureText(s).width + 32;
    lean(x, cx, 528, w, 36, '#FFFFFF');
    x.textBaseline = 'middle';
    text(x, s, cx + 2, 547, x.font, C['red-text'], 4);
    x.textBaseline = 'alphabetic';
  }

  function vs(x) {
    lean(x, W / 2 + 8, 236, 132, 92, C.red);
    lean(x, W / 2, 228, 132, 92, '#FFFFFF');
    x.textBaseline = 'middle';
    text(x, 'VS', W / 2, 276, '800 64px ' + COND, C['red-text'], 0);
    x.textBaseline = 'alphabetic';
  }

  function when(x, d) {
    if (!d) return;
    var parts = [fmt({ weekday: 'long' }, d).slice(0, 3).toUpperCase(),
      (fmt({ day: 'numeric' }, d) + ' ' + fmt({ month: 'short' }, d).replace('.', '')).toUpperCase(),
      fmt({ hour: '2-digit', minute: '2-digit', hour12: false }, d)];
    x.font = '800 66px ' + COND; x.letterSpacing = '0px';
    var dot = '  ·  ', wd = x.measureText(dot).width;
    var total = parts.reduce(function (sum, p) { return sum + x.measureText(p).width; }, 0) + wd * 2;
    lean(x, W / 2 + 8, 598, total + 88, 100, C.red);
    lean(x, W / 2, 590, total + 88, 100, '#FFFFFF');
    x.textBaseline = 'middle'; x.textAlign = 'left';
    var px = (W - total) / 2;
    parts.forEach(function (p, i) {
      if (i) { x.fillStyle = C['red-text']; x.fillText(dot, px, 642); px += wd; }
      x.fillStyle = C.ink; x.fillText(p, px, 642); px += x.measureText(p).width;
    });
    x.textBaseline = 'alphabetic';
  }

  function draw(st, img) {
    var g = st.game || {}, home = g.home || {}, away = g.away || {}, n = names(st);
    var c = document.createElement('canvas');
    c.width = W; c.height = H;
    var x = c.getContext('2d');
    C = colours(st);
    backdrop(x, img.photo);
    lockup(x, n.home, g.sport_label || 'BASKET');
    plate(x, String((g.group && g.group.name) || '').toUpperCase());
    shield(x, HOME_X, home, img.home, C.red, '#FFFFFF');
    shield(x, AWAY_X, away, img.away, away.color || '#5C6170', GREY);
    vs(x);
    name(x, n.home, HOME_X);
    name(x, n.away, AWAY_X);
    if (g.at_home === true || g.at_home === false) tag(x, g.at_home ? 'EM CASA' : 'FORA', HOME_X);
    when(x, gameDate(st));
    return c;
  }

  function clean(c) {
    try { c.getContext('2d').getImageData(0, 0, 1, 1); return true; } catch (e) { return false; }
  }

  function render(st) {
    var g = st.game || {};
    return Promise.all([loadFonts(), loadImage('/assets/pavilhao-demo.jpg'), loadImage(g.home && g.home.crest_url), loadImage(g.away && g.away.crest_url)])
      .then(function (r) {
        var c = draw(st, { photo: r[1], home: r[2], away: r[3] });
        return clean(c) ? c : draw(st, {});
      });
  }

  function download(c, st) {
    return new Promise(function (resolve, reject) { c.toBlob(function (b) { b ? resolve(b) : reject(new Error('PNG vazio')); }, 'image/png'); })
      .then(function (blob) {
        var a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = fileName(st);
        document.body.appendChild(a); a.click(); a.remove();
        setTimeout(function () { URL.revokeObjectURL(a.href); }, 1000);
      });
  }

  window.Miniatura = { render: render, download: download, fileName: fileName };
})();
