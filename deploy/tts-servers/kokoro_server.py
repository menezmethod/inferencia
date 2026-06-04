#!/usr/bin/env python3
"""OpenAI-compatible TTS server wrapping the Kokoro neural TTS engine.
See README.md for setup and usage.

Endpoints:
  POST /v1/audio/speech   — Generate speech from text (lazy-loads Kokoro)
  GET  /health            — Health check
  GET  /v1/models         — List available voices
  GET  /metrics           — Prometheus metrics

Environment:
  PORT  — listen port (default 50051)
  HOST  — listen address (default 127.0.0.1)
"""

from __future__ import annotations

import io
import json
import logging
import os
import signal
import sys
import time
from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager
from typing import Any

import numpy as np
import soundfile as sf

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%S%z",
    stream=sys.stdout,
)
logger = logging.getLogger("kokoro-tts")

# Prometheus (custom registry to avoid conflicts with libs like torch)
METRICS_REGISTRY = None
try:
    from prometheus_client import (
        CONTENT_TYPE_LATEST,
        CollectorRegistry,
        Counter,
        Gauge,
        Histogram,
        generate_latest,
    )
    METRICS_REGISTRY = CollectorRegistry()
    tts_requests_total = Counter(
        "tts_requests_total", "Total TTS requests", ["status"],
        registry=METRICS_REGISTRY,
    )
    tts_request_duration_seconds = Histogram(
        "tts_request_duration_seconds", "TTS request duration in seconds",
        buckets=[0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0],
        registry=METRICS_REGISTRY,
    )
    tts_characters_total = Counter(
        "tts_characters_total", "Total characters processed",
        registry=METRICS_REGISTRY,
    )
    tts_voices_loaded = Gauge(
        "tts_voices_loaded", "Number of voices currently loaded",
        registry=METRICS_REGISTRY,
    )
    METRICS_ENABLED = True
except ImportError:
    METRICS_ENABLED = False
    logger.warning("prometheus_client not installed — /metrics disabled")

from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse, Response
from pydantic import BaseModel, Field

# Lazy Kokoro — loaded on first request, not at import time
_pipeline = None
_voices: list[str] = []
_load_lock = False


def _ensure_pipeline():
    global _pipeline, _voices, _load_lock
    if _pipeline is not None:
        return _pipeline, _voices
    if _load_lock:
        raise RuntimeError("Kokoro is still loading")
    _load_lock = True

    logger.info("Loading Kokoro model (first request)...")
    from kokoro import KPipeline
    _pipeline = KPipeline(lang_code="a")

    _voices = [
        "af_bella", "af_heart", "af_jessica", "af_nicole", "af_nova",
        "af_sarah", "af_sky", "am_adam", "am_michael", "bf_emma",
        "bf_isabella", "bm_george", "bm_lewis", "af_alloy", "am_echo",
        "am_fenrir", "am_liam", "am_onyx", "am_puck", "am_santa", "am_aoede",
    ]
    if METRICS_ENABLED:
        tts_voices_loaded.set(len(_voices))
    logger.info("Kokoro ready with %d voices", len(_voices))
    return _pipeline, _voices


class TTSRequest(BaseModel):
    model: str = "kokoro"
    input: str = Field(..., min_length=1)
    voice: str = Field(default="af_bella")
    response_format: str = Field(default="wav", pattern=r"^(wav|mp3)$")
    speed: float = Field(default=1.0, ge=0.25, le=4.0)


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator[None, None]:
    yield
    logger.info("Shutting down Kokoro TTS server")


app = FastAPI(title="Kokoro TTS Server", version="1.0.0", lifespan=lifespan)


@app.get("/health")
async def health():
    if _pipeline is None:
        return JSONResponse(
            status_code=503,
            content={"status": "unavailable", "engine": "kokoro",
                     "version": "1.0.0",
                     "detail": "Kokoro not loaded (lazy init, hit /v1/models first)"},
        )
    return {"status": "ok", "engine": "kokoro", "version": "1.0.0",
            "voices_loaded": len(_voices)}


@app.get("/v1/models")
async def list_models():
    pipeline, voices = _ensure_pipeline()
    now = int(time.time())
    return {"object": "list", "data": [
        {"id": v, "object": "model", "created": now, "owned_by": "kokoro"}
        for v in voices
    ]}


@app.post("/v1/audio/speech")
async def audio_speech(req: TTSRequest):
    t0 = time.time()
    try:
        pipeline, voices = _ensure_pipeline()
        if req.voice not in voices:
            raise HTTPException(status_code=400,
                                detail=f"Unknown voice '{req.voice}'")

        audio_segs = []
        for result in pipeline(req.input, voice=req.voice, speed=req.speed):
            audio_segs.append(result.audio)
        if not audio_segs:
            raise HTTPException(status_code=500, detail="No audio generated")

        full = np.concatenate(audio_segs)
        buf = io.BytesIO()
        sf.write(buf, full, 24000, format=req.response_format)
        audio = buf.getvalue()

        if METRICS_ENABLED:
            tts_requests_total.labels(status="success").inc()
            tts_request_duration_seconds.observe(time.time() - t0)
            tts_characters_total.inc(len(req.input))

        ct = "audio/mpeg" if req.response_format == "mp3" else "audio/wav"
        return Response(content=audio, media_type=ct)

    except HTTPException:
        raise
    except Exception as exc:
        if METRICS_ENABLED:
            tts_requests_total.labels(status="error").inc()
        logger.error("TTS error: %s: %s", type(exc).__name__, exc)
        raise HTTPException(status_code=500, detail=str(exc))


@app.get("/metrics")
async def prometheus_metrics():
    if not METRICS_ENABLED:
        return JSONResponse(501, {"detail": "prometheus_client not installed"})
    return Response(content=generate_latest(METRICS_REGISTRY),
                    media_type=CONTENT_TYPE_LATEST)


def _handle_sigterm(signum, frame):
    logger.info("SIGTERM, shutting down")
    sys.exit(0)


signal.signal(signal.SIGTERM, _handle_sigterm)

if __name__ == "__main__":
    port = int(os.environ.get("PORT", "50051"))
    host = os.environ.get("HOST", "127.0.0.1")
    import uvicorn
    logger.info("Starting on %s:%s — lazy Kokoro init", host, port)
    uvicorn.run(app, host=host, port=port, log_level="info", access_log=True)
