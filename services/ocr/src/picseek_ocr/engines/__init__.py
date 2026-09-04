from picseek_ocr.types import OcrEngine


def build(name: str, model: str, threads: int, model_dir: str) -> OcrEngine:
    if name == "rapidocr":
        from picseek_ocr.engines.rapid import RapidEngine

        return RapidEngine(model=model, threads=threads, model_dir=model_dir)
    if name == "tesseract":
        from picseek_ocr.engines.tesseract import TesseractEngine

        return TesseractEngine(threads=threads)
    raise ValueError(f"unknown engine {name!r}")
