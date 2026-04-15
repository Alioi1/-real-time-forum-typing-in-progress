# Real-Time Forum

## How to run

### Without Docker

```bash
# 1. Install dependencies
go mod tidy

# 2. Run
go run .

# 3. Open browser at http://localhost:8080
```

### With Docker

```bash
# Build and run
docker build -t real-time-forum .
docker run -p 8080:8080 real-time-forum
```

## Project Structure

```
real-time-forum/
├── main.go                 # Entry point, routes
├── go.mod                  # Go module
├── Dockerfile
├── database/
│   └── db.go               # SQLite init + tables
├── handlers/
│   ├── auth.go             # Register, Login, Logout, Me
│   ├── posts.go            # Posts + Comments CRUD
│   ├── messages.go         # Private messages + users list
│   └── websocket.go        # WS Hub + Client
└── static/
    ├── index.html          # Single HTML page (SPA)
    ├── style.css           # Styles
    └── app.js              # All frontend logic
```

## Features

- **Registration** — nickname, email, password, age, gender, first/last name
- **Login** — by nickname OR email + password (bcrypt encrypted)
- **Sessions** — cookie-based (UUID token, 24h expiry)
- **Posts** — create, view, filter by category
- **Comments** — create & view per post
- **Private messages** — real-time via WebSocket, 10 messages loaded at a time, scroll up for more (throttled)
- **Online/Offline** — real-time status via WebSocket
- **Users list** — sorted by last message, then alphabetically
- **Single Page App** — one index.html, all navigation in JavaScript
