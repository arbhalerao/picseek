import argparse
import logging
import os
import signal
import threading
from concurrent import futures
from pathlib import Path

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc
from picseek.ocr.v1 import ocr_pb2, ocr_pb2_grpc

from picseek_ocr import engines, images
from picseek_ocr.lifecycle import ManagedEngine

SERVICE_NAME = "picseek.ocr.v1.OcrService"
MAX_SOCKET_PATH = 107
DRAIN_SECONDS = 10
MAX_MESSAGE_BYTES = 64 * 1024 * 1024

log = logging.getLogger("picseek_ocr")


class OcrServicer(ocr_pb2_grpc.OcrServiceServicer):
    def __init__(self, engine: ManagedEngine) -> None:
        self._engine = engine

    def Recognize(self, request: ocr_pb2.RecognizeRequest, context: grpc.ServicerContext):
        source = request.image.WhichOneof("source")
        try:
            image = images.load(
                path=request.image.path if source == "path" else None,
                data=request.image.data if source == "data" else None,
            )
            lines, engine = self._engine.recognize(image.pixels)
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
            engine=engine.name,
            engine_version=engine.version,
        )

    def Info(self, request: ocr_pb2.InfoRequest, context: grpc.ServicerContext):
        engine = self._engine.describe()
        return ocr_pb2.InfoResponse(engine=engine.name, engine_version=engine.version)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(prog="picseek-ocr")
    parser.add_argument("--socket", required=True, help="unix socket path to listen on")
    parser.add_argument("--engine", choices=["rapidocr", "tesseract"], default="rapidocr")
    parser.add_argument("--model", choices=["tiny", "small"], default="tiny")
    parser.add_argument("--threads", type=int, default=2)
    parser.add_argument("--model-dir", default=str(default_model_dir()))
    parser.add_argument("--idle-unload", type=float, default=300, help="seconds, 0 disables")
    parser.add_argument("--log-level", default="INFO")
    return parser.parse_args()


def default_model_dir() -> Path:
    cache = os.environ.get("XDG_CACHE_HOME") or Path.home() / ".cache"
    return Path(cache) / "picseek" / "models"


def listen_address(path: str) -> str:
    if len(path) > MAX_SOCKET_PATH:
        raise SystemExit(
            f"socket path {path!r} is {len(path)} bytes, the limit is {MAX_SOCKET_PATH}"
        )
    socket = Path(path)
    socket.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    socket.unlink(missing_ok=True)
    return f"unix://{socket}"


def main() -> None:
    args = parse_args()
    logging.basicConfig(
        level=args.log_level.upper(), format="%(asctime)s %(levelname)s %(name)s %(message)s"
    )

    engine = ManagedEngine(
        lambda: engines.build(args.engine, args.model, args.threads, args.model_dir),
        args.idle_unload,
    )
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=4, thread_name_prefix="rpc"),
        options=[
            ("grpc.max_receive_message_length", MAX_MESSAGE_BYTES),
            ("grpc.max_send_message_length", MAX_MESSAGE_BYTES),
        ],
    )
    ocr_pb2_grpc.add_OcrServiceServicer_to_server(OcrServicer(engine), server)
    health_servicer = health.HealthServicer()
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)

    address = listen_address(args.socket)
    server.add_insecure_port(address)
    server.start()
    for name in ("", SERVICE_NAME):
        health_servicer.set(name, health_pb2.HealthCheckResponse.SERVING)
    log.info("listening on %s (engine=%s pid=%d)", address, args.engine, os.getpid())

    draining = threading.Event()
    stopped = threading.Event()

    def shutdown(signum: int, _frame: object) -> None:
        if draining.is_set():
            return
        draining.set()
        log.info("received %s, draining", signal.Signals(signum).name)
        health_servicer.enter_graceful_shutdown()
        engine.close()
        server.stop(DRAIN_SECONDS).wait()
        stopped.set()

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)
    stopped.wait()
    Path(args.socket).unlink(missing_ok=True)
    log.info("stopped")


if __name__ == "__main__":
    main()
