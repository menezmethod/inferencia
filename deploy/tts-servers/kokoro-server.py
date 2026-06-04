#!/usr/bin/env python3
"""
Kokoro TTS HTTP Server
======================
OpenAI-compatible TTS endpoint backed by Kokoro (https://github.com/remsky/Kokoro-FastAPI).

Endpoints:
  POST /v1/audio/speech   — Generate speech from text
  GET  /health            — Health check
  GET  /v1/models         — List available voices
  GET  /metrics           — Prometheus metrics

Environment variables:
  PORT  — listen port (default 50051)
  HOST  — listen address (default 127.0.0.1; change to 0.0.0.0 for LAN)
"""

from __future__ import annotations

import logging
import os
import signal
import sys
import time
from contextlib import asynccontextmanager
from typing import AsyncGenerator

import uvicorn
from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.responses import JSONResponse, StreamingResponse
from pydantic import BaseModel, Field

# ---------------------------------------------------------------------------
# Prometheus metrics  (prometheus_client library)
# ---------------------------------------------------------------------------
try:
    from prometheus_client import Counter, Histogram, Gauge, generate_latest, CONTENT_TYPE_LATEST

    METRICS_ENABLED = True

    tts_requests_total = Counter(
        "tts_requests_total",
        "Total number of TTS requests",
        ["status"],  # "success", "error"
    )
    tts_request_duration_seconds = Histogram(
        "tts_request_duration_seconds",
        "TTS request duration in seconds",
        buckets=[0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0],
    )
    tts_characters_total = Counter(
        "tts_characters_total",
        "Total characters processed",
    )
    tts_voices_loaded = Gauge(
        "tts_voices_loaded",
        "Number of voices currently loaded in Kokoro",
    )
except ImportError:
    METRICS_ENABLED = False

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%S%z",
    stream=sys.stdout,
)
logger = logging.getLogger("kokoro-tts")

# ---------------------------------------------------------------------------
# Kokoro initialisation (deferred so we can fail gracefully)
# ---------------------------------------------------------------------------
kokoro_pipeline = None
available_voices: list[str] = []


def _load_kokoro() -> tuple[object, list[str]]:
    """Import and initialise Kokoro.  Returns (pipeline, voice_list)."""
    try:
        from kokoro import KPipeline
    except ImportError as exc:
        raise RuntimeError(
            "Kokoro is not installed. Run: pip install kokoro"
        ) from exc

    logger.info("Initialising Kokoro KPipeline...")
    pipeline = KPipeline(lang_code="a")  # 'a' = American English

    # Discover available voices from the built-in voice pack.
    # Kokoro ships voice .pt files; we enumerate via the pipeline's voice list.
    voices = sorted(pipeline.voices) if hasattr(pipeline, "voices") else []

    # Fallback: known voices from the Kokoro-FastAPI project
    if not voices:
        voices = [
            "af_bella",
            "af_heart",
            "af_jessica",
            "af_nicole",
            "af_nova",
            "af_sarah",
            "af_sky",
            "am_adam",
            "am_michael",
            "bf_emma",
            "bf_isabella",
            "bm_george",
            "bm_lewis",
            "af_alloy",
            "am_echo",
            "am_fenrir",
            "am_liam",
            "am_onyx",
            "am_puck",
            "am_santa",
            "am_aoede",
        ]

    if METRICS_ENABLED:
        tts_voices_loaded.set(len(voices))

    logger.info("Kokoro initialised with %d voices", len(voices))
    return pipeline, voices


def _validate_voice(voice: str) -> str:
    """Return the canonical voice ID or raise 400."""
    if voice in available_voices:
        return voice
    # Allow partial match (e.g. "bella" -> "af_bella")
    matches = [v for v in available_voices if voice in v]
    if len(matches) == 1:
        return matches[0]
    raise HTTPException(
        status_code=400,
        detail=f"Unknown voice '{voice}'. Available: {', '.join(available_voices[:10])}…",
    )


# ---------------------------------------------------------------------------
# Lifespan
# ---------------------------------------------------------------------------
@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator[None, None]:
    global kokoro_pipeline, available_voices
    try:
        kokoro_pipeline, available_voices = _load_kokoro()
    except RuntimeError as exc:
        logger.critical("Failed to load Kokoro: %s", exc)
        # The server will still start, but /health will return 503.
    yield
    # Graceful shutdown — nothing special needed for Kokoro.
    logger.info("Shutting down Kokoro TTS server")


# ---------------------------------------------------------------------------
# FastAPI app
# ---------------------------------------------------------------------------
app = FastAPI(
    title="Kokoro TTS Server",
    version="1.0.0",
    lifespan=lifespan,
)


