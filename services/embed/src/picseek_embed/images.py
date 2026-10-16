import io
from pathlib import Path

from PIL import Image

MAX_PIXELS = 100_000_000
DECODE_SIZE = 448

Image.MAX_IMAGE_PIXELS = MAX_PIXELS


class InvalidImageError(ValueError):
    pass


def load(path: str | None = None, data: bytes | None = None) -> Image.Image:
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
            if im.format == "JPEG":
                im.draft("RGB", (DECODE_SIZE, DECODE_SIZE))
            rgb = im.convert("RGB")
    except FileNotFoundError:
        raise
    except (Image.DecompressionBombError, Image.UnidentifiedImageError, OSError) as e:
        raise InvalidImageError(str(e)) from e
    rgb.thumbnail((DECODE_SIZE, DECODE_SIZE), Image.Resampling.BICUBIC)
    return rgb
