from picseek.common.v1 import image_pb2 as _image_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Box(_message.Message):
    __slots__ = ("x", "y", "width", "height")
    X_FIELD_NUMBER: _ClassVar[int]
    Y_FIELD_NUMBER: _ClassVar[int]
    WIDTH_FIELD_NUMBER: _ClassVar[int]
    HEIGHT_FIELD_NUMBER: _ClassVar[int]
    x: int
    y: int
    width: int
    height: int
    def __init__(self, x: _Optional[int] = ..., y: _Optional[int] = ..., width: _Optional[int] = ..., height: _Optional[int] = ...) -> None: ...

class Line(_message.Message):
    __slots__ = ("text", "box", "confidence")
    TEXT_FIELD_NUMBER: _ClassVar[int]
    BOX_FIELD_NUMBER: _ClassVar[int]
    CONFIDENCE_FIELD_NUMBER: _ClassVar[int]
    text: str
    box: Box
    confidence: float
    def __init__(self, text: _Optional[str] = ..., box: _Optional[_Union[Box, _Mapping]] = ..., confidence: _Optional[float] = ...) -> None: ...

class RecognizeRequest(_message.Message):
    __slots__ = ("image",)
    IMAGE_FIELD_NUMBER: _ClassVar[int]
    image: _image_pb2.ImageSource
    def __init__(self, image: _Optional[_Union[_image_pb2.ImageSource, _Mapping]] = ...) -> None: ...

class RecognizeResponse(_message.Message):
    __slots__ = ("lines", "width", "height", "engine", "engine_version")
    LINES_FIELD_NUMBER: _ClassVar[int]
    WIDTH_FIELD_NUMBER: _ClassVar[int]
    HEIGHT_FIELD_NUMBER: _ClassVar[int]
    ENGINE_FIELD_NUMBER: _ClassVar[int]
    ENGINE_VERSION_FIELD_NUMBER: _ClassVar[int]
    lines: _containers.RepeatedCompositeFieldContainer[Line]
    width: int
    height: int
    engine: str
    engine_version: str
    def __init__(self, lines: _Optional[_Iterable[_Union[Line, _Mapping]]] = ..., width: _Optional[int] = ..., height: _Optional[int] = ..., engine: _Optional[str] = ..., engine_version: _Optional[str] = ...) -> None: ...

class InfoRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class InfoResponse(_message.Message):
    __slots__ = ("engine", "engine_version")
    ENGINE_FIELD_NUMBER: _ClassVar[int]
    ENGINE_VERSION_FIELD_NUMBER: _ClassVar[int]
    engine: str
    engine_version: str
    def __init__(self, engine: _Optional[str] = ..., engine_version: _Optional[str] = ...) -> None: ...