# ---------------------------------------------------------------------------
# Request / Response models (OpenAI-compatible)
# ---------------------------------------------------------------------------
class TTSRequest(BaseModel):
    model: str = "kokoro"
    input: str = Field(..., min_length=1, description="Text to synthesise")
    voice: str = Field(default="af_bella", description="Voice ID")
    response_format: str = Field(default="wav", pattern=r"^(wav|mp3)$")
    speed: float = Field(default=1.0, ge=0.25, le=4.0)


class ModelInfo(BaseModel):
    id: str
    object: str = "model"
    created: int
    owned_by: str = "kokoro"


class ModelsResponse(BaseModel):
    object: str = "list"
    data: list[ModelInfo]


# ---------------------------------------------------------------------------
# Routes
# ---------------------------------------------------------------------------
@app.get("/health")
async def health():
    if kokoro_pipeline is None:
        return JSONResponse(
            status_code=503,
            content={
                "status": "unavailable",
                "engine": "kokoro",
                "version": "1.0.0",
                "detail": "Kokoro pipeline not loaded (check server logs)",
            },
        )
    return {
        "status": "ok",
        "engine": "kokoro",
        "version": "1.0.0",
        "voices_loaded": len(available_voices),
    }


@app.get("/v1/models")
async def list_models():
    """Return available voices as OpenAI-compatible model list."""
    now = int(time.time())
    models = [
        ModelInfo(id=voice, created=now, owned_by="kokoro")
        for voice in available_voices
    ]
    return ModelsResponse(data=models)


@app.post("/v1/audio/speech")
async def speech(request: TTSRequest):
    """Generate speech audio from text."""
    if kokoro_pipeline is None:
        raise HTTPException(status_code=503, detail="Kokoro not initialised")

    voice = _validate_voice(request.voice)
    text = request.input
    speed = request.speed

    start = time.monotonic()
    status_label = "success"

    try:
        logger.info(
            "TTS request: voice=%s chars=%d speed=%.2f",
            voice,
            len(text),
            speed,
        )

        # Kokoro's KPipeline yields (gs, (sr, audio)) tuples.
        # We concatenate all audio chunks into one WAV.
        import io
        import soundfile as sf
        import numpy as np

        generator = kokoro_pipeline(
            text,
            voice=voice,
            speed=speed,
        )

        audio_chunks: list[np.ndarray] = []
        sample_rate = 24000  # Kokoro default

        for gs, (sr, audio) in generator:
            if sr:
                sample_rate = sr
            if audio is not None and len(audio) > 0:
                audio_chunks.append(audio)

        if not audio_chunks:
            raise HTTPException(status_code=500, detail="No audio generated")

        full_audio = np.concatenate(audio_chunks, axis=0)

        buf = io.BytesIO()
        sf.write(buf, full_audio, sample_rate, format="WAV")
        audio_bytes = buf.getvalue()

        duration = time.monotonic() - start

        if METRICS_ENABLED:
            tts_characters_total.inc(len(text))
            tts_request_duration_seconds.observe(duration)

        logger.info(
            "TTS success: voice=%s chars=%d duration=%.2fs audio_size=%d",
            voice,
            len(text),
            duration,
            len(audio_bytes),
        )

        return Response(
            content=audio_bytes,
            media_type="audio/wav",
            headers={
                "X-TTS-Chars": str(len(text)),
                "X-TTS-Duration-S": f"{duration:.3f}",
                "X-TTS-Voice": voice,
            },
        )

    except Exception:
        status_label = "error"
        logger.exception("TTS generation failed")
        raise HTTPException(status_code=500, detail="TTS generation failed")
    finally:
        if METRICS_ENABLED:
            tts_requests_total.labels(status=status_label).inc()


@app.get("/metrics")
async def prometheus_metrics():
    if not METRICS_ENABLED:
        return JSONResponse(
            status_code=501,
            content={"detail": "prometheus_client not installed"},
        )
    data = generate_latest()
    return Response(content=data, media_type=CONTENT_TYPE_LATEST)


# ---------------------------------------------------------------------------
# Graceful shutdown
# ---------------------------------------------------------------------------
def _handle_sigterm(signum: int, frame: object) -> None:
    logger.info("Received SIGTERM, shutting down...")
    sys.exit(0)


signal.signal(signal.SIGTERM, _handle_sigterm)

# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    port = int(os.environ.get("PORT", "50051"))
    host = os.environ.get("HOST", "127.0.0.1")

    logger.info("Starting Kokoro TTS server on %s:%s", host, port)

    uvicorn.run(
        "kokoro-server:app",
        host=host,
        port=port,
        reload=os.environ.get("RELOAD", "").lower() in ("1", "true", "yes"),
        log_level="info",
        access_log=True,
    )
