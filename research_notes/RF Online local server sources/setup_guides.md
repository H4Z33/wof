# RF Online Local Private Server Setup on Windows (2.2.3.2 / Giga-era files)

Research date: 2026-10-16. No files were downloaded. Most usable guides are community posts from 2009 to 2019, plus a few RageZone threads from 2024–2025. RageZone (forum.ragezone.com) returned TLS certificate errors to the fetch tool, and rfdev.net and dev-breaker returned HTTP 403, so RageZone and RFDev content below comes only from search-result snippets unless noted. No Giga 5/6/7-specific guide was found. Everything here is for the 2.2.3.2 (also called "2232", "GU", "RF Offline") file family.

## Prerequisites (SQL Server version, ODBC, runtimes, 32/64-bit, ports)

### Takeaway
Classic 2.2.3.2 guides use 64-bit Windows with SQL Server 2005/2008 or 2008 R2 Express, set to Mixed Mode with TCP/IP enabled. Newer guides (about 2024) ship the Microsoft ODBC Driver 17 so the files work on modern Windows, and they require a **default** SQL instance. The ports to open are 10001 (login) and 27780 (zone). You also need the "RF dependency" pack (VC++ runtimes and .NET), or the executables fail with side-by-side errors.

### Cited Findings
- 2009 "RF Online Server 2.2.3 Support System" guide (v1.1.0, Magenik, 20 June 2009): requires Microsoft SQL Server 2005 and says 2008 can cause bugs. Recommends 64-bit Windows Server 2003 (Windows 7 64-bit for home), 8 GB RAM, and a web server for the launcher/updater. Configures AccountSystem.ini and LoginSystem.ini. — [Scribd: Tutorial making server rf online](https://www.scribd.com/document/899606108/Tutorial-making-server-rf-online)
- 2016 guide (HELLSCRE4M, Mar 2 2016, crediting murasmune of RageZone): 64-bit Windows only ("32-bit will not work"), 4 GB+ RAM, MSSQL 2008 Express 64-bit (2005 also OK). Run the RF dependency files first, or AccountServerSD and ZoneServer will error. — [PriorityMMO: [Guide] RF Online 2.2.3.2 Server Setup](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- RF Offline 2.2.3.2 GU tutorial (Oct 15 2017; edited 2018/2019; links updated Apr 21 2022) uses these settings:
  - SQL Server 2008 R2 Express with Tools (SQLEXPRWT_x64_ENU.EXE) on the default SQLEXPRESS instance.
  - Mixed Mode authentication with an SA password.
  - TCP/IP enabled in SQL Server Configuration Manager.
  - SQL Server Browser service set to Automatic, then SQL restarted.

  — [rfworld.forumotion: Rf Offline 2.2.3.2 GU Server Files + Full Tutorial](https://rfworld.forumotion.com/t85-rf-offline-2-2-3-2-gu-server-files-full-tutorial)
- A newer guide (about 700 days old as of the search, so roughly late 2024) for current Windows Server:
  - RF needs a special ODBC driver on modern OSes, so msodbcsql_17 (Microsoft ODBC Driver 17) ships with the server files.
  - Use the **Default Instance**. A named instance makes the server unable to find the databases.
  - Use Mixed Mode authentication and run the SQL service as NT AUTHORITY\SYSTEM.

  (search snippet only; the page now returns 404) — [ariq.nauf.al: Install RF Online Private Server on Windows Server](https://ariq.nauf.al/blog/install-rf-online-private-server-on-windows-server/)
- ODBC Driver 17 supports SQL Server 2019 only from v17.3. The connection string uses `Driver={ODBC Driver 17 for SQL Server}` (msodbcsql17.dll). — [connectionstrings.com](https://connectionstrings.com/microsoft-odbc-driver-17-for-sql-server/info-and-download/)
- RF-Dev starter guide ("3: Install Microsoft SQL Server") notes the following (search snippet; site DNS failed at fetch):
  - SQL Server TCP/IP is not enabled by default on Windows 7 the way it is on Windows 10.
  - If TCP/IP is not enabled, the ZoneServer hits a connection bug at game login.

  — [RF-Dev wiki starter guide](https://rf-dev.net/wiki/starter-guide/3-install-microsoft-sql-server-mssql-r3/)
- SQL Server 2019 Developer, Standard and Express are supported on Windows 10 Home/Pro/Enterprise, 1507 or later. — [SQLCoffee](https://www.sqlcoffee.com/SQLServer2019_0002.htm)
- Ports: a 2025 RageZone poster says to open **10001** (login, used by the launcher and LoginServer) and **27780** (ZoneServer). Internal config IPs can stay at their defaults; set the launcher IP. Running the server in a VM and playing from the host works with firewall/port-forward plus the launcher IP. — [RageZone: 2025 rf server set-up](https://forum.ragezone.com/threads/2025-rf-server-set-up.1242748/) (snippet)
- Other ports in the 2016 guide:
  - SQL Server: 1433 by default, or a custom port such as 61433.
  - FireGuard (fg.ini): port 15444.
  - FireGuard fgrs.ini: port 15445.

  — [PriorityMMO guide](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- A "side-by-side configuration" error on Windows 10 is fixed by installing the RF dependency package. "Couldn't run update library" is fixed by installing Microsoft C++ and .NET Framework packages. — [rfworld.forumotion: Problem and Solving ALL ABOUT RF OFFLINE (Dec 24 2018)](https://rfworld.forumotion.com/t535-problem-and-solving-all-about-rf-offline)

### Inferences
- On Windows 10/11 today, the practical route is the following, but it is unverified by any single guide:
  - SQL Server Express or Developer (2019/2022), installed as a default instance (MSSQLSERVER).
  - Mixed Mode authentication and TCP/IP enabled.
  - An ODBC driver the server binaries accept (legacy "SQL Server", Native Client 10/11, or ODBC 17).
  - Older guides use SQL 2005/2008 because the files were built around that era. The specific bugs in newer SQL versions are not documented.
- The binaries are 64-bit (ZoneServerUD_x64.exe, "zoneserver64bit"), so DSNs probably belong in the 64-bit ODBC administrator (`C:\Windows\System32\odbcad32.exe`). No guide states this explicitly.
- The specific VC++ redistributable versions are unknown. Guides only say "RF dependency pack".

### Gaps
- No source names exact VC++ redistributable years (2005/2008/2010 etc.) or a DirectX requirement for the server side.
- No Giga 5/6/7 setup guide was found. Whether Giga files need different SQL versions or DSNs is unknown.
- No source explicitly confirms 32-bit vs 64-bit ODBC administrator usage.

## Databases (which DBs, restore, logins, accounts/GM)

### Takeaway
2.2.3.2 uses three databases: **BILLING, RF_User, RF_World**. The 2009 guide's script names RF_WORLD_NOVUS. Restore them from .bak with Overwrite into a non-default folder. Create an "rf" SQL login plus the hardcoded RU logins **rfrusaccount** (RF_User) and **rfrusworld** (RF_World). Accounts are rows in RF_User.dbo.tbl_rfaccount; GMs go in tbl_StaffAccount.

### Cited Findings
- 2009 guide: a provided SQL script creates the RF_User and RF_WORLD_NOVUS databases, then you create a User DSN. — [Scribd](https://www.scribd.com/document/899606108/Tutorial-making-server-rf-online)
- 2016 guide, database restore:
  - Create BILLING, RF_User and RF_World, then restore each from its .bak with "Overwrite" checked.
  - Restore into a custom folder such as C:\RFDB. Restoring to the MSSQL default directory fails.

  — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- 2016 guide, SQL logins:
  - Create an SQL-auth login "rf" mapped to all three DBs, with all roles except the deny roles. Uncheck the password policy.
  - Create `rfrusaccount` mapped to RF_User and `rfrusworld` mapped to RF_World.
  - These two logins are **hardcoded in AccountServer/ZoneServer for NationCode RU**. Other nation codes need an ODBC trace or SQL Profiler to discover the logins.
  - Recheck db_owner on each login.

  — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- 2017 GU tutorial:
  - Restore RF_World, RF_User and BILLING with "Overwrite the existing database".
  - Create the same three logins. The rfrusaccount and rfrusworld passwords are fixed strings set in the tutorial, which are not repeated here.

  — [rfworld.forumotion t85](https://rfworld.forumotion.com/t85-rf-offline-2-2-3-2-gu-server-files-full-tutorial)
- Accounts:
  - A test account is an INSERT into `RF_User.dbo.tbl_rfaccount`.
  - A GM account is an INSERT into `RF_User.dbo.tbl_StaffAccount`, with the GM ID also added to Login_Filter_Dev.ini.
  - The exact SQL appears in the guide.

  — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- Bank error fix: in SSMS, modify `tbl_AccountTrunk` and change the Trunk Pass column type to int. — [rfworld.forumotion t535](https://rfworld.forumotion.com/t535-problem-and-solving-all-about-rf-offline)
- GM panel tools such as "Capiroto" connect with the RF_World, RF_User and BILLING names plus the sa login, and can unlock XP caps. — [rfworld.forumotion t535](https://rfworld.forumotion.com/t535-problem-and-solving-all-about-rf-offline)
- A GitHub admin CP (zerdnem/rf-online-admin-cp) targets the "server 2232 database". — [GitHub](https://github.com/zerdnem/rf-online-admin-cp)

### Inferences
- When restoring old 2005/2008 .bak files to SQL 2019/2022, SQL upgrades the compatibility level. Whether stored procedures keep working is not documented in the sources found.

### Gaps
- No source discussed SQL collation requirements (for example Korean_Wansung or Cyrillic vs Latin1). The t535 thread explicitly does not cover collation.
- There is no list of required stored procedures. They come embedded in the .bak files.

## Config files, IP settings, and start order

### Takeaway
You edit about ten INI files across the "Account and Login\RF_Bin" and "ZoneServer" folders. Start the servers in this order: **AccountServer(SD) → BillingServer → LoginServer(SD) (type `/open`) → ZoneServer(64-bit)**. Set WorldInfo.ini ServerType=0, and comment out the version-check key.

### Cited Findings
- 2016 guide: create `HBK` and `History` folders in the server directory. — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- INI files in `Account and Login\RF_Bin\Initialize` (2016 guide) — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup):
  - **AccountSystem.ini** sets:
    - AccountDB machine, and name = RF_User.
    - NationCode=RU.
    - DB_INFO: IP, port 1433, BILLING database, login.
    - [World] Name0, Address0, and DBName0 = RF_World.
    - MaxAccountNum.
  - **BillingSystem.ini** sets ZoneAddress, NationCode, and the BILLING section (IP, port, CP_ID, GROUP_NUM, SERVER_NUM, GAME_ID).
  - **LoginSystem.ini** sets:
    - AccountAddress.
    - CHECK ADDRESS=TRUE and AUTH_LOG=TRUE.
    - Billing Check=FALSE and MOTP Service=FALSE.
    - IP filter.
    - BILL_RU DB settings.
  - **Login_Filter_Dev.ini** holds the GM ID filter.
- `Account and Login\RF_Bin\rfacc.ini`: DBSTR (driver, server, database, login). **Copy it into ZoneServer\RF_Bin.** — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- ZoneServer files (2016 guide) — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup):
  - **ZoneServer\WorldInfo\WorldInfo.ini** sets WorldName, ServerType (change from 1 to 0, or clients cannot connect), HBKPath, HistoryPath, NationCode, rates and events.
  - **ZoneServer\RF_Bin\Initialize\WorldSystem.ini** sets AccountAddress and LimUserNum. Comment out the version-check key.
  - **ServerRate.ini**: ItemRootRate above 10 crashed the author's server.
  - **ZoneServer\RF_Bin\fireguard\fg.ini** sets the server name and FireGuard IP:15444.
  - **fgrs.ini** sets port 15445 and changes "Use" from NO to FALSE.
- Replace 127.0.0.1 and placeholder IPs with your LAN or WAN IP. A local-only setup can stay on 127.0.0.1. — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup); [rfworld t85](https://rfworld.forumotion.com/t85-rf-offline-2-2-3-2-gu-server-files-full-tutorial)
- ODBC DSNs:
  - 2016 guide: User DSNs named BILLING, RF_User and RF_World, using the SQL Server Native Client 10.0 driver with SQL authentication and the matching default DB. Click Test Data Source. — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
  - 2017 tutorial: three "SQL Server" driver DSNs pointing to `127.0.0.1\SQLEXPRESS` with Windows authentication. — [rfworld t85](https://rfworld.forumotion.com/t85-rf-offline-2-2-3-2-gu-server-files-full-tutorial)
- Start order:
  - 2016 guide: accountserversd.exe → billingserver.exe → loginserver.exe (use `/open`) → 64-bit zoneserver. — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
  - 2017 GU tutorial: AccountServerSD.exe, LoginServerSD.exe and ZoneServerUD_x64.exe, all "Run as Administrator". If the login server closes on launcher login, type `/Open` in the LoginServerSD window and wait for "ExternalConnect Open". — [rfworld t85](https://rfworld.forumotion.com/t85-rf-offline-2-2-3-2-gu-server-files-full-tutorial)
- The general workflow from the RageZone RF Tutorials index:
  1. Get the files.
  2. Install the DB engine.
  3. Import the DBs.
  4. Edit configs with your address and ports.
  5. Start processes in sequence.
  6. Point the client at the server.

  — [RageZone RF Tutorials](https://forum.ragezone.com/community/rf-tutorials.480/) (snippet)
- Client side:
  - The client IP must be hex-edited, or a launcher (EQB, Genesis) used.
  - In the client, r3engine.ini `language = USA`.
  - Client `Datatable\En-GB\FGCB.ini`: USEFG toggle for a stuck FireGuard.
  - Some packs use an Apache-based "Emulator" for the launcher/updater.

  — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup); [rfworld t535](https://rfworld.forumotion.com/t535-problem-and-solving-all-about-rf-offline)

### Inferences
- The BillingServer step appears in some packs (2016) and is absent in others (2017 GU pack). It depends on the release.
- Use `/open` on the LoginServer in every pack.

### Gaps
- The exact INI key names and values were not fully visible. Reading the primary guide pages is needed for exact syntax.
- The role of the 27780 zone port in the configs (which INI sets it) was not confirmed.

## Common errors and fixes

### Takeaway
Most failures are ODBC/login mismatches: "AccountDB Connect Failed", "ConfigUserODBC Failed", "Connect World DB Failed", "DB INIT failed" and CashDB errors. Fixes:
- Correct the DSN names and driver.
- Create the hardcoded RU logins.
- Enable TCP/IP or Named Pipes.
- Run as Administrator.
- Re-restore the databases.

### Cited Findings
- "AccountDB Failed" at every AccountServerSD start comes from a missing or misconfigured RF_User login. Create the `rfrusaccount` login. — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup)
- Other fixes from RageZone/RFDev threads (search snippets, dated 2011–2019):
  - Re-check the ODBC DSN for BILLING.
  - Run as Administrator.
  - Replace rfacc.ini with a working copy.
  - Use the "SQL Server" driver rather than SQL Native Client (2019 post).
  - Use Windows auth with Native Client for RF_User.
  - Enable **Named Pipes** in SQL configuration.
  - Re-restore or swap the RF_World DB.
  - Make sure NationCode=RU.

  — [RageZone: Zoneserver "Connect World DB Failed"](https://forum.ragezone.com/threads/zoneserver-connect-world-db-failed.991449/); [RageZone: AccountDB ConfigUserODBC Failed](https://forum.ragezone.com/threads/accountdb-configuserodbc-failed.849637/); [RageZone: ZoneServer Connect World DB Failed](https://forum.ragezone.com/threads/zoneserver-connect-world-db-failed.1058164/); [RageZone: AccountDB & ZoneServer Problem](https://forum.ragezone.com/threads/accountdb-zoneserver-problem.930241/); [RageZone video: Fixing AccountDB Connect Failed](https://forum.ragezone.com/threads/video-rfodev-fixing-accountdb-connect-failed.875660/); [RFDev: CashDB Error](https://rfdev.net/topic/9-cashdb-error/); [rfworld: RF_World DB Fail SOLVED](https://rfworld.forumotion.com/t599-solved-setup-rf-offline-rf_world-db-fail)
- The ZoneServer error "Failed to create empty document" has two possible causes:
  - Check ODBC, IP/INI settings and NationCode RU. — [rfworld t535](https://rfworld.forumotion.com/t535-problem-and-solving-all-about-rf-offline)
  - It can also come from corrupt script files, for example an UpperItem.dat with the wrong size (search snippet aggregation).
- If TCP/IP is not enabled, the ZoneServer hits a connection bug at login. — [RF-Dev wiki](https://rf-dev.net/wiki/starter-guide/3-install-microsoft-sql-server-mssql-r3/) (snippet)
- If the SQL Server is a named instance instead of the default, the server cannot find the databases. — [ariq.nauf.al](https://ariq.nauf.al/blog/install-rf-online-private-server-on-windows-server/) (snippet)
- Fixes for other errors (2016 guide) — [PriorityMMO](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup):
  - If the restore fails, use a non-default restore folder.
  - If clients cannot connect, set WorldInfo ServerType=0.
  - If the server crashes, keep ItemRootRate at 10 or below.
  - For "client not latest version", comment out the version-check key.
- Windows 10 errors (2018 thread) — [rfworld t535](https://rfworld.forumotion.com/t535-problem-and-solving-all-about-rf-offline):
  - A side-by-side error means you need to install the dependencies.
  - "Roles already exist in another database" was addressed via server roles (db_securityadmin). The poster flagged this fix as unverified.
  - Crashes when buying Council armor happen because the NPC store data is missing on the server.

### Inferences
- Error families map to components:
  - AccountDB errors point to RF_User DSN/login.
  - World DB errors point to RF_World/rfrusworld and rfacc.ini in ZoneServer\RF_Bin.
  - CashDB/Billing errors point to the BILLING DSN.

### Gaps
- No source found on SQL collation errors.
- No source found on Windows 11-specific issues. Only Windows 10 is mentioned.

## Notable guides, videos, and Docker/VM/Wine approaches

### Takeaway
The key written guides are:
- The 2016 PriorityMMO repost of murasmune's RageZone guide.
- The 2017 rfworld.forumotion GU tutorial.
- The RageZone RF Tutorials section, including the 2025 setup thread.
- The RFDev "[2.2.3.2] Basic server setup" guide.
- The RF-Dev wiki starter guide.

Nothing was found on Docker or Wine/Linux. A VM (Windows guest) approach is reported to work.

### Cited Findings
- [PriorityMMO: [Guide] RF Online 2.2.3.2 Server Setup](https://prioritymmo.boards.net/thread/55/guide-rf-online-server-setup): Mar 2 2016, the most detailed one fetched. Outdated SQL versions.
- [rfworld.forumotion: Rf Offline 2.2.3.2 GU Server Files + Full Tutorial](https://rfworld.forumotion.com/t85-rf-offline-2-2-3-2-gu-server-files-full-tutorial): 2017, links updated 2022, Indonesian/English. Plus its [troubleshooting thread](https://rfworld.forumotion.com/t535-problem-and-solving-all-about-rf-offline) (2018–2019).
- RageZone threads (not fetchable, so dates are approximate from snippets):
  - [RF Tutorials index](https://forum.ragezone.com/community/rf-tutorials.480/)
  - [[Guide] ** RF v2.2.3.2 Server SetUP **](https://forum.ragezone.com/f480/guide-rf-v2-2-3-a-803273/) (about 2011, "over 15 years old")
  - [Rf online 2.2.3.2 server setup](https://forum.ragezone.com/threads/rf-online-2-2-3-2-server-setup.889335/)
  - [[Video Guide] Setup RF Online (server only)](https://forum.ragezone.com/threads/video-guide-setup-rf-online-server-only.1134986/) (described as a rough walkthrough)
  - [2025 rf server set-up](https://forum.ragezone.com/threads/2025-rf-server-set-up.1242748/)
  - [Inside RF Online's ZoneServer: Startup and Entering the World](https://forum.ragezone.com/threads/inside-rf-onlines-zoneserver-startup-and-entering-the-world.1269875/) (recent technical deep-dive)
  - [RF Online 2.1.5 Server Files (slightly modified)](https://forum.ragezone.com/threads/rf-online-2-1-5-server-files-slightly-modified-client-slightly-modified.1244500/)
- RFDev guide: [[2.2.3.2] Basic server setup](https://rfdev.net/topic/453-2232-basic-server-setup/) (returned 403 at fetch).
- YouTube: [RF ONLINE- SET UP SQL AND MAKE YOUR OWN RF ONLINE SERVER PART 4](https://www.youtube.com/watch?v=Dulnw3AeqbE) (multi-part series; date not verified).
- Windows Server guide (about 2024, now 404): [ariq.nauf.al](https://ariq.nauf.al/blog/install-rf-online-private-server-on-windows-server/).
- The 2009 Magenik guide on [Scribd](https://www.scribd.com/document/899606108/Tutorial-making-server-rf-online).
- GitHub tools (not server setup READMEs): [rf-online-admin-cp](https://github.com/zerdnem/rf-online-admin-cp), [Kohaku-Dev RF mods](https://github.com/Nur1Labs/Kohaku-Dev-RF-Online-), [Rf-Edit](https://github.com/rtsme/Rf-Edit), [rf-online-helper](https://github.com/agusjanardana/rf-online-helper).
- VM: running the server in a VM and playing from the host is viable with firewall/port forwarding and the launcher IP set. — [RageZone 2025 thread](https://forum.ragezone.com/threads/2025-rf-server-set-up.1242748/) (snippet)

### Inferences
- Because the server binaries depend on Windows ODBC/MSSQL, Wine/Docker attempts seem unlikely to be common. A Windows VM is the safest isolation option. This is speculation, since no source covers it.

### Gaps
- No Docker, Wine or Linux attempts for RF Online servers were found.
- No elitepvpers or Russian-forum guides surfaced in searches.
- YouTube video dates and content were not verified.
- No Giga 5/6/7-specific documentation was found.
