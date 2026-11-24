# Team, squad, fixture and crest data sources

Research for the `Provider` interface (roadmap phase A, step 5) and the providers after it. The scope is any country, any level (youth, amateur, semi-pro, professional) and the five target sports: basketball, futsal, handball, volleyball and soccer.

Checked on 2026-11-24 against each source's own pages: API docs, terms, licence pages and GitHub repos. Quotes are copied from those pages. **Not verified** marks anything I could not read from a primary source that day, usually because the site refused automated access (HTTP 403, a Cloudflare challenge or HTTP 429). Those entries need a person with a browser to confirm them before anyone builds against them.

This replaces the 2026-11-24 Portugal-only pass. Its Portuguese findings still hold and are kept in [Portugal](#portugal).

## Summary

Data for professional clubs is plentiful and licensed for money. Data for youth and amateur clubs sits with federations and the platforms they rent, and few of them publish an API. Where they do, it usually takes a key that the federation or the platform hands out after review (PlayHQ, nuLiga, FIBA GDAP, LeagueApps).

Two findings drive the recommendation.

1. **Calendar feeds are everywhere.** TeamSnap, SportsEngine, GameChanger, GameDay, fotball.no, the Dutch basketball database and calendariofpb.pt all give a team its own iCalendar (`.ics`) link. One `.ics` import covers fixtures for more grassroots clubs than any federation API, in any country and any sport.
2. **The Netherlands is the one country with open, documented, keyless federation data in two of our sports.** Nevobo (volleyball) publishes an OpenAPI-described API, and the NBB database (basketball) offers JSON, XML and iCalendar for every Dutch competition down to under-10s.

For professional clubs, the general APIs (TheSportsDB, API-Sports, football-data.org, BallDontLie) work only if each club brings its own key, and every one of them says crests belong to the clubs. Sportmonks forbids storing its data without written consent, which rules it out for an app that must work offline.

Wikidata is the only openly licensed source of colours and crests, and its coverage is thin even at the top. Six of the 41 teams it lists in the Premier League have a logo, because Wikimedia Commons accepts only free files and most crests are not free. Crests should come from the user's own file or the club's federation, with a generated colour-and-initials badge as the default.

Ranked providers to build next: an `.ics` import, Nevobo plus the NBB database, PlayHQ, FPV through Data Project, and a bring-your-own-key provider for professional leagues that starts with TheSportsDB. Details are in [Recommendation](#recommendation).

## Sources at a glance

Level: **P** professional, **S** semi-pro and lower national divisions, **A** amateur and regional, **Y** youth.

| Source | Sport | Region | Level | Data | Access | Licence and terms |
|---|---|---|---|---|---|---|
| `.ics` feeds (many platforms) | Any | Worldwide | A, Y, S | Fixtures only | Public per-team URL | Per platform; a team's own calendar |
| CSV (manual) | Any | Worldwide | Any | Squads, fixtures | User file | The club's own data |
| FPB via calendariofpb.pt (current) | Basketball | PT | P to Y | Teams, squads, fixtures, crests, colours | Public, paced | Unofficial; no reuse licence |
| FPV via Data Project | Volleyball | PT (vendor used by 65+ bodies) | S, A, Y | Teams, squads with numbers, fixtures | Public HTML | No terms found |
| Nevobo | Volleyball | NL | P to Y | Clubs, teams, matches, halls; squads with a key | Public API, OpenAPI spec; squads need a key | Not stated; API marked unstable |
| NBB database (db.basketball.nl) | Basketball | NL | P to Y | Clubs, teams, results, fixtures, stats | Public JSON, XML, iCalendar, RSS | Volunteer-run; no licence stated |
| PlayHQ | Basketball, others | AU, NZ, UK (Basketball England) | S, A, Y | Organisations, seasons, grades, fixtures | API key after review by PlayHQ and the sport body | Per agreement |
| handball.net | Handball | DE | P to Y | Everything | Undocumented endpoints | No API terms found |
| nuLiga | Handball, volleyball, others | DE, AT, CH | A, Y | Fixtures, results | Client ID and secret per portal | Per portal |
| FFBB | Basketball | FR | P to Y | Clubs, competitions, matches | Undocumented app API | Not sanctioned |
| DBB (basketball-bund.net) | Basketball | DE | S, A, Y | Fixtures, tables | No API found | Not found |
| fotball.no, handball.no | Football, handball | NO | A, Y | Fixtures | `.ics` per team (fotball.no) | Not checked |
| FIBA GDAP, LiveStats, Genius Sports | Basketball, volleyball, football | Worldwide | P, S | Everything | Contract or federation agreement | Commercial, non-sublicensable |
| TheSportsDB | Many | Worldwide | P, some S | Teams, players, events, colours, artwork | Key `123` free, 30 req/min; Patreon from $9/month | Free use for development only; artwork licence per image |
| API-Sports | Football, basketball, handball, volleyball and more | Worldwide | P, S | Teams, squads, fixtures, logos | Key; free 100 req/day (not verified today) | Not verified |
| football-data.org | Football | Mostly Europe | P | Teams, squads, fixtures, crest URLs | Key; free 10 req/min | Attribution; no keys in open-source repos; logos copyrighted |
| BallDontLie | Basketball (NBA, EuroLeague, ACB), soccer and more | US, EU | P | Teams, players, games | Key; free 5 req/min | Very permissive: cache, store, redistribute |
| Sportmonks | Football | Worldwide | P, S | Everything | Key; free plan 2 leagues; from €29/month | Storage forbidden without consent |
| SportsDataIO, Sportradar, Stats Perform | Many | Worldwide | P | Everything | Contract; trials non-commercial | Commercial |
| OpenLigaDB | Any league format | Mostly DE | Whatever users enter | Teams, fixtures, results | Public, keyless reads | ODbL 1.0; crests excluded |
| openfootball | Football | Top leagues | P | Fixtures, results, clubs | Static JSON | CC0 |
| Wikidata | All | Worldwide | Mostly P | Identity, colours (P6364), logo file (P154), labels in many languages | SPARQL | CC0 data; images carry own licence |
| DBpedia | All | Worldwide | Mostly P | Wikipedia infobox data | SPARQL | CC BY-SA 3.0 and GFDL |
| GitHub logo repos | Football, US leagues | Europe, US | P | Crest images | Git | Repo licence covers files, not the marks |
| Team apps: TeamSnap, SportsEngine, GameChanger, GameDay | Any | US, CA, AU | A, Y | Fixtures (`.ics`), rosters (CSV export) | Per-team feed, or OAuth (TeamSnap) | Per service |
| Spond, LeagueApps, Exposure Events | Any | NO, US | A, Y | Events, rosters | Spond: no API. Others: key from the organiser | Per service |
| ESPN hidden API | US pro and college | US | P | Everything | Undocumented | Disney terms bar automated access |
| Transfermarkt, Sports-Reference, Sofascore, Flashscore, zerozero.pt, MaxPreps | Many | Worldwide | P to Y | Everything | Scraping | Forbidden or all rights reserved |

## 1. Multi-sport and multi-country APIs

Privacy for this section: every commercial API publishes player names and most publish photos and birth dates. The professional players they cover are adults in almost every case. The exception is youth internationals and academy squads, which some APIs list. A provider must drop birth date, height and nationality at parse time, whatever the level.

### TheSportsDB

- **Coverage.** Many sports and countries, mostly professional. With the free key `123`, `all_sports.php` returned only Soccer and Motorsport today, and `all_leagues.php` returned 10 soccer leagues, so the free key is limited to a sample. What a paid key covers in handball, volleyball and futsal is **not verified**.
- **Data.** A team record carries `strTeam`, `strTeamAlternate` ("Arsenal Football Club, AFC, Arsenal FC"), `strTeamShort` ("ARS"), `strColour1` to `strColour3` as hex, `strBadge`, `strLogo`, and descriptions in 14 languages on Arsenal's record (`strDescriptionEN`, `…PT`, `…ES`, `…DE`, `…FR`, `…NL`, `…NO`, `…SE` and others). Squads, events and venues have their own endpoints.
- **Access and cost.** The [docs](https://www.thesportsdb.com/documentation) list the free key and limits per endpoint: "Search Teams … Free Limit: 1 Premium Limit: 100" and "List Teams … Free Limit: 10 Premium Limit: 3000". The v2 API's search returns up to 10. [Pricing](https://www.thesportsdb.com/pricing): free is 30 requests per minute; "Single Developer $9 /mo" is 100 per minute.
- **Terms** ([terms of use](https://www.thesportsdb.com/docs_terms_of_use.php), updated 17/09/2026):
  - "You can scrape, copy and modify content returned from the API using the official end points … Please do not scrape our website."
  - "Free API Usage: You may use our API to lookup data and artwork for your development projects. You cannot publish apps to an appstore unless you are a paid subscriber."
  - "Paid API Usage: You may use our API to develop apps and services as long as you stay within the rate limit. … must mention us as the source of the data. A paid API subscription does not grant rights to third-party artwork."
  - On the `strCreativeCommons` flag: "Artwork marked unknown must not be used in app store submissions or publicly".
- **Fit.** The terms allow caching API responses, which an offline app needs. The free tier is for development, so a shipped Hallstream would need each club to bring a paid key, or a project-level agreement with TheSportsDB. Ask them which applies to a free desktop app distributed from GitHub.

### API-Sports (API-Football, API-Basketball, API-Handball, API-Volleyball)

- **Coverage.** Separate APIs per sport, worldwide, professional leagues and some lower national divisions. The lower-division depth is **not verified**.
- **Access and cost.** The earlier pass read a free plan of 100 requests per day per API with every endpoint and no card, from api-football.com. Today api-sports.io served a Cloudflare challenge, and api-football.com/terms and /pricing returned HTTP 403. **Not verified today.**
- **Terms, caching, logos, languages.** **Not verified.** I could not read the terms at all.
- **Fit.** The widest coverage of our five sports in one vendor, if the terms allow a desktop app to cache. Someone must read the terms in a browser before building it.

### football-data.org

- **Coverage.** Football only. The free tier is a fixed set of top competitions, the Primeira Liga among them ([coverage](https://www.football-data.org/coverage)).
- **Limits** ([policies](https://docs.football-data.org/general/v4/policies.html)): "Non-authenticated clients are allowed for 100 requests per 24 hours and can only access the area and competition list resources. Registered clients are allowed for 10 requests/minute in the free plan, 30 requests/minute in Standard plan and 60 requests/minute in all plans above."
- **Terms** ([registration terms](https://www.football-data.org/client/register)):
  - 6.1: "Developer credentials may not be stored in code repositories of open source projects."
  - 7.1: "You agree to include the following attribution to Football-Data in your app or website: 'Football data provided by the Football-Data.org API'."
  - 9.1: "After cancellation of the subscription to the Service, the Customer is not permitted to reference the football data … obtained through the Football-Data API on their own site or service."
  - 9.2: "All graphics, including team logos and profile photos, are copyrighted by their legal owner."
- **Languages.** No language parameter found. **Not verified.**
- **Fit.** Usable only with each club's own key, an attribution line, and a cache the app deletes when the key is removed (9.1).

### Sportmonks

- **Coverage.** Football, "2,200+ leagues". [Pricing](https://www.sportmonks.com/football-api/plans-pricing/): Starter €29 a month for "any 5 leagues worldwide" at "2,000 API calls per entity per hour"; Growth €99 for 30 leagues. The free plan "gives you two full leagues to build against, no credit card, no expiry."
- **Languages.** A `locale` parameter translates team, league, player and venue names, falling back to English ([translations, beta](https://docs.sportmonks.com/v3/api/translations-beta); read through search results, the page itself came back empty).
- **Terms** ([terms of service](https://www.sportmonks.com/terms-of-service/)): "Reproduction, transfer, distribution, or storage of Sportmonks' services or data is prohibited without prior written consent from Sportmonks." On images: "All logos, profile pictures, and/or videos accessible via the Sportmonks Account are protected by copyright, and Sportmonks does not own the intellectual property rights thereto."
- **Fit.** None. The storage clause conflicts with an offline cache.

### SportsDataIO

- **Terms** ([online terms](https://sportsdata.io/terms-and-conditions)): the licence is limited to a "Permitted Use", headshots are "limited to editorial use", and the customer "shall not permit or suffer to permit such Data to be made available to any of its users" beyond that use. The [data rights help page](https://sportsdata.io/help/data-rights-and-licensing-questions) says commercial licences permit "ingesting, caching, and storing API responses", but "Redistributing or reselling the feed itself … is not permitted."
- **Free trial.** "The free trial lets developers check response structures, fields, and integration code using scrambled data" ([free trial](https://sportsdata.io/free-trial)).
- **Fit.** None. Commercial contract, US-centred, and the trial data is scrambled.

### Sportradar

- **Coverage.** Every sport we target, futsal and handball included, at professional level ([developer portal](https://developer.sportradar.com/getting-started/docs/get-started)).
- **Terms** ([master terms](https://developer.sportradar.com/terms-and-conditions)): "Free Trial means non-commercial use of the Service provided by the Company to prospective customers for internal testing and evaluation purposes only".
- **Languages.** A `{language_code}` path segment, for example `/soccer/{access_level}/v4/{language_code}/…`, with a list that differs per product ([Soccer API basics](https://developer.sportradar.com/soccer/docs/soccer-ig-api-basics); read through search results).
- **Fit.** None. Paid contract.

### Genius Sports, FIBA LiveStats and FIBA GDAP

- **Genius Sports.** The [Developer Centre](https://developer.geniussports.com/) documents a Warehouse REST API "to query information regarding matches, players, statistics etc.", per sport (basketball, football, volleyball, hockey and others). Credentials come with a Genius contract.
- **FIBA Organizer** is free to federations: "FIBA Organizer is a digital competition management solution, developed by Genius Sports in collaboration with FIBA, provided at no cost to basketball federations and leagues" ([FIBA](https://about.fiba.basketball/en/services/data-and-video-solutions/competition-management)). Its data is reachable only through the federation.
- **FIBA GDAP** ([terms](https://gdap-portal.fiba.basketball/terms-of-services)): "FIBA hereby grants you a revocable, non-exclusive, non-sublicensable, limited license", "You may not sublicense the FIBA GDAP APIs for use by a third party", and FIBA "reserves the right to charge fees". The [documentation page](https://gdap-portal.fiba.basketball/documentation) covers FIBA competition data.
- **The public LiveStats `data.json` URLs** that community tools read (`fibalivestats.dcd.shared.geniussports.com/data/<match>/data.json`): I found no primary documentation. **Not verified**, and not sanctioned.
- **Fit.** A federation that runs FIBA Organizer can ask Genius for API publishing and could then hand Hallstream access. A third-party app cannot sign up.

### Stats Perform (Opta)

The Opta product page returned an empty shell to a plain client. **Not verified.** It is enterprise-only by every public account, with no self-serve tier.

### ESPN's hidden API

Community gists list endpoints such as `site.api.espn.com/apis/site/v2/sports/football/college-football/teams/:team` ([gist](https://gist.github.com/akeaswaran/b48b02f1c94f873c6655e7129910fc3b)). Nothing is documented by ESPN. The [Disney terms](https://disneytermsofuse.com/english/) that cover ESPN forbid it: users may not "access, monitor, copy or extract the Disney Products using a robot, spider, script, or other automated means", nor "use the Disney Products for any commercial or business-related use". Avoid.

### BallDontLie

- **Coverage.** "20+ leagues", NBA, WNBA, EuroLeague and Liga ACB among them, plus soccer leagues, the NFL, MLB and others ([home](https://www.balldontlie.io/)). Professional only.
- **Access and cost** ([docs](https://docs.balldontlie.io/)): free tier with Teams, Players and Games at 5 requests per minute; ALL-STAR $9.99 and GOAT $39.99 a month, "per league".
- **Terms** ([terms](https://www.balldontlie.io/terms)) are the most permissive I found: "You may use Data you lawfully obtain for any lawful personal or commercial purpose … This includes copying, caching, storing, archiving, modifying, combining, analyzing, publishing, displaying, distributing, sublicensing, selling or reselling Data", and "No attribution … is required". The one limit: "You may not use our Data to offer a sports-data API or equivalent data feed that substitutes for the BALLDONTLIE API." On marks: "We do not claim ownership over underlying sports facts, team or player names, logos, or league trademarks".
- **Fit.** Good for professional basketball clubs in the leagues it covers, with each club's own key. Whether the free tier covers EuroLeague and ACB, or only the NBA, is **not verified**.

### OpenLigaDB

A community database for anything in a league format ([openligadb.de](https://www.openligadb.de/)). Reads need no key. Any registered user can create a league and administer it. The [licence page](https://www.openligadb.de/lizenz) puts the data under ODbL 1.0, asks for an attribution such as "Daten von OpenLigaDB (www.openligadb.de), lizenziert unter ODbL 1.0", and excludes "Vereinsnamen, Wappen und Logos". Clean, generic and almost empty outside German football. A volunteer could enter a regional league there.

### openfootball

[football.json](https://github.com/openfootball/football.json): "The football.json schema, data and scripts are dedicated to the public domain", "No API Key Required". [openfootball/clubs](https://github.com/openfootball/clubs) (CC0) lists clubs and stadiums worldwide. Top leagues only, no squads, no crests.

### Wikidata and DBpedia

- **Wikidata.** "All data in Wikidata has a CC0 license" ([Wikidata:Licensing](https://www.wikidata.org/wiki/Wikidata:Licensing)). Useful properties: [P6364 official color](https://www.wikidata.org/wiki/Property:P6364), [P154 logo image](https://www.wikidata.org/wiki/Property:P154), P118 league, P641 sport, P17 country, plus labels and aliases in every language. The query service allows "60 seconds of processing time each 60 seconds" per client and blocks clients without a proper `User-Agent` ([user manual](https://www.mediawiki.org/wiki/Wikidata_Query_Service/User_Manual#Query_limits)). Coverage counts are in [Crests and colours](#4-crests-and-colours).
- **DBpedia** extracts Wikipedia infoboxes "under the terms of the Creative Commons Attribution-ShareAlike 3.0 License and the GNU Free Documentation License" ([about](https://www.dbpedia.org/about/)). Share-alike adds obligations Wikidata does not have, and the infobox crest fields point to the same non-free Wikipedia files. No reason to use it over Wikidata.

### Transfermarkt

[Terms](https://www.transfermarkt.com/intern/anb), 11.1: "The User is not permitted to access or copy the Digital Content using bots, spiders, screen scraping or other automated processes." No public API. Avoid.

### Sports-Reference (Basketball-Reference and siblings)

The terms and bot-traffic pages returned HTTP 403. Through search results, the [data use page](https://www.sports-reference.com/data_use.html) says you should not "create websites or tools based on data you scrape from Sports Reference". **Not verified directly.** Professional and college statistics only. Avoid.

### Open GitHub crest datasets

- [luukhopman/football-logos](https://github.com/luukhopman/football-logos): "PNG logos (139x181px) for all football teams in the top 25 European leagues", seasons 2021/22 to 2026/27. GitHub reports no licence.
- [klunn91/team-logos](https://github.com/klunn91/team-logos): NFL, NCAA, MLB, NBA and Premier League logos. MIT licence.
- A repository licence covers the contributor's own work. It cannot license a club's crest, which stays the club's copyright and trademark. Hallstream should not bundle or fetch from these.

## 2. Federations and competition platforms

Privacy for this section: this is where minors are. Federation squad pages list children by full name, and several add birth year, height or a photo (FPV's Data Project pages, the NBB database). Any provider in this section parses only shirt number, display name and, if the user switches photos on, the photo. See [Privacy for minors](#privacy-for-minors).

### Portugal

Kept from the earlier pass, still accurate:

- **FPB, basketball (current provider).** [calendariofpb.pt](https://calendariofpb.pt/) is "Calendário não oficial. Os dados vêm de fpb.pt" and "Projeto pessoal independente, não afiliado com a FPB." It lists every FPB age group from Baby-Basket and Mini 8 to Sub 20, Sénior and Masters. `internal/fpb` already paces requests, keeps a disk cache and limits photo hosts.
- **FPV, volleyball, on Data Project.** [fpv-web.dataproject.com](https://fpv-web.dataproject.com/) publishes national divisions, Sub21 and Juniores competitions. Team pages (`/CompetitionTeamDetails.aspx?TeamID=<id>&ID=<competition>`) have squads with shirt number, name, position, height and birth year. Plain HTML, stable query-string ids, no key, no terms found.
- **FPF, football and futsal.** Regional associations point clubs to `resultados.fpf.pt`, which returned HTTP 403 to a plain client. No API or reuse terms.
- **FAP, handball.** [portal.fpa.pt](https://portal.fpa.pt/) loads results client side from an undocumented app backend. "Todos os direitos reservados."
- **zerozero.pt.** "© 2003-2026 ZOS, Lda. - Todos os direitos reservados." Avoid.

### Data Project (volleyball, many countries)

Data Project sells the "Web Competition Manager" to federations and says "65+ tra Leghe e Federazioni, tra cui CEV, ogni anno si affidano a noi per la raccolta e gestione dei dati dei loro campionati" ([dataproject.com](https://www.dataproject.com/)). Its hosted sites share one layout, so a parser written for FPV could read other federations on `*-web.dataproject.com`. Which federations those are is **not verified**: I found no public client list. No terms on the hosted sites.

### Netherlands: Nevobo (volleyball)

- **API.** `api.nevobo.nl` serves an OpenAPI description (`/docs.jsonopenapi`, version 2026.11.18) with 181 paths: clubs (`/relatiebeheer/verenigingen`), teams (`/competitie/teams`), team types, matches (`/competitie/wedstrijden`, with a `/live` variant), sports halls and beach volleyball.
- **Auth.** "Gebruik een API-key als bearer-token … of een access token via OAuth2." Most reads are open. Squads are not: `/competitie/teamleden` is "Lijst van teamleden (authenticatie verplicht)".
- **Stability.** "Deze API is op dit moment _niet_ stabiel. Backwards compatibility garanderen we _alleen_ voor projecten die de Nevobo zelf is gestart." And: "Identifiers van objecten zijn _niet_ stabiel … Alleen een UUID verwijst gegarandeerd steeds naar hetzelfde object."
- **Rate limits.** On HTTP 429, wait at least as long as `Retry-After`. Send a `User-Agent` with the app name, version and "een e-mailadres of website" so Nevobo can raise the limit. Hallstream should send the project website, never the user's address.
- **Age groups.** `/competitie/teamtypes` returns 30 types, for example "Meiden A" (MA), "Jongens B" (JB), "CMV Niveau 3" (N3), "Heren < 21" (HJ), "Dames Recreatief" (DR), "Volley Stars Level 5" (L5).
- **Licence.** None stated. Ask webmaster@nevobo.nl before shipping.

### Netherlands: NBB database (basketball)

[db.basketball.nl](https://db.basketball.nl/help) holds results, fixtures and statistics for "alle basketball competities in nederland", copied daily from Foys, the NBB's competition system. Its [integration page](https://db.basketball.nl/help/koppelingen) offers RSS, iCalendar, XML, JSON, a PHP client and plugins, keyed by the database's own ids for clubs, teams and competitions (looked up at `basketballstats.nl/db/service/howtolink.pl`). The site is run by a volunteer ("© 2026 Jaap Voets"). Today's results page shows youth games down to U10 and U12. No licence is stated. The [privacy page](https://db.basketball.nl/help/privacy) says it may hold a player's photo, height and birth date, with "beperkt" and "anoniem" options. Fit: good for Dutch basketball clubs, after asking the maintainer.

### Germany, Austria, Switzerland

- **handball.net (DHB).** The DHB's platform covers German handball "vom Amateurbereich bis zu ausgewählten Top-Wettbewerben" ([handball.net](https://www.handball.net/nutzungsbedingungen)). It took over schedules from handball4all on 1 January 2025. Community posts list undocumented `handball.net/api/new/…` endpoints for teams, matches and calendars ([ioBroker forum](https://forum.iobroker.net/topic/83769/handball-spielplan-per-javascript-widget/2)). No API terms. Ask the DHB.
- **nuLiga** (handball and volleyball regional associations, and other sports). The [Contao bundle](https://packagist.org/packages/contaobayern/contao-nuligadata-bundle) needs a client ID and secret per league portal. Others parse the HTML ([nuScrape](https://pkg.go.dev/github.com/taskmedia/nuScrape/pkg/parser)). Not open.
- **DBB (basketball-bund.net).** No documented API. A TYPO3 extension that imported its data is deprecated ([docs](https://docs.typo3.org/typo3cms/extensions/jh_basketballbund_import2/Introduction/Index.html)), and SpielerPlus imports DBB games through its support team from an exported file. **Not verified** whether the DBB offers `.ics`.
- **Swiss Volley.** I found no public API documentation. **Not verified.**

### France: FFBB

Community clients such as [ffbb-api-client-v3](https://pypi.org/project/ffbb-api-client-v3/) call the federation's search indexes with tokens they pick up from the FFBB app. Nothing from the FFBB documents or sanctions this. Avoid until the FFBB publishes terms.

### Spain: FEB

`baloncestoenvivo.feb.es` publishes live and result pages. I found no API documentation or terms. **Not verified.**

### United Kingdom: Basketball England

Basketball England moved its leagues to PlayHQ in 2025: "More than 20 Local Leagues have joined Basketball England's PlayHQ system" ([news](https://www.basketballengland.co.uk/news/2025/more-than-20-local-leagues-have-joined-basketball-england-s-playhq-system)) and PlayHQ's [case study](https://get.playhq.com/customer-success-stories/70-leagues-six-months-one-platform-basketball-englands-transformation-with-playhq) claims 70 leagues. **Not verified** directly: basketballengland.co.uk returned HTTP 403 and these come through search results.

### Nordics

- **Norway.** fotball.no gives every team a calendar file: "Select 'Subscribe to calendars' … You'll then see a link to a calendar file" ([Spond help](https://help.spond.com/app/en/articles/129669-import-matches-from-handball-no-and-fotball-no)). Spond imports handball.no by club search, which suggests handball.no has a feed or API that Spond reads. **Not verified.**
- **Denmark.** Spond also documents "Match import from dbu.dk". **Not verified** how dbu.dk exposes it.
- **Sweden and others: Profixio.** Profixio LEAGUE is "used by several Nordic top leagues in, for example, football, basketball, bandy and volleyball", and the Swedish basketball federation runs on it ([Profixio](https://profixio.com/?p=4006)). No public API docs found.
- **Finland: Torneopal.** No primary documentation found. **Not verified.**

### European and world bodies

The EHF sold data rights to Sportradar for ten years ([EHF](https://ehfec.eurohandball.com/news/en/ehf-and-ehf-marketing-sign-10-year-agreements-with-sportradar-for-data-collection-and-distribution-rights/)). Data Project names the CEV among its clients. None of these bodies publish amateur or youth club data.

### United States and Canada

- **TeamSnap.** OAuth 2 API at `api.teamsnap.com/v3` ([authorization docs](https://www.teamsnap.com/documentation/apiv3/authorization)), roster export as CSV, and a per-team calendar: on the Schedule tab, "Sync Calendar / Export" gives a subscribe link ([help](https://helpme.teamsnap.com/article/1245-subscribe-to-a-team-schedule)).
- **SportsEngine.** Team pages have an "iCal button" with a feed link, refreshed "once every 30 minutes" on sites ([help](https://help.sportsengine.com/en/articles/6307106-how-to-subscribe-to-an-ical-feed)). Admins get a CSV schedule export. The developer portal loads client side; **not verified** what it offers.
- **GameChanger.** "Schedule Sync" gives a calendar link; followers "will only be able to sync the game schedule" ([help](https://help.gc.com/hc/en-us/articles/115005457626-Integrating-Your-Personal-Calendar); read through search results, the page returned 403). No public API found.
- **LeagueApps.** An API for organisers and partners; "You must have a LeagueApps organizer account to generate a Private or Public API Key" ([support](https://support.leagueapps.com/hc/en-us/articles/360039865553-Downloading-API-Keys); read through search results).
- **Exposure Events** (tournaments). A documented REST API ([getting started](https://exposureevents.com/api/getting-started), [events](https://exposureevents.com/api/resources/events)) with signed requests using the event director's key and secret. Events, divisions, teams and games.
- **Spond** (Norway, used widely in Europe). Calendar sync to the phone only, and the help centre says it does not offer an open API. [Olen/Spond](https://github.com/Olen/Spond) is an unofficial GPL-3.0 library.
- **MaxPreps** (US high school). The site returned 403. A scraper's README says scraping "violates CBS Sports Interactive Terms of Use" ([maxprepsr](https://rdrr.io/github/daranzolin/maxprepsr/f/README.md)). **Not verified** from MaxPreps itself. Avoid.

### Australia and New Zealand

- **PlayHQ.** [API documentation](https://docs.playhq.com/tech) covers a REST API, webhooks and a data-warehouse feed. Through PlayHQ's support pages (403 to a plain client, read through search results): public endpoints need an `x-api-key` header, some guides add `x-phq-tenant` with the sport ("bv", "afl", "ca"), production is `api.playhq.com` for Australia and New Zealand and `api.euprod.playhq.com` for the UK and EU, credential requests "will be reviewed by PlayHQ and the administration body for the relevant sport", hidden organisations are left out, and games pages return `metadata.hasMore` and `metadata.nextCursor`. Endpoints include Grade Fixture Collection and Games Collection by organisation and date.
- **GameDay (MyGameDay).** Leagues can turn on "Sync to Calendar", which runs through eCal and asks for an email address ([help](https://support.mygameday.app/help/sync-your-teams-fixtures-to-your-calendar); read through search results). Whether that yields a plain `.ics` URL Hallstream can read is **not verified**.

## 3. Generic formats any source can feed

### iCalendar (.ics)

[RFC 5545](https://www.rfc-editor.org/rfc/rfc5545.txt) defines the format. Per-team feeds confirmed on: TeamSnap, SportsEngine, GameChanger, fotball.no, the NBB database, calendariofpb.pt (from the earlier pass). Probable but **not verified**: GameDay through eCal, handball.no, DBB.

What a feed gives: `DTSTART` (often with a `TZID`), `SUMMARY` ("Home FC vs Away FC" in some order and language), `LOCATION`, and sometimes `DESCRIPTION` with the competition. What it does not give: which side is home, the opponent as a separate field, colours, crests or squads. The importer needs:

- A rule per feed for splitting `SUMMARY` into home and away, with the club's own name matched against its aliases, and a manual fallback where the user picks the opponent and home or away once per event.
- `UID` as the fixture's stable id, so a refresh updates games instead of duplicating them.
- Refresh on request and on a schedule, with `ETag` or `Last-Modified`. TeamSnap says it pushes changes within an hour; SportsEngine site feeds refresh every 30 minutes.
- Times kept in UTC with the feed's `TZID`. `store.Fixture.Start` is "Lisbon time" today and has to become UTC plus a zone for clubs elsewhere.

### CSV

There is no shared convention. TeamSnap exports rosters, SportsEngine exports schedules, Spond exports attendance to Excel. The CSV importer `internal/store/csv.go` already reads squads and fixtures. To cover other platforms' exports it needs a column-mapping step (pick which column is the number and which is the name), UTF-8 with or without BOM, `;` as well as `,`, and day-first and month-first dates chosen by the user's locale.

### schema.org

[SportsEvent](https://schema.org/SportsEvent) has `homeTeam`, `awayTeam` and `competitor`. [SportsTeam](https://schema.org/SportsTeam) has `athlete`, `coach`, `gender` and `sport`. I fetched the home pages of calendariofpb.pt, handball.net, fotball.no, basketball.nl, competitions.ffbb.com, baloncestoenvivo.feb.es, basketball-bund.net, volleybal.nl, playhq.com, volleyball.ch, fpv-web.dataproject.com, fpf.pt, dbu.dk, basket.se and handball.no. None carried SportsEvent or SportsTeam markup. Their JSON-LD is only `WebSite`, `Organization`, `BreadcrumbList` and `SearchAction`. Team and match pages were not checked. schema.org is not a source to build on today.

## 4. Crests and colours

### Wikidata coverage

`query.wikidata.org` answered every request with HTTP 429 ("Aggressively rate-limiting to 1 req / min - this rule was created during active wdqs outage"). The counts below come from the same SPARQL run on [QLever's Wikidata endpoint](https://qlever.cs.uni-freiburg.de/api/wikidata), which serves a recent Wikidata dump. The dump date was not checked, so treat the numbers as close to today, not exact.

Items counted: sport (P641) and country (P17) set, and instance of (P31) one of sports club, sports team, association football club, basketball team, or similar club and team classes. "In a league" means the item has P118, which leans toward higher levels.

| Sport | Country | Items | With logo (P154) | With colours (P6364) | In a league (P118) | In a league, with logo |
|---|---|---|---|---|---|---|
| Basketball | PT | 29 | 0 | 0 | 5 | 0 |
| Basketball | ES | 428 | 15 | 2 | 57 | 7 |
| Basketball | FR | 314 | 11 | 2 | 45 | 6 |
| Basketball | DE | 299 | 41 | 12 | 55 | 13 |
| Basketball | US | 2148 | 48 | 2 | 964 | 45 |
| Basketball | AU | 100 | 2 | 0 | 17 | 1 |
| Basketball | NL | 77 | 2 | 0 | 4 | 0 |
| Football | PT | 634 | 13 | 17 | 252 | 10 |
| Football | ES | 1918 | 45 | 34 | 833 | 24 |
| Football | FR | 1964 | 88 | 47 | 389 | 46 |
| Football | DE | 6732 | 735 | 86 | 690 | 301 |
| Football | US | 1092 | 77 | 3 | 550 | 63 |
| Football | AU | 416 | 2 | 22 | 224 | 2 |
| Football | NL | 536 | 35 | 5 | 172 | 25 |

Volleyball and handball are thinner still outside Germany: German volleyball 296 items with 52 logos, German handball 583 with 111; every other country has at most 5 handball logos and 2 volleyball logos.

Top divisions, counting teams with P118 set to the league (teams that ever played in it):

| League | Teams | With logo | With colours |
|---|---|---|---|
| National Basketball Association | 38 | 19 | 0 |
| Liga ACB | 20 | 4 | 0 |
| Basketball Bundesliga | 19 | 4 | 1 |
| Premier League | 41 | 6 | 21 |
| Bundesliga | 29 | 13 | 16 |
| Primeira Liga | 23 | 4 | 7 |

Germany is the outlier. My guess, not checked, is that many German crests are simple enough to sit on Commons as below the threshold of originality. Elsewhere, even professional clubs mostly lack a logo on Wikidata, and amateur clubs mostly lack an item at all. Colours are a little better for top football clubs and near zero for basketball.

### Commons licensing

Commons "only accepts free content" and "does not accept fair use justifications" ([Commons:Licensing](https://commons.wikimedia.org/wiki/Commons:Licensing)). Most crests live on individual Wikipedias as non-free files, and English Wikipedia says logos "are usually non-free images" ([Wikipedia:Logos](https://en.wikipedia.org/wiki/Wikipedia:Logos)). A crest on Commons tagged public domain is free of copyright only: "a logo can still be protected by trademark law, even if it is not subject to copyright."

### Showing an opponent's crest

This is not legal advice.

- **EU.** The Trade Mark Directive 2015/2436, [Article 14(1)(c)](https://eur-lex.europa.eu/eli/dir/2015/2436/oj/eng), lets a third party use a mark "for the purpose of identifying or referring to goods or services as those of the proprietor", provided the use is "in accordance with honest practices in industrial or commercial matters" (Article 14(2)). These quotes come from the earlier pass; EUR-Lex returned an empty page today. Showing an opponent's crest next to its name on a scoreboard identifies the opponent. EU member states implement the Directive in national law, so the national wording governs.
- **US.** The Lanham Act excludes "any fair use, including a nominative or descriptive fair use … of a famous mark by another person other than as a designation of source for the person's own goods or services" from dilution claims ([15 U.S.C. § 1125(c)(3)(A)](https://www.law.cornell.edu/uscode/text/15/1125)). Nominative fair use against infringement claims comes from case law (the Ninth Circuit's *New Kids on the Block* test: the use is needed to identify, takes only as much of the mark as needed, and suggests no endorsement). I did not read the case itself.
- **Copyright** in the crest artwork is a separate question that neither text answers.

What follows for Hallstream: never bundle crests, fetch one only when the user asks, keep it in the user's local cache, record where it came from, and show nothing that suggests the opponent endorses the stream. The club's own crest is the club's to use.

### Colours

Hex values are facts. Taking them from Wikidata P6364 (CC0), from TheSportsDB's `strColour1` to `strColour3` through a licensed key, from a federation page, or from the crest's pixels is safe. Wikidata's colours are items ("red", "royal blue"), so the provider needs a table from colour item to hex, or reads the item's own hex property.

## 5. Languages

### How sources name things

| Source | Names in several languages |
|---|---|
| Wikidata | Labels and aliases per language on every item. A club usually has a label in its own language and English; others vary. |
| TheSportsDB | One `strTeam`, plus `strTeamAlternate` (comma-separated aliases) and `strTeamShort`. Descriptions in up to 14 languages. No language parameter. |
| Sportmonks | `locale` parameter, English fallback. |
| Sportradar | `{language_code}` in the path; the language list differs per product. |
| football-data.org, API-Sports, BallDontLie | No language parameter found. API-Sports **not verified**. |
| Federations | One name, in the federation's language, often with a sponsor in it ("Liga SOLVERDE.PT", "Zorg en Zekerheid MSE 1"). |

### What the app should store

- `Name`: the source's own name, exactly as given. This is what matching, refreshes and the next-game card's "as the source writes it" field use (`provider.Side.FullName` today).
- `Short`: the source's short name, or one the club types. Scoreboards need three or four letters.
- `Labels`: an optional map from language tag (BCP 47, `pt`, `en`, `nl`) to a display name, filled only when the source supplies it (Wikidata, Sportmonks) or the user types it. The overlay uses the club's chosen label, then `Name`.
- `Aliases`: other spellings the source gives (TheSportsDB `strTeamAlternate`, Wikidata aliases), used only to match `.ics` summaries and search results.

Club names should not be machine-translated. "Sporting Clube de Portugal" stays as it is in an English UI. Only the generic words around it come from `web/i18n/<lang>.json`.

### Age groups and categories

Every federation names its groups differently, and some names carry no age at all.

| Country, sport | Source labels | Notes |
|---|---|---|
| PT, basketball (FPB) | Baby-Basket, Mini 8, Mini 10, Mini 12, Sub 14, Sub 16, Sub 18, Sub 20, Sénior, Masters | From calendariofpb.pt, earlier pass |
| PT, volleyball (FPV) | Juniores, Juniores A, B, B1, Sub21 | Data Project site, earlier pass |
| NL, volleyball (Nevobo) | Meiden A/B/C, Jongens A/B/C, Mix A/B/C, CMV Niveau 1 to 6, Heren < 21, Dames < 21, Heren Master, Volley Stars Level 3 to 5 | `/competitie/teamtypes`, today. A/B/C are ages, CMV levels are skill bands for the youngest |
| NL, basketball (NBB) | Mannen U16, Vrouwen U12, Gemengd U10, M18, X12, MSE | Team names in today's db.basketball.nl results |
| ES, basketball | Benjamín, Alevín, Infantil, Cadete, Junior | Regional school-sport rules with birth years; the FEB's own table **not verified** |
| FR, basketball (FFBB) | U7, U9, U11, U13, U15, U18, U21 | [Hauts-de-France table](https://hautsdefrancebasketball.org/wp-content/uploads/2024/08/TABLEAU-SURCLASSEMENTS-2024-2025.pdf), 2024/25 |
| DE, football and handball | A- to F-Jugend; U-numbers | **Not verified** today |
| US, many sports | 14U, JV, Varsity, grade levels | **Not verified** today |

How to map them:

- Keep the source label as text, always. It is what volunteers recognise.
- Add an optional normalised record next to it: `kind` (`youth`, `senior`, `masters`, `open`), `under` (the U-number when there is one), `gender` (`m`, `f`, `mixed`), and `tier` (free text for "A", "B1", "Niveau 3").
- Fill the record from a small per-provider table (Nevobo's "JB" is youth, boys, tier B), and leave `under` empty where the source gives no age. "Meiden B" maps to an age only through Nevobo's yearly birth-year table, which the API does not return.
- Let the user correct it. The `youth` flag drives the privacy defaults below, so it must be right.

## Privacy for minors

- **GDPR.** [Recital 38](https://gdpr-info.eu/recitals/no-38/): "Children merit specific protection with regard to their personal data". [Article 6(1)(f)](https://gdpr-info.eu/art-6-gdpr/) allows legitimate interests "except where such interests are overridden by the interests or fundamental rights and freedoms of the data subject … in particular where the data subject is a child." [Article 8](https://gdpr-info.eu/art-8-gdpr/) sets the consent age (16, or down to 13 by national law) for information society services offered directly to a child. Hallstream is not offered to children, so Article 8 does not bind it, but the club that streams a youth game is the controller for the names and faces on air.
- **COPPA.** The rule binds an "operator" of "a website located on the internet or an online service" that collects personal information from children under 13 ([16 CFR 312.2](https://www.ecfr.gov/current/title-16/chapter-I/subchapter-C/part-312/section-312.2); read through search results). Hallstream is a local desktop program and collects nothing from children, so COPPA most likely does not apply to the project. A US club streaming under-13 games should still get parental permission under its league's rules. The FTC's COPPA FAQ returned 404 at the address I tried.
- **What the sources expose.** FPV's squads list birth year and height. The NBB database may hold photo, height and birth date. FPB serves photos. Nevobo keeps squads behind a key.

What every provider must do:

1. Parse only shirt number, display name and an optional photo reference. Drop birth date, birth year, height, position and nationality at parse time, and never write them to the cache.
2. Keep photos off by default, per team, so a club can show seniors' photos and leave its under-14s without.
3. For youth teams, default the display name to "First L." with options for the full name or the number only.
4. Expire cached squads at season end and give the user a "clear cached squads" button.
5. Use invented people in tests and fixtures. Never record a real youth squad into test data.

## Recommendation

### Build next, in order

1. **iCalendar (`.ics`) fixtures import.** Grassroots tier, worldwide, any sport. Confirmed feeds on TeamSnap, SportsEngine, GameChanger, fotball.no, the NBB database and calendariofpb.pt cover most US and Canadian youth teams and a good share of European ones. It is fixtures only, so it pairs with the existing CSV squad import. It is also the fallback for every federation without an open API (FPF, FAP, DBB, handball.no).
2. **Netherlands: Nevobo and the NBB database.** Grassroots and pro, keyless reads, youth included. Nevobo is the only federation API with a published OpenAPI spec, rate-limit rules and a stable id (the UUID). The NBB database adds Dutch basketball with JSON and iCalendar. Building these proves the interface outside Portugal and in a second language. Ask both maintainers first. Nevobo squads need a key, so start with teams and fixtures there.
3. **PlayHQ.** Grassroots tier for Australia, New Zealand and the UK, Basketball England's 70 leagues included. Documented API with cursor paging. The catch is that every deployment needs credentials approved by PlayHQ and the sport body, so the provider must take a key the club or its league obtains. Apply for a project key early to learn what the review asks.
4. **FPV through Data Project.** Grassroots tier for Portuguese volleyball, with squads and youth competitions. The same parser may later read other federations on Data Project's hosted sites. Ask the FPV, pace like FPB, strip birth year and height at parse.
5. **A bring-your-own-key provider for professional leagues, starting with TheSportsDB.** Pro tier. TheSportsDB's terms are the clearest of the general APIs and allow caching API responses, and its team records carry colours, aliases and descriptions in up to 14 languages. Each club enters its own paid key. Ask TheSportsDB whether a free GitHub-distributed app with user keys counts as "publish apps to an appstore". Add API-Sports behind the same provider once someone reads its terms, since it covers all five sports. BallDontLie is the candidate for professional basketball clubs in the NBA, EuroLeague and ACB, because its terms allow storing and redistributing without attribution.

Alongside these, **Wikidata as an enrichment source** for colours and localised labels of a club the user has already picked. It is CC0 and keyless, but too thin to be a provider of record.

### Avoid

- **Sofascore, Flashscore, zerozero.pt, Transfermarkt, Sports-Reference, MaxPreps:** terms forbid automated access or reserve all rights.
- **ESPN's hidden API:** Disney's terms bar robots and commercial use.
- **Sportmonks:** forbids storing its data without written consent, which an offline cache needs.
- **Sportradar, SportsDataIO, Stats Perform, Genius Sports, FIBA GDAP, EHF:** paid contracts, non-commercial trials, elite coverage only.
- **Undocumented federation backends** (FFBB's app API, handball.net's `/api/new`, FAP's app host, FIBA LiveStats `data.json`, the unofficial Spond library): unsanctioned and liable to change. Ask the federation, and build only on a yes.
- **football-data.org as a default:** usable only with each club's key, an attribution line and a delete-on-cancel cache, for top football leagues the club rarely needs.
- **GitHub crest repos and Wikimedia Commons as a crest source:** the files do not carry the right to the mark, and coverage is poor.

## What the `Provider` interface needs

Today `provider.Provider` is `ID()` and `Fixtures(store.Group)`, with FPB fields (`FPBTeamID`, `FPBPlayerID`, `FPBAthleteID`, `NumberFromFPB`) spread through `store.Group` and `store.Player`. The sources above need the following.

**Capabilities.** Each provider declares what it can do, because most do one or two things: `.ics` gives fixtures only, Wikidata colours and labels only, Nevobo teams and fixtures without a key and squads with one. In Go this fits small optional interfaces checked by type assertion (`ClubSearcher`, `TeamLister`, `SquadLister`, `FixtureLister`, `CrestSource`) plus an `Info()` that lists them for the panel. A capability can depend on a key being present.

**Info.** Per provider: id, name for the panel, sports, countries, whether it needs a key and where to get one, attribution text, licence note, terms URL, allowed image hosts (as `PhotoHosts` does now), minimum request interval, and whether it is opt-in. Every network provider stays off until the user turns it on.

**Search and browse.** `SearchClubs(query, filter)` with sport, country and locale in the filter. Some sources have no search and only browse (Data Project by competition, PlayHQ by organisation then season then grade, Nevobo by region), so browsing a tree is a first-class path: `Children(node)` returning competitions, groups or teams with a kind on each.

**Teams and age groups.** `Teams(club, season)` returns teams with sport, season, gender, the source's age-group label and the optional normalised record from [Age groups](#age-groups-and-categories). A season has a label ("2026/27", "2026") and dates, since southern-hemisphere and US seasons do not run July to June.

**Squads.** `Squad(team)` returns, per player, the source's stable id, shirt number, display name and an optional photo reference, and nothing else. A `youth` flag on the team switches on the privacy defaults.

**Fixtures.** `Fixtures(team, from, to)` returns the current `provider.Fixture` fields plus a stable id from the source (`UID` for `.ics`, the UUID for Nevobo), the start in UTC with the IANA zone, a `Timed` flag (kept), status (scheduled, postponed, cancelled, played), and home and away as given or "unknown" for feeds that do not say.

**Crests and colours.** Separate calls, each result carrying its source URL, licence note and whether it is the club's own federation image or a third-party file. Colours as `#RRGGBB`. The UI shows where a crest came from and lets the user replace it.

**Ids.** References namespaced by provider: `fpb:1234`, `nevobo:<uuid>`, `ics:<feed-hash>/<UID>`. Store the id the source promises is stable, which is not always the obvious one (Nevobo: the UUID, not the path id). A local team links to several providers at once (squad from CSV, fixtures from `.ics`, colours from Wikidata), so `store.Group` gets a list of links in place of the FPB fields, and `store.Player` a source reference in place of `FPBPlayerID` and `FPBAthleteID`. The FPB season-rollover rule (`FPBTeamKey`) generalises to "find next season's team by the age-group key".

**Paging and rate limits.** A cursor or page token on every list call (PlayHQ `nextCursor`, Nevobo pages, Exposure pages). A per-provider pacer that honours HTTP 429 and `Retry-After` and also enforces fixed quotas the source states: 100 a day (API-Sports, football-data.org without a key), 10 a minute (football-data.org free), 5 a minute (BallDontLie free), 30 a minute (TheSportsDB free), one client at 60 seconds of work per minute (Wikidata). The FPB client's `turn` and `retryAfter` already work this way and can move behind the interface. Send a `User-Agent` naming Hallstream, its version and the project website.

**Keys.** Optional per-user credentials (API key, client ID and secret, OAuth token) kept in `dados/` beside the other logins, never in the repository (football-data.org 6.1 forbids it) and never in logs. Removing a key deletes that provider's cached data where the terms require it (football-data.org 9.1).

**Attribution.** Each provider returns the exact attribution string its licence asks for ("Football data provided by the Football-Data.org API", "Daten von OpenLigaDB (www.openligadb.de), lizenziert unter ODbL 1.0"), shown in settings, and a flag for whether the overlay must show it too.

**Offline cache.** Every result carries `fetchedAt` and the source's validator (`ETag`, `Last-Modified`). The app reads the cache first, works offline after one load, refreshes on request and on a schedule, and expires squads at season end. `Status()` returning reachability and the cache age, as `fpb.Client` does, belongs on the interface. Where the terms limit storage, the provider says so in `Info` and the cache obeys.

**Locale.** Search and list calls take the user's language. Results carry the source's `Name`, optional `Labels` by language tag and `Aliases`, as in [What the app should store](#what-the-app-should-store). A provider never translates a name itself.
