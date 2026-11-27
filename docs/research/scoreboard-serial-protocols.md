# Scoreboard console serial protocols

Research for a serial (RS-232) console source. It covers three console families: Daktronics All Sport, Fair-Play (Trans-Lux) MP-70/MP-80, and Bodet Sport Scorepad / BT6000.

Checked on 2026-11-27. I read each manual as a PDF where I could get one, read the source of each open-source parser, and checked the checksum rules against real captured bytes. **Inferred** marks anything I worked out rather than read. **Not verified** marks anything I could not read from the source itself that day.

## Verdicts

| Console | Verdict | Source of the byte layout |
|---|---|---|
| Daktronics All Sport 5000 | **PUBLISHED** (fields), reverse-engineered (framing) | Daktronics ED-12483 for field positions. Framing and checksum come from four open-source parsers and a real capture, and every captured frame checks out. |
| Daktronics All Sport 3000 / 1600 | **NOT FOUND** | No RTD document for these models turned up. |
| Fair-Play MP-70 / MP-80 | **PARTIAL**, weak | Fair-Play publishes no RS-232 byte layout. One GPL-3.0 project has a hockey-only guess. |
| Bodet Scorepad | **PUBLISHED** | Bodet manual 608264 "Network output and protocols". It gives framing, checksum and byte tables. |
| Bodet BT6000 | **NOT FOUND** | No serial output spec for the older BT6000 console turned up. |

## 1. Daktronics All Sport 5000

### Documents

