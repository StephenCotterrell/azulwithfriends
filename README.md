# Azul with Friends

A browser game in development with Go, React, TypeScript and PostgreSQL. Choose **Create a room** to take the first seat, then share the room URL with a friend. They can open it and choose **Join room** to take the second seat. Each player can use the default seat name or pick a display name.

Rooms are unlisted. Anyone with the URL can view the seats and claim an open one. A private cookie remembers each player's seat in that browser for up to 30 days. Sharing the URL shares access to the room, while your seat stays with your browser. Tile drafting and scoring are the next milestone.

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

Store credentials in Railway service variables. Use the development Neon branch for remote development and the production branch for the deployed app. Keep production cookies secure. To check a release, create a room in one browser, open its invite in a private window and join. Refresh both windows and check that each returns to its own seat.
