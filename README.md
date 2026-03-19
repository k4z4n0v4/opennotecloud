# OpenNoteCloud

A lightweight, self-hosted cloud sync server for Supernote tablets. Single Go binary, SQLite storage, no external dependencies. Works with unmodified tablet firmware, just point your device at it and go.

This is a focused reimplementation of the Supernote private cloud server covering the tablet-facing sync API. No web UI, no admin panel, no bloat. If you want something small that syncs your notes reliably, this is it.

## Quick Start (Docker)

```bash
git clone https://github.com/k4z4n0v4/opennotecloud.git
cd opennotecloud
docker compose up -d

# Create a user (password-hash is the MD5 hex of your password)
docker compose exec opennotecloud opennotecloud create-user \
  --email=you@example.com \
  --password-hash=$(echo -n 'your-password' | md5sum | cut -d' ' -f1)
```

## Quick Start (Binary)

```bash
go build -o opennotecloud .
./opennotecloud create-user \
  --email=you@example.com \
  --password-hash=$(echo -n 'your-password' | md5sum | cut -d' ' -f1)

OPENNOTE_BASE_URL=https://your-server.example.com ./opennotecloud
```

**Important:** This server was developed and tested by migrating a device that was already registered on the official Supernote private cloud. The migration path is straightforward, just point the tablet at your server and log in. However, fresh device registration (brand new tablet, never connected to any cloud) may not work out of the box since some of the initial activation endpoints haven't been exercised.

## Configuration

All configuration is done through environment variables:

| Variable | Default | Description |
|---|---|---|
| `OPENNOTE_LISTEN` | `:8080` | HTTP listen address |
| `OPENNOTE_DATA_DIR` | `/data/files` | Root directory for file storage |
| `OPENNOTE_DB_PATH` | `/data/opennotecloud.db` | SQLite database path |
| `OPENNOTE_BASE_URL` | `http://localhost:8080` | External URL the tablet uses to reach this server. Must be set correctly, the tablet uses this to construct upload/download URLs. |

The JWT signing secret is auto-generated on first startup and persisted in the database. Storage capacity reported to the tablet is read from the actual filesystem.

## What Works

Everything the tablet needs for day-to-day sync:

- **File sync**: notes, PDFs, documents. Upload, download, move, copy, rename, delete.
- **Folder management**: create, delete, nested folder trees.
- **To-Dos**: full CRUD, task groups, batch updates. Round-trips correctly with the tablet's todo system.
- **Digests**: create, edit, delete, group management, file attachments.
- **Socket.IO keepalive**: the tablet opens a WebSocket connection for push notifications.
- **Chunked uploads**: large files are uploaded in parts and reassembled server-side.

## What Doesn't

These are intentionally not implemented. This is a tablet sync server, not a full cloud platform.

- **Web UI**: no browser-based file management
- **File conversion**: no note-to-PDF or note-to-PNG rendering
- **Sharing**: no share links or collaborative features
- **Recycle bin**: deletes are permanent
- **User registration via API**: users are created with the CLI tool
- **SMS/phone login**: email + password only

## Known Limitations

- The Socket.IO endpoint does not verify the HMAC key parameter (`random`) that the official server checks. Authentication is done via JWT token on the WebSocket connection, which is functionally equivalent, but it's a deviation from the official protocol. If Ratta ever decides to enforce HMAC validation on the client side, this would need to be implemented.

- The official server signs upload/download URLs with a shared secret between the file service and the OSS (object storage) service. This server uses the JWT secret for URL signing since there's no separate storage service, it's all one binary.

## Architecture

~3,000 lines of Go. Flat `package main`, no framework, stdlib `net/http` router.

## License

AGPL-3.0. See [LICENSE](LICENSE).
