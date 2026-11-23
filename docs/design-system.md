# Hallstream design system: the control panel

The on-air graphics have their own system in `docs/design/SYSTEM-R3.md` (amended by R4 and R8) and take each club's colours. This file covers the **control panel** (`web/painel/`), which wears Hallstream's own brand.

# Product

## Design Context

- **Audience:** volunteers at amateur sports clubs in Portugal (parents, coaches, a club secretary). Not technical. Often on an old Windows laptop in the sports hall, sometimes a phone or tablet on the hall's wifi, in a noisy room, during a live game.
- **Use cases:** before the game, pick the game, check the squad, connect OBS and YouTube; during the game, switch screens (timeout, half-time, final), show a lower third, watch that the score and stream are live; after, put the result online. One action at a time, under time pressure.
- **Brand personality:** warm, proud, scrappy.
- **Brand source:** `../marketing/brand/hallstream-brand/` (README, style guide, lockups). The idea: "Every home game starts at 0:0. The gaps draw the hall's H."
- **Reference sites (my choice):** a club scoreboard operator's desk, the Linear app's calm density for the task UI, the Ableton Live / sports-scoring-table feel of "big clear state, few controls". Football club match programmes for warmth.
- **Anti-references:** TV-network sports graphics (cold, chrome, glossy), crypto / LCD techno, SaaS dashboard cards with gradients, anything pharmacy-coloured (the mark's zeros can read as capsules).
- **Light or dark:** light (Soalho ground) for the panel; the header band is Marcador. Dark mode not in scope.
- **Colours to use:** Soalho `#F2E4C9` (ground, 60), Marcador `#1C1A17` (ink, 30), Lâmpada `#E5482A` (accent, 10), tint `#F7C9B8` (fills only). Lâmpada on white text fails contrast (3.97:1), so a dark accent is needed for buttons and accent text (to be settled by this restyle).
- **Colours to avoid:** pharmacy green, pill pink; CNN's old red/navy/gold.
- **Type:** the drawn lowercase "hallstream" is for the name only. The text typeface is open: the user rejected Nunito; the panel ships Barlow and Barlow Condensed today.
- **Direction (picked 2026-11-23): Programme desk.** The panel is the night's match programme printed on Soalho. Thick-thin ink rules open each section instead of cards; the screen tiles are drawn thumbnails of what each screen puts on air, in the club's colours; Lâmpada only where something is lit (on air, the newest score digit, the live timer); the active nav item is a Marcador fill leaning 9°; status marks are the mark's colon segments. Type: Archivo along its width axis (condensed 74 for nav, heads, tile names and score; wide 113 for the page title and codes; 100 for text). Radius 2px on frames, stadium ends on buttons only. Tone steps: Lâmpada funda #B8381E (Lâmpada as text on Soalho, 4.6:1), Soalho claro #F8EFDD (the one raised surface). One filled primary button per view.
- **What the owner disliked:** a first round read as "AI built": cream cards stacked everywhere, identical rounded text tiles, a lit word inside a heading, italic sporty display type, pills on non-controls. Nunito.
- **Constraints:** `web/` ships embedded with no build step and nothing from a CDN (fonts are woff2 files in `web/fonts/`). All copy European Portuguese. Touch targets usable on a phone.
