from collections.abc import Iterable
from dataclasses import dataclass
from importlib.metadata import version

import numpy as np
from fastembed import ImageEmbedding, TextEmbedding
from PIL import Image


@dataclass(frozen=True, slots=True)
class ModelSpec:
    name: str
    dimensions: int

    @property
    def version(self) -> str:
        return f"fastembed-{version('fastembed')}"


TEXT = ModelSpec("sentence-transformers/all-MiniLM-L6-v2", 384)
CLIP_IMAGE = ModelSpec("Qdrant/clip-ViT-B-32-vision", 512)
CLIP_TEXT = ModelSpec("Qdrant/clip-ViT-B-32-text", 512)


class TextEncoder:
    def __init__(self, spec: ModelSpec, model_dir: str, threads: int) -> None:
        self._model = TextEmbedding(spec.name, cache_dir=model_dir, threads=threads)

    def encode(self, texts: list[str]) -> np.ndarray:
        return normalize(np.array(list(self._model.embed(texts, batch_size=32))))


class ImageEncoder:
    def __init__(self, spec: ModelSpec, model_dir: str, threads: int) -> None:
        self._model = ImageEmbedding(spec.name, cache_dir=model_dir, threads=threads)

    def encode(self, images: Iterable[Image.Image]) -> np.ndarray:
        return normalize(np.array(list(self._model.embed(list(images), batch_size=16))))


def normalize(vectors: np.ndarray) -> np.ndarray:
    norms = np.linalg.norm(vectors, axis=1, keepdims=True)
    return vectors / np.where(norms == 0, 1, norms)
