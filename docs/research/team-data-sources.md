# Team, squad, fixture and crest data sources

Research for roadmap step A.5, the `Provider` interface. Checked on 2026-11-24 against the sources' own pages. Where a page refused automated access or I could not reach the primary text, the entry says so.

## Summary

Hardly any open data covers amateur and youth clubs in Portugal. The federations hold it and none of them publishes an API or a reuse licence. Volleyball is the exception in practice. The FPV runs its competitions on Data Project's hosted site (`fpv-web.dataproject.com`), which lists teams, squads with shirt numbers, fixtures and youth competitions (Juniores, Sub21) as plain HTML. Futsal and football (FPF) and handball (FAP) render their competitions client side or block plain HTTP clients, and neither documents an endpoint.

The open, properly licensed sources (Wikidata, OpenLigaDB, openfootball) are clean legally but empty for our users. Wikidata has 29 Portuguese basketball club items and none of them has a logo or official colours. The commercial APIs (TheSportsDB, API-Football, football-data.org) cover professional leagues, need a per-user key, and say outright that crests belong to the clubs. Sofascore, Flashscore and zerozero.pt reserve all rights or forbid scraping.

The best next providers, in order:

1. **FPV through Data Project**, opt-in and paced like FPB, after asking FPV for permission.
2. **An iCalendar (`.ics`) fixtures import.** It is generic and fixtures-only, and it covers calendariofpb.pt, team-app calendars and any federation that exports a calendar.
3. **Nevobo**, the Dutch volleyball federation's public, keyless API. It is the first European source that covers amateur and youth teams.

Crests should come from the user's own file, or from the federation that already serves them, and fall back to a colour-and-initials badge. Player photos stay off by default. No provider should store birth years or heights, even when the source publishes them.

## Sources at a glance

| Source | Sports | PT amateur and youth coverage | Data | Access | Licence and terms | Fit for offline desktop app |
|---|---|---|---|---|---|---|
| FPB via calendariofpb.pt (current) | Basketball | Yes, all age groups (Baby-Basket to Sénior) | Teams, fixtures, squads, crests, colours | Public, paced | Unofficial site, "Os dados vêm de fpb.pt"; no reuse licence | Yes, already built |
| FPV via Data Project | Volleyball | Yes: national divisions, Sub21, Juniores A/B/B1 | Teams, squads (number, name, position, height, birth year), fixtures, standings | Public HTML (ASP.NET pages), no key | No terms found on the site | Good, with a scraper and permission |
| FPF (resultados.fpf.pt) | Football, futsal | Yes, regional associations publish there | Fixtures, results | Returned HTTP 403 to a plain client | No API or terms found | Poor without an agreement |
| FAP (portal.fpa.pt) | Handball | Yes, national and regional | Fixtures, results | Rendered client side; no documented endpoint | "Todos os direitos reservados" | Poor without an agreement |
| Nevobo (NL) | Volleyball | No (Dutch clubs only) | Teams, poules, matches, results, halls | Public, no key | Not found; ask | Good for NL clubs |
| OpenLigaDB | Any sport in a league format | Only leagues someone enters | Teams, fixtures, results | Public, no key for reads | ODbL 1.0; crests and logos excluded | Good |
| Wikidata | All | Very thin (see counts below) | Club identity, colours (P6364), logo file (P154) | SPARQL, 60 s per query | CC0 for data; images have their own licences | Fine for enrichment only |
| TheSportsDB | Many | Pro and semi-pro only | Teams, players, events, artwork | Key `123` free, 30 req/min, 1 search result | Free tier is for development; app-store use needs a paid plan | Weak |
| football-data.org | Football | Primeira Liga and Taça de Portugal only | Teams, squads, fixtures, crest URLs | Free key, 10 req/min | Attribution required; keys may not sit in open-source repos; logos copyrighted | Weak |
| API-Football and API-Sports | Football, basketball, handball, volleyball and more | Pro leagues | Teams, squads, fixtures, logos | Free 100 requests/day | Terms page refused automated access; not verified | Weak |
| openfootball | Football | Top leagues only | Fixtures, results | Static JSON on GitHub | CC0 | Good, but irrelevant to our users |
| FIBA Organizer, LiveStats, Genius Sports | Basketball, volleyball, football | Whatever each federation enters | Everything | Genius customers only | Commercial | No |
| EHF, CEV, UEFA, Sportradar | Elite competitions | No | Everything | Commercial contracts | Commercial | No |
| nuLiga | Handball, volleyball, others (DE, AT, CH) | No | Fixtures, results | Client ID and secret per portal | Per portal | No |
| FFBB (FR) | Basketball | No | Clubs, competitions, standings | Undocumented app API used by community clients | Not sanctioned | No |
| TeamSnap, Spond | Any | Only the club's own data | Rosters, events | TeamSnap: OAuth API and CSV export. Spond: no official API | Per service | Via CSV export or calendar feed |
| Sofascore, Flashscore, zerozero.pt | Many | Some Portuguese youth football | Everything | Scraping | Forbidden or all rights reserved | Avoid |

