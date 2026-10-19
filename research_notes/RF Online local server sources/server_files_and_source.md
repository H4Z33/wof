# RF Online Server Files, Server Source Code and Emulators (state as of Oct 2026)

Method note: GitHub and several community sites were fetched directly. RaGEZONE (forum.ragezone.com) and rf-cheats.ru could not be fetched from this network (RaGEZONE returned a "Web Filter Violation" page, and rf-cheats.ru had a TLS certificate error). rfdev.net returned 403. Everything cited from those three sites comes from search-engine snippets and summaries, so it is lower confidence. No files were downloaded.

Legal note: Everything under "official" server files (AccountServer, LoginServer, BillingServer, ZoneServer binaries and DBs) is leaked proprietary CCR software. Decompiled code made from it is a derivative of copyrighted code. The GoMMO emulator says it was written from scratch, but it still needs an official client, which is also proprietary. Running any of these publicly can infringe the rights holders' copyright. RF official service ended in most regions in 2023–2025 — [search summary citing industry articles](https://www.gameindustry.ph/article/rf-online).

## 1. Which server-file versions circulate, and which are most stable/complete?

### Takeaway
The 2.2.3.2 ("2232", Golden Age / GU) leak is the de facto standard. It is the most documented, most patched and most tooled build, and almost every guide, hook, GameCP and decompiled-source project targets it. Other versions turn up in smaller numbers: older 2.1.5, 2.2.1, 2.1.6, 1.5, EP2 and Giga 4 Ep2, and newer 4.15 Age of Patron (AoP) and 4.55 Dragonborn. For AoP and Dragonborn, the main working option in 2026 is the GoMMO from-scratch emulator rather than leaked binaries. I found no public release of standalone Giga 5, Giga 6 or Giga 7 server files.

