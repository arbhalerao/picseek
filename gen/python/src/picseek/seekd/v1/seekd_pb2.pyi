import datetime

from google.protobuf import duration_pb2 as _duration_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class MatchKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    MATCH_KIND_UNSPECIFIED: _ClassVar[MatchKind]
    MATCH_KIND_EXACT: _ClassVar[MatchKind]
    MATCH_KIND_NEAR: _ClassVar[MatchKind]
    MATCH_KIND_MEANING: _ClassVar[MatchKind]
    MATCH_KIND_VISUAL: _ClassVar[MatchKind]
MATCH_KIND_UNSPECIFIED: MatchKind
MATCH_KIND_EXACT: MatchKind
MATCH_KIND_NEAR: MatchKind
MATCH_KIND_MEANING: MatchKind
MATCH_KIND_VISUAL: MatchKind

class Hit(_message.Message):
    __slots__ = ("path", "rank", "best", "matches", "score")
    PATH_FIELD_NUMBER: _ClassVar[int]
    RANK_FIELD_NUMBER: _ClassVar[int]
    BEST_FIELD_NUMBER: _ClassVar[int]
    MATCHES_FIELD_NUMBER: _ClassVar[int]
    SCORE_FIELD_NUMBER: _ClassVar[int]
    path: str
    rank: int
    best: MatchKind
    matches: _containers.RepeatedScalarFieldContainer[MatchKind]
    score: float
    def __init__(self, path: _Optional[str] = ..., rank: _Optional[int] = ..., best: _Optional[_Union[MatchKind, str]] = ..., matches: _Optional[_Iterable[_Union[MatchKind, str]]] = ..., score: _Optional[float] = ...) -> None: ...

class ServiceHealth(_message.Message):
    __slots__ = ("name", "healthy", "total")
    NAME_FIELD_NUMBER: _ClassVar[int]
    HEALTHY_FIELD_NUMBER: _ClassVar[int]
    TOTAL_FIELD_NUMBER: _ClassVar[int]
    name: str
    healthy: int
    total: int
    def __init__(self, name: _Optional[str] = ..., healthy: _Optional[int] = ..., total: _Optional[int] = ...) -> None: ...

class Status(_message.Message):
    __slots__ = ("folders", "indexed", "queued_live", "queued_backfill", "failed", "images_per_second", "eta", "services", "skipped")
    FOLDERS_FIELD_NUMBER: _ClassVar[int]
    INDEXED_FIELD_NUMBER: _ClassVar[int]
    QUEUED_LIVE_FIELD_NUMBER: _ClassVar[int]
    QUEUED_BACKFILL_FIELD_NUMBER: _ClassVar[int]
    FAILED_FIELD_NUMBER: _ClassVar[int]
    IMAGES_PER_SECOND_FIELD_NUMBER: _ClassVar[int]
    ETA_FIELD_NUMBER: _ClassVar[int]
    SERVICES_FIELD_NUMBER: _ClassVar[int]
    SKIPPED_FIELD_NUMBER: _ClassVar[int]
    folders: _containers.RepeatedScalarFieldContainer[str]
    indexed: int
    queued_live: int
    queued_backfill: int
    failed: int
    images_per_second: float
    eta: _duration_pb2.Duration
    services: _containers.RepeatedCompositeFieldContainer[ServiceHealth]
    skipped: int
    def __init__(self, folders: _Optional[_Iterable[str]] = ..., indexed: _Optional[int] = ..., queued_live: _Optional[int] = ..., queued_backfill: _Optional[int] = ..., failed: _Optional[int] = ..., images_per_second: _Optional[float] = ..., eta: _Optional[_Union[datetime.timedelta, _duration_pb2.Duration, _Mapping]] = ..., services: _Optional[_Iterable[_Union[ServiceHealth, _Mapping]]] = ..., skipped: _Optional[int] = ...) -> None: ...

class AddFoldersRequest(_message.Message):
    __slots__ = ("paths",)
    PATHS_FIELD_NUMBER: _ClassVar[int]
    paths: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, paths: _Optional[_Iterable[str]] = ...) -> None: ...

class AddFoldersResponse(_message.Message):
    __slots__ = ("added",)
    ADDED_FIELD_NUMBER: _ClassVar[int]
    added: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, added: _Optional[_Iterable[str]] = ...) -> None: ...

class ForgetFolderRequest(_message.Message):
    __slots__ = ("path",)
    PATH_FIELD_NUMBER: _ClassVar[int]
    path: str
    def __init__(self, path: _Optional[str] = ...) -> None: ...

class ForgetFolderResponse(_message.Message):
    __slots__ = ("removed_paths",)
    REMOVED_PATHS_FIELD_NUMBER: _ClassVar[int]
    removed_paths: int
    def __init__(self, removed_paths: _Optional[int] = ...) -> None: ...

class RetryFailedRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class RetryFailedResponse(_message.Message):
    __slots__ = ("requeued",)
    REQUEUED_FIELD_NUMBER: _ClassVar[int]
    requeued: int
    def __init__(self, requeued: _Optional[int] = ...) -> None: ...

class SearchRequest(_message.Message):
    __slots__ = ("query", "limit")
    QUERY_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    query: str
    limit: int
    def __init__(self, query: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class SearchResponse(_message.Message):
    __slots__ = ("hits", "took")
    HITS_FIELD_NUMBER: _ClassVar[int]
    TOOK_FIELD_NUMBER: _ClassVar[int]
    hits: _containers.RepeatedCompositeFieldContainer[Hit]
    took: _duration_pb2.Duration
    def __init__(self, hits: _Optional[_Iterable[_Union[Hit, _Mapping]]] = ..., took: _Optional[_Union[datetime.timedelta, _duration_pb2.Duration, _Mapping]] = ...) -> None: ...

class StatusRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class StatusResponse(_message.Message):
    __slots__ = ("status",)
    STATUS_FIELD_NUMBER: _ClassVar[int]
    status: Status
    def __init__(self, status: _Optional[_Union[Status, _Mapping]] = ...) -> None: ...

class WatchStatusRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchStatusResponse(_message.Message):
    __slots__ = ("status",)
    STATUS_FIELD_NUMBER: _ClassVar[int]
    status: Status
    def __init__(self, status: _Optional[_Union[Status, _Mapping]] = ...) -> None: ...
