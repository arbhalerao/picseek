import io
from dataclasses import dataclass
from pathlib import Path

import numpy as np
from PIL import Image

MAX_PIXELS = 100_000_000
MAX_SIDE = 4096

Image.MAX_IMAGE_PIXELS = MAX_PIXELS


class InvalidImageError(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class LoadedImage:
    pixels: np.ndarray
    width: int
    height: int
    scale: float


def load(path: str | None = None, data: bytes | None = None) -> LoadedImage:
    if path:
        source: Path | io.BytesIO = Path(path)
        if not source.is_file():
            raise FileNotFoundError(path)
    elif data:
        source = io.BytesIO(data)
    else:
        raise InvalidImageError("image source is empty")

    try:
        with Image.open(source) as im:
            width, height = im.size
            if im.format == "JPEG" and max(width, height) > MAX_SIDE:
                im.draft("RGB", (MAX_SIDE, MAX_SIDE))
            rgb = im.convert("RGB")
    except (Image.DecompressionBombError, Image.UnidentifiedImageError, OSError) as e:
        if isinstance(e, FileNotFoundError):
            raise
        raise InvalidImageError(str(e)) from e

    if max(rgb.size) > MAX_SIDE:
        rgb.thumbnail((MAX_SIDE, MAX_SIDE), Image.Resampling.LANCZOS)
    scale = rgb.size[0] / width
    return LoadedImage(pixels=np.asarray(rgb), width=width, height=height, scale=scale)
