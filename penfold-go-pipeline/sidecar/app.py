"""MLX Embeddings Sidecar Service.

A minimal FastAPI service that provides embedding generation using mlx-embeddings
with the mxbai-embed-large-v1 model (768 dimensions).
"""

import logging
from typing import List, Union

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from prometheus_fastapi_instrumentator import Instrumentator

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(
    title="MLX Embeddings Sidecar",
    description="Embedding generation service using mlx-embeddings",
    version="1.0.0",
)

# Prometheus metrics - exposes /metrics endpoint
Instrumentator().instrument(app).expose(app)

# Model instance (lazy loaded)
_model = None
_tokenizer = None
_model_name = "mixedbread-ai/mxbai-embed-large-v1"


def get_model():
    """Lazy load the embedding model and tokenizer."""
    global _model, _tokenizer
    if _model is None:
        logger.info(f"Loading model: {_model_name}")
        import mlx_embeddings
        _model, _tokenizer = mlx_embeddings.load(_model_name)
        logger.info("Model loaded successfully")
    return _model, _tokenizer


class EmbeddingRequest(BaseModel):
    """Request schema for embeddings endpoint."""
    input: Union[str, List[str]]
    model: str = "mxbai-embed-large-v1"


class EmbeddingData(BaseModel):
    """Single embedding in response."""
    object: str = "embedding"
    embedding: List[float]
    index: int


class EmbeddingResponse(BaseModel):
    """Response schema for embeddings endpoint."""
    object: str = "list"
    data: List[EmbeddingData]
    model: str
    usage: dict


class HealthResponse(BaseModel):
    """Health check response."""
    status: str
    model: str
    model_loaded: bool


@app.on_event("startup")
async def startup_event():
    """Pre-load the model on startup."""
    try:
        get_model()
        logger.info("Model pre-loaded successfully")
    except Exception as e:
        logger.warning(f"Failed to pre-load model: {e}")


@app.post("/v1/embeddings", response_model=EmbeddingResponse)
async def create_embeddings(request: EmbeddingRequest):
    """Generate embeddings for input text(s).

    Args:
        request: EmbeddingRequest with input text(s) and optional model name

    Returns:
        EmbeddingResponse with embeddings for each input
    """
    try:
        import mlx_embeddings
        model, tokenizer = get_model()

        # Normalize input to list
        texts = request.input
        if isinstance(texts, str):
            texts = [texts]

        if not texts:
            raise HTTPException(status_code=400, detail="Input cannot be empty")

        logger.info(f"Generating embeddings for {len(texts)} text(s)")

        # Generate embeddings using mlx_embeddings.generate
        result = mlx_embeddings.generate(model, tokenizer, texts)

        # Extract embeddings from the result (text_embeds contains normalized embeddings)
        import numpy as np
        embeddings = np.array(result.text_embeds)

        # Build response
        data = []
        for i, emb in enumerate(embeddings):
            data.append(EmbeddingData(
                embedding=emb.tolist(),
                index=i,
            ))

        total_tokens = sum(len(t.split()) for t in texts)  # Approximate token count

        return EmbeddingResponse(
            data=data,
            model=request.model,
            usage={
                "prompt_tokens": total_tokens,
                "total_tokens": total_tokens,
            },
        )

    except Exception as e:
        logger.error(f"Error generating embeddings: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Check service health and model status."""
    global _model
    return HealthResponse(
        status="ok",
        model=_model_name,
        model_loaded=_model is not None,
    )


@app.get("/")
async def root():
    """Root endpoint with service info."""
    return {
        "service": "MLX Embeddings Sidecar",
        "version": "1.0.0",
        "model": _model_name,
        "endpoints": {
            "embeddings": "/v1/embeddings",
            "health": "/health",
        },
    }


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8001)
