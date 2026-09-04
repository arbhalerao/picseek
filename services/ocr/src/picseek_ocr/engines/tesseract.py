import os
from collections import defaultdict

import numpy as np
from PIL import Image, ImageOps

from picseek_ocr.types import Box, Line

DARK_MEAN = 110
UPSCALE_BELOW = 1500


class TesseractEngine:
    name = "tesseract"

    def __init__(self, threads: int) -> None:
        import pytesseract

        os.environ["OMP_THREAD_LIMIT"] = str(threads)
        self._tesseract = pytesseract
        self.version = str(pytesseract.get_tesseract_version())

    def recognize(self, image: np.ndarray) -> list[Line]:
        gray = Image.fromarray(image).convert("L")
        if np.asarray(gray).mean() < DARK_MEAN:
            gray = ImageOps.invert(gray)
        factor = 2 if max(gray.size) < UPSCALE_BELOW else 1
        if factor > 1:
            size = (gray.width * factor, gray.height * factor)
            gray = gray.resize(size, Image.Resampling.LANCZOS)

        data = self._tesseract.image_to_data(
            gray, lang="eng", output_type=self._tesseract.Output.DICT
        )
        words: dict[tuple[int, int, int], list[int]] = defaultdict(list)
        for i, text in enumerate(data["text"]):
            if text.strip() and float(data["conf"][i]) >= 0:
                words[(data["block_num"][i], data["par_num"][i], data["line_num"][i])].append(i)

        lines = []
        for indices in words.values():
            left = min(data["left"][i] for i in indices)
            top = min(data["top"][i] for i in indices)
            right = max(data["left"][i] + data["width"][i] for i in indices)
            bottom = max(data["top"][i] + data["height"][i] for i in indices)
            box = Box(
                x=left // factor,
                y=top // factor,
                width=(right - left) // factor,
                height=(bottom - top) // factor,
            )
            text = " ".join(data["text"][i] for i in indices)
            confidence = sum(float(data["conf"][i]) for i in indices) / len(indices) / 100
            lines.append(Line(text=text, box=box, confidence=confidence))
        return lines
