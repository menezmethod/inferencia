# Kokoro TTS Server

Production-ready HTTP server wrapping [Kokoro](https://github.com/remsky/Kokoro-FastAPI) TTS as an OpenAI-compatible API. Designed to run 24/7 on the Mac Mini M4 Max (`192.168.0.109`) and serve TTS requests from **inferencia** (Go binary on Raspberry Pi 5) over LAN.

## Architecture

```
 Pi5 (inferencia)  ──HTTP──►  Mac M4 Max (kokoro-server)
 192.168.0.108                 192.168.0.109:50051
```

- inferencia connects via HTTP — **not** a subprocess call.
- The server listens on `127.0.0.1:50051` by default (localhost only for security).
- To expose to LAN, set `HOST=0.0.0.0` in the environment or edit the plist.

## Files

| File | Purpose |
|---|---|
| `kokoro-server.py` | FastAPI server with OpenAI-compatible `/v1/audio/speech` |
| `requirements.txt` | Python dependencies |
| `com.menez.kokoro-tts.plist` | macOS launchd service definition |
| `README.md` | This file |

## Quick Start

### 1. Install dependencies

```bash
cd /Users/luisgimenez/Development/04-infrastructure/llm-local-proxy/inferencia/deploy/tts-servers
python3 -m venv venv
source venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
```

### 2. Test manually

```bash
python3 kokoro-server.py
```

Then in another terminal:

```bash
# Health check
curl http://127.0.0.1:50051/health

# List voices
curl http://127.0.0.1:50051/v1/models | jq .

# Generate speech
curl -X POST http://127.0.0.1:50051/v1/audio/speech \
  -H "Content-Type: application/json" \
  -d '{"input":"Hello from Kokoro!","voice":"af_bella"}' \
  --output test.wav

# Metrics
curl http://127.0.0.1:50051/metrics
```

### 3. Install as launchd service (starts on login, auto-restart)

```bash
# Create log directory
mkdir -p ~/Library/Logs/kokoro-tts

# Load the service
launchctl load ~/Library/LaunchAgents/com.menez.kokoro-tts.plist

# Check status
launchctl list com.menez.kokoro-tts

# Tail logs
tail -f ~/Library/Logs/kokoro-tts/stdout.log
tail -f ~/Library/Logs/kokoro-tts/stderr.log
```

### 4. Managing the service

```bash
# Unload (stop)
launchctl unload ~/Library/LaunchAgents/com.menez.kokoro-tts.plist

# Restart
launchctl unload ~/Library/LaunchAgents/com.menez.kokoro-tts.plist
launchctl load   ~/Library/LaunchAgents/com.menez.kokoro-tts.plist

# Check if running
launchctl list | grep kokoro
```

## API Endpoints

### POST `/v1/audio/speech`

OpenAI-compatible TTS request.

**Request body:**
```json
{
  "model": "kokoro",
  "input": "Text to speak",
  "voice": "af_bella",
  "response_format": "wav",
  "speed": 1.0
}
```

| Field | Default | Description |
|---|---|---|
| `model` | `kokoro` | Ignored (always uses Kokoro) |
| `input` | — | Text to synthesise (required) |
| `voice` | `af_bella` | Voice ID (see `/v1/models`) |
| `response_format` | `wav` | `wav` or `mp3` |
| `speed` | `1.0` | Playback speed (0.25–4.0) |

**Response:** Binary WAV audio with `Content-Type: audio/wav`.

### GET `/health`

```json
{"status":"ok","engine":"kokoro","version":"1.0.0","voices_loaded":21}
```

Returns `503` if Kokoro failed to load.

### GET `/v1/models`

OpenAI-compatible model list — each entry is a voice.

```json
{"object":"list","data":[{"id":"af_bella","object":"model","created":...,"owned_by":"kokoro"},...]}
```

### GET `/metrics`

Prometheus text-format metrics (requires `prometheus-client`):

- `tts_requests_total{status="success|error"}`
- `tts_request_duration_seconds` (histogram)
- `tts_characters_total`
- `tts_voices_loaded`

## Configuration

| Env var | Default | Description |
|---|---|---|
| `PORT` | `50051` | HTTP listen port |
| `HOST` | `127.0.0.1` | Listen address (`0.0.0.0` for LAN access) |
| `RELOAD` | (unset) | Enable auto-reload for development (`1`/`true`) |

## Voice Reference

Kokoro voices follow the pattern `<gender><lang>_<name>`:

- `af_*` — American English female
- `am_*` — American English male
- `bf_*` — British English female
- `bm_*` — British English male

Popular voices: `af_bella`, `af_heart`, `af_nicole`, `am_adam`, `am_michael`, `bf_emma`, `bm_george`.

## Troubleshooting

**Kokoro not importable:**
```bash
source venv/bin/activate
pip install kokoro
```

**Port already in use:**
```bash
lsof -i :50051
kill <PID>
```

**launchd not starting:**
```bash
# Check launchd error log
cat ~/Library/Logs/kokoro-tts/stderr.log

# Validate plist
plutil -lint ~/Library/LaunchAgents/com.menez.kokoro-tts.plist
```
