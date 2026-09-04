from dataclasses import dataclass
from typing import Protocol

import numpy as np


@dataclass(frozen=True, slots=True)
class Box:
    x: int
    y: int
    width: int
    height: int


@dataclass(frozen=True, slots=True)
class Line:
    text: str
    box: Box
    confidence: float


class OcrEngine(Protocol):
    name: str
    version: str

    def recognize(self, image: np.ndarray) -> list[Line]: ...
