# Scoreboard console serial protocols: follow-up

Follow-up to [scoreboard-serial-protocols.md](scoreboard-serial-protocols.md). It covers four questions: real Daktronics captures, the full Bodet 608264 J, other consoles, and the All Sport 5000 RTD connector. Nothing here repeats the first document unless a new source changes it.

Checked on 2026-11-27. **Inferred** marks anything I worked out rather than read.

## Verdicts

| Question | Verdict | Most useful source |
|---|---|---|
| 1. Real basketball or volleyball RTD capture | **NOT FOUND.** One more real football capture, with no licence. | [ChatLurking/Daktronics-Scoreboard `daktronics.cap`](https://github.com/ChatLurking/Daktronics-Scoreboard) |
| 2. Full Bodet 608264 J | **FOUND**, complete, 53 pages | [static-prod.bodet-sport.com 608264](https://static-prod.bodet-sport.com/assets/base-documentaire/en/scorepad/608264-Network%20output%20and%20protocols-Scorepad.pdf) |
| 3. Other consoles | No manufacturer publishes a byte layout. One MIT project reverse-engineered about 40 brands from a commercial app. | [fpgt-broadcast/scorereader `docs/vendors/`](https://github.com/fpgt-broadcast/scorereader/tree/main/docs/vendors) |
| 4. RTD connector | **J6 "MAIN PORT", DB-25.** RS-232 on pins 2, 3 and 7, plus a current-loop copy on pins 22 and 7. | Daktronics [ED-11976](https://www.daktronics.com/web-documents/customer-service-manuals/ed11976.pdf), drawing 1196-R04A-102142 (PDF page 179) |

## 1. Real Daktronics captures

I listed the files of every Daktronics repo that `gh search repos daktronics` returned (44 repos) and opened every file that looked like a capture.

| File | Licence | Console and code | Size | Real? | Settles volleyball? |
|---|---|---|---|---|---|
| [ChatLurking/Daktronics-Scoreboard `daktronics.cap`](https://github.com/ChatLurking/Daktronics-Scoreboard) | **none** (no licence file, repo from 2015) | All Sport 5000 RTD, **football 6601** | 10,597 bytes, 197 frames | Yes | No |
| [JoshuaCarroll/scoredata `raw-J5-output.txt`, `raw-J6-output.txt`](https://github.com/JoshuaCarroll/scoredata) | MIT | RTD, football | 9,214 and 7,534 bytes | Yes (already in the first document) | No |
| [DDrev/BouncePass `src/bouncepass/capture.txt`](https://github.com/DDrev/BouncePass) | GPL-3.0 | Older Daktronics "TV feed" at 9600 baud, basketball, with SOH and EOT stripped. **Not RTD.** | 5,370 bytes, 179 lines | Yes (inferred from the clock counting down 8:00, 7:59, ...) | No |
| [KGeetings/DaktronicsToScorebug `RTD_AS5-Basketball.json`](https://github.com/KGeetings/DaktronicsToScorebug) | none | Decoded field values exported from a DMP-8000 player, not bytes | 166,890 bytes | Decoded, not raw | No |
| [junglejim63/daiquiri-adapter `*.bin`](https://github.com/junglejim63/daiquiri-adapter) | MIT | OmniSport swim timing, 38400 baud. Different protocol. | 2,272 and 3,504 bytes | Probably | No |

**What the football capture adds.** I ran the first document's checksum rule over `daktronics.cap`. All 197 frames pass. It holds 196 `004210` frames and one `003010006` frame, the same odd frame the scoredata capture has. The rebuilt buffer reads `6601` at positions 184 to 187. That matches sport-streamer's claim that the sport code sits at zero-based index 183, from a second, unrelated console. The buffer also shows `HOME`/`GUEST` at 48 and 68 and the score `  28`/`  13` at 108 and 112.

**Volleyball points (items 108/112 vs 240/260): not settled.** No raw volleyball RTD capture exists in any repo I found. Three indirect signs point to 108/112 carrying the running points. None is a capture:

- ScoreConnect III, the commercial Sportzcast app, reads volleyball score from columns 109 to 110 and 113 to 114, which are items 108 and 112. It reads games won from 215 and 217 as separate fields. This comes from [scorereader `docs/vendors/daktronics_serial.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/daktronics_serial.md), whose author worked it out from ScoreConnect's behaviour.
- Daktronics' own player template [`AS5-Volleyball.itf`](http://dakfiles.daktronics.com/downloads/data/ITF/AS5-Volleyball.itf) gives "Home Team Score" 4 characters with sample text `8888`, and "Home Score - Current Game" 2 characters with `88`. The positions in all four Daktronics volleyball templates (`AS5-Volleyball`, `-9200`, `-9220`, `DSTI AS5-Volleyball`) add up exactly to ED-12483's items, so the templates confirm the table but say nothing about which field moves.
- [BristolTNCitySchools `Volleyball-2101/VolleyballSerialParserCOM.py`](https://github.com/BristolTNCitySchools/Scoreboard-Data-for-Daktronics-All-Sport-CG/blob/main/Volleyball-2101/VolleyballSerialParserCOM.py) (MIT) was "tested using game code 2101 on a Daktronics All Sport 5000". It reads the All Sport **CG** feed (9600 baud), not RTD. It finds the running score at bytes 9 to 12, separate from games won (17, 18), the set number (19) and per-set scores (20 on). ScoreReader's CG volleyball layout gives the same offsets. So the console keeps running points apart from set scores. That 240/260 then mirrors 108/112 is **inferred**.

To settle it, record RTD from a volleyball code (2101 or 2105) and watch which positions change when a point is scored.

**Sport codes for volleyball.** [mikedahlgren/daktronics-vb-scoreboard `protocol/sport_codes.md`](https://github.com/mikedahlgren/daktronics-vb-scoreboard/blob/main/protocol/sport_codes.md) (MIT) lists 2101 (match/game), 2105 (with player stats) and 2111. It cites ED-11976 Appendix C. I did not check those codes against the manual.

**Downloads.** I downloaded no captures. The only basketball or volleyball byte streams are GPL-3.0 (BouncePass) or carry no licence. The `captures/` folder was not created.

## 2. Bodet 608264 revision J

### Where it is

The full English revision J is live again on a new host: [static-prod.bodet-sport.com/assets/base-documentaire/en/scorepad/608264-Network output and protocols-Scorepad.pdf](https://static-prod.bodet-sport.com/assets/base-documentaire/en/scorepad/608264-Network%20output%20and%20protocols-Scorepad.pdf). It is 2,059,120 bytes and 53 pages, made with InDesign on 2021-09-10, and the cover reads "Réf.: 608264 J". The [Wayback copy from 2026-10-11](http://web.archive.org/web/20261011125043id_/https://static-prod.bodet-sport.com/assets/base-documentaire/en/scorepad/608264-Network%20output%20and%20protocols-Scorepad.pdf) is complete and has the same SHA-256 (`07d899e5…dbe374`).

I found no French 608263 in the Wayback index for the new host. The English revision J covers everything we need. The PDF has no copyright notice and no terms in its text.

The same folder on the new host also has every Scorepad sport user manual, for example [607931 Volleyball](https://static-prod.bodet-sport.com/assets/base-documentaire/en/scorepad/607931-VOLLEYBALL-User-manual-Bodet-Scorepad.pdf) and [608084 Futsal](https://static-prod.bodet-sport.com/assets/base-documentaire/en/scorepad/608084-FUTSAL-User-manual-Bodet-Scorepad.pdf).

### TCP: the PC is the server

Page 4 shows the network diagram. The Scorepad is the **TCP client** and the PC is the **TCP server**: "The PC must run in 'TCP server' mode, default port: 4001 (configurable)". The example addresses are Scorepad 192.168.1.100 and PC 192.168.1.200. On the Scorepad, the technician sets "Protocol TV", the PC's IP address and the port. A green rectangle on the Scorepad screen confirms that it reached the PC (page 7).

In the serial case (pages 9 to 13), the Moxa interface sits where the PC would be, and the Scorepad connects to it the same way.

So a Hallstream source for Bodet over the network must **listen** on a TCP port. It does not dial out to the Scorepad. That it reconnects after a dropped link is **inferred**, since the manual does not say.

### Idle output

The manual says nothing about keep-alives or idle traffic. It says only "Start the chosen sport and the match. The protocol will be automatically transmitted on the RJ45 output" (page 8, and page 13 for serial). So, **inferred**, a source should expect silence until a match starts, and should not treat silence as a dead link.

### Volleyball (section 15, pages 51 to 53)

These tables confirm the first document's paraphrase byte for byte.

**Message 06**: current set, sets won, points, timeouts, timeout countdown.

| Byte | Content | Example |
|---|---|---|
| 1, 2 | «0» «6» | |
| 3 | Status word | |
| 4 | «0» = Volleyball | |
| 5 | Current set number | «4» |
| 6 | Sets won by Home | «2» |
| 7 | Sets won by Guest | «1» |
| 8, 9 | Score Home * 10, * 1 | «1» «2» |
| 10, 11 | Score Guest * 10, * 1 | 20H «8» |
| 12 | Number of time out Home | «1» |
| 13 | Number of time out Guest | «3» |
| 14, 15 | Seconds of time out in progress * 10, * 1 | «2» «8» |

Status word for 06 and 07: b1 = 1 clock OFF. b2 = 1 horn ON. **b3 = 1 home ("local") team serving, 0 guest serving.** b7 = 1. The rest are ignored.

**Message 07**: «0» «7», status word, «0», then Minutes * 10, Minutes * 1, Seconds * 10, Seconds * 1. A leading zero shows as 20H.

**Message 08**: ended sets only. "The current set is not sent (sent in the message 06), only the ended sets are sent. The current set is blanked." It is «0» «8», status word, «0», then home and guest points for sets 1 to 4, two bytes each (bytes 5 to 20). The table holds copy errors from the tennis section: the text says "Games" instead of points, byte 2 is printed «8» (36H), and the hex in brackets does not match the example digits. Read the digits, not the hex (inferred).

### Futsal (section 12, page 46)

**Message 45** is the only futsal message.

| Byte | Content |
|---|---|
| 1, 2 | «4» «5» |
| 3 | Status word |
| 4 to 7 | Minutes * 10, * 1, Seconds * 10, * 1 |
| 8 | Tenths |
| 9 to 11 | Score Home, 3 characters |
| 12 to 14 | Score Guest, 3 characters |
| 15 | Period number, or 'E' in extra time |
| 16 | Extra time number |

Status: b0 = 0 game clock, 1 rest timer. b1 = 1 clock OFF. b2 = horn (the text says "game clock OFF/ON", a copy slip). b7 = 1.

The score padding matches the first document's warning. The examples show 5 as `20 35 20`, 12 as `20 31 32` and 104 as `31 30 34`. Futsal has **no** team fouls, timeouts or sport-code byte in this protocol.

### Changes since revision A that affect existing decoders

- **Basketball extra time is 'O', not 'E'.** Message 18 says "During extra time the transmitted character is 'O'" (pages 18, 19, and the 3x3 section). Handball, football and futsal still say 'E'. A decoder should accept both (inferred).
- **Status b0 now means rest timer.** Basketball message 18, handball message 01, futsal 45 and football 10 all give b0 = 1 for "Rest timer (between periodes, before-match)". Revision A said "ignored".
- **Handball horn polarity is fixed.** Message 01 b2 now reads "= 0: horn OFF. = 1: horn ON.", the same as basketball. Revision A had it reversed.
- **New basketball message 60, bonus indicator** (page 27): «6» «0» «5», then a home status byte and a guest status byte. b0 = 1 means the foul indicator is on.
- **Message 32** now carries guest player fouls, beside 33 and 34.
- **Team names 98 and 99** gain a three-letter abbreviation in bytes 21 to 24.
- **Football, rugby and beach soccer** use message 10: clock, 3-character scores, period and extra time number (page 45).

Revision J prints no sample frames, so the LRC contradiction in revision A's message 50 traces stays open.

### Cross-check

ScoreConnect III ships `Bodet_Basketball` and `Bodet_Volleyball` decoders, according to [scorereader `docs/vendors/bodet.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/bodet.md). That note says ScoreConnect's volleyball decoder writes `HomeGamesWon` twice and never sets `GuestGamesWon`, a bug a Hallstream decoder should not copy.

## 3. Other consoles

No manufacturer I checked publishes a byte-level score protocol. The useful find is [fpgt-broadcast/scorereader](https://github.com/fpgt-broadcast/scorereader), MIT, © 2026 Jack Singewald. It has a decoder, a vendor note and tests for about 40 brands.

**Read this before using it.** Its README says the layouts were "worked out from the ScoreConnect III software the owner already has", and the notes cite the "Windows decompile" as the primary source. Most layouts therefore describe the wire **after** a Sportzcast ScoreBot box. Many arrive through that box's PIC re-framed as `55 AA` packets at 57600 baud, which is not what the console itself sends. Almost every note says "No captures were available; tests use synthetic frames". Its MIT licence covers the author's code. Whether layouts taken from a decompiled commercial app are safe to reuse is a legal call I cannot make. Treat them as leads.

| Console | Verdict | Source | Licence | Sports | Notes |
|---|---|---|---|---|---|
| Nevco (MPC series) | **PARTIAL** | [scorereader `nevco.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/nevco.md) | MIT (decompile-derived) | basketball, football, baseball, others | Only as decoded by the ScoreBot PIC (`55 AA B7 …`, 57600). Nevco's own wire format is not given. Nevco's [docs bucket](https://nevcodocs.s3.amazonaws.com/Controls/MPC-X2/VolleyballX2.PDF) holds operator manuals only. Daktronics says Nevco needs "Code 134" for its apps ([KB 000002613](https://www.daktronics.com/en-us/support/kb/000002613)). |
| Electro-Mech | **PARTIAL** | [scorereader `electromech.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/electromech.md) | MIT (decompile-derived) | football, basketball, soccer, baseball | Raw current loop, 2400 baud inverted, 2-byte address/value cells with no checksum. "No sample capture from real hardware." The [LX2770 manual](https://www.electro-mech.com/Manuals/pdf/basketball/LX2770%20Manual.pdf) names a serial data jack but gives no format. |
| OES (ISC series) | **PARTIAL** | [scorereader `oes.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/oes.md); Daktronics [`OES Basketball.itf`](http://dakfiles.daktronics.com/downloads/data/ITF/OES%20Basketball.itf) | MIT (decompile-derived); Daktronics file has no licence | basketball, football, hockey, soccer, volleyball, baseball, wrestling, lacrosse | Raw RS-232 at 9600: `STX kind data checksum CR`, checksum = sum & 0x7F, plus 0x20 if below 0x20. That is the closest of all to the console's own wire. OES has a "Standard" and a factory-default "Pro" protocol ([OES one-page instructions](https://www.oes-inc.com/app/uploads/sites/2/2021/12/One-page-instructions.pdf)), and Daktronics says the console needs special OES firmware for its integration ([KB DD2621379](https://www.daktronics.com/en-us/support/kb/DD2621379)). The [ISC-9000 manual](https://www.oes-inc.com/app/uploads/sites/2/2021/12/ISC9000-UserManual_J12_1.3.pdf) gives no byte layout. |
| Daktronics All Sport 3000 / 1600 | **PARTIAL** | [scorereader `daktronics_serial.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/daktronics_serial.md), [`daktronics.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/daktronics.md) | MIT (decompile-derived) | many | Covers the All Sport 4000 7-segment frame (`AA` 16 bytes `AB`), the All Sport CG feed, and the current-loop scoreboard output (`55 AA` after the PIC). Nothing names the 3000 or 1600 RTD. [ED-17652](https://daktronics.com/web-documents/customer-service-manuals/ed-17652.pdf) (now read) lists only "All Sport console", input AS5-[sport], 19200 8N1. |
| Stramatel (wired, not Bluetooth) | **PARTIAL** | [scorereader `stramatel.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/stramatel.md) | MIT (decompile-derived) | basketball, hockey, ice hockey, futsal, handball, volleyball, tennis, table tennis, badminton | RS-485 at 19200 (BE04030 at 9600). 54-byte packets, a channel byte `F8` to `FD`, then a sport character, ending `0D`, with no checksum. "No captures were available." Stramatel's manuals ([452 ME 800 Futsal](https://www.stramatel.com/wp-content/uploads/2019/07/452-ME-800-FUTSAL.pdf)) give no protocol. Worth comparing with Hallstream's own Stramatel Bluetooth decoder (inferred). |
| Swiss Timing | **PARTIAL** | [scorereader `swisstiming.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/swisstiming.md) | MIT (decompile-derived) | basketball, hockey, water polo, swimming | `STX type payload ETX xor`. A Swiss Timing manual with a "Transmission Protocol" section, 3449.500, returned 404 on [swisstiming.com](https://www-swt-com.web.swisstiming.com/fileadmin/Resources/Instruction_Manuals/3449.500.pdf) and has no Wayback copy. |
| Mobatime | **NOT FOUND** | | | | No document and no decoder beyond [Panel2Net](https://github.com/tomkohler/Panel2Net), which has no licence. |
| Colorado Time Systems | **PARTIAL** | [scorereader `coloradotiming.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/coloradotiming.md); [hwbrill/vsCTS](https://github.com/hwbrill/vsCTS); [chrissabato/dakbot `colorado.py`](https://github.com/chrissabato/dakbot) | MIT; vsCTS and dakbot: not checked | water polo, swimming | Swim timing, not our sports. |
| Fair-Play | **PARTIAL** | [scorereader `fairplay.md`](https://github.com/fpgt-broadcast/scorereader/blob/main/docs/vendors/fairplay.md) | MIT (decompile-derived) | basketball, hockey, lacrosse, soccer, football, baseball | New since the first document: an RS-485 layout, `code type ?? L data sum16`, with a 16-bit little-endian sum. It reads like the ScoreBot's view, not the MP-70 RS-232 "ProLine" output. |
| Nautronic, Varsity, Spectrum, Eversan, others | **PARTIAL** | [scorereader `docs/vendors/`](https://github.com/fpgt-broadcast/scorereader/tree/main/docs/vendors) | MIT (decompile-derived) | various | Same caveats. Varsity's note says "Nothing was verified against real hardware." |

**A second Daktronics source worth knowing.** Daktronics publishes the parser templates its own video players use at [dakfiles.daktronics.com/downloads/data/ITF/](http://dakfiles.daktronics.com/downloads/data/ITF/), with sibling `DID` and `DSRC` folders. The `AS5-*.itf` files are ED-12483's field tables in a machine-readable INI format (name, length, justify, sample text), one per sport, and they are current. The folder also has templates for OES, CTS, Omega Quantum and FinishLynx. These are positional templates over an already-framed buffer, not wire specs. The files carry no licence.

## 4. The All Sport 5000 RTD connector

Source: Daktronics [ED-11976](https://www.daktronics.com/web-documents/customer-service-manuals/ed11976.pdf), "All Sport 5000 Series Control Console Operation Manual", Rev 32, 256 pages, Appendix A drawing **"Rear View, A/S 5010 Connector Assignments", 1196-R04A-102142 rev 3 (05 Oct 2001)**, PDF page 179.

**J6, "MAIN PORT", is a 25-pin connector on the back of the console.** The drawing's pin table:

| Pin | Function |
|---|---|
| 1 | Earth |
| 2 | RS232 Receive + |
| 3 | RS232 Transmit + |
| 7 | C. L. Output 4 − / RS232 GND |
| 8, 21 | Switch input 1 +, − |
| 9, 10 | Current loop input +, − |
| 11, 23 | Relay output +, − |
| 14 to 19 | Current loop outputs 1 to 3 (+, −) |
| 22 | Current loop output 4 + |
| 24, 25 | 10 V AC/DC input |
| 4 to 6, 12, 13, 20 | Not used |

The same drawing lists what each output carries: "CL OUT 1/2/3: scoreboard output" and **"CL OUT 4: data stream"**. J1 to J3 are the ¼" jacks for current-loop outputs 1 to 3 (tip +, ring −, shaft ground).

**The RTD comes out both ways.** It is RS-232 on pin 3 (console transmit) with ground on pin 7, and a current-loop copy on pins 22 (+) and 7 (−):

- Daktronics [KB DD2180294](https://www.daktronics.com/en-us/support/kb/DD2180294): "With only pin 3 connected to All Sport, All Sport data is shown in DSTI." The fix for that case was a ground wire on pin 7.
- Daktronics [KB DD3090951](https://www.daktronics.com/en-us/support/kb/DD3090951): garbled data came from "one of the scoreboard outputs (pins 14-19) instead of the RTD CL output". The fix: "Move the feed going to the signal converter to pins 22 (+) and 7 (-)". That is current-loop output 4, which the drawing calls the data stream.
- ED-11976's settings: "RTD port settings are 19200 Baud, 8 data bits, and no parity" (Event Counter section). Pitch and Speed uses 1200 baud on the same port.

**Cabling.** Daktronics' bootload procedure inside ED-11976 (ED-18728) connects a PC to J6 "using a 9-pin to 25-pin serial cable" (W-1249). It also mentions an optional **J5 "COMPUTER PORT"** with a standard 9-pin cable. That fits the scoredata repo holding both `raw-J5-output.txt` and `raw-J6-output.txt`. J6 marks pin 2 as receive and pin 3 as transmit, which is how a DCE device is wired. So a PC needs a straight cable, and a DCE device such as a Lantronix box needs a null modem, which matches [KB 000027621](https://daktronics.com/en-us/support/kb/000027621) (inferred). A J-box in the path can block the RS-232 lines ([KB DD2127458](https://www.daktronics.com/en-us/support/kb/DD2127458)).

**For Hallstream:** a club with a USB-to-RS-232 adapter reads J6 pins 3 and 7 at 19200 8N1. Pins 22 and 7 need a current-loop to RS-232 converter first. The ¼" J1 to J3 jacks carry the scoreboard current loop (MDP), which is a different protocol from RTD. [mikedahlgren/daktronics-vb-scoreboard `docs/protocol.md`](https://github.com/mikedahlgren/daktronics-vb-scoreboard/blob/main/docs/protocol.md) calls their public documentation "thin".
