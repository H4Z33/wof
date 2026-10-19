# Convergence

Cultivation MMO prototype: Go server + Godot 4 client. Design: [docs/TECHNICAL_DESIGN.md](docs/TECHNICAL_DESIGN.md).

## Status: M0 (skeleton)

- Dev login over HTTP (any name; no accounts or database yet).
- WebSocket gateway with protobuf messages, rate limiting and Hello handshake.
- One zone (`jade_terraces`), single goroutine at 20 Hz, server-authoritative movement with an anti-speed-hack input budget.
- Godot client: login screen, client-side prediction with reconciliation, 100 ms interpolation for other players, third-person camera, nameplates.
- Three character models on the shared Quaternius UAL skeleton: male and female mannequins (CC0) and Zuo Mo (from the world_of_cultivation sculpt, rigged with `tools/blender/rig_to_ual.py`).

## Requirements

- Go 1.27+ (`winget install GoLang.Go`)
- Godot 4.7+ (`winget install GodotEngine.GodotEngine`)
- Blender 5.x, only to rig new character models

## Run

Server and two clients side by side:

```bash
powershell -File tools/run-dev.ps1
```

Or by hand:

```bash
cd server && go run ./cmd/server
```

Then open `client/project.godot` in Godot and press Play (once per player; Debug → Customize Run Instances lets you run several).

Client options after `--`: `--name=Alice --model=zuo_mo --autoconnect --server=http://host:7777 --window=x,y,w,h --autowalk --cam_yaw=deg --screenshot=path`.

## Test

```bash
cd server && go test ./...
```

```bash
cd server && go run ./cmd/bot -n 50 -duration 15s
```

The bot run needs a server running; each bot walks randomly and checks that it sees every other bot.

## Protocol changes

Edit `proto/convergence/v1/*.proto`, then regenerate the Go and GDScript code:

```bash
powershell -File tools/gen.ps1
```

GDScript comes from our own `tools/protoc-gen-gdscript` (no Godot addon needed). It supports scalars, enums, nested messages, repeated fields and message-only oneofs; anything else fails generation with a clear error.

## Layout

| Path | Contents |
|---|---|
| `proto/` | Wire protocol (single source of truth) |
| `server/` | Go module: `cmd/server`, `cmd/bot`, `internal/{auth,gateway,zone}`, generated `gen/` |
| `client/` | Godot project: `net/` (connection), `game/` (scenes, world, characters), generated `gen/`, `assets/` |
| `tools/` | Codegen, Blender rigging script, dev launcher |
| `docs/` | Design documents |
| `research_notes/`, `reports/` | Earlier RF Online research |
