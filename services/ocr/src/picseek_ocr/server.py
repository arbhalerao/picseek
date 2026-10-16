import argparse

import grpc
from picseek.ocr.v1 import ocr_pb2, ocr_pb2_grpc
from picseek_runtime import Lazy, configure_logging, default_model_dir, serve

from picseek_ocr import engines, images
from picseek_ocr.types import OcrEngine

SERVICE_NAME = "picseek.ocr.v1.OcrService"


class OcrServicer(ocr_pb2_grpc.OcrServiceServicer):
    def __init__(self, engine: Lazy[OcrEngine]) -> None:
        self._engine = engine

    def Recognize(self, request: ocr_pb2.RecognizeRequest, context: grpc.ServicerContext):
        source = request.image.WhichOneof("source")
        try:
            image = images.load(
                path=request.image.path if source == "path" else None,
                data=request.image.data if source == "data" else None,
            )
            with self._engine.use() as engine:
                lines = engine.recognize(image.pixels)
                name, engine_version = engine.name, engine.version
        except FileNotFoundError as e:
            context.abort(grpc.StatusCode.NOT_FOUND, f"file not found: {e}")
        except images.InvalidImageError as e:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(e))

        inverse = 1 / image.scale
        return ocr_pb2.RecognizeResponse(
            lines=[
                ocr_pb2.Line(
                    text=line.text,
                    confidence=line.confidence,
                    box=ocr_pb2.Box(
                        x=round(line.box.x * inverse),
                        y=round(line.box.y * inverse),
                        width=round(line.box.width * inverse),
                        height=round(line.box.height * inverse),
                    ),
                )
                for line in lines
            ],
            width=image.width,
            height=image.height,
            engine=name,
            engine_version=engine_version,
        )

    def Info(self, request: ocr_pb2.InfoRequest, context: grpc.ServicerContext):
        with self._engine.use() as engine:
            return ocr_pb2.InfoResponse(engine=engine.name, engine_version=engine.version)


def main() -> None:
    parser = argparse.ArgumentParser(prog="picseek-ocr")
    parser.add_argument("--socket", required=True, help="unix socket path to listen on")
    parser.add_argument("--engine", choices=["rapidocr", "tesseract"], default="rapidocr")
    parser.add_argument("--model", choices=["tiny", "small"], default="tiny")
    parser.add_argument("--threads", type=int, default=2)
    parser.add_argument("--model-dir", default=str(default_model_dir()))
    parser.add_argument("--idle-unload", type=float, default=300, help="seconds, 0 disables")
    parser.add_argument("--log-level", default="INFO")
    args = parser.parse_args()
    configure_logging(args.log_level)

    engine = Lazy(
        args.engine,
        lambda: engines.build(args.engine, args.model, args.threads, args.model_dir),
        args.idle_unload,
    )
    serve(
        args.socket,
        SERVICE_NAME,
        lambda server: ocr_pb2_grpc.add_OcrServiceServicer_to_server(OcrServicer(engine), server),
        on_shutdown=engine.close,
    )


if __name__ == "__main__":
    main()
