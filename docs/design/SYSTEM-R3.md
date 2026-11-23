# Hallstream broadcast system, round 3 (one system for all seven screens)

Two independent critics agreed: the screens look like seven designers each made one. To reach an 8 the set must be ONE system. Rebuild your screen on these parts exactly. Exact numbers matter: other designers are building the other screens from this same file in parallel, and the pieces must match across screens.

## Data (what each screen may show)
Live from the scoreboard box: team names (max 9 letters, uppercase), scores (up to 3 digits), game clock ("07:45" or "9.4"), period, team fouls per team, 24-second clock. NOTHING ELSE is live: no period-by-period scores, no time-outs remaining, no possession arrow, no player points or fouls.
Typed by the volunteer before the game: player numbers/names/positions, coach, competition + round, date + time, sponsors. The time-out screen does not say which team called it (the user).
Venue: "Pavilhão Municipal do Vale". Same match numbers as before: live 2º período 07:45, 41-38, fouls 2 and 4, 24s 14; half-time 44-40; final 82-76.

## Tokens
- red #BD1522, kit red #8B2223, navy #181841, gold #D6BD2F, white #FFFFFF, light grey #ECEAE6, muted label rgba(255,255,255,0.72) on navy (no lavender), placeholder grey #8A8F9C
- Graduate: team names only. Barlow Condensed 600-800: numbers (tabular-nums) and labels. Barlow: sentences.
- Gold means GAME STATE ONLY: period, the 24s box, the foul warning (4+), and the screen state word (INTERVALO, DESCONTO DE TEMPO, RESULTADO FINAL, O JOGO COMEÇA ÀS). Other labels (COM O APOIO DE, PLANTEL, competition) are rgba(255,255,255,0.72) on navy or #4A4A66 on white. No decorative gold anywhere.
- Minimum text size at 1080: 22px for any label, 24px for any secondary information. Only the grey placeholder label inside a logo slot may be 18px.
- Surfaces: navy is the field. Home team = red. Visitor team = white. Nothing else gets its own colour block.

## The master diagonal (the kit band)
- One shape: a navy #181841 band with a 6px white line on its left edge (white | navy). It always slants the same way: top edge further right than bottom edge, at 12 degrees from vertical (horizontal shift = 0.2126 x height).
- Band width: 20px on the scoreboard and lower third, 40px on full-screen layouts.
- It appears ONCE per screen, as the seam between home and visitor, or as the edge of a panel. Never as decoration.
- Draw it with clip-path polygons computed from that angle.

## The team unit (the only building block for a team)
Order: [emblem tile][name][score]. Mirrored for the visitor when the two units face each other across a seam.
- Home: tile white #FFFFFF holding the crest (/_blob/594604fee97726efd14dc644fe154a1a, object-fit contain, crest height 76% of tile height). Name field red #BD1522 with white Graduate name. Score cell kit red #8B2223 with white Barlow Condensed 800 tabular numerals.
- Visitor: tile light grey #ECEAE6 with a square dashed 2px #B9B4AC safe area (78% of the tile) and the label "Emblema" in #8A8F9C 18px, plus a 6px awayColor strip along the tile's bottom (awayColor is a data-prop tweak, default #8A8F9C). Name field white with navy Graduate name. Score cell light grey #ECEAE6 with navy numerals.
- Size S (scoreboard): height 64. Tile 64x64. Name 30px, field width 236px. Score cell 96px wide, numerals 48px.
- Size L (time-out, half-time, final, squad header): height 120. Tile 120x120. Name 54px. Score cell 190px wide, numerals 96px.
- Name fields have fixed widths so 9 letters fit; score cells have fixed widths so 3 digits fit.

## The sponsor rail (built once, used on opener, time-out, half-time, final)
- Full-width navy #181841 strip, height 148, touching the bottom edge (y 932 to 1080). A 2px #BD1522 line along its top edge.
- Content inset 64px left/right, vertically centred. Left: the label "COM O APOIO DE" (Barlow Condensed 700, 22px, letter-spacing 3px, rgba(255,255,255,0.72)), 220px wide column.
- Then slots: one principal slot 380x96, then four equal slots 236x96, gaps 20px. Each slot is a white #FFFFFF box with the centred label "Logótipo patrocinador" (Barlow Condensed 600, 18px, #8A8F9C). One frame only (no inner dashed box).
- If the screen needs the Instagram handle or a kit sponsor line ("Equipamento <sponsor>"), put it on the right end of the rail as small text (22px, rgba(255,255,255,0.72)), replacing the fourth equal slot.

## Shared rules
- Team fouls (scoreboard only): word FALTAS, number, 5 pips; at 4+ number and pips gold; otherwise white, empty pips white 25%. Pips at least 12px wide x 22px tall. Foul number at least 30px.
- Shot clock: gold box, navy text, "24s" label at least 22px, number at least 40px.
- Dimmed photos: flat scrim rgba(8,8,16,0.62).
- Safe area: text and logos 64px from frame edges (the sponsor rail background may touch edges).
- Format rules from BRIEF.md still apply (inline styles, {{holes}} plain lookups, close every tag, keep support.js line, sc-for for repeats). Do not render or open a browser.
