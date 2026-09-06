from importlib.metadata import version

import numpy as np
from rapidocr import LangDet, LangRec, ModelType, OCRVersion, RapidOCR

from picseek_ocr.types import Box, Line

MODEL_TYPES = {"tiny": ModelType.TINY, "small": ModelType.SMALL}


class RapidEngine:
    name = "rapidocr"

    def __init__(self, model: str, threads: int, model_dir: str) -> None:
        if model not in MODEL_TYPES:
            raise ValueError(f"unknown rapidocr model {model!r}, expected {list(MODEL_TYPES)}")
        self.version = f"{version('rapidocr')}/PP-OCRv6-{model}"
        self._ocr = RapidOCR(
            params={
                "Global.use_cls": False,
                "Global.log_level": "error",
                "Global.model_root_dir": model_dir,
                "Det.ocr_version": OCRVersion.PPOCRV6,
                "Det.model_type": MODEL_TYPES[model],
                "Det.lang_type": LangDet.EN,
                "Det.limit_type": "max",
                "Det.limit_side_len": 1280,
                "Rec.ocr_version": OCRVersion.PPOCRV6,
                "Rec.model_type": MODEL_TYPES[model],
                "Rec.lang_type": LangRec.EN,
                "EngineConfig.onnxruntime.intra_op_num_threads": threads,
                "EngineConfig.onnxruntime.inter_op_num_threads": 1,
            }
        )

    def recognize(self, image: np.ndarray) -> list[Line]:
        result = self._ocr(image)
        if result.boxes is None or result.txts is None or result.scores is None:
            return []
        lines = []
        for polygon, text, score in zip(result.boxes, result.txts, result.scores, strict=True):
            xs = [p[0] for p in polygon]
            ys = [p[1] for p in polygon]
            x, y = int(min(xs)), int(min(ys))
            box = Box(x=x, y=y, width=int(max(xs)) - x, height=int(max(ys)) - y)
            lines.append(Line(text=text, box=box, confidence=float(score)))
        return lines
