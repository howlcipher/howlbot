# HowlBot

HowlBot is an event-driven Discord operations bot and external HowlFrame dogfooding application. Discord users may propose actions, but trusted identity evidence and deterministic HowlFrame policy decide what the system is authorized to do.

## Architecture

```
Discord User
     ↓
Discord Event
     ↓
Trusted Discord Adapter (Go)
     ↓
normalized intent + trusted identity evidence
     ↓
HowlFrame
     ↓
ALLOW / DENY / REQUIRE_APPROVAL
     ↓
Bounded Action Catalog
     ↓
Discord / external effect
```

HowlBot uses a thin Go adapter to handle Discord's gateway communication. This adapter acts as the trusted perimeter: it translates raw Discord interaction events into normalized intent, captures hard cryptographic identity evidence (e.g., user IDs, validated roles), and passes this structured payload to HowlFrame.

HowlFrame (in `src/howlbot.howl`) evaluates this payload deterministically and outputs a bounded action decision. The adapter executes this decision. The Discord message is intent, not authority.

## Setup

1. Copy `.env.example` to `.env` and configure your Discord bot token.
2. Ensure you have the `howlframe` binary in your PATH or configured in `.env`.
3. Copy `config/howlbot.example.json` to `config/howlbot.json` and configure your Discord roles.
4. Run `make run`.

### Discord Permissions
The bot requires:
- `application.commands` scope to register and handle slash commands.
- `Send Messages` permission in the channels it operates in.
No Administrator permission is needed.

## Security Boundary

HowlFrame alone authorizes operations. No user input can directly force an execution branch without being validated against trusted evidence (roles, IDs) collected independently by the Go adapter.

See [GitHub Pages](docs/index.html) for more details.
