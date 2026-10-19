# RF Online Game Client Availability and Client-Side Tooling for a Local Private Server

Research date: 2026-10-16. Nothing was downloaded. RageZone threads returned TLS certificate errors ("unable to verify the first certificate") and RFDev (rfdev.net) returned HTTP 403 to the fetch tool, so findings from those sites come from search-result summaries only and have not been read in full. Treat them as lower confidence.

## 1. Where can clean clients be obtained, and which versions match which server builds?

### Takeaway
Archive.org now holds several preservation uploads of original clients, from Giga 3 open beta through 2.2.3.2 (CCRPlay and PlayRF builds) to Episode 2 Codemasters. A client and a server must be exactly the same build, because versions are not interchangeable. 2.2.3.2 ("Golden Age" / Grand Update) is the community's de facto standard and has the most tools. The original game's servers shut down on 31 Dec 2025. RF Online Next (Netmarble, Unreal Engine 5) is a separate sequel and cannot be used with these servers.

### Cited Findings
**Archive.org client items** (all found through the archive.org advancedsearch API; sizes come from item metadata):
- `ccrplay-2232`, "RF Online CCRPLAY Client 2232". About 2.34 GB, published 2026-09-23. The description calls it the last official CCRPlay client and gives a SHA-256. — [archive.org/details/ccrplay-2232](https://archive.org/details/ccrplay-2232)
- `playrf-2232`, "RF Online PlayRF Client 2232". About 2.13 GB, published 2026-03-09. An original PlayRF client (about 2010): a setup exe plus split data parts, with checksums. — [archive.org/details/playrf-2232](https://archive.org/details/playrf-2232)
- `rfonline-ep2-installer`, "RF Online Episode 2 Client". The Codemasters client, about 1.59 GB, with RF_online.bin dated 2007-10-10. — [archive.org/details/rfonline-ep2-installer](https://archive.org/details/rfonline-ep2-installer)
- `rfonline_ob_103`, the official installer for "RFOnline Giga3 Rising Force Online Open Beta 1.0.3". About 1.63 GB (4Game/Russian OBT). — [archive.org/details/rfonline_ob_103](https://archive.org/details/rfonline_ob_103)
- `rf-online-giga-final-collection`, "RF Online Episode 1 – GIGA FINAL Collection". 6.3 GB 7z, published 2025-03-26. Its contents listing shows:
  - Codemasters `RFOnline.iso` base (about 1.34 GB) and `RFOnline_Giga_Final_Part1/Part2.7z`
  - SEGA `RF_SPDVD.iso` (about 1.94 GB)
  - 4Game Giga3 RU patch and OBT 1.0.3 installer
  - A `Just RF CCR2(127.0.0.1).exe` (a client exe apparently preset for localhost)
  - An SDK folder with `RF-Server2005.rar` (about 79.5 MB), `ServerGiga3.7z` (about 66.9 MB), an engine/client/server source archive (about 3.8 MB), Giga3 patch/string tools, and a README.txt

  — [archive.org/details/rf-online-giga-final-collection](https://archive.org/details/rf-online-giga-final-collection), [contents view](https://ia903209.us.archive.org/view_archive.php?archive=/32/items/rf-online-giga-final-collection/RF%20Online%20GIGA%20FINAL%20Collection.7z)
- `RISINGFORCE-BRDISC1/2/3`, "RF Online Disc 1–3 (Brazilian Level Up! release)", 2006, published by Level Up! Interactive. Disc 1 is about 748 MB. Uploaded for preservation with CD matrix/SID codes. — [archive.org/details/RISINGFORCE-BRDISC1](https://archive.org/details/RISINGFORCE-BRDISC1)
- Also on archive.org: a Codemasters "ArchLord / RF Online" DVD-ROM item, a `rf-2232-4x-textures.-7z` texture mod (2019), and `vanadium2_202304`, a UI mod (about 97 MB). — [archive.org search](https://archive.org/advancedsearch.php?q=%22RF+Online%22+OR+%22Rising+Force+Online%22&fl[]=identifier,title,date&rows=60&output=json)
- Softpedia lists an "RF Legends Client 2.2.3" download of about 1.77 GB. This is a third-party mirror, and the version is 2.2.3, not 2.2.3.2. — [Softpedia](https://games.softpedia.com/get/Online-Games-Clients/RF-Online.shtml)
- rflib.ru (Russian community) offers a "Clear client 2.2.3.2" (region not stated), plus tools and mods. — [rflib.ru/en/files](https://rflib.ru/en/files)
- RageZone hosts a re-upload thread, "[Re-Upload] RF ONLINE 2.2.3.2 CLIENT PlayRF" (not read; TLS error). — [RageZone](https://forum.ragezone.com/threads/re-upload-rf-online-2-2-3-2-client-playrf.907594/page-6)

**Version-to-server matching:**
- Old School Ops' client-matching table (an emulator project):

  | Server | Client build | How to identify it |
  |---|---|---|
  | Dragonborn (4.55) | RF Online Taiwan, built 2025-07-16 | `RF_Online.bin` = 8,430,808 bytes; CCR-signed, SHA-256 `46528aac…c558` |
  | 4.15 Age of Patron | RedFox Games final, built 2023-01-02 | `RF_Online.bin` = 8,685,880 bytes |
  | 2.2.3.2 Golden Age | Episode 2.2.3.2 retail | Version the client reports |
  | 2.1.6 Secret of Holystone | 2.1.6 retail | Version the client reports |
  | 2.1.5 Red Army | 2.1.5 retail | Version the client reports |
  | Giga 4 Part 2 | Giga 4 Part 2 retail | Version the client reports |
  | 1.5 | GamesCampus 1.5b, built 2013-03-13 | `RF_Online.bin` = 10,153,352 bytes |

  The page says builds are "not interchangeable", and two other "1.5" builds (Korean Jan 2012, English Nov 2012) are unsupported. For 4.15, the `RFOnline_Setup.exe` installer installs a 2018 build; you then run `RF.exe` once so it patches to the 2023 binary, and the final step errors because RedFox's patch servers are offline. — [Old School Ops: Game Clients](https://oldschoolops.com/docs/games/rf/game-clients/)
- Old School Ops describes Giga 4 Part 2 as the last version before the cash shop. — [Old School Ops Giga 4](https://oldschoolops.com/games/rf/giga4/)
- Version lineage from a RageZone version-history thread (search summary):
  - Giga 3
  - Giga 4
  - 2.1.5 (Episode 2 Part 1 Update 5)
  - 2.1.6 (Final Update of Part 1)
  - 2.2.1
  - 2.2.3
  - 2.2.3.2, called "probably the most stable and reliable version"

  — [RageZone: The Version History of RF Online](https://forum.ragezone.com/threads/the-version-history-of-rf-online.948192/)
- The RF wiki describes Age of Patron (4.15) as a major update applied to all versions after 2.2.3.2 ("Golden Mystery" in the wiki's naming), and Dragonborn (4.55) as the final update. — [RF Online Wiki patch notes](https://wiki.rfdatabase.net/patch-notes)
- Server-side tooling is concentrated on 2.2.3.2:
  - Yorozuya, "Defense for RFOnline server version 2.2.3.2" (archived)
  - RF Online GS Editor, "2.2.3.2 only"
  - QuestViewer for 2.2.3.2
  - GCP for 2.2.3/2.2.3.2

  — [goodwinxp/Yorozuya](https://github.com/goodwinxp/Yorozuya), [bump44/rf-online-gs-editor](https://github.com/bump44/rf-online-gs-editor), [TheKingScott/QuestViewer](https://github.com/TheKingScott/QuestViewer), [nunavx/RFOnline-GCP](https://github.com/nunavx/RFOnline-GCP)

**Official status and RF Online Next:**
- RF Online Next is the official sequel, built from scratch in Unreal Engine 5. It launched in South Korea, Japan and Taiwan in late September 2025, and globally on PC and mobile on 16 June 2026 (Netmarble press release). It is distributed through the Netmarble Launcher and Epic, and is free-to-play. The original game's servers were sunset on 31 Dec 2025 (per the same search summary). — [Gamespress press release](https://www.gamespress.com/RF-ONLINE-NEXT-TO-LAUNCH-GLOBALLY-ON-June-16), [RPG Site](https://www.rpgsite.net/news/20625-rf-online-next-releases-worldwide-for-pc-mobile-on-june-16), [MassivelyOP tag](https://massivelyop.com/tag/rf-online-next) (the MassivelyOP page could not be fetched directly because of a certificate error)

### Inferences
- The task brief's "launched 2025" is only partly right: Next launched in Asia in Sept 2025 and globally in June 2026. It is confirmed as a separate game with a separate engine, so its client is useless for classic private servers.
- For a local 2.2.3.2 server, the most verifiable clean sources are the archive.org `ccrplay-2232` and `playrf-2232` items, which come with published SHA-256 hashes. Giga-era server builds (the SDK in the GIGA FINAL collection) would need the matching Giga 3 / Giga Final Codemasters or SEGA clients in the same archive.
- The `Just RF CCR2(127.0.0.1).exe` in the GIGA FINAL collection suggests community-prepared localhost-patched executables exist. It is unverified and potentially unsafe, and should be inspected rather than trusted.

### Gaps
- No archive.org or other clean client was found for Philippines (Level Up! Games PH), Indonesia (Lyto), or Russia's later 2.x clients (4Game/Innova; "Gamenet/GoGames" was not confirmed in any source). The ID/PH/RU publisher names are not verified by sources in this session.
- No reliable source mapped "Giga 5/6/7" naming to version numbers. Sources use Giga 3 → Giga 4 → 2.1.5 (Episode 2), not Giga 5–7. "Giga 5/6/7" may be informal or regional naming.
- Whether archive.org items remain online long-term is unknown, given the takedown risk after the official shutdown.
- The sunset date (31 Dec 2025) comes from a search-result summary only; it should be checked against an official notice.

## 2. How is the client pointed at a local server, and which files must match the server?

### Takeaway
On classic clients, the login/server address is either baked into the executables (players hex-edit `RF_Online.bin` / `rflauncher.exe` on older 2.2.3.2 setups) or passed in by a launcher, which writes `System\DefaultSet.tmp` (an XOR-encoded binary holding the zone IP/port, account, session key, language code and flags) and then starts `RF_Online.bin`. Typical ports are 10001 for login and 27780 for zone. Client `.edf` data (Item/NDItem, Store, resource, etc.) must correspond to the server's `.dat`/script data.

### Cited Findings
- RageZone users say that to use their own IP they hex-edit `rf.exe` and `rflauncher.exe`; another reply suggests running `rf.exe` without the launcher. — [RageZone: Getting RF Client to my Server](https://forum.ragezone.com/threads/getting-rf-client-to-my-server.917982/) (search summary; page not fetched because of a TLS error)
- A RageZone RF Help summary says that on many builds the server address sits inside a packed client file rather than a plain-text config. — [RageZone RF Help](https://forum.ragezone.com/community/rf-help.972/page-3) (search summary)
- Port guidance from a RageZone guide: 10001 is the login server, and 27780 is the port the launcher connects to on the zone server. Server-side, use the LAN IP in most configs and the public IP only in the launcher, zone server GateIP and login server. — [RageZone: How to make ur RF server ONLINE?](https://forum.ragezone.com/threads/how-to-make-ur-rf-server-online.871306/), [RageZone: Connecting to my server](https://forum.ragezone.com/threads/connecting-to-my-server.1177197/), [PriorityMMO 2.2.3.2 Server Setup guide](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup) (WorldSystem.ini should point to the WAN or LAN IP) (all search summaries)
- `DefaultSet.tmp` (search summary across RF-Dev, RF2232 and RageZone; the individual posts were not read):
  - It lives in the client's `System` folder and is read/written by the client.
  - A "DefaultSetTmp Reader" community tool shows its last login, zone IP and port, account ID and session key.
  - Custom launchers write it to pass settings to `RF_Online.bin`. Bytes are XOR-encoded, and byte index 49 controls a premium flag.
  - A language code in the file selects the client language.
  - A "Fail to create file DefaultSet.tmp" error is fixed by clearing the folder's read-only attribute.
  - Patchers exclude it from updates.

  — [RF2232 forum](http://forum.rf2232.net/index.php?/discover/), [RF-Dev profile posts](https://rf-dev.net/profile/4-princeray/reputation/?type=forums_topic_post&page=3), [RageZone: RF Online Source Code Launcher (English)](https://forum.ragezone.com/threads/rf-online-source-code-launcher-english-version.1133891/), [RageZone: C# RF Launcher & PatchMaker 2232 GU [UPDATED2026]](https://forum.ragezone.com/threads/release-c-rf-launcher-patchmaker-2232-gu-free-updated2026.1253205/), [RageZone: Building RF Launcher](https://forum.ragezone.com/threads/building-rf-launcher.908031/)
- Old School Ops says the server address is not stored in the client for its supported builds, so no address editing is needed. You copy its `rflauncher.exe` into the client folder and run it instead of the game exe. It "writes the client's required settings and starts the client signed in", and its settings sit in a file beside it. For 4.55, the nation code must stay TW, because only the zh-tw language pack in `DataTable\` is current. — [Old School Ops: Game Clients](https://oldschoolops.com/docs/games/rf/game-clients/)
- Matching data files:
  - Client `Item.edf`/`NDItem.edf` changes also need the matching line/block added server-side.
  - To find a client counterpart of a server item, match the item's ncount in `item.dat`.
  - Item.edf workflow: convert edf → dat, hex-edit, convert back to edf (labelled as working for 2.2.3.2).
  - RFDev's 2.2.3.2 basic setup lists client `Store.edf` among the files to edit alongside server files.

  — [RageZone: Item.edf & NDItem.edf With New Line](https://forum.ragezone.com/threads/release-item-edf-nditem-edf-with-new-line.1019845/), [RageZone: NDItem/Item.edf guide](https://forum.ragezone.com/threads/guide-nditem-item-edf.847705/), [Indonesian forum: Membuat Item.Edf](https://rfonline.forumotion.net/t44-membuat-item-edf), [RFDev 2.2.3.2 Basic server setup](https://rfdev.net/topic/453-2232-basic-server-setup/), [RFDev default server & client files](https://rfdev.net/topic/6-2232-default-server-and-client-files/page/3/) (all search summaries; RFDev returned 403)
- RageZone has a guide to editing `resource.edf`. — [RageZone: EDITING RESOURCE.edf](https://forum.ragezone.com/threads/guide-editing-resource-edf.1019160/)
- Client files named by Old School Ops:
  - `RF_Online.bin`: the main executable, used to identify the build
  - `R3Engine.ini`: window mode
  - `DataTable\`
  - `HackShield\<locale>\ehsvc.dll` (1.5)
  - `.bik` movies: deleting them hangs the title screen
  - `RF.exe` (4.15 patcher)

  — [Old School Ops](https://oldschoolops.com/docs/games/rf/game-clients/)

### Inferences
- For localhost, the practical paths are:
  1. A launcher that writes `DefaultSet.tmp` with 127.0.0.1 (open-source launchers exist; see section 3).
  2. Hex-editing the IP string in `RF_Online.bin` / `rflauncher.exe` on builds where it is hard-coded.
  3. Using a prepared localhost exe, which is the riskiest option.
- Map files (`.bsp`/`.r3m`/`.spt`) and `.edf` data generally need to come from the same build as the server, because server map/collision data and item code tables are keyed to them. This is inferred from the "not interchangeable" statement and the item-matching guides.

### Gaps
- The exact `DefaultSet.tmp` byte layout and XOR key were not obtained, since the primary forum posts could not be fetched.
- I could not verify the names "WideServerList" and "ServerIP"; no source mentioned them.
- The full list of client↔server must-match files (beyond Item/NDItem/Store/resource.edf) is not confirmed.

## 3. Client-side tools, Windows 10/11 compatibility, and GitHub repos

### Takeaway
There is a modest open-source ecosystem:
- launchers (Go, Java, C#, Electron)
- a 2.2.3.2 server-data editor
- a Blender add-on for `.msh`/`.ani`/`.bn`/`.r3e`/`.bsp`
- a C# map exporter covering `.bsp`/`.r3m`/`.r3t`/`.spt` to glTF
- quest and emblem viewers

EDF encrypt/decrypt and EDF↔DAT converters circulate on Russian sites. Some retail clients embed GameGuard or HackShield, which community patchers disable by byte patches. For Windows 10/11 the generic approach is admin rights, avoiding Program Files, and optionally dgVoodoo2 (D3D8 wrapper). No RF-specific dgVoodoo guide was found.

### Cited Findings
**GitHub repos** (from the GitHub search API, Oct 2026):
- [Cardboard-box-a/cbb-rf-online-addon](https://github.com/Cardboard-box-a/cbb-rf-online-addon): Blender 5.0 add-on. Imports/exports `.msh`, `.ani`, `.bn` (+ `.bbx`); imports only `.R3E` entities and `.BSP` maps. BSP export exists but is disabled; SPT import is off by default. The MESH08 format is untested before 2.2.3.2. 20 stars, active Oct 2026.
- [xuevertalex-web/RFMapToolMain](https://github.com/xuevertalex-web/RFMapToolMain) ("RFMapToolSharp"): .NET 8 console tool. Exports BSP/EBP geometry, R3M materials, R3T textures (DDS DXT1/3/5/BC7 → PNG), SPT helper volumes, and RVP + `.cam` flight paths to `.glb`. An RPK parser exists and MSH/MOD are optional. GLB→BSP is planned. 41 maps exported; the "2004"-format compressed vertices (Dungeon00) are unsupported.
- [bump44/rf-online-gs-editor](https://github.com/bump44/rf-online-gs-editor): "RF Online GS Editor (2.2.3.2 only)". Electron/JS, MIT, archived 2023-03-01. Fork under Omashu.
- [bump44/rf-online-oswald-launcher](https://github.com/bump44/rf-online-oswald-launcher) (JS launcher), [uargb/rfolt](https://github.com/uargb/rfolt) ("RF Online launching tools", Go, 2021), [Hocksten/java-loader](https://github.com/Hocksten/java-loader) (RFOnline launcher, Java, 2017), [princerayz/RF-Online-MiniLaucnher](https://github.com/princerayz/RF-Online-MiniLaucnher) (C#, 2026).
- [TheKingScott/QuestViewer](https://github.com/TheKingScott/QuestViewer) (2.2.3.2 quest viewer, C#), [josemumuchan/rfemblemeditor](https://github.com/josemumuchan/rfemblemeditor) (guild emblem editor), [nunavx/rfonline-dllblocker](https://github.com/nunavx/rfonline-dllblocker) (DLL blocker, an Enigma Protector plugin, i.e. client protection), [radioneko/rfx](https://github.com/radioneko/rfx) (RF Online proxy/filter in C, 2014), [goodwinxp/Yorozuya](https://github.com/goodwinxp/Yorozuya) (2.2.3.2 server defense, archived), [PapaNayel69/RF-Online-Full-Source](https://github.com/PapaNayel69/RF-Online-Full-Source) (C++, no description, provenance unknown), plus several autoloot bots and memory scanners.
- I found no dedicated GitHub `.edf`/`.dat` parser. The search returned only unrelated EDF formats (biosignal, Endless Online).

**Other tools:**
- rflib.ru lists:
  - EDF-DAT Converter + NDLanguageTools (for in-game text)
  - EdfCrypt/EdfDeCrypt (encrypt/decrypt EDF)
  - RFUFree
  - web tools (fitting, damage calculator, emblem editor, database)

  — [rflib.ru/en/files](https://rflib.ru/en/files)
- An "RF Parser" guide converts binary `.DAT` configs to spreadsheets (Excel) and rebuilds them for the client. — [studylib: RF Parser Guide](https://studylib.net/doc/27177141/rf-parserguide)
- A C# RF Launcher & PatchMaker for 2232 GU is still listed as updated in 2026 on RageZone; there is also an English-version launcher source release. — [RageZone release](https://forum.ragezone.com/threads/release-c-rf-launcher-patchmaker-2232-gu-free-updated2026.1253205/), [RageZone launcher source](https://forum.ragezone.com/threads/rf-online-source-code-launcher-english-version.1133891/)
- GoMMO Client Patcher (Old School Ops):
  - Byte-patches `RF_Online.bin` at fixed offsets to disable retail checks, with a backup and full restore.
  - Disables GameGuard/HackShield-type checks on supported builds, but not on 2.2.3.2, 2.1.5 or Giga 4 Part 2.
  - Sets window or full-screen mode through `R3Engine.ini`.
  - Offers admin elevation if the client sits under Program Files, where writes are blocked.

  — [Old School Ops](https://oldschoolops.com/docs/games/rf/game-clients/)

**Windows 10/11:**
- dgVoodoo2 for D3D8 games: copy `dgVoodooCpl.exe`, `dgVoodoo.conf` and `D3D8.dll` next to the game exe. — [PCGamingWiki: dgVoodoo 2](https://www.pcgamingwiki.com/wiki/DgVoodoo_2), [dgVoodoo2 readme](https://dege.freeweb.hu/dgVoodoo2/ReadmeGeneral/)
- Antivirus or Google sometimes flags dgVoodoo2 as malware, a false positive. — [Steam discussion (Deus Ex IW)](https://steamcommunity.com/app/6920/discussions/0/2565312892642209665)
- The Blender add-on was tested on Windows 10 and Linux Mint, which says nothing about the game client itself. — [cbb-rf-online-addon](https://github.com/Cardboard-box-a/cbb-rf-online-addon)

### Inferences
- Anti-cheat removal on classic clients, described only: patchers NOP or skip the GameGuard/HackShield initialisation checks in `RF_Online.bin` so the client launches without the retail protection services. For 2.2.3.2, community private servers usually distribute clients already stripped of it. That second point is inferred from the patcher not needing to handle 2.2.3.2.
- For a 32-bit D3D8/D3D9 client on Win11, try in this order: install outside Program Files, run as admin, Windows XP SP3 or Windows 7 compatibility mode, windowed mode via `R3Engine.ini`, then dgVoodoo2 if you see crashes or black screens. This is general practice and has not been verified on RF.

### Gaps
- No PCGamingWiki page for classic RF Online was found. Whether the classic client uses D3D8 or D3D9 for each version was not confirmed by a source.
- No RF-specific Win10/11 compatibility reports were found, because the forum pages were inaccessible.
- The authors, licences and safety of the RageZone/rflib binary tools (EdfCrypt, RF Parser, launchers) were not verified.
- No open specification for the `.edf`, `.dat`, `.rfs` or `DefaultSet.tmp` formats was found; the `.r3m`/`.bsp` knowledge exists only inside the two GitHub tools' code.
