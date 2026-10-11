# Chirathantha Gem & Jewellery

A full-stack gem and jewelry store with a custom-design workflow: a
customer configures a piece (metal, gemstone, size, engraving), previews
it in 3D, and submits it for review; an admin approves or declines it and
the customer is notified.

- **`backend/`** — Go + PostgreSQL API (chi, pgx, JWT cookie auth)
- **`frontend/`** — Next.js App Router + TypeScript, Tailwind, Three.js
- **`docker-compose.yml`** — local Postgres

## Running it

Three terminals, or run them one after another.

**1. Database**

```bash
docker compose up -d
```

Creates a `gemstore` database on **host port 5434** (5432 is commonly
already taken by another local Postgres; the in-container port is still
5432). `backend/schema.sql` is applied automatically on first start.

**2. API**

```bash
cd backend
go run ./cmd/seedadmin   -email admin@gemstore.local -password "admin12345" -name "Store Owner"
go run ./cmd/seedproducts -file seed/products.json
go run ./cmd/api
```

Configuration comes from `backend/.env`, which `config.Load` reads
automatically — no exports needed. Copy `backend/.env.example` to
`backend/.env` first if it doesn't exist. The API listens on **:8081**
here, matching `PORT` in `.env` and `NEXT_PUBLIC_API_BASE_URL` in
`frontend/.env.local`; change all three together.

Check it: `curl localhost:8081/health` → `{"status":"ok"}`

**3. Frontend**

```bash
cd frontend
npm install
npm run dev
```

Open <http://localhost:3000>.

## Accounts

Public signup only ever creates a `customer` — there is deliberately no
self-service admin path, so the first admin has to be created with
`cmd/seedadmin` as above. Sign up normally at `/signup` to exercise the
customer flow.

## Tests

```bash
cd backend  && go test ./...          # 19 tests, no DB or network needed
cd frontend && npm run typecheck      # tsc --noEmit, strict
```

## Notes

- Auth is HttpOnly cookies, not tokens in a JSON body — see the "Auth
  model" section of `backend/README.md`. The frontend therefore holds no
  token at all and relies on `credentials: "include"`.
- There is no checkout flow for ready-made catalogue products yet; only
  custom orders (`POST /orders/custom`) work end to end. `GET /products`
  serves the catalogue, which `cmd/seedproducts` populates.
- Detailed API reference, security notes, and known gaps live in
  `backend/README.md` and `frontend/README.md`.