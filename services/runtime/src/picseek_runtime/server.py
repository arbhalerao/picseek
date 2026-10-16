import logging
import os
import signal
import threading
from collections.abc import Callable
from concurrent import futures
from pathlib import Path

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc

MAX_SOCKET_PATH = 107
DRAIN_SECONDS = 10
MAX_MESSAGE_BYTES = 64 * 1024 * 1024

log = logging.getLogger(__name__)


def serve(
    socket: str,
    service_name: str,
    register: Callable[[grpc.Server], None],
    on_shutdown: Callable[[], None] = lambda: None,
    rpc_threads: int = 4,
) -> None:
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=rpc_threads, thread_name_prefix="rpc"),
        options=[
            ("grpc.max_receive_message_length", MAX_MESSAGE_BYTES),
            ("grpc.max_send_message_length", MAX_MESSAGE_BYTES),
        ],
    )
    register(server)
    health_servicer = health.HealthServicer()
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)

    address = listen_address(socket)
    server.add_insecure_port(address)
    server.start()
    for name in ("", service_name):
        health_servicer.set(name, health_pb2.HealthCheckResponse.SERVING)
    log.info("listening on %s (pid=%d)", address, os.getpid())

    draining = threading.Event()
    stopped = threading.Event()

    def shutdown(signum: int, _frame: object) -> None:
        if draining.is_set():
            return
        draining.set()
        log.info("received %s, draining", signal.Signals(signum).name)
        health_servicer.enter_graceful_shutdown()
        on_shutdown()
        server.stop(DRAIN_SECONDS).wait()
        stopped.set()

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)
    stopped.wait()
    Path(socket).unlink(missing_ok=True)
    log.info("stopped")


def listen_address(path: str) -> str:
    if len(path) > MAX_SOCKET_PATH:
        raise SystemExit(
            f"socket path {path!r} is {len(path)} bytes, the limit is {MAX_SOCKET_PATH}"
        )
    socket = Path(path)
    socket.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    socket.unlink(missing_ok=True)
    return f"unix://{socket}"


def default_model_dir() -> Path:
    cache = os.environ.get("XDG_CACHE_HOME") or Path.home() / ".cache"
    return Path(cache) / "picseek" / "models"


def configure_logging(level: str) -> None:
    logging.basicConfig(
        level=level.upper(), format="%(asctime)s %(levelname)s %(name)s %(message)s"
    )
