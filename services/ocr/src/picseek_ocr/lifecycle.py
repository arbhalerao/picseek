import gc
import logging
import threading
import time
from collections.abc import Callable

import numpy as np

from picseek_ocr.types import Line, OcrEngine

log = logging.getLogger(__name__)


class ManagedEngine:
    def __init__(self, factory: Callable[[], OcrEngine], idle_unload: float) -> None:
        self._factory = factory
        self._idle_unload = idle_unload
        self._lock = threading.Lock()
        self._engine: OcrEngine | None = None
        self._last_used = time.monotonic()
        self._stop = threading.Event()
        if idle_unload > 0:
            threading.Thread(target=self._reap, name="idle-unload", daemon=True).start()

    def recognize(self, image: np.ndarray) -> tuple[list[Line], OcrEngine]:
        with self._lock:
            engine = self._load()
            try:
                return engine.recognize(image), engine
            finally:
                self._last_used = time.monotonic()

    def describe(self) -> OcrEngine:
        with self._lock:
            return self._load()

    def close(self) -> None:
        self._stop.set()

    def _load(self) -> OcrEngine:
        if self._engine is None:
            started = time.monotonic()
            self._engine = self._factory()
            elapsed = time.monotonic() - started
            log.info("engine %s loaded in %.2fs", self._engine.name, elapsed)
        return self._engine

    def _reap(self) -> None:
        interval = min(30.0, self._idle_unload)
        while not self._stop.wait(interval):
            with self._lock:
                idle = time.monotonic() - self._last_used
                if self._engine is not None and idle >= self._idle_unload:
                    self._engine = None
                    gc.collect()
                    log.info("engine unloaded after %.0fs idle", idle)