### Cited Findings
- RaGEZONE "RF Releases" lists server file packs for 2.2.3, EP2, 2.2.1, 1.5 and 2.1.6, plus Giga 4 Episode 2. 2.2.3.2 GU is the central build. — [RaGEZONE RF Releases (search summary)](https://forum.ragezone.com/community/rf-releases.479/)
- RaGEZONE has an "Official – RF Online 2.2.3.2 Server Files" entry in its download section, described by its uploader (Ron) as "clean from my archives". — [RaGEZONE download 65 (search summary)](https://forum.ragezone.com/downloads/rf-online-2-2-3-2-server-files.65/); there is also a thread at [RaGEZONE 1228966](https://forum.ragezone.com/threads/rf-online-2-2-3-2-server-files.1228966/)
- There is a "[Reupload] RF 2232 Server files" thread whose poster says the files are unmodified. Another post notes the older 2232 thread has partly dead links. — [RaGEZONE 880351 (search summary)](https://forum.ragezone.com/threads/reupload-rf-2232-server-files.880351/)
- 2.1.5 server and client files (slightly modified) are on RaGEZONE. The 2.2.3.2 and 2.1.5 guides need 64-bit Windows and MSSQL 2005/2008 Express. — [RaGEZONE 1244500](https://forum.ragezone.com/threads/rf-online-2-1-5-server-files-slightly-modified-client-slightly-modified.1244500/) (search summary)
- RFDev hosts "[2.2.3.2] Default Server and Client Files", described as default files suitable for learning. RFDev says files in its server-files section are "NOT supported by RFDev". — [RFDev topic 6](https://rfdev.net/topic/6-2232-default-server-and-client-files/), [RFDev forum 10](https://rfdev.net/forum/10-server-client-files/) (search summaries; direct fetch got 403)
- "RF Online Server Files 2.2.3.2 Golden Age" on rf-dev.net says it includes "many fixes" not in the original post. The domain no longer resolves (ENOTFOUND, Oct 2026). — [rf-dev.net topic 154 (dead)](https://rf-dev.net/topic/154-rf-online-server-files-version-2232-golden-age/)
- An older community release of "RF Online Server Files 2.2.3.X GU" exists, plus a 2016 setup guide — [prioritymmo thread 46](https://prioritymmo.boards.net/thread/46/release-rf-online-server-files), [prioritymmo thread 55](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- A GitHub gist ("RF History") lists a "2.2.3.2 Server Files Complete" pack credited to Fyurith and repacked by Fajri, with MSSQL query, launcher, client installer and a localhost "emulator" set. It links to third-party file hosts. — [gist codexmark/RF History](https://gist.github.com/codexmark/d4d845257aebcb4fb70f3617a9195d27)
- 4.15 Age of Patron leaked server files: RaGEZONE threads "AOP 4.15 server files" and "Age Of Patron 4.1.5 Server Files With Fix Bug Time". In one thread a user reported the accountserver flagged 3/67 on VirusTotal. Users report reusing the 2232 RF_USER/RF_WORLD DBs, building a separate BILLING DB for AoP, and using an un-updated PlayPark 4.15 client. — [RaGEZONE 1201977](https://forum.ragezone.com/threads/aop-4-15-server-files.1201977/), [RaGEZONE 1258346](https://forum.ragezone.com/threads/age-of-patron-4-1-5-server-files-with-fix-bug-time.1258346/page-2) (search summaries)
- A "RF online 4.55 Files?" request thread exists. 4.55 is now served by GoMMO (see section 3). — [RaGEZONE 1234049](https://forum.ragezone.com/threads/rf-online-4-55-files.1234049/)
- Russian archive rf-cheats.ru has a "Готовые сборки RF Online серверов" (ready-made server builds) section, mostly 2.2.3.2 GU repacks. — [rf-cheats.ru f-566 (search summary)](https://www.rf-cheats.ru/forum/archive/index.php/f-566.html); there is also a Russian 2.2.3 setup guide — [zone-game.info 4129](https://forum.zone-game.info/showthread.php?tid=4129)
- Live private servers labelled "Novus" (e.g. RF NOVUS, opened 2025) are based on Age of Patron. "Novus" is a server name, not a separate server-file version. — [bestgames RF NOVUS](https://rf.bestgames.to/project/242)

### Inferences
- For a local server, 2.2.3.2 is the most practical leaked-binary choice: it has the most guides, fixes, the Yorozuya hook, GameCPs, decompiled source and a from-scratch emulator. 4.15 AoP leaked files exist but carry malware flags and DB mismatches.
- "Giga 4/5/6/7" names in the request map to official update naming. 2.2.3.x is roughly the Giga 6/7 / Golden Age era in community usage, but I could not confirm the exact mapping from sources.

### Gaps
- I found no source for public Giga 5, 5.1, 6 or 7 or 2.4.x server-file releases distinct from 2.2.3.x. The search returned nothing.
- I could not read thread first posts on RaGEZONE (blocked), so link liveness and the contents of each pack are unverified.

## 2. Is there any server source code (leaked or decompiled)?

### Takeaway
Yes, in three forms:
- An old (around 2004-era) "Client + Server + Engine" source leak that reportedly does not compile.
- 2025-era mass Hex-Rays-style decompilations of the 2232 ZoneServer, LoginServer and AccountServer (around 43,877 .cpp files) and of the 4.15 ZoneServer (around 37,745 .c files). These are pseudocode that needs heavy refactoring before it can be rebuilt.
- Ongoing hand reconstruction efforts, including a "Native C++ 2.2.3.2 server" (Windows/Linux, MSSQL) in beta.

All are distributed on RaGEZONE/RFDev, often behind login and password-protected archives. None is on GitHub.

### Cited Findings
- "[Release] 2232 Zoneserver+LoginServer+AccountServer decompiled source code / C++ Source file" by DevtopZ: all pseudocode decompiled from the 2232 zoneserver, about 43,877 .cpp files, with Account/Login to follow. It is offered "as a base". — [RaGEZONE 1253046](https://forum.ragezone.com/threads/release-2232-zoneserver-loginserver-accountserver-decompiled-source-code.1253046/) (search summary)
- "415 Zoneserver C++ Source file" by wafotopz: 37,745 .c files. The author warns you must refactor all files before rebuilding the binary. — [RaGEZONE 1253062](https://forum.ragezone.com/threads/415-zoneserver-c-source-file.1253062/) (search summary)
- "Inside RF Online's ZoneServer: Startup and Entering the World" by Diantro, roughly mid-2026: reconstructing the source of ZoneServerUD_x64.exe (build 2232), with a write-up on startup and packet handling. — [RaGEZONE 1269875](https://forum.ragezone.com/threads/inside-rf-onlines-zoneserver-startup-and-entering-the-world.1269875/) (search summary)
- Old leak: "[Release] RF Online Sourcecode (Client + Server + Engine)" by KillerStefan, plus a reupload. Commenters say it is not compilable, and that a "2.2.1" upload was mislabeled as a different build. — [RaGEZONE 615314](https://forum.ragezone.com/threads/release-rf-online-sourcecode-client-server-engine.615314/), [RaGEZONE 1056553](https://forum.ragezone.com/threads/release-reupload-rf-online-sourcecode-client-server-engine.1056553/) (search summaries); a related discussion is at [RaGEZONE 1253727](https://forum.ragezone.com/threads/rf-online-source-code.1253727/)
- "[Linux/Windows] Native C++ RF online Server 2.2.3.2 – Active Development" by rezpounz: Version 1.0.0 (Beta), Windows/Linux, MSSQL 2025. At indexing time it had 0 replies and 192 views, posted around Sep 2026. The posting dates seen on RaGEZONE and RFDev differ slightly. — [RFDev forum listing](https://rfdev.net/forum/10-server-client-files/) (search summary)
- "[Help Wanted] Developing a Remastered RF Online Server in C++" by Caos is a recruiting post, not a release. — [RFDev topic 464](https://rfdev.net/topic/464-help-wanted-developing-a-remastered-rf-online-server-in-c/), [RaGEZONE 1241995](https://forum.ragezone.com/threads/help-wanted-developing-a-remastered-rf-online-server-in-c-rfonline.1241995/)
- Russian rf-cheats.ru archive has a thread titled "[Server 5/6/223/2232] Full decompilation", plus threads about fully decrypted client and server sources. — [rf-cheats.ru f-463 p3 (search summary)](https://www.rf-cheats.ru/forum/archive/index.php/f-463-p-3.html)
- A RaGEZONE thread mentions a ZoneServer PDB (debug symbols) posted behind registration. — [RaGEZONE search summary](https://forum.ragezone.com/threads/2025-rf-server-set-up.1242748/page-2)
- GitHub searches for "ZoneServerUD_x64", "rf online zoneserver" and "yorozuya rf" return 0 repositories, apart from the Yorozuya repo found by searching "rfonline". — [GitHub search ZoneServerUD_x64](https://github.com/search?q=ZoneServerUD_x64&type=repositories), [GitHub search rf online zoneserver](https://github.com/search?q=rf+online+zoneserver&type=repositories)

### Inferences
- Build environment: the decompilations are IDA/Hex-Rays pseudocode of x64 MSVC binaries, so any rebuild would be Visual Studio x64 on Windows. Yorozuya, built against the same binary, uses VS2017 x64. The native C++ rebuild also claims Linux.
- None of the decompiled dumps is a ready-to-compile project. They are reference material for hooks and reimplementation.

### Gaps
- I could not verify file counts, buildability, licences or download liveness, because RaGEZONE and rf-cheats were not reachable from this network.

## 3. Open-source or public emulators and reimplementations

### Takeaway
GitHub has almost nothing server-side for RF Online. The repos that exist are bots, GameCPs, map tools and DB editors.

The one serious, actively maintained reimplementation in 2026 is **GoMMO by Old School Ops (OSO)**. It is a closed-source (no public repo), from-scratch Go server framework with free binary bundles for many RF versions (2232 Golden Age v0.8.0, 4.55 Dragonborn v0.4.0, 4.15 AoP v0.2.0, RF 1.5, Giga 4 Part 2, Secret of Holystone, Red Army). It runs on Windows and Linux with SQLite or MySQL.

### Cited Findings
- **GoMMO (Old School Ops)**:
  - Servers "written from scratch on one Go framework" that match the original client protocol, with Lua scripting. About 2.1M lines of Go in 8,010 files. Development started Jan 2026.
  - Runs on Windows and Linux x64, with SQLite by default and MySQL for production.
  - No GitHub repo or licence is listed. There are public servers offline, and the latest release is GoMMO Studio 1.3.0 on 15 Oct 2026.
  - Source: [oldschoolops.com](https://oldschoolops.com/)
- GoMMO RF bundles:

  | Bundle | Version | Status | Size |
  |---|---|---|---|
  | Golden Age | 0.8.0 | Primary | 94 MB |
  | Dragonborn | 0.4.0 | Primary | 107 MB |
  | RF 1.5 | 0.3.1 | Primary | 188 MB |
  | Age of Patron | 0.2.0 | Maintained | 102 MB |
  | Giga 4 Part 2 | 0.3.9 | Maintained | 50 MB |
  | Secret of Holystone | 0.1.6 | Maintained | 65 MB |
  | Red Army | 0.1.9 | Maintained | 62 MB |

  Several bundles need the GoMMO Client Patcher, which removes the GameGuard check, the built-in expiry and US-territory GameGuard from an official client. — [OSO downloads](https://oldschoolops.com/downloads/), [OSO game clients doc](https://oldschoolops.com/docs/games/rf/game-clients/), [RaGEZONE 4.55 Dragonborn GoMMO thread 1273919](https://forum.ragezone.com/threads/rf-online-4-55-dragonborn-server-files-gommo-emulator-by-oso-linux-windows.1273919/) (search summary; recommends Ubuntu 24 LTS)
- **mvolotov/RF-OSO-Docker**: Docker Compose wrapper for the GoMMO rf2232 bundle (v0.7.0 example).
  - Uses SQLite, or MySQL 8.4 with DBs gommo_accounts, gommo_rf2232 and gamedata_rf2232. About 2.7M rows of game data are imported on first start.
  - Ports: 27800 (game), 27801 (launcher) and 8080 (web/control panel).
  - 1 star, 2 commits.
  - Source: [GitHub](https://github.com/mvolotov/RF-OSO-Docker)
- GitHub topic pages "rf-online" and "rfonline" contain only client-side or tooling repos: RFAutoLoot (C#, 5 stars), bot-autoloot-rf (Java, 6 stars), RF-Memory-Scanner (Python), RFMapToolMain (C#, updated Sep 2026), rfemblemeditor, rfsentinel, and a Bellato–Cora client mod. — [topics/rf-online](https://github.com/topics/rf-online), [topics/rfonline](https://github.com/topics/rfonline)
- Server-adjacent GitHub repos:
  - webdes27/rf-gamecp: Java GameCP for 2.2.3.2, 2 stars, 2020.
  - nunavx/RFOnline-GCP: PHP GCP for 2.2.3/2.2.3.2, 3 stars, 2018.
  - naupaw/RFweb-scripts: PHP status and registration scripts, 2013.
  - dct-leni/RF-SErverRate: Pascal server-rate editor, 2017.
  - josuerf/rfo-serverdateditor: Java editor for 2232 server .dat files, 2023.
  - manuka21/rfonline-strs: client/server structs, 2018.
  - xetrin/rf-ultimatum: "Free RF Online game server", 2015, no language shown, not verified.
  - DevTopz/rfonline-setup: setup guide, Nov 2025. DevTopz is likely the same handle as the 2232 decompile releaser, but this is unconfirmed.
  - lh-dd/RFOnline: EDF encrypt/decrypt, patch tools, rflauncher_server, 25 commits.
  - nunavx/rfonline-dllblocker: Pascal.
  - Sources: [GitHub search "rf online server"](https://github.com/search?q=rf+online+server&type=repositories), [GitHub search "rfonline"](https://github.com/search?q=rfonline&type=repositories), [lh-dd/RFOnline](https://github.com/lh-dd/RFOnline)

### Inferences
- There is no community open-source emulator comparable to rAthena for RF Online. GoMMO is free to use but not open source as far as I can tell.

### Gaps
- GoMMO's licence and terms, and how complete each version is in gameplay terms, are not documented on the pages I fetched.
- I could not check GitLab, and search engines did not index it for these terms.

## 4. Server-side DLL hook/extension projects

### Takeaway
The best-known public, open-source hook is **Yorozuya** (goodwinxp). It is an MIT-licensed C++ DLL/addon framework for the 2.2.3.2 ZoneServer that fixes around 40 exploits, built with VS2017 x64. It was archived in Feb 2021. Other protection layers are commercial or closed: FireGuard (bundled with the 2232 files), CrespoGuard, and paid Russian protection sources. I found no public trace of "NexusPro" or "RFGuard" as RF server hooks.

### Cited Findings
- **goodwinxp/Yorozuya**: "Free defense for RFOnline server version 2.2.3.2".
  - Modules: Yorozuya, YorozuyaGS, YorozuyaGSLib, Addons, ClientRun, Common, plus a server addon zip and client patch list.
  - Around 40 fixes: dupes (items, money, quests, mail, auction, trade), damage and potion overflow, speed/fly/wall/teleport hacks, siege/guild war/PvP controls, SQL injection, crash dumps.
  - MIT licence, VS2017 15.5.2+ x64, 39 stars, 48 forks, 346 commits. Archived (read-only) Feb 2021; the GitHub search listing shows the last update as Feb 2018.
  - Source: [GitHub](https://github.com/goodwinxp/Yorozuya)
- FireGuard anti-cheat components (fg.ini, fgrs.ini; ports 15444, 15445 and 80) are part of the standard 2.2.3.2 ZoneServer setup. — [prioritymmo guide](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- **CrespoGuard Community Edition**: documentation repo "for RF Online server owners", with 41 commits and a 2026-09 relay-gamecp archive folder. The page does not state supported versions or features. — [GitHub](https://github.com/CrespoCyberConsultingLLC/crespoguard-docs)
- A paid protection source for RF Online 2.2.3/2.2.3.2 was advertised for sale on zone-game.info. — [zone-game.info 20664](https://forum.zone-game.info/showthread.php?tid=20664) (search summary)

### Gaps
- "NexusPro" and "RFGuard": several searches found no reference to these as RF Online server hooks. They may be private or Discord-only products, or misremembered names.

## 5. Architecture (classic leaked 2.2.3.2 stack, and GoMMO for contrast)

### Takeaway
The leaked 2232 stack runs on Windows x64 with MSSQL (2005/2008 Express commonly) and ODBC DSNs. It has four main processes, started in this order: AccountServerSD.exe, then BillingServer.exe, then LoginServer.exe (/open), then ZoneServer (ZoneServerUD_x64.exe / ZoneServer64bit.exe). They use three DBs (BILLING, RF_User, RF_World) configured through INI files.

The ZoneServer gets its DB location from the AccountServer over the network. GoMMO collapses this into a single Go gameserver plus a webserver.

### Cited Findings
- Executables and start order: AccountServerSD, then BillingServer, then LoginServer (/open), then ZoneServer64bit. The stack needs Windows 64-bit, 4 GB+ RAM and MSSQL 2008 Express 64-bit (2005 is also mentioned). — [prioritymmo guide (2016)](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- Databases: BILLING, RF_User and RF_World, restored from .bak, with per-DB logins hardcoded in AccountServer and ZoneServer. There are three ODBC User DSNs on SQL Server Native Client 10.0, and MSSQL listens on 1433. — [prioritymmo guide](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- Config files:
  - AccountSystem.ini, BillingSystem.ini, LoginSystem.ini and Login_Filter_Dev.ini (the GM list)
  - rfacc.ini (shared by Account/Login and Zone)
  - WorldInfo.ini (ServerType must be 0; HBK and History folders)
  - WorldSystem.ini and ServerRate.ini
  - NationCode=RU
  - Source: [prioritymmo guide](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- Other guides list the start sequence as AccountServerSD.exe, LoginServerSD.exe and ZoneServerUD_x64.exe. — [rfworld forumotion t85](https://rfworld.forumotion.com/t85-rf-offline-2-2-3-2-gu-server-files-full-tutorial) (search summary)
- Some guides name four DBs (exgame, rf_account, rf_world, rf_web) and list ZoneServer, launcher and webserver. For internet hosting you set GateIP=<public IP> in the zoneserver config, and the public IP also goes in the launcher and login server. — [RaGEZONE search summaries](https://forum.ragezone.com/threads/how-to-make-ur-rf-server-online.871306/)
- ZoneServer internals, from the reverse-engineering write-up:
  - The zone stores only the DB login and password, and its port is hardcoded in the binary.
  - The AccountServer sends it the DB name and address over the network before it connects.
  - All character data is saved at logout in one go.
  - Fixed slots: about 2,532 players, 30,000 monsters and 5,064 loot boxes.
  - Source: [RaGEZONE 1269875](https://forum.ragezone.com/threads/inside-rf-onlines-zoneserver-startup-and-entering-the-world.1269875/) (search summary)
- GoMMO rf2232 architecture: a single gameserver (ports 27800 for game and 27801 for launcher; live stats on 29913, loopback only) plus a webserver/control panel (8080). It uses SQLite or MySQL, and there are no MSSQL or ODBC requirements. — [RF-OSO-Docker](https://github.com/mvolotov/RF-OSO-Docker)

### Inferences
- The guides disagree on DB names (BILLING/RF_User/RF_World versus rf_account/rf_world/exgame/rf_web) and on executable names (ZoneServer64bit versus ZoneServerUD_x64; LoginServer versus LoginServerSD). These likely reflect different repacks of the same 2232 leak.

### Gaps
- I could not confirm whether a separate "BotServer" process exists in the 2232 stack. No source mentioned one.
- I could not confirm default inter-server ports (Account to Login to Zone) from accessible sources.
