# Azul with Friends

A two-player browser adaptation of Azul built with Go, React, TypeScript and PostgreSQL. Create a room and choose **Add bot** to play against the computer. You can remove the bot before starting. To play with another person, share a **Link only** invitation or offer the open seat in the **Public lobby**. Opening an invitation automatically claims an open seat. Guests can also join directly from the lobby. Once both seats are filled, the host chooses **Start game**. You can play as a guest without signing in or choosing a display name. [Play Azul with Friends](https://azulwithfriends.com)

The game uses the standard colored wall. The server randomly selects the first player. On your turn, select a color from one factory or the center, then choose a highlighted pattern line or the floor. The bot takes its turns automatically. **Last draft** shows what it picked. Both boards score automatically after each round. Completing a wall row triggers final scoring at the end of that round. To play again, create a new room. [Publisher’s rulebook](https://cdn.svc.asmodee.net/production-nextmove/uploads/sites/4/2024/06/EN-Azul-Rules-Next-Move-web.pdf)

Rooms have a four-word name and an invitation at `https://azulwithfriends.com/room/<name>`. Link-only rooms are the default. The host can list or unlist an open seat. A public listing lasts one hour and disappears when the second player joins. The invitation continues to work after unlisting or expiry.

A private cookie remembers each player's seat on the original host in that browser for up to 30 days. Earlier UUID invitations still open the same room. If you started a game on the Railway hostname, use its original link to resume your seat there.

Go validates each draft against the current turn and board revision, then commits it to PostgreSQL under a room lock. The saved position includes the bag and shuffle state so a restart can resume mid-round. The browser receives the visible boards, offers, and legal moves.

The bot searches two turns ahead using the visible tiles. It weighs each legal draft against the opponent's strongest reply in the current round, estimating wall points, floor penalties, and progress on unfinished lines. The search follows the minimax approach in [Dom Wilson’s Azul AI](https://domwil.co.uk/posts/azul-ai/).

## Run locally

Use Go 1.26 or later, Node 24 or later, and a local PostgreSQL server. These commands assume your OS user can create and connect to a local database. Run them from the repository root.

```sh
createdb azul_dev
export DATABASE_URL=postgresql:///azul_dev
export DATABASE_URL_UNPOOLED="$DATABASE_URL"
go run ./cmd/server migrate
npm --prefix web ci
npm --prefix web run build
ALLOW_INSECURE_COOKIES=true go run ./cmd/server
```

Open `http://localhost:8080`. For frontend development, leave Go running and run `npm --prefix web run dev` in a second terminal. Vite forwards `/api` requests to Go. Set `ALLOW_INSECURE_COOKIES=true` only for local HTTP development.

## Verify a change

```sh
createdb azul_test
TEST_DATABASE_URL=postgresql:///azul_test go test -race ./...
go vet ./...
npm --prefix web run build
```

Use a dedicated test database. The PostgreSQL integration test creates a temporary schema. The CI workflow also builds the production Docker image, runs its migration command and checks its HTTP endpoints.

## Deploy

The production service is defined in `.railway/railway.ts`. After linking the Railway CLI to the existing project, run `npm ci` at the repository root and `railway config plan --environment production` to review configuration changes. Apply the reviewed configuration with `railway config apply --environment production`. GitHub pushes to `master` deploy application code.

Railway builds the Go binary and frontend together from the Dockerfile. Its pre-deploy step runs `/app/server migrate`. The server checks the schema at startup. The `/healthz` route checks the running process without waking the database on each probe.

| Setting | Purpose |
| --- | --- |
| `DATABASE_URL` | Pooled Neon connection for application queries, using the application role |
| `DATABASE_URL_UNPOOLED` | Direct Neon connection for the role that owns the schema and runs migrations |
| `PORT` | Listen port, default `8080` |
| `WEB_DIST_DIR` | Compiled frontend directory, default `web/dist` |

Store credentials in Railway service variables. Use the development Neon branch for remote development and the production branch for the deployed app. Keep production cookies secure. To check a release, create a room, join from a private window, start a game, draft in each window, and reload both. Confirm that each returns to its own seat with the same board and scores.