## Portuguese federations

### FPB, basketball (current provider)

[calendariofpb.pt](https://calendariofpb.pt/) describes itself as "Calendário não oficial. Os dados vêm de fpb.pt." and "Projeto pessoal independente, não afiliado com a FPB." It lists every age group the FPB runs, from Baby-Basket and Mini 8 to Sub 20, Sénior and Masters. `internal/fpb` already handles pacing (`turn`, `retryAfter`), a cache directory, a host allowlist for photos and a separate client for opponent logos on other hosts.

Stability depends on one unofficial site that is not affiliated with the federation, and on the FPB pages it reads. The provider is opt-in and paced, which is the right default.

### FPV, volleyball: Data Project

The FPV homepage links to `portugalvoleibol.com/classificacao` for standings, and that host answered HTTP 403 to a plain client. The competition data itself sits on Data Project's hosted competition site, [fpv-web.dataproject.com](https://fpv-web.dataproject.com/), titled "Competition Site by Data Project".

What it publishes (2026/27 season, checked today):

- Competitions: Liga SOLVERDE.PT, Liga UNA Seguros, 2ª Divisão (both genders), Taça de Portugal, Supertaça, Sub21 1ª and 2ª Divisão, Juniores, Juniores A, B and B1, plus regional phases such as the Azores.
- Team pages at `/CompetitionTeamDetails.aspx?TeamID=<id>&ID=<competition>` with a squad table: shirt number, name, position, height, birth year.
- Fixtures ("Jogos"), standings ("Classificação"), player rankings and a live score page.

Access is plain HTML with stable query-string IDs, no key and no visible rate limit. I found no terms of use on the site, so reuse rights are unknown. Data Project is a vendor whose hosted sites share one layout, so a parser written for the FPV may work for other federations on the same vendor. I did not verify which federations those are.

Fit: good. It is the only Portuguese federation besides basketball where squads with numbers are reachable without a login. Ask the FPV first, pace requests like FPB, and drop height and birth year at parse time (see [Privacy](#privacy-for-minors)).

### FPF, football and futsal

Regional associations send people to the FPF site for fixtures. AF Lisboa's circulars, for example, say "marcações de jogos, resultados e classificações, podem ser consultadas no site da FPF" and point to "resultados" > "competições" > "AF Lisboa" ([AF Lisboa CA 142](https://afl.pt/wp-content/uploads/2025/12/CA-142-03.04.01.2026.pdf)). `resultados.fpf.pt` returned HTTP 403 to a plain HTTP client. I found no API documentation, no open-data page and no terms that allow reuse.

Fit: poor without an agreement with the FPF. Futsal and football clubs should use `manual`, CSV or `.ics` until then.

### FAP, handball

[portal.fpa.pt](https://portal.fpa.pt/) is a WordPress site. Its "Competições Nacionais" and "Competições Regionais" pages load results client side ("A carregar..."), and the only public JSON is the standard WordPress `wp-json` for posts. The page loads assets for the FAP mobile app from a contractor-run host (`fpa-api-v2.joaosousa.pro`), which suggests an app backend, but nothing documents it. The footer reads "© 2026 Federação de Andebol de Portugal. Todos os direitos reservados."

[dados.gov.pt](https://dados.gov.pt/en/datasets/associacoes-de-andebol-em-portugal) has a dataset of the regional handball associations, which is useful for a picker of associations but holds no teams or fixtures.

Fit: poor. Ask the FAP whether the app API can be opened before writing anything against it.

### zerozero.pt

The site footer: "© 2003-2026 ZOS, Lda. - Todos os direitos reservados. A utilização deste sítio implica o seu acordo com o Termos e Condições". I found no public API. zerozero has deep Portuguese youth football coverage, but nothing allows reuse. Avoid unless ZOS agrees to a partnership.

## European and international

### FIBA, Genius Sports

FIBA gives its member federations FIBA Organizer and FIBA LiveStats at no cost. From [FIBA's competition management page](https://about.fiba.basketball/en/services/data-and-video-solutions/competition-management): "FIBA Organizer is a digital competition management solution, developed by Genius Sports in collaboration with FIBA, provided at no cost to basketball federations and leagues." It manages "membership data (database of clubs, players, coaches, referees and venues)". [FIBA's Genius Sports page](https://about.fiba.basketball/en/services/data-and-video-solutions/genius-sports) adds that "custom API publishing services ... can be provided to all FIBA members".

The [Genius Sports Developer Centre](https://developer.geniussports.com/) documents a Warehouse REST API, a streaming API and a Fixtures API for basketball, volleyball and football, but credentials come with a Genius contract. A federation that runs on FIBA Organizer can ask Genius for API publishing. A third-party app cannot sign up.

Fit: not directly. If a federation (FPB or another) ever offers us Genius API access, the `Provider` interface should absorb it like any other source.

### EHF, CEV, UEFA

The EHF signed a 10-year data collection and distribution deal with Sportradar ([EHF announcement](https://ehfec.eurohandball.com/news/en/ehf-and-ehf-marketing-sign-10-year-agreements-with-sportradar-for-data-collection-and-distribution-rights/)). Sportradar's [volleyball docs](https://developer.sportradar.com/volleyball/reference/indoor-volleyball-overview) cover FIVB and CEV tournaments behind a paid key. None of these bodies touches amateur or youth club data. Avoid.

### National federations and platforms

- **Netherlands, Nevobo (volleyball).** A public API at `api.nevobo.nl` covers poules, teams, matches, results and sports halls for every Dutch level, youth included. The [NevoboJS README](https://cdn.jsdelivr.net/npm/nevobo-js@0.2.6/README.md) says the only official documentation is a 2016 Nevobo news post, and its [type definitions](https://cdn.jsdelivr.net/npm/nevobo-js@0.2.6/dist/wedstrijden/index.d.ts) note pages of up to 1000 matches. The host answered HTTP 406 to a request that did not ask for JSON, which is consistent with content negotiation. I found no licence. Fit: good for Dutch clubs, and the cleanest public amateur source I found anywhere.
- **Germany, DBB (basketball).** basketball-bund.net publishes schedules and tables, but a TYPO3 extension's docs say that by default there is no way to show that data on your own site ([jh_basketballbund_import2](https://docs.typo3.org/typo3cms/extensions/jh_basketballbund_import2/Introduction/Index.html), an old page). No documented API.
- **Germany, Austria, Switzerland: nuLiga.** Handball and volleyball federations run on nuLiga. A Contao integration ([contao-nuligadata-bundle](https://packagist.org/packages/contaobayern/contao-nuligadata-bundle)) needs a client ID and secret per league portal, and community tools such as [nuScrape](https://pkg.go.dev/github.com/taskmedia/nuScrape/pkg/parser) parse the HTML instead. Not open.
- **France, FFBB (basketball).** Community clients such as [ffbb-api-client-v3](https://pypi.org/project/ffbb-api-client-v3/) call the FFBB app's API with tokens they pick up automatically. Nothing shows the FFBB sanctions this. Avoid until the FFBB publishes terms.
- **Nordics, Profixio.** Profixio runs league and cup management for several Nordic federations. Its [help pages](https://profixio.helpdocs.io/l/en/article/nyiofli8vt-main-modules-in-profixio-league) mention data exchange by API for licensing, but I found no public API docs.
- **Spain (FEB) and Switzerland (Swiss Volley).** I found no primary documentation for either in this pass. Not verified.
- **Basketball-Reference and similar stat sites** cover professional leagues only. Not relevant.

### Team-management apps (TeamSnap, Spond, SportsEngine)

These hold the club's own roster, which is the data our users actually need, but each club would have to connect its own account.

- **TeamSnap** has an OAuth 2 API at `api.teamsnap.com/v3` with read and write scopes ([authorization docs](https://www.teamsnap.com/documentation/apiv3/authorization)), and a manual roster export: "Roster Exports generate a downloadable CSV File" ([help article](https://helpme.teamsnap.com/article/1859-how-to-export-roster-data)).
- **Spond** has no official public API. [Olen/Spond](https://github.com/Olen/Spond) is a "Simple, unofficial library" under GPL-3.0.
- **SportsEngine** is mostly used in the US. Not checked further.

Fit: better served by CSV and `.ics` import than by an OAuth integration per vendor. A coach exports the roster, and `manual` reads it.

## Open and community datasets

### Wikidata

All structured data is CC0: "All structured data (i.e. the main, Property, Lexeme, and EntitySchema namespaces) is released into the public domain under Creative Commons Zero" ([Wikidata:Licensing](https://www.wikidata.org/wiki/Wikidata:Licensing)). [P6364 official color](https://www.wikidata.org/wiki/Property:P6364) links colour items, and [P154 logo image](https://www.wikidata.org/wiki/Property:P154) points to a Commons file, which carries its own licence.

The [query service limits](https://www.mediawiki.org/wiki/Wikidata_Query_Service/User_Manual#Query_limits): a 60-second deadline per query, "One client (user agent + IP) is allowed 60 seconds of processing time each 60 seconds", HTTP 429 with `Retry-After` when exceeded, and clients without a proper `User-Agent` "may be blocked completely".

Coverage for Portugal, from a SPARQL count today (items with sport P641 and country Portugal, typed as a club or team):

| Sport | Items | With logo (P154) | With colours (P6364) |
|---|---|---|---|
| Basketball | 29 | 0 | 0 |
| Football | 633 | 13 | 17 |
| Handball | 6 | 0 | 0 |
| Volleyball | 10 | 1 | 1 |

Multi-sport clubs often carry only their main sport, so these counts undercount a little, but the pattern is clear: Wikidata knows some football clubs and almost no basketball, handball or volleyball clubs, and it has crests and colours for very few. Fit: optional enrichment for a club the user has already picked. It cannot be a squad or fixtures provider.

### OpenLigaDB

A community database for anything that fits a league schema ([openligadb.de](https://www.openligadb.de/)). "Das Abrufen der Daten über den Webservice erfordert keinerlei Authentifizierung." Any registered user can create a league and becomes its administrator ([samples repo](https://github.com/OpenLigaDB/OpenLigaDB-Samples)). The data is under ODbL 1.0, and the [licence page](https://www.openligadb.de/lizenz) requires attribution such as "Daten von OpenLigaDB (www.openligadb.de), lizenziert unter ODbL 1.0" and says: "Ebenfalls nicht von dieser Lizenz erfasst sind Vereinsnamen, Wappen und Logos."

Fit: legally the cleanest generic provider, and a Portuguese regional league could be entered there by a volunteer. Today it holds almost nothing Portuguese, so it is a runner-up rather than a priority.

### openfootball

[football.json](https://github.com/openfootball/football.json) is CC0 static JSON for top football leagues, "No API key required". Clean, but it holds nothing for our users.

### TheSportsDB

From the [terms](https://www.thesportsdb.com/docs_terms_of_use.php), updated 17/09/2026:

- "You can scrape, copy and modify content returned from the API using the official end points ... Please do not scrape our website."
- "Free API Usage: You may use our API to lookup data and artwork for your development projects. You cannot publish apps to an appstore unless you are a paid subscriber."
- "Paid API Usage: ... must mention us as the source of the data. A paid API subscription does not grant rights to third-party artwork."
- "Any trademarked sports logos must be used 'As is' ... This does not grant permission to use a third-party trademark."
- Artwork with `strCreativeCommons` unknown "must not be used ... publicly".

The [docs](https://www.thesportsdb.com/documentation) list the free key `123`, 30 requests per minute, and "Search Teams ... Free Limit: 1". [Pricing](https://www.thesportsdb.com/pricing) starts at $9 a month for 100 requests per minute. Coverage is professional and semi-professional. Fit: weak. The free tier is for development, one search result is not enough for a club picker, and Portuguese youth clubs are not there.

### football-data.org

The free tier includes the Primeira Liga ([coverage](https://www.football-data.org/coverage)) at "10 requests/minute in the free plan" ([policies](https://docs.football-data.org/general/v4/policies.html)). From the [terms](https://www.football-data.org/client/register):

- 6.1: "Developer credentials may not be stored in code repositories of open source projects."
- 7.1: "You agree to include the following attribution to Football-Data in your app or website: 'Football data provided by the Football-Data.org API'."
- 9.2: "All graphics, including team logos and profile photos, are copyrighted by their legal owner. You'll have to obtain consent from the respective owners".

Fit: weak. Top leagues only, and every user would need their own key.

### API-Football and API-Sports

The official site lists a free plan of 100 requests per day with access to every endpoint and no credit card, with separate APIs for basketball, handball and volleyball ([api-football.com](https://www.api-football.com/pricing)). Both api-football.com and api-sports.io refused automated access, so I could not read the terms. Coverage is professional leagues. Fit: weak, for the same reasons as football-data.org, plus unverified terms.

### Sofascore and Flashscore

Flashscore's [terms](https://www.flashscore.com/terms-of-use/) (Livesport s.r.o.): "no extraction (copying) or utilization (making available to the public) of Database Content or of a qualitatively or quantitatively substantial part thereof is permitted without our explicit consent", and "you are not permitted to use the content of the website by embedding, aggregating, scraping or recreating it without our express consent". Sofascore's terms page refused automated access, and it offers no public API. Avoid both.

## Crests and colours

- **Wikimedia Commons** "only accepts free content" and "does not accept fair use justifications" ([Commons:Licensing](https://commons.wikimedia.org/wiki/Commons:Licensing)). Most club crests are therefore not on Commons at all. They sit on individual Wikipedias as non-free files, and English Wikipedia says logos "are usually non-free images" ([Wikipedia:Logos](https://en.wikipedia.org/wiki/Wikipedia:Logos)). The crests that are on Commons are mostly simple text-and-shape logos or old ones, tagged public domain for copyright but still trademarks: "a logo can still be protected by trademark law, even if it is not subject to copyright." Commons also notes that an emblem, like a trademark, "cannot be used for any purpose by others" ([Commons:Coats of arms](https://commons.wikimedia.org/wiki/Commons:Coats_of_arms)).
- **Every data API excludes crests from its licence:** OpenLigaDB (not under ODbL), football-data.org (9.2), TheSportsDB ("does not grant permission to use a third-party trademark").
- **A club streaming its own games.** The club owns its own crest, so showing it is not in question. Showing the opponent's crest to identify the opponent is the use that trademark law tolerates most: EU Trade Mark Directive 2015/2436, [Article 14(1)(c)](https://eur-lex.europa.eu/eli/dir/2015/2436/oj/eng), lets third parties use a mark to identify or refer to the proprietor's goods or services, provided the use follows honest practices (Article 14(2)). Copyright in the crest artwork is a separate question, and the Directive does not answer it. This is not legal advice. Hallstream should not ship or bundle crests, should fetch them only on the user's request, and should keep them in the user's local cache.
- **Colours** are facts, not works. Taking hex values from Wikidata P6364, a federation page or the crest itself is safe.

Practical rule: crest = user-supplied file, or the federation's own image (FPB already serves these), cached locally. Default to a generated badge (club colours and initials) when there is none.

## Privacy for minors

- The sources publish more than we need. FPV's Data Project squad tables list height and birth year for every player, Juniores teams included. FPB serves player photos.
- [GDPR Recital 38](https://gdpr-info.eu/recitals/no-38/): "Children merit specific protection with regard to their personal data". Article 8's age threshold (16, or as low as 13 where a member state sets it) covers information society services offered directly to a child ([Article 8](https://gdpr-info.eu/art-8-gdpr/)). Hallstream is not such a service, so the article does not apply directly, but the club that broadcasts a youth game is the controller for the names and faces on the stream and needs its own lawful basis, usually parental consent.

What the providers should do:

1. Parse only shirt number, display name and, if the user turns photos on, the photo. Drop birth year, height and position at parse time, and never write them to the cache.
2. Keep photos off by default (already the rule), and keep it off per team, so a club can enable photos for seniors and leave them off for Sub 14.
3. Offer a display-name option for youth teams: full name, "First L.", or shirt number only.
4. Expire cached squads (for example at season end) and give the user a "clear cached squads" button.
5. Keep invented people in fixtures and tests. Never record real youth squads from FPV or FPB into test data.

## Recommendation

### Build next, after `manual` and FPB

1. **FPV via Data Project.** It is the only other Portuguese federation that publishes squads with shirt numbers and youth competitions without a login, and volleyball is on the roadmap. Ask the FPV for permission, ship it opt-in and paced like FPB, and strip birth year and height at parse. The vendor's shared layout may later cover other federations on Data Project.
2. **iCalendar (`.ics`) fixtures import.** Fixtures-only, but it works with calendariofpb.pt's team calendars, team-app calendar feeds and any federation or association that exports a calendar, in any sport. That covers futsal, football and handball clubs whose federations have no usable source. It is small to build, and paired with CSV squads it gives every club a full setup.
3. **Nevobo.** Public, keyless, and it covers amateur and youth volleyball across the Netherlands. It is the cheapest way to prove the interface outside Portugal. OpenLigaDB is the runner-up: properly licensed (ODbL, keyless reads) and sport-agnostic, but empty for Portugal today.

### Avoid

- **Sofascore, Flashscore, zerozero.pt:** terms forbid extraction and scraping, or reserve all rights.
- **TheSportsDB free tier:** development use only, one search result, no Portuguese amateur clubs.
- **football-data.org and API-Football:** professional leagues only. Each user would need their own key, because football-data.org forbids keys in open-source repos and API-Football's free quota is 100 requests a day.
- **FFBB's app API and other undocumented federation backends (FAP's app host):** unsanctioned and liable to break. Ask the federation first.
- **FIBA/Genius, EHF/Sportradar, CEV, UEFA:** commercial contracts and elite-only coverage.
- **Wikidata and Commons as a crest source:** almost no Portuguese small-club crests, and the files that exist still carry trademark rights. Use Wikidata for colours on known clubs at most.

### What the `Provider` interface must allow

- **Capabilities.** Each provider declares what it can do (search clubs, teams, squads, fixtures, crests, colours), because most do only part: `.ics` gives fixtures only, Wikidata gives colours only.
- **Search clubs** by name, filtered by sport and country or region. Some sources have no search and only browse by competition (Data Project), so browsing must be a first-class path too.
- **List teams per club** with sport, season, gender and age group. Keep the source's age-group label ("Sub 14", "Juniores B1", "Mini 10") as text and add an optional normalised value. Each federation names its groups differently.
- **List squad** returns number, display name and an optional photo reference. Nothing else crosses the interface.
- **List fixtures** for a team over a date range, with opponent name, opponent crest reference, venue and kickoff in UTC with the source's time zone.
- **Crest and colours** as separate calls, each with its source URL and licence note, so the UI can show where a crest came from and the user can replace it.
- **Stable IDs** namespaced by provider (`fpb:1234`, `fpv:1903`), plus a way to link a local team to more than one provider (squad from CSV, fixtures from `.ics`, colours from Wikidata).
- **Paging and rate limits.** A cursor or page token, plus a per-provider pacer that honours HTTP 429 and `Retry-After`. The FPB client's `turn` and `retryAfter` already work this way and can move behind the interface.
- **Credentials.** Optional per-user API keys stored locally, never in the repository, for any keyed provider added later.
- **Attribution.** Each provider returns an attribution string ("Daten von OpenLigaDB, lizenziert unter ODbL 1.0") for the settings screen, and the overlay credit if a licence requires one.
- **Offline cache.** Every result carries `fetchedAt` and the source's validator (`ETag` or `Last-Modified`). The app reads from the cache first, works fully offline, refreshes on request, and expires squads on a schedule. `Status()` returning reachability and `cachedAt`, as `fpb.Client` already does, belongs on the interface.
- **Opt-in and host allowlists.** Network providers are off until the user enables them, and image downloads stay limited to hosts the provider names, as `PhotoHosts` does today.