- **ED-12483, "All Sport 5000 Series Enhanced RTD Reference Manual", Rev 2, 26 November 2003**, product 1196, 79 pages. Daktronics does not appear to host it. Copies sit in [fimion/pydak](https://github.com/fimion/pydak) and [JoshuaCarroll/scoredata](https://github.com/JoshuaCarroll/scoredata), both as `DaktronicsAllSport5000-RTD-Reference.pdf`, and a third-party copy is on [manualsdir.com](https://www.manualsdir.com/manuals/763166/daktronics-all-sport-5000-series-enhanced-rtd.html). The PDF's internal title is `ED-12483_REV2.doc`.
- The manual lists field positions for 20 sports: auto racing, baseball, basketball, cricket, event counter, football, hockey/lacrosse, judo, karate, lane timer, pitch and speed, rodeo, soccer, strike out count, taekwondo, tennis, track, volleyball, water polo and wrestling. **It has no handball and no futsal section.**
- The manual does not describe the wire framing. It contains no SOH, STX, EOT, ETB, SYN or checksum text. Section 1.2 only says that RTD "is a string of ASCII data (8 bit > 20h) organized by position", and that a receiver uses a field by "referencing its position and length within the string".

### Serial settings

ED-12483 section 1.2 gives:

- "Enhanced RTD protocol"
- "Baud rate of 19,200"
- "Word format of 8 data bits, no parity, and 1 stop bit"

The exception is that one football code outputs at 9600 baud. Daktronics KB [000011346](https://www.daktronics.com/en-us/support/kb/000011346) gives the cause as "Using All Sport code 6604 which outputs at a baud of 9600 instead of 19200." KB [DD2093979](https://daktronics.com/en-us/support/kb/DD2093979) sets up a Venus 1500 RTD input at 19200 with no parity.

Wiring notes from Daktronics:

- The console's connector is 25-pin. A J-box can block the RS-232 data ([DD2127458](https://www.daktronics.com/en-us/support/kb/DD2127458)).
- When the feed comes from the scoreboard outputs (pins 14 to 19) instead of the RTD output, the data comes out garbled. The fix is to "Move the feed going to the signal converter to pins 22 (+) and 7 (-)" ([DD3090951](https://www.daktronics.com/en-us/support/kb/DD3090951)). That article calls the output "RTD CL", which suggests a current-loop output (inferred).
- A direct console-to-Lantronix connection needs a null modem cable ([000027621](https://daktronics.com/en-us/support/kb/000027621)).

### Framing (reverse-engineered)

Four independent parsers agree on the framing, and so does a raw capture.

Each frame on the wire:

```
0x16                SYN, sent as "sync idle" between frames
"00000000"          8 ASCII digits, meaning unknown, ignored by every parser
0x01                SOH
"004210" + "NNNN"   ASCII header: a fixed prefix, then a 4-digit decimal offset
0x02                STX (missing on some non-data frames)
data                ASCII bytes, written into the RTD buffer at the offset
0x04                EOT
"CC"                checksum, 2 ASCII uppercase hex digits
0x17                ETB, end of frame
```

A real frame from [scoredata `raw-J5-output.txt`](https://github.com/JoshuaCarroll/scoredata):

```
\x16 00000000 \x01 0042100000 \x02 16:0916:09   16:0916:09    s    \x04 49 \x17
```

- **Offset.** The last four header digits are a **zero-based** offset into the RTD buffer. The manual's "Item" column is a **one-based** position. So a frame with offset `0111` starts at manual position 112, which is Guest Team Score. [scorebox-consoles](https://github.com/FlantasticDan/scorebox-consoles) (`consoles/daktronics.py`) adds 1 to the offset. The Rust crate writes `data[offset..]` and reads a field at `position - 1` (`src/rtd_state.rs`).
- **Frames are partial updates.** The console sends only the part of the buffer that changed. The Rust crate notes that "the console will not dump the initial state when your program initially reads from the serial. You can press the `STOP` button for this to happen." ([`src/packet.rs`](https://github.com/zabackary/daktronics-allsport-5000-rs/blob/main/src/packet.rs)).
- **Other frame types.** The capture holds one frame with header `003010006`, no STX and no data. The Rust crate rejects any header not starting `004210` ("I can't really work out the other packets"). [sport-streamer](https://github.com/JimThatcher/sport-streamer) (`DakAccess.cs`) accepts headers starting `00421` with length 10.
- **Checksum.** Add every byte from the first byte after SYN up to and including EOT. Keep the low 8 bits and print them as two uppercase hex digits. The Rust crate computes it this way, and so does sport-streamer, which also starts summing right after SYN. I ran that rule over all 247 frames in `raw-J5-output.txt` and it matched every one.

### RTD buffer: shared header (all sports)

From ED-12483. "Item" is the one-based position in the buffer. "Future" means Daktronics reserved the field.

| Field | Description | Item | Length | Justify | Comment |
|---|---|---|---|---|---|
| 1 | Main Clock Time (mm:ss/ss.t) | 1 | 5 | L | |
| 2 | Main Clock Time (mm:ss.t) | 6 | 8 | L | |
| 3 | Main Clock/Time Out/TOD (mm:ss/ss.t) | 14 | 5 | L | |
| 4 | Main Clock/Time Out/TOD (mm:ss.t) | 19 | 8 | L | |
| 5 | Main Clock =0 (' ' or 'z') | 27 | 1 | L | |
| 6 | Main Clock Stopped (' ' or 's') | 28 | 1 | L | |
| 7 | Main Clock/Time Out Horn (' ' or 'h') | 29 | 1 | L | |
| 8 | Main Clock Horn (' ' or 'h') | 30 | 1 | L | |
| 9 | Time Out Horn (' ' or 'h') | 31 | 1 | L | |
| 10 | Time Out Time (mm:ss) | 32 | 8 | L | |
| 11 | Time of Day (hh:mm:ss) | 40 | 8 | L | |
| 12 | Home Team Name | 48 | 20 | L | |
| 13 | Guest Team Name | 68 | 20 | L | |
| 14 | Home Team Abbreviation | 88 | 10 | L | Future |
| 15 | Guest Team Abbreviation | 98 | 10 | L | Future |
| 16 | Home Team Score | 108 | 4 | R | |
| 17 | Guest Team Score | 112 | 4 | R | |
| 18 | Home Time Outs Left - Full | 116 | 2 | R | |
| 19 | Home Time Outs Left - Partial | 118 | 2 | R | |
| 20 | Home Time Outs Left - Television | 120 | 2 | R | Future |
| 21 | Home Time Outs Left - Total | 122 | 2 | R | |
| 22 | Guest Time Outs Left - Full | 124 | 2 | R | |
| 23 | Guest Time Outs Left - Partial | 126 | 2 | R | |
| 24 | Guest Time Outs Left - Television | 128 | 2 | R | Future |
| 25 | Guest Time Outs Left - Total | 130 | 2 | R | |
| 26 | Home Time Out Indicator (' ' or '<') | 132 | 1 | L | |
| 27 | Home Time Out Text (' ' or 'TIME') | 133 | 4 | L | |
| 28 | Guest Time Out Indicator (' ' or '>') | 137 | 1 | L | |
| 29 | Guest Time Out Text (' ' or 'TIME') | 138 | 4 | L | |
| 30 | Period (basketball) / Game (volleyball) / Half (soccer) | 142 | 2 | R | |
| 31 | Period Text ('1st ', 'OT ', 'OT/2') | 144 | 4 | L | Future |
| 32 | Period Description ('End of 1st') | 148 | 12 | L | Future |
| 33 | Internal Relay (' ' or 'z', 's', 'h') | 160 | 1 | L | |
| 34 | Ad Panel / Caption Power ('c') | 161 | 1 | L | |
| 35 | Ad Panel / Caption #1 (' ' or 'c') | 162 | 1 | L | |
| 36 | Ad Panel / Caption #2 (' ' or 'c') | 163 | 1 | L | |
| 37 | Ad Panel / Caption #3 (' ' or 'c') | 164 | 1 | L | Future |
| 38 | Ad Panel / Caption #4 (' ' or 'c') | 165 | 1 | L | Future |
| 39 | Reserved for Future Use | 166 | 35 | R | |

Fields 1 to 39 match word for word across basketball, volleyball and soccer, apart from what field 30 is called.

**Sport code (not in the manual).** sport-streamer reads a 4-character sport code at zero-based buffer index 183 (positions 184 to 187, inside the "Reserved" field 39) to tell which sport the console runs. It maps `1101`, `1102`, `1103`, `1104`, `1105`, `1301`, `1401` and `1402` to basketball, and `7701`, `7711`, `7611`, `7601`, `7501` and `7604` to soccer. ED-12483 does not document this. It is one project's observation, not verified elsewhere.

### Basketball (ED-12483 section 4), fields after 39

| Field | Description | Item | Length | Justify | Comment |
|---|---|---|---|---|---|
| 40 | Shot Clock Time (mm:ss) | 201 | 8 | L | |
| 41 | Shot Clock Horn (' ' or 'h') | 209 | 1 | L | |
| 42 | Home Possession Indicator (' ' or '<') | 210 | 1 | L | |
| 43 | Home Possession Arrow (' ' or '<') | 211 | 1 | L | Future |
| 44 | Home Possession Text (' ' or 'POSS') | 212 | 4 | L | |
| 45 | Guest Possession Indicator (' ' or '>') | 216 | 1 | L | |
| 46 | Guest Possession Arrow (' ' or '>') | 217 | 1 | L | Future |
| 47 | Guest Possession Text (' ' or 'POSS') | 218 | 4 | L | |
| 48 | Home 1-on-1 Bonus Indicator (' ' or '<') | 222 | 1 | L | |
| 49 | Home 2-shot Bonus Indicator (' ' or '<') | 223 | 1 | L | |
| 50 | Home Bonus Text (' ' or 'BONUS') | 224 | 5 | L | |
| 51 | Guest 1-on-1 Bonus Indicator (' ' or '>') | 229 | 1 | L | |
| 52 | Guest 2-shot Bonus Indicator (' ' or '>') | 230 | 1 | L | |
| 53 | Guest Bonus Text (' ' or 'BONUS') | 231 | 5 | L | |
| 54 | Home Team Fouls | 236 | 2 | R | |
| 55 | Guest Team Fouls | 238 | 2 | R | |
| 56 | Home Player-Foul-Points ('nn-nn-nn') | 240 | 8 | L | |
| 57 | Guest Player-Foul-Points ('nn-nn-nn') | 248 | 8 | L | |
| 58 | Player-Foul ('nnn') | 256 | 3 | L | |
| 59 | Player-Foul-Points ('nnnnn') | 259 | 5 | N | |
| 60 to 68 | Home Score - Period 1 to 9 | 264, step 2 | 2 each | R | |
| 69 | Home Score - Current Period | 282 | 2 | R | |
| 70 to 78 | Guest Score - Period 1 to 9 | 284, step 2 | 2 each | R | |
| 79 | Guest Score - Current Period | 302 | 2 | R | |

From field 80 on, the manual lists in-game and roster player status, number, fouls and points per team (positions 304 to 474 home, 475 to 645 guest), plus "From DSTI Application Only" team stats. The full list is in the Rust crate's [`src/sports/basketball.rs`](https://github.com/zabackary/daktronics-allsport-5000-rs/blob/main/src/sports/basketball.rs) and its `sports_data/sports_data.xlsx`, both typed from ED-12483.

For an overlay, the basketball fields that matter are: clock (1 or 2), clock stopped (6), home and guest score (16, 17), period (30), timeouts left total (21, 25), team fouls (54, 55), shot clock (40), possession (42, 45) and bonus (48 to 53).

### Volleyball (ED-12483 section 19), fields after 39

| Field | Description | Item | Length | Justify | Comment |
|---|---|---|---|---|---|
| 40 | Home Serve Indicator (' ' or '<') | 201 | 1 | L | |
| 41 | Home Serve Arrow (' ' or '<') | 202 | 1 | L | Future |
| 42 | Home Serve Text (' ' or 'SERVE') | 203 | 5 | L | |
| 43 | Guest Serve Indicator (' ' or '>') | 208 | 1 | L | |
| 44 | Guest Serve Arrow (' ' or '>') | 209 | 1 | L | Future |
| 45 | Guest Serve Text (' ' or 'SERVE') | 210 | 5 | L | |
| 46 | Home Games Won | 215 | 2 | R | |
| 47 | Guest Games Won | 217 | 2 | R | |
| 48 | Match Number | 219 | 3 | R | |
| 49 to 57 | Home Score - Game 1 to 9 | 222, step 2 | 2 each | R | |
| 58 | Home Score - Current Game | 240 | 2 | R | |
| 59 to 67 | Guest Score - Game 1 to 9 | 242, step 2 | 2 each | R | |
| 68 | Guest Score - Current Game | 260 | 2 | R | |

Field 30 is "Game", the current set. Fields 69 on are player and team stats (aces, kills, blocks, digs), listed in [`src/sports/volleyball.rs`](https://github.com/zabackary/daktronics-allsport-5000-rs/blob/main/src/sports/volleyball.rs).

### Soccer (ED-12483 section 14)

Field 30 is "Half". Fields 40 to 63 are possession and per-period scores, all marked "Future". The live fields after 39 include:

| Field | Description | Item | Length | Justify |
|---|---|---|---|---|
| 68 | Home Shots On Goal - Total | 259 | 3 | R |
| 73 | Home Saves - Total | 270 | 3 | R |
| 78 | Home Corner Kicks - Total | 281 | 3 | R |
| 83 | Home Penalty - Total | 292 | 3 | R |
| 88 | Guest Shots On Goal - Total | 303 | 3 | R |
| 93 | Guest Saves - Total | 314 | 3 | R |
| 98 | Guest Corner Kicks - Total | 325 | 3 | R |
| 103 | Guest Penalty - Total | 336 | 3 | R |
| 118 | Home Fouls - Total | 369 | 3 | R |
| 123 | Guest Fouls - Total | 380 | 3 | R |

The full list is in [`src/sports/soccer.rs`](https://github.com/zabackary/daktronics-allsport-5000-rs/blob/main/src/sports/soccer.rs). Soccer has no shot clock field.

**Handball and futsal.** ED-12483 has no section for either. Which All Sport sport code a handball or futsal club would run, and so which layout applies, is unknown.

### All Sport 3000 and 1600

**NOT FOUND.** No RTD field document for these models turned up. The 1600 series console manual is ED-12462, which I found only as an unofficial [Scribd copy](https://www.scribd.com/document/443275193/Consola-estadio-ED12462-pdf) and did not read. A search summary says Daktronics' "Standard RTD Inputs" guide ([ED-17652](https://daktronics.com/web-documents/customer-service-manuals/ed-17652.pdf)) shows an All Sport input at 19200 baud, 8 data bits, no parity, 1 stop bit. I did not read ED-17652 (**not verified**). Whether the 3000 and 1600 use the same Enhanced RTD layout is unknown.

### Open-source implementations

| Project | Language | Licence | Notes |
|---|---|---|---|
| [zabackary/daktronics-allsport-5000-rs](https://github.com/zabackary/daktronics-allsport-5000-rs) (crate `daktronics-allsport-5000`) | Rust | MIT | The most complete. It validates the checksum, has every sport typed from ED-12483, and ships a spreadsheet of offsets "for those interested in porting". Its README notes "this protocol is technically proprietary". |
| [FlantasticDan/scorebox-consoles](https://github.com/FlantasticDan/scorebox-consoles) | Python | MIT | Daktronics and Colorado Time Systems. No checksum check. |
| [fimion/pydak](https://github.com/fimion/pydak) | Python | MIT | Football table only. Also reads RTD over UDP multicast. Holds a copy of ED-12483. |
| [JoshuaCarroll/scoredata](https://github.com/JoshuaCarroll/scoredata) | Python, C# | MIT | Football only. Holds the raw captures `raw-J5-output.txt` and `raw-J6-output.txt` and a copy of ED-12483. |
| [JimThatcher/sport-streamer](https://github.com/JimThatcher/sport-streamer) | C# | MIT (the web UI in `ClientApp/src` is LGPL-3.0, per its own `LICENSE.txt`) | Validates the checksum and detects the sport code. |

### Terms

ED-12483 carries a standard copyright page: "No part of this book covered by the copyrights hereon may be reproduced or copied in any form or by any means ... without written permission of the publisher." That restricts copying the manual. It says nothing about writing software that reads the console. Hallstream should not ship the PDF. Field positions as facts are a different matter from copying the document, but that is a legal call I cannot make here.

## 2. Fair-Play (Trans-Lux) MP-70 / MP-80

**Verdict: PARTIAL, and weak.** I found no published RS-232 byte layout from Fair-Play or Trans-Lux.

### What the manufacturer says

The MP-70/50 Series User Guide is document 98-0002-29, version 2303.03, firmware 3.09. I read the copy in [sworrl/SLAP `src/docs/MP-70_Manual.pdf`](https://github.com/sworrl/SLAP). Third-party copies are on [manualmachine.com](https://manualmachine.com/fairplay/mp53/23024077-user-manual) and [manuals.online](https://manuals.online/translux/controllers/fair-play-mp-70-series/user-manual/1083k).

- The football, hockey, soccer and volleyball chapters each carry a `VIDEO CHAR? Y/N` setting: "Enter YES to enable data out on the RS-232 connector for use with the VideoStamp+ character generator. Responding NO will default to ProLine data on the RS-232 connector for most sports."
- "MP-70 and MP-69 data outputs" describes two formats for the **scoreboard** connectors. "MP-69 is a data format developed in 1984", and MP-70 is "the current data format". The manual gives no byte layout for either, and none for the RS-232 "ProLine" or VideoStamp+ output.
- The only baud rate in the manual is for FinishLynx timing software sending **into** the MP-70: "In the Baud box, choose 9600", with the `Dak.lss` script. That is an input, not the output a source would read.

Integration guides confirm that people read the MP-70 serial port, but they give no protocol. [ProScoreboard](https://support.renewedvision.com/hc/en-us/articles/360011596914-How-to-connect-a-Fair-Play-MP-70-Controller-with-ProScoreboard) lists an FTDI adapter, a null modem cable and a female-to-female gender changer. [ScoreBird](https://scorebird.com/integrations/scoreboards/fairplay/mp-70) also uses the ¼" scoreboard ports. [BoxCast](https://support.boxcast.com/en/articles/4235162-capture-scoreboard-data-from-a-translux-fair-play-scoreboard) supports Fair-Play through its own box.

I found nothing on an "MP-80" model.

### Reverse-engineered: SLAP (hockey only)

[sworrl/SLAP](https://github.com/sworrl/SLAP), GPL-3.0, last commit 2026-08-09. It reads the MP-70 RS-232 port for ice hockey. Its `src/docs/PROTOCOL_NOTES.md` opens with the protocol being "not publicly documented by the manufacturer". It claims:

- 9600 baud "(standard) or 19200 (some firmware/models)", 8 data bits, no parity, 1 stop bit, no flow control.
- ProLine frames start with STX (0x02) and end with ETX (0x03). Byte 1 is a type: `'C'` (0x43) for the clock and `'H'` (0x48) for score and game state.
- A "standard 80-byte" `'H'` layout: home score at `[13:15]`, away score at `[29:31]`, period at `[45:46]`, four penalty times from `[52:56]` to `[67:71]`, with long "Unknown" gaps.
- A 55-byte `'H'` variant that its README calls "confirmed from real hardware".

Treat this as a lead, not a spec. The parser tries "5 parsing strategies" and "auto-detects unknown packet formats", the layouts differ between the README and the notes, and the project names no checksum. It covers hockey only, not our five sports. Because it is GPL-3.0, its code cannot be copied into Hallstream unless Hallstream's licence allows it.

The ¼" scoreboard wire uses a different, non-RS-232 signal. [will62794/MP-69D-Scoreboard-Decoder](https://github.com/will62794/MP-69D-Scoreboard-Decoder) decodes the older MP-69D signal with an Arduino, and SLAP's notes copy its BCD nibble positions for basketball (team fouls at nibbles 8 to 11, period 17, scores 18 to 21, clock 22 to 25, shot clock 32 to 33). That needs extra hardware and does not fit a plain USB-serial source.

### What is missing

Any Fair-Play document of the ProLine RS-232 format, a checksum rule, and layouts for basketball, volleyball, handball, futsal and soccer. Getting them means a capture from a real MP-70 in each sport, or asking Fair-Play support, whose number SLAP gives as (800) 462-2716.

## 3. Bodet Sport (France)

### Scorepad: PUBLISHED

**Document: Bodet "SCOREPAD Network output and protocols Manual", ref. 608264.** The French edition is 608263, "Notice Sortie Protocole Scorepad".

- The current English revision (608264 J) was at [static.bodet-sport.com/…/608264-Network output and protocols-Scorepad.pdf](https://static.bodet-sport.com/images/stories/EN/support/Pdfs/manuals/Scorepad/608264-Network%20output%20and%20protocols-Scorepad.pdf), linked from a [vMix forum thread](https://forums.vmix.com/posts/t24799-Bodet-scoreboard). On 2026-11-27 that host did not resolve in DNS. The [Wayback Machine copy](http://web.archive.org/web/20220810040530/https://static.bodet-sport.com/images/stories/EN/support/Pdfs/manuals/Scorepad/608264-Network%20output%20and%20protocols-Scorepad.pdf) is cut off at 1 MiB of 2,059,120 bytes. I could read its first 11 pages, including a contents list: badminton p15, basketball p18, basketball 3x3 p28, handball p33, ice hockey p37, floorball p41, football/rugby/beach soccer p45, futsal p46, tennis p47, table tennis p49, volleyball p51.
- **Revision A (608264A)**, 24 pages, made with InDesign on 2018-07-04, is complete in [tomkohler/Panel2Net](https://github.com/tomkohler/Panel2Net) as `608264A-Network output and protocols-Scorepad (1).pdf`. It covers basketball, handball and ice hockey only. Everything quoted below comes from revision A unless marked.
- A 56-page French edition with volleyball and futsal is on [ManualsLib (600740)](https://www.manualslib.fr/manual/600740/Bodet-Scorepad.html). ManualsLib refuses scripts (HTTP 403). I saw it only through a fetch tool that paraphrases, so its volleyball and futsal tables below are **not verified** byte for byte.

**Physical path.** The Scorepad sends the protocol out of its RJ45 port. A PC either receives it as a TCP server (default port 4001 in revision J), or a Moxa NPort RJ45-to-RS232 interface in TCP server mode turns it into serial. "The protocol output is only available on the Scorepad MAIN keyboard." The technician code that enables it is 4934 (revision A, page 3).

**Serial format** (revision A, page 4, quoted exactly, typos included):

> Thuis protocol transmit ASCII messages (8 bits) only.
> The format of the data is: 8 bits + 1 start bit + 1 stop bit + no parity.
> Data transmission baud rate: 9600 Bauds.
> The keyboad sends frame of type: « SOH address STX CTRL Message ETX LRC »
> SOH = 01 hexadecimal
> Address = 1 byte, to be ignored (attention : : useful to calculate the LRC).
> STX = 02 hexadecimal
> CTRL = 1 byte, to be ignored (attention : : useful to calculate the LRC).
> Message = several bytes (cf details)
> ETX = 03 hexadecimal
> LRC = 1 byte: exclusif OR of the bytes between SOH(excluded) and ETX(included)
> LRC = LRC and 0x7f
> IF (LRC < 32) THEN LRC = LRC + 32

In every sample frame in the manual the address is `0x7F` and CTRL is `0x47` ('G'). The first two message bytes are the message number in ASCII. The status-word bytes have bit 7 set (for example `0x80`), so the line carries 8-bit values, not 7-bit ASCII.

**LRC check against the manual's own traces.** Revision A prints 41 sample frames (pages 16 to 18). 35 of them match the rule as written. The 6 frames of message 50 (shot clock) match only when ETX is left out of the XOR, for example `01 7F 02 47 35 30 80 32 34 03 39`. A decoder should accept either form, since the source contradicts itself (inferred).

#### Basketball (sport code «5»)

**Message 18: game clock, period, timeouts**

| Byte | Content | Example |
|---|---|---|
| 1 | «1» (31H) | |
| 2 | «8» (38H) | |
| 3 | Status word | |
| 4 | «5» = Basketball | |
| 5 | Minutes * 10 | «1» |
| 6 | Minutes * 1 | «6» |
| 7 | Seconds * 10 | «5» |
| 8 | Seconds * 1 | «4», so 16:54 |
| 9 | Number of time out Home team | «1» 31H |
| 10 | Number of time out Guest team | «3» 33H |
| 11 | (blank) | |
| 12 | (blank) | |
| 13 | Period number or extra time | «2» 32H. "During extra time the transmitted character is 'E', otherwise it's the period number." |
| 14 | (blank) | |

In the last minute, bytes 5 to 8 become Seconds * 10, Seconds * 1, «D» (44H), Seconds * 0.1, so `56D4` is 56.4.

Status word for message 18: b1 = 1 clock OFF, 0 clock ON. b2 = 1 horn ON, 0 horn OFF. b4 = 0 possession time in seconds, 1 in tenths. b6 = 1 new match, 0 match in progress (page 7; page 8 repeats the b4 text for b6, a copy error). b7 = 1. b0, b3 and b5 are ignored.

**Message 36: tenths in the last minute.** «3» «6», then Seconds * 10, Seconds * 1, Seconds * 0.1.

**Message 50: shot clock**

| Byte | b4 = 0 | b4 = 1 |
|---|---|---|
| 1 | «5» (35H) | «5» (35H) |
| 2 | «0» (30H) | «0» (30H) |
| 3 | Status word | Status word |
| 4 | Seconds * 10 | Seconds * 1 |
| 5 | Seconds * 1 | Seconds * 0.1 |

Status word for message 50: b1 = 1 timer OFF. b2 = 1 horn ON. b3 = 1 "shot clock blanked", 0 shows it. b4 as above. b7 = 1.

**Message 30: scores**

| Byte | Content |
|---|---|
| 1 | «3» (33H) |
| 2 | «0» (30H) |
| 3 | «5» = Basket Ball |
| 4 to 6 | score Home, 3 characters |
| 7 to 9 | score Guest, 3 characters |

The manual's examples pad with spaces (20H) but do not agree on which side. Home 5 shows as `20 35 20`, home 12 as `20 31 32`, guest 18 as `31 38 20`, and 104 and 118 as three digits. Trim spaces and parse what is left (inferred).

**Message 31: team fouls and last personal foul**

| Byte | Content |
|---|---|
| 1 | «3» (33H) |
| 2 | «1» (31H) |
| 3 | «5» = Basket Ball |
| 4 | Ignore |
| 5 | Home team fouls (example «3») |
| 6 | Ignore |
| 7 | Guest team fouls (example «5») |
| 8 | player nbr * 10 |
| 9 | player nbr * 1 |
| 10 | number of fouls of the player |
| 11 | team of the player: (31H) Home, (32H) Guest |

"The total of the team fouls is limited to configured number." About 10 seconds after a foul, bytes 8 to 10 turn to 20H.

**Message 19: timeout countdown.** «1» «9» «5», then Home Time Out indicator, Guest Time Out indicator, Seconds * 10, Seconds * 1. While a timeout runs, the indicator for one timeout alternates between 0x30 and 0x31. When it stops, it is 0x30 + the number of timeouts.

**Other basketball messages in revision A:**

- 33 and 34 give personal fouls per player for home and guest players 4 to 15. Each byte is an 8-bit bitmap, from 0x80 for no fouls to 0xbf for six. The manual labels both "Message 33", but the second one's byte 2 is «4».
- 37 and 38 give the home and guest player numbers.
- 56 gives one player's points.
- 98 and 99 give the home and guest team names, 18 characters each. Message 99's byte 2 is printed as «8» (39H), a typo.
- 20 gives the local time, hhmm.

#### Handball (sport code «4»)

- **Message 01**: «0» «1», status word, «4», then Minutes * 10, Minutes * 1, Seconds * 10, Seconds * 1, timeouts home, timeouts guest, period number. The status word b2 is printed here as "= 1: horn OFF. = 0: horn ON.", the opposite of basketball.
- **Message 02**: «0» «2» «4», then score Home * 100, * 10, * 1 and score Guest * 100, * 10, * 1.
- **Message 03**: «0» «3» «4», then six penalty timers as m, s * 10, s * 1: three for home, then three for guest.
- **Message 04**: «0» «4» «4», then the timeout indicators and countdown, the same as basketball message 19.

#### Volleyball and futsal (revision J, French edition, **not verified**)

As paraphrased from ManualsLib pages 53, 54 and 47:

- **Volleyball message 06**: «0» «6», status word, «0» = Volleyball, current set, home sets, guest sets, home score tens and units, guest score tens and units, home timeouts, guest timeouts, timeout seconds tens and units. Status b3 = 1 means the home team serves, 0 the guest team.
- **Volleyball message 07**: «0» «7», status word, «0», then Minutes * 10, Minutes * 1, Seconds * 10, Seconds * 1.
- **Volleyball message 08**: present on page 55, content not seen.
- **Futsal message 45**: «4» «5», status word, minutes * 10 and * 1, seconds * 10 and * 1, tenths, home score (3 bytes), guest score (3 bytes), period ('E' in extra time), extra time number (20H or the number). Status b0 = 0 game clock, 1 break clock.

These need checking against the PDF itself before anyone builds on them.

### BT6000: NOT FOUND

The BT6000 manuals ([ManualsLib](https://www.manualslib.com/manual/1259335/Bodet-Bt6000.html), [manualmachine](https://manualmachine.com/bodet/bt6000/19671367-installation-and-use-manual)) and the [vMix BT6000 thread](https://forums.vmix.com/posts/t9438-Anyone-knows-how-to-import-BT6000-data-into-vmix) gave no output protocol. Revision A's sample traces come from a tool labelled "Soft Control RS - Version 1.1b du 16-11-93", so the frame format may be older than the Scorepad and may match the BT6000 (inferred, not verified).

Bodet also sells the BOXPRO box for TV and stats protocols such as EBStats ([BOXPRO manual on ManualsLib](https://www.manualslib.com/manual/3053856/Bodet-Boxpro.html)). I did not read its protocol notes.

### Open-source implementations

None decode Bodet. [Panel2Net](https://github.com/tomkohler/Panel2Net) only carries the revision A PDF. Its code handles Mobatime, SwissTiming and Stramatel, and the repo has no licence file (last commit 2019-03-31).

### Terms

Revision A has no copyright notice and no terms in its text. Bodet published it for integrators.

## What a Hallstream serial source needs, per console

- **Daktronics**: 19200 8N1 (9600 for football code 6604). Sync on 0x16 and frame to 0x17. Check the sum-of-bytes checksum. Write each frame's data into a buffer at the zero-based offset, and read fields at one-based position minus 1. Tables exist for basketball, volleyball and soccer. Handball and futsal have no table.
- **Bodet Scorepad**: 9600 8N1, over a Moxa NPort or straight TCP. Frame from 0x01 to 0x03 plus the LRC byte. Accept the LRC with or without ETX. Dispatch on the two-digit message number. Basketball and handball are published in full. Volleyball and futsal need the full revision J PDF.
- **Fair-Play**: nothing reliable to build on. Needs captures.
