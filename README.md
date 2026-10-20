# Convergence

Cultivation MMO prototype: Go server + Godot 4 client. Design: [docs/TECHNICAL_DESIGN.md](docs/TECHNICAL_DESIGN.md).

## Status: M1 (living zone)

- Accounts (Argon2id passwords) and up to 4 characters per account in PostgreSQL; level, experience, health and position persist (write-behind every 30 s, and on logout, level-up and shutdown).
- HTTP account API: register, login, character list/creation, play ticket. Per-IP rate limit on register/login.
- WebSocket gateway with protobuf messages, rate limiting and Hello handshake; logging in again drops the older session.
- One zone (`jade_terraces`), single goroutine at 20 Hz: server-authoritative movement with an anti-speed-hack input budget, and interest management on a 24 m grid.
- NPCs from YAML (`content/npcs`) with behavior in sandboxed Lua (`content/ai`): wander, aggro, chase, attack, leash home and heal, respawn.
- Tab-target combat: two skills from `content/skills.yaml` (Strike, Qi Bolt), server-checked cooldown and range, death and respawn, experience and levels.
- Godot client: login and character screens, prediction and reconciliation, interpolation, click/Tab targeting, skill bar with cooldowns, health and experience bars, target frame, damage numbers, attack/hit/death animations, Qi Bolt projectile.
- Character models on the shared Quaternius UAL skeleton: male and female mannequins (CC0), Zuo Mo (rigged with `tools/blender/rig_to_ual.py`) and a tinted mannequin for bandits.

## Requirements

- Go 1.27+ (`winget install GoLang.Go`)
- Godot 4.7+ (`winget install GodotEngine.GodotEngine`)
- PostgreSQL 17 (`winget install PostgreSQL.PostgreSQL.17`) with a `convergence` role and database
- Blender 5.x, only to rig new character models

Local settings live in `.env` at the repo root (not committed):

| Variable | Used by |
|---|---|
| `DATABASE_URL` | Server (migrations run automatically at startup) |
| `TEST_DATABASE_URL` | Account tests (skipped when unset; the database is wiped by the tests) |
| `DEV_ACCOUNT_PASSWORD` | `tools/run-dev.ps1` dev accounts |

## Run

Server and two clients side by side, each logged in to its own dev account:

```bash
powershell -File tools/run-dev.ps1
```

Or by hand (from the repo root, so the server finds `content/` and `.env`):

```bash
go -C server run ./cmd/server
```

Then open `client/project.godot` in Godot and press Play; create an account and a character on the first screen.

Controls: WASD move, click or Tab to target, 1–2 skills, right-drag camera, wheel zoom, Esc logout.

Client test options after `--`: `--user=... --password=... --character=Name --model=zuo_mo --autoconnect --autowalk --autofight --server=http://host:7777 --window=x,y,w,h --cam_yaw=deg --screenshot=path`.

## Test

```bash
go -C server test ./...
```

Bots log in, create characters and play against a running server. `-fight` makes them hunt NPCs; start the server with `-auth-burst 500` so the per-IP login limit does not block them:

```bash
go -C server run ./cmd/bot -n 50 -duration 20s -fight
```

## Content

| File | Defines |
|---|---|
| `content/skills.yaml` | Player skills: range, cooldown, damage, animation |
| `content/npcs/*.yaml` | NPCs: stats, attack, respawn, AI script |
| `content/ai/*.lua` | NPC behavior; `think(npc, mem)` runs 5 times per second |
| `content/zones/*.yaml` | Zone size, player spawn, NPC spawn groups |
| `content/items.yaml` | Items (no effects yet; kept in the inventory) |
| `content/fate.yaml` | Fate: deck size, triggers and their chances, luck/karma odds, loot, and the action catalog |

The server validates references at startup and refuses to start on errors.

## Fate (luck and karma)

A proof of concept of an LLM "game master". Each character has **luck** (rolled at creation) and **karma** (changes with deeds). In the background, the Fate agent prepares a small deck of possible fates for each player: situations and narrations that fit their place, deeds and standing. During play, triggers (a hit, a kill, a long walk, a rest, a close call) roll dice fed by the player's own inputs; when fate strikes, the server draws the matching card and applies it at once. Actions include treasure, healing, experience, critical hits, exposed weaknesses, auras, hidden caches, extra spoils, ambushes, mishaps, misses and enraged enemies.

The agent only writes cards: the server decides when something happens, how good or bad, and validates every card against the catalog (anything invalid becomes a fallback card). The game never waits on it and works without it.

Server flags: `-fate-model` (default `deepseek-v4.1-flash:cloud`, empty to disable the agent), `-fate-url` (default `http://127.0.0.1:11434`), `-fate-timeout`.

Live check of the agent against Ollama:

```bash
FATE_OLLAMA_MODEL=deepseek-v4.1-flash:cloud go -C server test ./internal/fate -run Live -v
```

## Protocol changes

Edit `proto/convergence/v1/*.proto`, then regenerate the Go and GDScript code:

```bash
powershell -File tools/gen.ps1
```

GDScript comes from our own `tools/protoc-gen-gdscript` (no Godot addon needed). It supports scalars, enums, nested messages, repeated fields and message-only oneofs; anything else fails generation with a clear error. Names that clash with Godot globals (like `Error`) get a trailing underscore.

## Layout

| Path | Contents |
|---|---|
| `proto/` | Wire protocol (single source of truth) |
| `content/` | Game data (YAML) and NPC scripts (Lua) |
| `server/` | Go module: `cmd/server`, `cmd/bot`, `internal/{account,api,auth,content,db,gateway,persist,rules,script,zone}`, generated `gen/` |
| `client/` | Godot project: `net/` (connection), `game/` (scenes, world, HUD, characters), generated `gen/`, `assets/` |
| `tools/` | Codegen, Blender rigging script, dev launcher |
| `docs/` | Design documents |
| `research_notes/`, `reports/` | Earlier RF Online research |
