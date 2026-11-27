/* Mock do servidor para testar o painel sem o programa: abrir com ?mock=1.
   Responde aos endpoints do docs/CONTRACT.md com dados em memória e um SSE a fingir. */
(function () {
  'use strict';
  if (!/[?&]mock=1\b/.test(location.search)) return;

  function logo(text, bg, fg) {
    var svg = '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="120" viewBox="0 0 240 120">' +
      '<rect width="240" height="120" rx="10" fill="' + bg + '"/>' +
      '<text x="120" y="72" font-family="Arial,sans-serif" font-size="30" font-weight="700" fill="' + fg + '" text-anchor="middle">' + text + '</text></svg>';
    return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
  }
  function crest(text, bg) {
    var svg = '<svg xmlns="http://www.w3.org/2000/svg" width="120" height="120" viewBox="0 0 120 120">' +
      '<path d="M60 6 L108 22 V60 C108 88 86 106 60 114 C34 106 12 88 12 60 V22 Z" fill="' + bg + '"/>' +
      '<text x="60" y="72" font-family="Arial,sans-serif" font-size="30" font-weight="700" fill="#fff" text-anchor="middle">' + text + '</text></svg>';
    return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
  }

  function photo(n) { return '/overlay/fixtures/photos/p' + n + '.svg'; }

  var nextId = 100;
  var db = {
    settings: {
      fpb_base_url: 'https://calendariofpb.pt', phone_access: false, pin: '',
      obs: { enabled: true, host: '127.0.0.1', port: 4455, password: '', timeout_scene: '', game_scene: 'Jogo', camera_source: '', camera_fit: 'contain', camera_full: null,
        cameras: [{ source: '', scene: '', label: '', clean: false }], graphics_scene: '', camera_fulls: {}, timeout_camera: 0, screen_cameras: { desconto: 0 },
        setup_at: '', launch: false, close_on_exit: false },
      start_with_windows: false,
      auto_screens: true, instagram: '', club: { name: 'CASA', short: 'CAS', primary: '#2F4B7C', secondary: '#1C1A17', crest: '/assets/crest.svg', sport: 'basketball', country: 'PT' },
      photos: { squad: true, lower_third: false },
      overlay: { bug_position: 'top-left' }, credit: true, desktop_shortcut: ''
    },
    groups: [
      { id: 'senior-m', name: 'Sénior Masculino', coach: 'Jorge Matos Lima', fpb_team_id: 64791, fpb_team_name: 'Clube Exemplo · Sénior', sport: 'basketball' },
      { id: 'sub16-m', name: 'Sub 16', coach: 'Ana Lopes', fpb_team_id: null, fpb_team_name: '', sport: 'basketball' },
      { id: 'seniores-basquetebol', name: 'Seniores', coach: '', fpb_team_id: 64792, fpb_team_name: 'Clube Exemplo · Seniores', sport: 'basketball' },
      { id: 'seniores-futsal', name: 'Seniores', coach: '', fpb_team_id: 64793, fpb_team_name: 'Clube Exemplo · Seniores', sport: 'futsal' }
    ],
    players: {
      'senior-m': [
        ['4', 'Tiago Marques', 9101, 'Tiago Filipe Marques'], ['5', 'Duarte Sousa', 9102, 'Duarte Sousa'],
        ['6', 'Miguel Pinto', null], ['7', 'João Carvalho', 9104, 'João Pedro Carvalho'],
        ['8', 'Rafael Antunes', null], ['9', 'Bruno Teixeira', 9106, 'Bruno Teixeira'],
        ['10', 'Gonçalo Ribeiro', null], ['11', 'Pedro Almeida', null], ['12', 'Diogo Martins', null],
        ['13', 'André Costa', null], ['14', 'Nuno Ferreira', null], ['15', 'Rodrigo Lopes', null], ['21', 'Francisco Gomes', null]
      ].map(function (r, i) { return { id: 'p' + (i + 1), number: r[0], display_name: r[1], fpb_player_id: r[2], fpb_name: r[3] || null, photo_url: r[2] ? photo(i % 3 + 1) : '' }; }),
      'sub16-m': [
        ['3', 'Tomás Leal'], ['5', 'Afonso Brito'], ['8', 'Martim Rocha'], ['11', 'Santiago Dias'], ['14', 'Vasco Pires'], ['20', 'Lourenço Matos']
      ].map(function (r, i) { return { id: 'j' + (i + 1), number: r[0], display_name: r[1], fpb_player_id: null, fpb_name: null, photo_url: '' }; }),
      'seniores-basquetebol': [{ id: 'x1', number: '9', display_name: 'Ex Jogador', fpb_player_id: null, fpb_name: null, photo_url: '', inactive: true }],
      'seniores-futsal': [{ id: 'x1', number: '9', display_name: 'Ex Jogador', fpb_player_id: null, fpb_name: null, photo_url: '', inactive: true }]
    },
    sponsors: {
      geral: [
        { id: 's1', name: 'Câmara Municipal', url: logo('CÂMARA', '#ffffff', '#181841'), principal: true },
        { id: 's2', name: 'Junta de Freguesia', url: logo('JUNTA', '#0F5C6E', '#ffffff'), principal: false }
      ],
      'senior-m': [
        { id: 's3', name: 'Padaria Estrela', url: logo('ESTRELA', '#D6BD2F', '#181841'), principal: true },
        { id: 's4', name: 'Óptica Central', url: logo('ÓPTICA', '#ffffff', '#BD1522'), principal: false },
        { id: 's5', name: 'Seguros Atlântico', url: logo('ATLÂNTICO', '#2E6FD8', '#ffffff'), principal: false },
        { id: 's6', name: 'Talho Silva', url: logo('SILVA', '#8B2223', '#ffffff'), principal: false }
      ],
      'sub16-m': [],
      'seniores-basquetebol': [],
      'seniores-futsal': []
    },
    suggestions: [
      { fpb_player_id: 9201, name: 'Miguel Ângelo Pinto', number: '6', games: 7, photo_url: photo(2), best_match: 'p3' },
      { fpb_player_id: 9202, name: 'Gonçalo Ribeiro Santos', number: '10', games: 5, photo_url: photo(3), best_match: 'p7' },
      { fpb_player_id: 9203, name: 'Henrique Vaz', number: '17', games: 3, photo_url: null, best_match: null },
      { fpb_player_id: 9204, name: 'Rafael Antunes', number: '8', games: 6, photo_url: photo(1), best_match: 'p5' }
    ],
    game: {
      group_id: 'senior-m', fpb_game_id: 55102,
      away: { name: 'FARPAS', short: 'FAR', color: '#2E6FD8', crest_url: crest('FAR', '#2E6FD8') },
      start: '2026-11-15T18:00:00+00:00', venue: 'Pavilhão Municipal do Vale',
      competition: 'Campeonato Regional', round: 'Jornada 5',
      squad: ['p1', 'p2', 'p3', 'p4', 'p5', 'p6', 'p7', 'p8', 'p9', 'p10'].map(function (id) { return { player_id: id, number: '' }; })
    },
    live: { home_name: 'CASA', away_name: 'VISITANTE', home: 41, away: 38, clock: '07:45', period: '2', home_fouls: 2, away_fouls: 4, shot: '14', sport: 'basketball' },
    screen: { current: 'marcador', auto: true, since: new Date().toISOString() },
    lower_third: { visible: false, type: 'jogador', number: '', first_name: '', last_name: '', label: '', until: null },
    demo: false
  };
  Object.assign(db.settings.overlay, { sponsor_corner: true, sponsor_every: 4 });
  db.settings.club_setup = '2026-11-01T10:00:00Z';
  // Promo clips play the fixture videos ("/promo/../overlay/…" resolves to them).
  function clip(id, name, o, dur, extra) {
    return Object.assign({ id: id, name: name, file: '../overlay/fixtures/promo-' + o + '.mp4', size: 7000, orientation: o, width: 0, height: 0, duration: dur,
      kicker: 'LOJA DO CLUBE', headline: 'Equipamento oficial', line: 'Disponível na receção do clube', link: '', show_qr: false,
      sound: false, volume: 60, end: 'back', added: new Date().toISOString() }, extra || {});
  }
  db.promo = { clips: [
    clip('c1', 'Loja · Halloween', 'vertical', 15, { link: 'https://exemplo.pt/loja', show_qr: true, sound: true }),
    clip('c2', 'Equipamento 2026/27', 'horizontal', 30),
    clip('c3', 'Campanha de sócios', 'horizontal', 20, { kicker: 'SÓCIOS', headline: 'Faça-se sócio' })
  ], timeout_clip: '' };
  db.promoShow = { clip: null, previous: '', seq: 0 };
  db.game.squad.forEach(function (s) { s.number = db.players['senior-m'].filter(function (p) { return p.id === s.player_id; })[0].number; });

  var team = { id: 64791, name: 'Clube Exemplo', short_name: 'CAS', logo_url: '' };
  function opp(id, name, short) { return { id: id, name: name, short_name: short, logo_url: '' }; }
  var fpbGames = {
    team: team,
    games: [
      { id: 55102, date: '2026-11-15', time: '18H00', location: 'Pavilhão Municipal do Vale', competition_name: 'Campeonato Regional', game_week: 5, phase_name: '1ª Fase', home: team, away: opp(311, 'FARPAS', 'FAR') },
      { id: 55118, date: '2026-11-22', time: '21H45', location: 'Pavilhão Municipal de Porto Novo', competition_name: 'Campeonato Regional', game_week: 6, phase_name: '1ª Fase', home: opp(312, 'Clube Desportivo de Porto Novo', 'CDO'), away: team },
      { id: 55131, date: '2026-11-29', time: '17H30', location: 'Pavilhão Municipal do Vale', competition_name: 'Campeonato Regional', game_week: 7, phase_name: '1ª Fase', home: team, away: opp(313, 'Sport Lagoa Seca', 'SAD') },
      { id: 55140, date: '2026-12-03', time: '20H00', location: 'Pavilhão do Lagoense', competition_name: 'Taça de Portugal', game_week: null, phase_name: '2ª Eliminatória', home: opp(314, 'Futebol Clube Lagoense', 'FCB'), away: team },
      { id: 55152, date: '2026-12-06', time: '18H00', location: 'Pavilhão Municipal do Vale', competition_name: 'Campeonato Regional', game_week: 8, phase_name: '1ª Fase', home: team, away: opp(315, 'Atlético Clube da Foz', 'ACP') }
    ]
  };
  var fpbTeams = [
    { id: 64791, name: 'Clube Exemplo', age: 'Sénior', gender: 'Masculino', slug: 'casa-senior-m', squad_label: '' },
    { id: 64802, name: 'Clube Exemplo', age: 'Sub 18', gender: 'Masculino', slug: 'casa-sub18-m', squad_label: '' },
    { id: 64811, name: 'Clube Exemplo', age: 'Sub 16', gender: 'Masculino', slug: 'casa-sub16-m', squad_label: '' },
    { id: 64812, name: 'Clube Exemplo', age: 'Sub 16', gender: 'Masculino', slug: 'casa-sub16-m-b', squad_label: 'B' },
    { id: 64820, name: 'Clube Exemplo', age: 'Sub 14', gender: 'Feminino', slug: 'casa-sub14-f', squad_label: '' }
  ];

  // Bluetooth: ?box=procura (searching, box not connected) or ?box=off (Bluetooth off). Default: connected.
  var boxMode = (/[?&]box=([a-z]+)/.exec(location.search) || [])[1] || 'ligado';
  var BOX = 'C4:4F:33:12:A0:9B';
  function dev(address, name, rssi, services) {
    return { address: address, name: name, name_quoted: JSON.stringify(name), rssi: rssi, services: services,
      likely: /stramatel|livestream|live stream/.test(name.trim().toLowerCase()) || services.indexOf('a002') >= 0 };
  }
  db.devices = boxMode === 'off' ? [] : [
    dev(BOX, 'Livestream Stramatel', -58, ['a002']),
    dev('5C:F3:70:8A:11:02', 'Galaxy A52 do Rui', -64, []),
    dev('F8:4E:17:C2:9D:40', 'JBL Tune 510BT', -73, ['fe2c']),
    dev('7A:21:9E:03:BB:5D', '', -79, []),
    dev('E0:6D:17:4A:C8:31', 'Mi Smart Band 6', -86, ['fee0']),
    dev('4B:D0:66:91:2E:A7', '', -93, ['fd6f'])
  ];
  // Gravar a consola: ?capture=on (recording, counting up), stopped (hit the cap) or off.
  var capMode = (/[?&]capture=([a-z]+)/.exec(location.search) || [])[1] || 'off';
  db.capture = { on: capMode === 'on', file: capMode === 'off' ? '' : 'captura-2026-11-28_19-42-10.txt', t0: Date.now(), base: 1840 };
  function captureStatus() {
    var c = db.capture, fr = c.on ? c.base + Math.floor((Date.now() - c.t0) / 250) : c.file ? c.base : 0;
    if (capMode === 'stopped' && !c.on) return { on: false, file: c.file, bytes: 52428800, frames: 1117000, dropped: 0, max: 52428800, stopped: 'cap' };
    return { on: c.on, file: c.file, bytes: c.file ? 150 + fr * 47 : 0, frames: fr, dropped: 0, max: 52428800, stopped: c.file && !c.on ? 'off' : '' };
  }
  db.ble = boxMode === 'ligado' ? { connected: true, saved: BOX } : { connected: false, saved: '' };
  db.settings.box_address = db.ble.saved;

  // Configuração inicial: ?setup=new (default), part (some ticks), done; ?pc=0 is a phone;
  // ?chan=ok (default), off (live not on), unverified, blocked, fail (the check fails).
  var setupMode = (/[?&]setup=([a-z]+)/.exec(location.search) || [])[1] || 'new';
  var onPC = !/[?&]pc=0\b/.test(location.search);
  // &update=1 offers 9.9.9 to install, link only links to it, down is downloading, fail has failed, ask asks once, restart
  // stays on restarting, busy answers Atualizar agora with 409. After Atualizar agora each status poll moves the install on: installing,
  // restarting, two polls with no answer (the old copy has quit), then the new version.
  var updMode = (/[?&]update=([a-z0-9]+)/.exec(location.search) || [])[1] || '';
  var updState = { down: 'downloading', fail: 'failed', restart: 'restarting' }[updMode] || '', updPolls = 0;
  function updateStep() {
    if (updMode === 'down' || updMode === 'restart') return '';
    if (updState === 'downloading') updState = 'installing';
    else if (updState === 'installing') updState = 'restarting';
    else if (updState === 'restarting' && ++updPolls > 2) return 'new';
    else if (updState === 'restarting') return 'gone';
    return '';
  }
  function updateStatus() {
    if (!updMode || updMode === 'ask') return null;
    return { version: '9.9.9', url: 'https://github.com/kauredo/hallstream/releases/tag/v9.9.9', on_pc: onPC, install: onPC && updMode !== 'link',
      state: onPC ? updState : '', progress: onPC && updState === 'downloading' ? 38 : -1,
      error: onPC && updState === 'failed' ? 'A cópia que descarreguei não bate certo com a publicada, por isso não a usei.' : '' };
  }
  var chanMode = (/[?&]chan=([a-z]+)/.exec(location.search) || [])[1] || 'ok';
  db.setup = { done: setupMode === 'done' ? '2026-11-09T19:30:00Z' : '', later: false,
    steps: setupMode === 'new' ? {} : { 'ensaio-demo': 'done', 'ensaio-previa': 'done' } };
  if (setupMode === 'done') ['ensaio-desconto', 'ensaio-promo', 'canal-diretos', 'canal-verificado'].forEach(function (k) { db.setup.steps[k] = 'done'; });
  function setupReply() { return reply(200, Object.assign({ pc: onPC }, JSON.parse(JSON.stringify(db.setup)))); }
  function scan() {
    if (boxMode === 'off') return { scanning: false, devices_seen: 0, bluetooth: 'off', message: 'O Bluetooth do computador está desligado ou não existe.', address: '', name: '', saved: db.ble.saved };
    if (db.ble.connected) return { scanning: false, devices_seen: db.devices.length, bluetooth: 'ok', message: 'Ligado à caixa.', address: BOX, name: 'Livestream Stramatel', saved: db.ble.saved };
    return { scanning: true, devices_seen: db.devices.length, bluetooth: 'ok', address: '', name: '', saved: db.ble.saved,
      message: 'Vejo ' + db.devices.length + ' dispositivos, mas nenhum parece a caixa. Feche a app SL Live Stream no tablet ou experimenta um da lista.' };
  }

  function group(id) { return db.groups.filter(function (g) { return g.id === id; })[0]; }
  function state() {
    var g = db.game, grp = group(g.group_id) || {};
    var players = db.players[g.group_id] || [];
    var sp = (db.sponsors[g.group_id] || []).concat(db.sponsors.geral || []);
    var principal = sp.filter(function (s) { return s.principal; })[0];
    return {
      box: { connected: db.ble.connected, last_frame_at: db.ble.connected ? new Date(Date.now() - 1000).toISOString() : null, box_type: 'Pro', scan: scan() },
      live: db.live,
      periods: [{ period: 1, home: 22, away: 19, home_points: 22, away_points: 19 }],
      game: {
        group: { id: grp.id, name: grp.name, coach: grp.coach },
        home: { name: db.settings.club.name, short: db.settings.club.short, crest_url: db.settings.club.crest, color: db.settings.club.primary, initials: db.settings.club.short },
        sport: grp.sport || db.settings.club.sport, sport_label: 'BASKET',
        away: { name: g.away.name, short: g.away.short, crest_url: g.away.crest_url || null, color: g.away.color || '#5C6170', initials: g.away.short },
        start: g.start, venue: g.venue, competition: g.competition, round: g.round,
        squad: g.squad.map(function (s) {
          var p = players.filter(function (x) { return x.id === s.player_id; })[0];
          return { number: s.number, name: p ? p.display_name : '', photo_url: (p && p.photo_url) || '' };
        }),
        sponsors: { principal: principal ? { url: principal.url, name: principal.name } : null, others: sp.filter(function (s) { return s !== principal; }).map(function (s) { return { url: s.url, name: s.name }; }) },
        instagram: db.settings.instagram,
        photos: db.settings.photos || { squad: false, lower_third: false },
        overlay: Object.assign({ bug_position: 'top-left', sponsor_corner: true, sponsor_every: 4, back_line: '' }, db.settings.overlay),
        credit: db.settings.credit !== false,
        at_home: false,
        next: { at_home: true, opponent: { name: 'RIBEIRA', short: 'RBC', crest_url: null, color: '#2E3192', initials: 'RBC' }, full_name: 'Ribeira Basket Clube',
          start: '2026-11-20T21:45:00Z', venue: 'Pav. Municipal do Vale - Clube Exemplo', competition: '2ª Divisão Masculina', round: 'Jornada 7' },
        next_note: ''
      },
      screen: db.screen,
      lower_third: db.lower_third,
      obs: { live_camera: obsMode === 'off' ? 0 : db.obsLive, clean: obsMode !== 'off' && !!((db.settings.obs.cameras || [])[db.obsLive - 1] || {}).clean,
        inset_camera: db.inset, inset: mockInset() },
      promo: Object.assign({}, db.promoShow, { mini: db.promo.clips.filter(function (c) { return c.id === db.promo.timeout_clip; })[0] || null }),
      demo: db.demo
    };
  }

  var streams = [];
  function emit() {
    var data = JSON.stringify(state());
    streams.forEach(function (s) { (s.l.state || []).forEach(function (f) { f({ data: data }); }); });
  }
  function FakeES() {
    var self = this; this.l = {}; streams.push(this);
    setTimeout(function () { if (self.onopen) self.onopen(); emit(); }, 30);
  }
  FakeES.prototype.addEventListener = function (t, f) { (this.l[t] = this.l[t] || []).push(f); };
  FakeES.prototype.close = function () { var i = streams.indexOf(this); if (i >= 0) streams.splice(i, 1); };
  window.EventSource = FakeES;

  var secs = 7 * 60 + 45;
  setInterval(function () {
    if (secs > 0) secs--;
    db.live.clock = (secs < 600 ? '0' : '') + Math.floor(secs / 60) + ':' + ('0' + secs % 60).slice(-2);
    db.live.shot = String(Math.max(0, (+db.live.shot || 24) - 1) || 24);
    if (Math.random() < 0.08) db.live[Math.random() < 0.5 ? 'home' : 'away'] += Math.random() < 0.3 ? 3 : 2;
    if (db.lower_third.visible && db.lower_third.until && new Date(db.lower_third.until) < new Date()) db.lower_third.visible = false;
    emit();
  }, 1000);

  function reply(status, data) {
    return new Promise(function (resolve) {
      setTimeout(function () {
        resolve(new Response(data === undefined ? '' : JSON.stringify(data), { status: status, headers: { 'Content-Type': 'application/json' } }));
      }, 180);
    });
  }
  function err(status, msg) { return reply(status, { error: msg }); }
  function fileUrl(fd) {
    var f = fd.get('file');
    return f ? URL.createObjectURL(f) : '';
  }

  // Cameras: ?cams=1|2|3|4|noscene (three, camera 3 without a scene yet)|vdo (three, camera 2 a phone), camera 3 clean; the one on program ?live=0|1|2|3|4
  // (0: a scene that is no camera's), and ?camerr=1 makes OBS refuse a switch.
  var camsQ = (/[?&]cams=([a-z0-9]+)/.exec(location.search) || [])[1];
  var CAMS = [{ source: 'DroidCam Video', scene: 'Câmara 1', label: '', clean: false }, { source: 'Câmara HDMI', scene: 'Câmara 2', label: 'Bancada', clean: false }, { source: 'HD Webcam', scene: 'Câmara 3', label: '', clean: true }, { source: 'Câmara USB', scene: 'Câmara 4', label: 'Cesto visitante', clean: false }];
  if (camsQ === 'noscene') db.settings.obs.cameras = [CAMS[0], CAMS[1], Object.assign({}, CAMS[2], { scene: '' })];
  else if (camsQ === 'vdo') db.settings.obs.cameras = [CAMS[0], { source: 'Telemóvel 1', scene: 'Câmara 2', label: 'Bancada', vdo: 'mockmockmockmockmock', has_vdo: true }, CAMS[2]];
  else if (camsQ) db.settings.obs.cameras = CAMS.slice(0, Math.min(4, Math.max(1, +camsQ || 1)));
  var liveQ = /[?&]live=([0-4])/.exec(location.search);
  db.obsLive = liveQ ? +liveQ[1] : 1;
  // The inset's camera: ?inset=0|1|2|3|4; ?inseterr=1 makes OBS refuse it.
  var insetQ = /[?&]inset=([0-4])/.exec(location.search);
  db.inset = insetQ ? +insetQ[1] : 0;
  function mockInset() {
    var c = (db.settings.obs.cameras || [])[db.inset - 1], cur = db.screen.current;
    if (obsMode === 'off' || !c || db.inset === db.obsLive || (cur !== 'marcador' && cur !== 'nada')) return null;
    return { label: c.label || 'Câmara ' + db.inset, x: 1376, y: 746, w: 480, h: 270 };
  }
  // The camera for time-outs: ?tcam=0|1|2|3|4; for the Nada screen: ?ncam=0|1|2|3|4.
  var tcamQ = /[?&]tcam=([0-4])/.exec(location.search), ncamQ = /[?&]ncam=([0-4])/.exec(location.search);
  db.settings.obs.timeout_camera = tcamQ ? +tcamQ[1] : 0;
  db.settings.obs.screen_cameras = { desconto: db.settings.obs.timeout_camera, nada: ncamQ ? +ncamQ[1] : 0 };

  // OBS as the panel sees it: ?obs=off (default), ready, problem, live, nokey, choose.
  var obsMode = (/[?&]obs=([a-z]+)/.exec(location.search) || [])[1] || 'off';
  var obsLive = obsMode === 'live' ? Date.now() - 4723000 : 0;
  function streamNow() { return obsMode === 'off' ? null : { active: !!obsLive, reconnecting: false, duration_ms: obsLive ? Date.now() - obsLive : 0 }; }
  // Camera health: ?health=ok (default), noimage, frozen, mixed (ok, no_image, frozen for cameras 1 to 3, ok for camera 4), skipped,
  // none (a server older than camera health: no cameras or skipped in the status).
  var healthQ = (/[?&]health=([a-z]+)/.exec(location.search) || [])[1] || 'ok';
  function camHealth() {
    var one = { noimage: 'no_image', frozen: 'frozen' }[healthQ] || 'ok';
    return (db.settings.obs.cameras || [{}]).map(function (c, i) {
      return { index: i + 1, state: healthQ === 'mixed' ? ['ok', 'no_image', 'frozen', 'ok'][i] : one };
    });
  }
  function obsStatus() {
    if (obsMode === 'off') return { connected: false, error: 'O OBS não responde em 127.0.0.1:4455', health: { checked: false, ready: false, problems: [] }, stream: null, cameras: [], skipped: null };
    var h = obsMode === 'problem'
      ? { checked: true, ready: false, problems: ['A fonte Hallstream overlay está mal: o endereço era http://127.0.0.1:8766/overlay.'] }
      : obsMode === 'choose' ? { checked: true, ready: false, problems: ['A cena Câmara 1 não tem câmara.'] }
      : { checked: true, ready: true, problems: [] };
    if (healthQ === 'none') return { connected: true, error: '', health: h, stream: streamNow() };
    return { connected: true, error: '', health: h, stream: streamNow(), cameras: camHealth(),
      skipped: healthQ === 'skipped' ? { frames: 214, total: 3600 } : null };
  }
  var cameras = [
    { value: 'device:DroidCam Video:\\\\?\\root#image#0000', type: 'device', label: 'DroidCam Video', droidcam: true },
    { value: 'source:Câmara HDMI', type: 'source', label: 'Câmara HDMI', droidcam: false },
    { value: 'device:HD Webcam:\\\\?\\usb#vid_0c45', type: 'device', label: 'HD Webcam', droidcam: false }
  ];
  // The cameras a setup ends with: the list it was sent (a new phone gets an id), else the saved ones.
  function setupCameras(b) {
    var old = db.settings.obs.cameras || [], phones = 0;
    if (!(b && b.cameras)) return [Object.assign({}, old[0] || { label: '' }, { source: 'DroidCam Video', scene: 'Câmara 1' })].concat(old.slice(1));
    return b.cameras.map(function (r, i) {
      var was = old[r.was - 1] || {}, c = { source: '', scene: was.scene || 'Câmara ' + (i + 1), label: r.label, clean: r.clean == null ? !!was.clean : !!r.clean };
      // A saved phone keeps its id while its row keeps its source, as on the server.
      if (was.vdo && r.choice === 'source:' + was.source) { c.source = was.source; c.vdo = was.vdo; c.has_vdo = true; }
      else if (r.choice === 'vdo') { c.source = 'Telemóvel ' + (++phones + 1); c.vdo = ('mock' + Math.random().toString(36).slice(2) + 'xxxxxxxxxxxxxxxx').slice(0, 20); c.has_vdo = true; }
      else if (/^source:/.test(r.choice)) c.source = r.choice.slice(7);
      else if (r.choice === 'plugin') c.source = 'DroidCam';
      else c.source = (cameras.filter(function (x) { return x.value === r.choice; })[0] || { label: 'DroidCam Video' }).label;
      return c;
    });
  }
  function setupReport(b) {
    var step = function (id, label, result, detail) { return { id: id, label: label, result: result, detail: detail }; };
    // ?setupfail=noobs: OBS does not answer, so a camera list is not saved.
    if (/[?&]setupfail=noobs\b/.test(location.search)) {
      return { ok: false, ready: false, saved: false, camera: '', graphics: '', cameras: null, steps: [
        step('websocket', 'Ligação ao OBS', 'failed', 'O OBS não responde em 127.0.0.1:4455.')] };
    }
    if (obsMode === 'choose' && !(b && b.camera)) {
      return { ok: false, ready: false, camera: '', graphics: 'Grafismo', cameras: [{ source: '', scene: 'Câmara 1' }], choices: cameras, steps: [
        step('websocket', 'Ligação ao OBS', 'ok', 'O OBS responde.'),
        step('graphics', 'Cena do grafismo (Grafismo)', 'created', 'Criei a cena Grafismo.'),
        step('overlay', 'Grafismo (fonte Navegador)', 'created', 'Criei a fonte Hallstream overlay, 1920x1080, com o som pelo OBS.'),
        step('scene', 'Cena Câmara 1', 'created', 'Criei a cena Câmara 1.'),
        step('camera', 'Câmara', 'choose', 'O OBS tem várias câmaras. Escolha uma e carregue em Usar esta câmara.'),
        step('video', 'Resolução 1920x1080', 'ok', 'O OBS já está em 1920x1080.'),
        step('program', 'Cena no ar', 'fixed', 'Mudei o OBS da cena Cena para Câmara 1.')] };
    }
    obsMode = 'ready';
    cameras[0] = { value: 'source:DroidCam Video', type: 'source', label: 'DroidCam Video', droidcam: true };
    db.settings.obs.setup_at = new Date().toISOString(); db.settings.obs.launch = true; db.settings.obs.camera_source = 'DroidCam Video';
    db.settings.obs.graphics_scene = 'Grafismo';
    db.settings.obs.cameras = setupCameras(b);
    db.settings.obs.camera_source = db.settings.obs.cameras[0].source;
    return { ok: true, ready: true, saved: !!(b && b.cameras), camera: db.settings.obs.camera_source, graphics: 'Grafismo', cameras: db.settings.obs.cameras.map(function (c) { return { source: c.source, scene: c.scene }; }), steps: [
      step('websocket', 'Ligação ao OBS', 'fixed', 'Liguei o servidor WebSocket do OBS, com uma palavra-passe nova. Abri o OBS.'),
      step('graphics', 'Cena do grafismo (Grafismo)', 'created', 'Criei a cena Grafismo.'),
      step('overlay', 'Grafismo (fonte Navegador)', 'fixed', 'Corrigi a fonte Hallstream overlay: o endereço era http://127.0.0.1:8766/overlay, não estava na cena.'),
      step('scene', 'Cena Câmara 1', 'created', 'Criei a cena Câmara 1.'),
      step('camera', 'Câmara', 'created', 'Juntei a câmara DroidCam Video à cena.'),
      step('order', 'Grafismo por cima da câmara', 'fixed', 'Pus a cena Grafismo por cima da câmara na cena Câmara 1.'),
      step('video', 'Resolução 1920x1080', 'fixed', 'Mudei a resolução de 1280x720 para 1920x1080.'),
      step('program', 'Cena no ar', 'ok', 'O OBS mostra a cena Câmara 1.'),
      step('fit', 'Câmara ajustada ao ecrã', 'ok', 'A câmara ocupa o ecrã inteiro.')] };
  }

  // YouTube: ?yt=off (default, not set up), on, expired, ended (the unlisted card), endfail (privacy change failed).
  var ytMode = (/[?&]yt=([a-z]+)/.exec(location.search) || [])[1] || 'off';
  var avatar = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="88" height="88"><rect width="88" height="88" fill="#BD1522"/><text x="44" y="56" font-family="Arial" font-size="30" font-weight="700" fill="#fff" text-anchor="middle">CAS</text></svg>');
  var yt = { setup: onPC, configured: ytMode !== 'off', has_secret: ytMode !== 'off', client_id: ytMode !== 'off' ? '123456789012-abcdefg.apps.googleusercontent.com' : '',
    connected: ytMode !== 'off', expired: ytMode === 'expired', connecting: false, error: '', stream_ready: ytMode !== 'off',
    channel: ytMode !== 'off' ? { id: 'UC1', title: 'Clube Exemplo · Basquetebol', thumbnail: avatar } : null };
  function rec(id, title, escalao, date, result, privacy, external) {
    return { broadcast_id: id, video_id: id, title: title, escalao: escalao, date: date, result: result, score: '', privacy: privacy,
      link: 'https://youtu.be/' + id, external: !!external, thumbnail_set: true, privacy_error: '' };
  }
  var ytHist = [
    rec('kQ7mN2xVb4E', 'Casa vs Atlético Vale · Sénior Masculino · 2ª Divisão Masculina J6', 'Sénior Masculino', '2026-11-08T18:00:00Z', 'Casa 82-76 Atlético Vale', 'unlisted'),
    rec('Hc81LwPz0aQ', 'Lagoa Seca vs Casa · Sub 16 Masculino · Distrital J4', 'Sub 16 Masculino', '2026-11-02T11:00:00Z', 'Casa 54-61 Lagoa Seca', 'unlisted'),
    rec('Zt5vRb3m9Ks', 'Apresentação das equipas 2026/27', '', '2026-10-18T16:30:00Z', '', 'public', true),
    rec('p2YwQe7Jd1M', 'Casa vs Porto Novo · Sénior Masculino · 2ª Divisão Masculina J3', 'Sénior Masculino', '2026-10-12T21:00:00Z', 'Casa 70-65 Porto Novo', 'public'),
    rec('bN4sT6uLx8C', 'Casa vs Lagoense · Sénior Masculino · Taça de Portugal', 'Sénior Masculino', '2026-05-23T18:00:00Z', 'Casa 77-80 Lagoense', 'unlisted')
  ];
  var ytLive = null, ytEnded = null;
  if (ytMode === 'ended' || ytMode === 'endfail') {
    ytEnded = Object.assign({}, ytHist[0], ytMode === 'endfail'
      ? { privacy: 'public', privacy_error: 'Não consegui tornar o vídeo não listado: o YouTube respondeu 500 backendError: Backend Error' } : {});
  }
  function ytSummary() {
    if (!yt.connected) return null;
    if (obsLive && !ytLive) ytLive = Object.assign({}, ytHist[0], { privacy: 'public', result: '', on_air: true });
    return { connected: true, expired: yt.expired, live: obsLive ? ytLive : null, ended: ytEnded, waiting: false, error: '' };
  }
  function seasonOf(iso) { var d = new Date(iso), y = d.getFullYear() - (d.getMonth() < 7 ? 1 : 0); return y + '/' + String((y + 1) % 100).padStart(2, '0'); }
  function ytHistory(q) {
    var items = ytHist.filter(function (r) { return yt.connected || !r.external; });
    var esc = [], sea = [];
    items.forEach(function (r) {
      var g = r.external ? 'Fora do Hallstream' : r.escalao, s = seasonOf(r.date);
      if (g && esc.indexOf(g) < 0) esc.push(g);
      if (sea.indexOf(s) < 0) sea.push(s);
    });
    var e = q.get('escalao'), s = q.get('season');
    return { connected: yt.connected, remote_error: '', escaloes: esc.sort(), seasons: sea.sort().reverse(),
      items: items.filter(function (r) { return (!e || (r.external ? 'Fora do Hallstream' : r.escalao) === e) && (!s || seasonOf(r.date) === s); }) };
  }

  var realFetch = window.fetch.bind(window);
  window.fetch = function (input, opts) {
    opts = opts || {};
    var u = new URL(typeof input === 'string' ? input : input.url, location.href);
    if (u.pathname.indexOf('/api/') !== 0) return realFetch(input, opts);
    var m = (opts.method || 'GET').toUpperCase(), p = u.pathname, b = null, r;
    if (typeof opts.body === 'string') { try { b = JSON.parse(opts.body); } catch (e) { b = null; } }
    else b = opts.body;

    if (p === '/api/settings') {
      if (m === 'PUT') { db.settings = b; }
      return reply(200, db.settings);
    }
    if (p === '/api/status') {
      var step = updateStep();
      if (step === 'gone') return err(502, 'Failed to fetch');
      return reply(200, { version: step === 'new' ? '9.9.9' : '0.9.0-mock', port: 8765, overlay_url: 'http://127.0.0.1:8765/overlay', lan_url: 'http://192.168.1.40:8765/painel',
        box: state().box, obs: obsStatus(), youtube: yt, fpb: { reachable: true, cached_at: new Date().toISOString() },
        data_dir: 'C:\\Hallstream\\dados', shortcut: onPC, autostart: onPC, obs_app: true, obs_close: true, os: 'windows', club_setup: { running: false, done: 0, total: 0 },
        update: updateStatus(), update_check_ask: onPC && updMode === 'ask' });
    }
    if (p === '/api/update') {
      if (!onPC) return err(403, 'Isto só se muda no próprio computador.');
      if (updMode === 'busy' || updState === 'downloading' || updState === 'installing') return err(409, 'Já estou a atualizar.');
      if (obsMode === 'live') return err(409, 'A transmissão está no ar. Atualize depois de a terminar.');
      updState = 'downloading';
      return reply(200, { ok: true, version: '9.9.9' });
    }
    if (p === '/api/update/ask') {
      if (!onPC) return err(403, 'Isto só se muda no próprio computador.');
      db.settings.update_asked = true; db.settings.update_check = db.settings.update_check || !!(b && b.check); updMode = '';
      return reply(200, db.settings);
    }
    if (p === '/api/setup') return setupReply();
    if ((r = /^\/api\/setup\/steps\/([a-z-]+)$/.exec(p))) {
      if (b.state) db.setup.steps[r[1]] = b.state; else delete db.setup.steps[r[1]];
      return setupReply();
    }
    if (p === '/api/setup/done') { db.setup.done = new Date().toISOString(); db.setup.later = false; return setupReply(); }
    if (p === '/api/setup/later') { db.setup.later = true; return setupReply(); }
    if (p === '/api/setup/channel') {
      if (!yt.connected) return err(409, 'Ligue primeiro o YouTube, no passo anterior.');
      if (chanMode === 'fail') return err(502, 'sem ligação ao YouTube: dial tcp: i/o timeout');
      return reply(200, { live: chanMode === 'off' ? 'off' : 'on', verified: { unverified: 'no', blocked: 'blocked' }[chanMode] || (chanMode === 'off' ? 'no' : 'yes') });
    }
    if (p === '/api/groups') {
      if (m === 'POST') {
        if (!b || !b.name) return err(400, 'O escalão precisa de nome.');
        var g = { id: 'g' + (nextId++), name: b.name, coach: '', fpb_team_id: null, fpb_team_name: '', sport: db.settings.club.sport };
        db.groups.push(g); db.players[g.id] = []; db.sponsors[g.id] = [];
        return reply(201, g);
      }
      return reply(200, db.groups);
    }
    if ((r = /^\/api\/groups\/([^/]+)$/.exec(p))) {
      var gid = decodeURIComponent(r[1]), grp = group(gid);
      if (!grp) return err(404, 'Escalão não encontrado.');
      if (m === 'PUT') {
        // The server's two refusals, so the panel's error path can be tried.
        if (b && b.sport !== undefined && ['basketball', 'futsal', 'handball', 'soccer', 'volleyball'].indexOf(b.sport) < 0) return err(400, 'Escolha uma modalidade disponível para este escalão.');
        var after = Object.assign({}, grp, b);
        if ((b && (b.sport !== undefined || b.fpb_team_id !== undefined)) && after.fpb_team_id && after.sport !== 'basketball') {
          return err(400, 'Este escalão está ligado a uma equipa da FPB, que é de basquetebol. Desligue-o primeiro.');
        }
        Object.assign(grp, b);
        return reply(200, grp);
      }
      if (m === 'DELETE') { db.groups.splice(db.groups.indexOf(grp), 1); return reply(204); }
    }
    if ((r = /^\/api\/groups\/([^/]+)\/players(?:\/([^/]+))?(?:\/(link))?$/.exec(p))) {
      var gp = decodeURIComponent(r[1]), list = db.players[gp] || (db.players[gp] = []), pid = r[2] && decodeURIComponent(r[2]);
      if (pid === 'import' && m === 'POST') {
        var linked = list.filter(function (x) { return x.fpb_player_id; });
        var taken = {}; list.forEach(function (x) { if (x.fpb_player_id) taken[x.fpb_player_id] = 1; });
        return reply(200, { updated: linked.slice(0, 2), suggestions: db.suggestions.filter(function (s) { return !taken[s.fpb_player_id]; }) });
      }
      if (pid === 'from-fpb' && m === 'POST') {
        var np = { id: 'p' + (nextId++), display_name: b.name, number: b.number || '', fpb_player_id: b.fpb_player_id, fpb_name: b.name, photo_url: b.photo_url || '' };
        list.push(np); return reply(201, np);
      }
      if (!pid) {
        if (m === 'POST') {
          if (!b.display_name) return err(400, 'O jogador precisa de nome.');
          if (b.number && list.some(function (x) { return x.number === b.number; })) return err(409, 'Já há um jogador com o número ' + b.number + '.');
          var pl = { id: 'p' + (nextId++), display_name: b.display_name, number: b.number, fpb_player_id: null, fpb_name: null, photo_url: '' };
          list.push(pl); return reply(201, pl);
        }
        return reply(200, list);
      }
      var player = list.filter(function (x) { return x.id === pid; })[0];
      if (!player) return err(404, 'Jogador não encontrado.');
      if (r[3]) {
        if (m === 'POST') { player.fpb_player_id = b.fpb_player_id; player.fpb_name = b.fpb_name; player.photo_url = b.photo_url || ''; }
        else { player.fpb_player_id = null; player.fpb_name = null; player.photo_url = ''; }
        return reply(200, player);
      }
      if (m === 'PUT') { player.display_name = b.display_name; player.number = b.number; return reply(200, player); }
      if (m === 'DELETE') { list.splice(list.indexOf(player), 1); return reply(204); }
    }
    if (/^\/api\/groups\/[^/]+\/(fixtures|calendar)$/.test(p) && m === 'GET') return reply(200, []);
    if ((r = /^\/api\/groups\/([^/]+)\/sponsors(?:\/([^/]+))?$/.exec(p))) {
      var sg = decodeURIComponent(r[1]), sl = db.sponsors[sg] || (db.sponsors[sg] = []);
      if (r[2] && m === 'DELETE') { db.sponsors[sg] = sl.filter(function (s) { return s.id !== r[2]; }); return reply(204); }
      if (m === 'POST') {
        var sp = { id: 's' + (nextId++), name: b.get('name'), url: fileUrl(b), principal: sl.length === 0 };
        sl.push(sp); return reply(201, sp);
      }
      if (m === 'PUT') {
        db.sponsors[sg] = b.map(function (o) { var s = sl.filter(function (x) { return x.id === o.id; })[0]; s.principal = !!o.principal; return s; });
        return reply(200, db.sponsors[sg]);
      }
      return reply(200, sl);
    }
    if (p === '/api/fpb/teams') {
      var q = (u.searchParams.get('name') || '').toLowerCase();
      return reply(200, fpbTeams.filter(function (t) { return t.name.toLowerCase().indexOf(q) >= 0; }));
    }
    if ((r = /^\/api\/fpb\/teams\/([^/]+)\/next-games$/.exec(p))) return reply(200, fpbGames);
    if (p === '/api/game') {
      if (m === 'PUT') {
        db.game = b;
        if (b.fpb_game_id && !b.away.crest_url) {
          var palette = { FARPAS: '#2E6FD8', CDO: '#1F6E3A', SAD: '#5B2A86', FCB: '#B4410C', ACP: '#0F5C6E' };
          var col = b.away.color || palette[b.away.name] || '#5C6170';
          b.away.color = col; b.away.crest_url = crest(b.away.short, col);
        }
        if (!b.away.color) b.away.color = '#5C6170';
      }
      return reply(200, db.game);
    }
    if (p === '/api/game/opponent-logo') return reply(200, { crest_url: fileUrl(b) });
    if (p === '/api/club/crest') {
      db.settings.club.crest = m === 'DELETE' ? '/assets/crest.svg' : fileUrl(b);
      emit();
      return reply(200, db.settings.club);
    }
    if (p === '/api/game/reset') return reply(200, {});
    if (p === '/api/screen') { db.screen = { current: b.screen, auto: !!b.auto, since: new Date().toISOString() }; emit(); return reply(200, db.screen); }
    if (p === '/api/lower-third') {
      if (m === 'DELETE') db.lower_third = { visible: false, type: 'jogador', until: null };
      else if (b.player_id) {
        var gpl = (db.players[db.game.group_id] || []).filter(function (x) { return x.id === b.player_id; })[0];
        var sq = db.game.squad.filter(function (s) { return s.player_id === b.player_id; })[0];
        var parts = gpl.display_name.split(' ');
        db.lower_third = { visible: true, type: 'jogador', number: (sq && sq.number) || gpl.number, first_name: parts[0], last_name: parts.slice(1).join(' '), label: '', photo_url: gpl.photo_url || '', until: new Date(Date.now() + (b.seconds || 8) * 1000).toISOString() };
      } else db.lower_third = { visible: true, type: 'comentador', number: '', first_name: b.first_name, last_name: b.last_name, label: b.label, until: new Date(Date.now() + 8000).toISOString() };
      emit(); return reply(200, db.lower_third);
    }
    if (p === '/api/promo') {
      if (m === 'POST') {
        var f = b.get('file'), nc = clip('c' + (nextId++), f.name.replace(/\.[^.]+$/, ''), '', 0);
        nc.file = ''; db.promo.clips.push(nc); return reply(201, nc);
      }
      if (m === 'PUT') db.promo.timeout_clip = b.timeout_clip;
      return reply(200, db.promo);
    }
    if (p === '/api/promo/stop') {
      if (db.screen.current === 'promo') db.screen = { current: db.promoShow.previous || 'marcador', auto: db.screen.auto, since: new Date().toISOString() };
      db.promoShow.clip = null; emit(); return reply(200, { ok: true });
    }
    if ((r = /^\/api\/promo\/([^/]+)(\/show)?$/.exec(p))) {
      var pc = db.promo.clips.filter(function (c) { return c.id === r[1]; })[0];
      if (!pc) return err(404, 'Vídeo não encontrado.');
      if (r[2]) {
        if (db.screen.current !== 'promo') db.promoShow.previous = db.screen.current;
        db.promoShow.clip = { id: pc.id, url: '/promo/' + pc.file }; db.promoShow.seq++;
        db.screen = { current: 'promo', auto: db.screen.auto, since: new Date().toISOString() };
        emit(); return reply(200, { ok: true });
      }
      if (m === 'DELETE') { db.promo.clips.splice(db.promo.clips.indexOf(pc), 1); return reply(200, { ok: true }); }
      ['name', 'kicker', 'headline', 'line', 'show_qr', 'sound', 'volume', 'end', 'orientation', 'width', 'height', 'duration'].forEach(function (k) { if (k in b) pc[k] = b[k]; });
      pc.link = b.link && !/:\/\//.test(b.link) ? 'https://' + b.link : (b.link || '');
      pc.show_qr = pc.show_qr && !!pc.link;
      return reply(200, pc);
    }
    if (p === '/api/demo') { db.demo = !!b.on; emit(); return reply(200, { on: db.demo }); }
    if (p === '/api/ble/devices') {
      var now = new Date().toISOString();
      return reply(200, db.devices.map(function (d) { return Object.assign({ last_seen: now }, d, { rssi: d.rssi + Math.round(Math.random() * 4 - 2) }); }));
    }
    if (p === '/api/ble/try') {
      return new Promise(function (resolve) { setTimeout(resolve, 1400); }).then(function () {
        if (b.address !== BOX) return reply(200, { ok: false, error: 'Este dispositivo não é a caixa (não tem o serviço do marcador).' });
        db.ble = { connected: true, saved: BOX }; db.settings.box_address = BOX; emit();
        return reply(200, { ok: true });
      });
    }
    if (p === '/api/ble/capture') {
      if (m === 'POST') {
        db.capture.on = !!b.on; capMode = 'live';
        if (b.on) { db.capture.file = 'captura-2026-11-28_19-42-10.txt'; db.capture.t0 = Date.now(); db.capture.base = 0; } else db.capture.base += Math.floor((Date.now() - db.capture.t0) / 250);
      }
      return reply(200, captureStatus());
    }
    if (p === '/api/system/capture-folder') return reply(200, { ok: true });
    if (p === '/api/ble/forget') { db.ble = { connected: false, saved: '' }; emit(); return reply(200, { ok: true }); }
    if (p === '/api/system/bluetooth-settings') return reply(200, { ok: true });
    if (p === '/api/system/desktop-shortcut') {
      db.settings.desktop_shortcut = b && b.create ? 'created' : 'declined';
      return reply(200, { ok: true, path: b && b.create ? 'C:\\Users\\Clube\\Desktop\\Hallstream.lnk' : '' });
    }
    if (p === '/api/obs/stream') return reply(200, { known: obsMode !== 'off', stream: streamNow(), youtube: ytSummary() });
    if (p === '/api/obs/stream/start') {
      if (yt.expired) return err(409, 'A ligação ao YouTube expirou. Em Definições > YouTube, carregue em Ligar ao YouTube outra vez.');
      if (obsMode === 'nokey') return err(409, 'Configure a conta do YouTube no OBS (Definições > Transmissão) uma vez.');
      if (b && b.test && (!yt.connected || yt.expired)) return err(409, 'A transmissão de teste precisa do YouTube ligado em Definições > YouTube.');
      obsLive = Date.now(); ytLive = null; var ys = ytSummary();
      if (ys && ys.live) { ys.live.thumbnail_set = false; if (b && b.test) Object.assign(ys.live, { test: true, privacy: 'private', title: 'Teste · ' + ys.live.title }); }
      return reply(200, { known: true, stream: streamNow(), youtube: ys });
    }
    if (p === '/api/obs/stream/stop') {
      obsLive = 0;
      if (yt.connected && ytLive) { ytEnded = Object.assign({}, ytLive, { on_air: false, result: 'Casa ' + db.live.home + '-' + db.live.away + ' Farpas', privacy: ytLive.test ? 'private' : 'unlisted' }); }
      return reply(200, { known: true, stream: streamNow(), youtube: ytSummary() });
    }
    if (p === '/api/youtube') return reply(200, yt);
    if (p === '/api/youtube/client') { yt.client_id = b.client_id; yt.has_secret = yt.has_secret || !!b.client_secret; yt.configured = !!(yt.client_id && yt.has_secret); return reply(200, yt); }
    if (p === '/api/youtube/connect') {
      yt.connecting = true;
      setTimeout(function () { Object.assign(yt, { connecting: false, connected: true, expired: false, stream_ready: true, channel: { id: 'UC1', title: 'Clube Exemplo · Basquetebol', thumbnail: avatar } }); }, 1500);
      return reply(200, { auth_url: 'about:blank' });
    }
    if (p === '/api/youtube/disconnect') { Object.assign(yt, { connected: false, expired: false, channel: null, stream_ready: false }); return reply(200, yt); }
    if (p === '/api/youtube/thumbnail') {
      if (/[?&]thumb=fail\b/.test(location.search)) return err(502, 'O YouTube não aceitou a miniatura, porque o canal ainda não está verificado em youtube.com/verify. O direto continua com a imagem que o YouTube escolher.');
      return reply(200, { ok: true });
    }
    if (p === '/api/youtube/unlist') {
      ytHist.concat(ytEnded ? [ytEnded] : []).forEach(function (r) { if (r.broadcast_id === b.broadcast_id) { r.privacy = 'unlisted'; r.privacy_error = ''; } });
      return reply(200, { ok: true, privacy: 'unlisted' });
    }
    if (p === '/api/youtube/history') return reply(200, ytHistory(u.searchParams));
    if (p === '/api/obs/cameras') return reply(200, { cameras: cameras });
    if ((r = /^\/api\/obs\/cameras\/([1-4])\/link$/.exec(p))) {
      var pc = (db.settings.obs.cameras || [])[+r[1] - 1];
      return pc && pc.vdo ? reply(200, { link: 'https://vdo.ninja/?push=' + pc.vdo + '&webcam&autostart&facing=rear&quality=0&audiodevice=0' }) : err(404, '404 page not found');
    }
    if (p === '/api/obs/camera/live') {
      if (/[?&]camerr=1\b/.test(location.search)) return err(502, 'o OBS não mudou para a cena "Câmara ' + b.index + '": fake');
      db.obsLive = b.index; emit();
      return reply(200, { ok: true, live_camera: b.index, warning: '' });
    }
    if (p === '/api/obs/inset') {
      if (!(b.camera >= 0 && b.camera <= (db.settings.obs.cameras || []).length)) return err(400, 'Não há câmara ' + b.camera + '.');
      if (/[?&]inseterr=1\b/.test(location.search)) return err(502, 'a câmara ' + b.camera + ' não está na cena: fake');
      db.inset = b.camera; emit();
      return reply(200, { ok: true, camera: b.camera });
    }
    if (p === '/api/obs/setup') return reply(200, setupReport(b));
    if (p === '/api/system/autostart') { db.settings.start_with_windows = !!b.on; return reply(200, { ok: true, on: !!b.on }); }
    if (p === '/api/obs/sources') return reply(200, { scene: 'Jogo', auto: 'DroidCam', sources: [{ name: 'Hallstream', kind: 'browser_source' }, { name: 'DroidCam', kind: 'dshow_input' }, { name: 'Fundo', kind: 'image_source' }] });
    if (p === '/api/obs/camera/fit') return reply(200, { ok: true, warning: '', fit: { source: 'DroidCam', source_width: 1280, source_height: 720, canvas_width: 1920, canvas_height: 1080 } });
    if (p === '/api/obs/test') return reply(200, { connected: false, error: 'O OBS não respondeu em ' + b.host + ':' + b.port + '. Confirme que o servidor WebSocket está ligado.' });
    return err(404, 'Endpoint a fingir não existe: ' + m + ' ' + p);
  };

  // Headless Chrome will not go below 500px, so phone shots pin the page width (?w=390) and crop.
  var w = (/[?&]w=(\d+)/.exec(location.search) || [])[1];
  if (w) {
    var st = document.createElement('style');
    st.textContent = 'html{width:' + w + 'px;margin-left:calc((100vw - ' + w + 'px)/2);overflow-x:hidden}.modal{width:' + w + 'px;left:calc((100vw - ' + w + 'px)/2)}';
    document.head.appendChild(st);
  }

  // ?act=resultado draws the result card over the page for a 1080x1080 screenshot.
  // &case=away (visitor wins), nocrest (initials, no sponsor logo), intervalo (half time).
  function resultadoShot() {
    var c = (/[?&]case=([a-z]+)/.exec(location.search) || [])[1], st = JSON.parse(JSON.stringify(state()));
    st.live.period = '4'; st.live.clock = '00:00';
    st.live.home = 82; st.live.away = 76;
    st.periods = [{ period: 1, home: 22, away: 19 }, { period: 2, home: 44, away: 40 }, { period: 3, home: 63, away: 58 }, { period: 4, home: 82, away: 76 }];
    if (c === 'away') {
      st.live.home = 71; st.live.away = 78; st.game.away.name = 'ATLÉTICO CP'; st.game.away.short = 'ACP'; st.game.away.color = '#0F5C6E'; st.game.away.crest_url = crest('ACP', '#0F5C6E');
      st.periods = [{ period: 1, home: 18, away: 20 }, { period: 2, home: 37, away: 39 }, { period: 3, home: 55, away: 60 }, { period: 4, home: 71, away: 78 }];
    }
    if (c === 'nocrest') { st.game.home.crest_url = null; st.game.away.crest_url = null; st.game.sponsors.principal.url = null; }
    if (c === 'voleibol') {
      st.game.sport = 'volleyball'; st.game.sport_label = 'VOLEIBOL';
      st.game.rules = { id: 'volleyball', periods: 0, half: 0, length: 0, clock_down: false, fouls_reset: false, foul_warn: 0, foul_bonus: 0, shot_clock: false,
        final_hold: 0, timeouts: [2, 2, 2], scoring: 'sets', set_points: 25, last_set_points: 15, sets_to_win: 3, win_by: 2 };
      st.live.period = '4'; st.live.clock = ''; st.live.home = 0; st.live.away = 0; st.live.home_sets = 3; st.live.away_sets = 1; st.live.serve = '';
      st.periods = [];
      st.sets = [{ set: 1, home: 25, away: 20, winner: 'home' }, { set: 2, home: 18, away: 25, winner: 'away' }, { set: 3, home: 25, away: 22, winner: 'home' }, { set: 4, home: 25, away: 19, winner: 'home' }];
    }
    if (c === 'intervalo') { st.live.period = '3'; st.live.clock = '10:00'; st.live.home = 44; st.live.away = 40; st.periods = st.periods.slice(0, 2); st.screen.current = 'intervalo'; }
    window.Resultado.render(st).then(function (cv) {
      var img = new Image();
      img.src = cv.toDataURL('image/png');
      img.alt = window.Resultado.fileName(st);
      img.style.cssText = 'position:fixed;left:0;top:0;width:1080px;height:1080px;z-index:9999';
      document.body.appendChild(img);
    });
  }

  // Screenshot helpers: ?mock=1&act=import|ltshow|pin|test clicks through a state after load.
  var act = (/[?&]act=([a-z]+)/.exec(location.search) || [])[1];
  if (act) {
    window.addEventListener('load', function () {
      setTimeout(function () {
        if (act === 'import') { var b = document.querySelector('[data-act="import"]'); if (b) b.click(); setTimeout(function () { var l = document.querySelector('[data-act="sug-link"]'); if (l) l.click(); }, 600); }
        if (act === 'fpbgames') { var g = document.querySelector('[data-act="fpb-games"]'); if (g) g.click(); }
        if (act === 'ltshow') { var p = document.querySelector('[data-lt]'); if (p) p.click(); }
        if (act === 'teams') { var c = document.querySelector('#pGroup'); c.value = 'sub16-m'; c.dispatchEvent(new Event('change', { bubbles: true })); setTimeout(function () { document.querySelector('#pFpbSearch button').click(); }, 300); }
        if (act === 'pin') { document.cookie = 'pin=1234; path=/'; document.cookie = 'pin=; max-age=0; path=/'; document.getElementById('pin').hidden = false; }
        if (act === 'resultado') resultadoShot();
        if (act === 'setup') { var su = document.querySelector('[data-act="obs-setup"]'); if (su) su.click(); }
        if (act === 'stop') { var sp = document.querySelector('[data-act="stream-stop"]'); if (sp) sp.click(); }
        if (act === 'start') { var sa = document.querySelector('[data-act="stream-start"]'); if (sa) sa.click(); }
        if (act === 'test') { var te = document.querySelector('[data-act="stream-test"]'); if (te) te.click(); }
        // ?act=wz&n=1 opens a step of the Configuração inicial (&run=1 also clicks Configurar OBS).
        if (act === 'wz') {
          var n = (/[?&]n=(\d)/.exec(location.search) || [])[1], ws = n && document.querySelector('[data-wz="' + n + '"]');
          if (ws) ws.click();
          if (/[?&]run=1\b/.test(location.search)) setTimeout(function () { var r = document.querySelector('[data-wz-act="obs-setup"]'); if (r) r.click(); }, 300);
        }
        if (act === 'trynot') setTimeout(function () { var t = document.querySelector('[data-try="F8:4E:17:C2:9D:40"]'); if (t) t.click(); }, 300);
      }, 900);
    });
  }
})();
