import gc
import logging
import threading
import time
from collections.abc import Callable, Iterator
from contextlib import contextmanager

log = logging.getLogger(__name__)


class Lazy[T]:
    def __init__(self, name: str, factory: Callable[[], T], idle_unload: float) -> None:
        self._name = name
        self._factory = factory
        self._idle_unload = idle_unload
        self._lock = threading.Lock()
        self._value: T | None = None
        self._last_used = time.monotonic()
        self._stop = threading.Event()
        if idle_unload > 0:
            threading.Thread(target=self._reap, name=f"unload-{name}", daemon=True).start()

    @contextmanager
    def use(self) -> Iterator[T]:
        with self._lock:
            try:
                yield self._load()
            finally:
                self._last_used = time.monotonic()

    def close(self) -> None:
        self._stop.set()

    def _load(self) -> T:
        if self._value is None:
            started = time.monotonic()
            self._value = self._factory()
            log.info("%s loaded in %.2fs", self._name, time.monotonic() - started)
        return self._value

    def _reap(self) -> None:
        interval = min(30.0, self._idle_unload)
        while not self._stop.wait(interval):
            with self._lock:
                idle = time.monotonic() - self._last_used
                if self._value is not None and idle >= self._idle_unload:
                    self._value = None
                    gc.collect()
                    log.info("%s unloaded after %.0fs idle", self._name, idle)
