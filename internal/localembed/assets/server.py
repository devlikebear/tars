"""OpenAI-compatible /v1/embeddings server that TARS runs for memory_embed_provider: local.

Written by TARS (internal/localembed); edits are overwritten on the next start.
"""

import os
import sys
from typing import List, Union

import torch
import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer

MODEL_ID = os.environ["TARS_EMBED_MODEL"]
HOST = os.environ.get("TARS_EMBED_HOST", "127.0.0.1")
PORT = int(os.environ["TARS_EMBED_PORT"])
DEVICE = "mps" if torch.backends.mps.is_available() else ("cuda" if torch.cuda.is_available() else "cpu")

model = SentenceTransformer(MODEL_ID, trust_remote_code=True, device=DEVICE)
print(f"tars-local-embed: loaded {MODEL_ID} on {DEVICE}", flush=True)

app = FastAPI()


class EmbeddingRequest(BaseModel):
    model: str = MODEL_ID
    input: Union[str, List[str]]


@app.get("/health")
def health():
    return {"ok": True, "model": MODEL_ID, "device": DEVICE, "pid": os.getpid()}


@app.post("/v1/embeddings")
def embeddings(req: EmbeddingRequest):
    texts = [req.input] if isinstance(req.input, str) else req.input
    if not texts:
        raise HTTPException(status_code=400, detail="input is empty")
    vectors = model.encode(texts, normalize_embeddings=True, convert_to_numpy=True)
    return {
        "object": "list",
        "model": MODEL_ID,
        "data": [{"object": "embedding", "index": i, "embedding": v.tolist()} for i, v in enumerate(vectors)],
    }


if __name__ == "__main__":
    sys.stdout.reconfigure(line_buffering=True)
    uvicorn.run(app, host=HOST, port=PORT, log_level="warning")
