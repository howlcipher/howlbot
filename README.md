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

Releases need two people. When `/repo release` returns `REQUIRE_APPROVAL`, the
bot stores a pending request and replies with its id. `/repo approve <id>`
approves only that stored request, only once, within 24 hours, and only when the
approver is not the requester; anything else is denied. Pending requests are
kept in memory, so a restart forgets them and they must be requested again.

See [GitHub Pages](docs/index.html) for more details.

### Pending approval reliability

Discord approvers must currently hold the configured administrator role before a
request can be reserved. A request has at most one policy evaluation in flight.
Only an `ALLOW` response consumes it; a policy denial, malformed result or failed
evaluator preserves the original request for retry until its 24-hour expiry.
Successful consumption is final even if sending the Discord response fails, so a
response retry cannot authorize the operation twice. Concurrent history access
uses a locked snapshot.

Discord roles and a different requester/approver are service policy safeguards.
They do not implement independently authenticated owner approval against an agent
sharing the owner's operating-system account. The current policy demonstrates
messages, not actual protected releases. Real protected operations require the
independently protected executor, owner authentication and replay ledger selected
for Model B before they may be enabled.

Approval requests now reserve only after administrator authorization and commit after policy ALLOW; failures retain pending requests. Concurrent handler history is synchronized. These service roles do not establish Model B owner authentication.
