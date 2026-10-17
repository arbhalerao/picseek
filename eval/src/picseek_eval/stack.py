import os
import signal
import subprocess
import time
from contextlib import contextmanager
from pathlib import Path

import grpc
from picseek.seekd.v1 import seekd_pb2, seekd_pb2_grpc

REPO = Path(__file__).resolve().parents[3]


def write_config(path: Path, corpus: Path, socket_dir: Path, thresholds: dict[str, float]) -> None:
    path.write_text(
        f"""[index]
folders = ["{corpus}"]

[ocr]
workers = 3

[search]
default_limit = 20
semantic_min = {thresholds["semantic_min"]}
semantic_margin = {thresholds["semantic_margin"]}
visual_min = {thresholds["visual_min"]}
visual_margin = {thresholds["visual_margin"]}
type_min = {thresholds["type_min"]}

[services]
socket_dir = "{socket_dir}"
"""
    )


@contextmanager
def running(config: Path, data_dir: Path, socket_dir: Path, log: Path):
    env = {**os.environ, "XDG_DATA_HOME": str(data_dir), "PICSEEK_SOCKET_DIR": str(socket_dir)}
    with log.open("ab") as out:
        proc = subprocess.Popen(
            [str(REPO / "scripts" / "dev.sh"), "-config", str(config)],
            cwd=REPO,
            env=env,
            stdout=out,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
    channel = grpc.insecure_channel(f"unix://{socket_dir}/seekd.sock")
    try:
        yield Stack(seekd_pb2_grpc.SeekdServiceStub(channel), proc, log)
    finally:
        channel.close()
        os.killpg(proc.pid, signal.SIGTERM)
        try:
            proc.wait(timeout=30)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGKILL)


class Stack:
    def __init__(self, stub, proc: subprocess.Popen, log: Path) -> None:
        self.stub = stub
        self.proc = proc
        self.log = log

    def check_alive(self) -> None:
        if self.proc.poll() is not None:
            tail = self.log.read_text(errors="replace").splitlines()[-5:]
            raise RuntimeError("stack exited early:\n" + "\n".join(tail))


def wait_until_indexed(stack: Stack, expected: int, timeout: float = 900) -> dict:
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        stack.check_alive()
        try:
            st = stack.stub.Status(seekd_pb2.StatusRequest(), timeout=5).status
        except grpc.RpcError:
            time.sleep(1)
            continue
        healthy = all(s.healthy == s.total for s in st.services)
        queued = st.queued_live + st.queued_backfill + st.queued_enrich
        last = {"indexed": st.indexed, "queued": queued, "failed": st.failed, "healthy": healthy}
        if healthy and st.indexed >= expected and queued == 0:
            return last
        time.sleep(2)
    raise TimeoutError(f"stack did not finish indexing: {last}")


def search(stub, text: str, layers: list[str], limit: int = 50) -> list[tuple[str, float]]:
    kinds = {
        "exact": seekd_pb2.MATCH_KIND_EXACT,
        "near": seekd_pb2.MATCH_KIND_NEAR,
        "meaning": seekd_pb2.MATCH_KIND_MEANING,
        "visual": seekd_pb2.MATCH_KIND_VISUAL,
    }
    resp = stub.Search(
        seekd_pb2.SearchRequest(query=text, limit=limit, layers=[kinds[layer] for layer in layers]),
        timeout=30,
    )
    return [(Path(h.path).name, h.score) for h in resp.hits]
