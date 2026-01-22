# bifrost

A self-hosted WhatsApp Cloud API proxy that uses whatmeow (WhatsApp Web) under the hood. This allows you to use WhatsApp Cloud API compatible clients (like Whatomate) without Meta's official API.

## Features

- **Cloud API Compatible** - Exposes WhatsApp Cloud API compatible endpoints
- **Single Binary** - No external services needed (except SQLite for session storage)
- **Self-Hosted** - Full control, no Meta fees
- **QR Code Pairing** - Simple web-based QR code authentication
- **Session Persistence** - Sessions survive restarts (SQLite storage)

## Supported Features

| Feature | Status |
|---------|--------|
| QR code authentication | ✅ |
| Session persistence | ✅ |
| Send text messages | ✅ |
| Send media (image/video/audio/document) | ✅ |
| Send interactive buttons | ✅ |
| Receive messages | ✅ |
| Message status updates | ✅ |
| Mark as read | ✅ |

## Quick Start

### Docker Compose (Recommended)

The latest image is available on Docker Hub at [`shridh0r/bifrost:latest`](https://hub.docker.com/r/shridh0r/bifrost)

```bash
# Download compose file and sample config
curl -LO https://raw.githubusercontent.com/shridarpatil/bifrost/main/docker-compose.yml
curl -LO https://raw.githubusercontent.com/shridarpatil/bifrost/main/config.example.yaml

# Copy and edit config
cp config.example.yaml config.yaml
# Edit config.yaml with your webhook URL and instance details

# Run
docker compose up -d
```

Open `http://localhost:9000/auth/qr/{phone_id}` to pair WhatsApp.

---

### Build from Source

```bash
# Clone and build
git clone https://github.com/shridarpatil/bifrost.git
cd bifrost
go build -o bifrost .

# Configure
cp config.example.yaml config.yaml
# Edit config.yaml

# Run
./bifrost -config config.yaml
```

### Pair WhatsApp

1. Open `http://localhost:9000/auth/qr/{phone_id}` in your browser
2. Scan the QR code with WhatsApp (Settings → Linked Devices → Link a Device)
3. Wait for "Connected!" confirmation

## API Endpoints

### Authentication (Proxy-specific)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/auth/qr/{phone_id}` | GET | Display QR code for pairing |
| `/auth/qr/{phone_id}?format=json` | GET | Get QR code as JSON (base64 PNG) |
| `/auth/status/{phone_id}` | GET | Check connection status |
| `/auth/logout/{phone_id}` | POST | Disconnect and logout |

### Cloud API Compatible

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v{ver}/{phone_id}/messages` | POST | Send a message |
| `/v{ver}/{phone_id}/media` | POST | Upload media |
| `/v{ver}/{media_id}` | GET | Get media info/URL |
| `/media/{media_id}` | GET | Download media |

### Health Check

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |

## Usage Examples

### Send Text Message

```bash
curl -X POST http://localhost:9000/v1/123456789/messages \
  -H "Content-Type: application/json" \
  -d '{
    "messaging_product": "whatsapp",
    "to": "1234567890",
    "type": "text",
    "text": {
      "body": "Hello from bifrost!"
    }
  }'
```

### Send Image

```bash
curl -X POST http://localhost:9000/v1/123456789/messages \
  -H "Content-Type: application/json" \
  -d '{
    "messaging_product": "whatsapp",
    "to": "1234567890",
    "type": "image",
    "image": {
      "link": "https://example.com/image.jpg",
      "caption": "Check this out!"
    }
  }'
```

### Send Interactive Buttons

```bash
curl -X POST http://localhost:9000/v1/123456789/messages \
  -H "Content-Type: application/json" \
  -d '{
    "messaging_product": "whatsapp",
    "to": "1234567890",
    "type": "interactive",
    "interactive": {
      "type": "button",
      "body": {
        "text": "Choose an option:"
      },
      "action": {
        "buttons": [
          {"type": "reply", "reply": {"id": "btn1", "title": "Option 1"}},
          {"type": "reply", "reply": {"id": "btn2", "title": "Option 2"}}
        ]
      }
    }
  }'
```

### Upload Media

```bash
curl -X POST http://localhost:9000/v1/123456789/media \
  -F "file=@/path/to/image.jpg" \
  -F "messaging_product=whatsapp"
```

## Webhook Format

Incoming messages are forwarded to your webhook URL in Cloud API format:

```json
{
  "object": "whatsapp_business_account",
  "entry": [{
    "id": "123456789",
    "changes": [{
      "value": {
        "messaging_product": "whatsapp",
        "metadata": {
          "display_phone_number": "123456789",
          "phone_number_id": "123456789"
        },
        "contacts": [{
          "wa_id": "1234567890",
          "profile": {"name": "John Doe"}
        }],
        "messages": [{
          "from": "1234567890",
          "id": "wamid.xxx",
          "timestamp": "1234567890",
          "type": "text",
          "text": {"body": "Hello!"}
        }]
      },
      "field": "messages"
    }]
  }]
}
```

## Docker

### Using Docker Compose (Recommended)

See [Quick Start](#docker-compose-recommended) above.

### Manual Docker Run

```bash
# Build locally
docker build -t bifrost .

# Run
docker run -d \
  -p 9000:9000 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/config.yaml:/app/config.yaml \
  bifrost
```

## Integration with Whatomate

Configure Whatomate to use this proxy:

```yaml
whatsapp:
  api_base_url: "http://localhost:9000"
```

## Limitations

- **No Templates** - WhatsApp templates are a Meta-specific feature
- **Single Device** - WhatsApp Web limitation (one linked device per session)
- **Media Size** - 16MB limit (configurable)
- **Unofficial API** - Risk of account restrictions if abused

## License

MIT
