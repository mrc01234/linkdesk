# Self-Hosted URL Shortener

A simple, self-hosted URL shortener built with Go and SQLite. Create short, easy-to-share links with a clean web interface, REST API, and CLI.

## Features

- Web interface to create, manage, and track shortened URLs
- Custom short codes for memorable links
- QR code generation for any short link
- Click tracking per URL
- REST API for programmatic access
- CLI for scripting and automation
- All data stored locally in SQLite, no external services required
- Single binary, easy to deploy

## Security

> **Read this before exposing the application to the internet.**

This application has **no authentication by default**. Without authentication, anyone who can reach the server can create, view, and delete all URLs.

### Safe to run without authentication

- `localhost` only
- Private or home network with no public exposure
- Behind a VPN restricted to trusted users

### Must enable authentication

- Any deployment with a public IP or public domain name

### Enabling HTTP Basic Auth

Pass `--username` and `--password` at startup. Both flags must be provided together.

```bash
./url-shortener \
  --base-url "https://your-domain.com" \
  --username admin \
  --password your-secret-password
```

Short link redirects (`/{code}`) are always public so that anyone clicking a link is redirected without being prompted for a password. Only the management pages, admin API, and QR endpoints are protected.

### Docker with authentication

Edit `docker-compose.yml` and uncomment the auth lines:

```yaml
services:
  url-shortener:
    build: .
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - DB_PATH=/data/data.db
      - BASE_URL=https://your-domain.com
      - AUTH_USERNAME=admin
      - AUTH_PASSWORD=your-secret-password
    volumes:
      - url-shortener-data:/data
    restart: unless-stopped
```

### HTTPS

HTTP Basic Auth transmits credentials in base64. **Always put the application behind a reverse proxy with TLS in production.**

Caddy (automatic HTTPS):

```
your-domain.com {
    reverse_proxy localhost:8080
}
```

nginx:

```nginx
server {
    listen 443 ssl;
    server_name your-domain.com;
    ssl_certificate     /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

## Installation

### Download binary

Download the latest release from the [releases page](https://github.com/mstgnz/self-hosted-url-shortener/releases).

### Build from source

Requires Go 1.24+ and GCC (for the SQLite driver).

```bash
git clone https://github.com/mstgnz/self-hosted-url-shortener.git
cd self-hosted-url-shortener
go build -o url-shortener ./cmd
```

### Docker

```bash
docker compose up -d
```

## Usage

### Web interface

```bash
./url-shortener --port 8080 --base-url "https://your-domain.com"
```

Open `http://localhost:8080` in your browser.

### REST API

When Basic Auth is enabled, include credentials with every request (`-u username:password`).

#### Shorten a URL

```bash
curl -X POST http://localhost:8080/api/shorten \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/very/long/url", "custom_code": "my-link"}'
```

Response:

```json
{
  "id": 1,
  "short_code": "my-link",
  "long_url": "https://example.com/very/long/url",
  "short_url": "http://localhost:8080/my-link",
  "created_at": "2024-01-01T00:00:00Z",
  "clicks": 0
}
```

Error responses:

| Status | Meaning |
|--------|---------|
| `400 Bad Request` | Missing or invalid URL, invalid custom code format |
| `409 Conflict` | Custom code is already in use |
| `500 Internal Server Error` | Unexpected server error |

#### List all URLs

```bash
curl http://localhost:8080/api/urls
```

#### Get URL details

```bash
curl http://localhost:8080/api/url/my-link
```

#### Delete a URL

```bash
curl -X DELETE http://localhost:8080/api/url/my-link
```

### CLI

```bash
# Shorten
./url-shortener --cli shorten https://example.com/very/long/url
./url-shortener --cli shorten https://example.com/very/long/url --code my-link

# List
./url-shortener --cli list

# Get
./url-shortener --cli get my-link

# Generate QR code
./url-shortener --cli qr my-link --output qr.png

# Delete
./url-shortener --cli delete my-link
```

## Configuration

| Flag | Env variable | Default | Description |
|------|-------------|---------|-------------|
| `--port` | `PORT` | `8080` | HTTP server port |
| `--db` | `DB_PATH` | `data.db` | SQLite database path |
| `--base-url` | `BASE_URL` | `http://localhost:8080` | Base URL used in generated short links |
| `--templates` | | `templates` | Templates directory |
| `--username` | `AUTH_USERNAME` | | Username for HTTP Basic Auth |
| `--password` | `AUTH_PASSWORD` | | Password for HTTP Basic Auth |
| `--cli` | | | Run in CLI mode instead of starting the server |

### Custom short code rules

- Only letters, digits, hyphens (`-`), and underscores (`_`)
- Maximum 50 characters
- The following paths are reserved and cannot be used: `api`, `static`, `urls`, `shorten`, `qr`, `delete`, `favicon.ico`

## Development

```bash
# Run
go run ./cmd

# Test
go test ./...

# Build
go build -o url-shortener ./cmd
```

## License

MIT, see [LICENSE](LICENSE).
