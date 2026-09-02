from picseek.common.v1 import image_pb2 as _image_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Vector(_message.Message):
    __slots__ = ("values",)
    VALUES_FIELD_NUMBER: _ClassVar[int]
    values: _containers.RepeatedScalarFieldContainer[float]
    def __init__(self, values: _Optional[_Iterable[float]] = ...) -> None: ...

class ModelInfo(_message.Message):
    __slots__ = ("name", "version", "dimensions")
    NAME_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    DIMENSIONS_FIELD_NUMBER: _ClassVar[int]
    name: str
    version: str
    dimensions: int
    def __init__(self, name: _Optional[str] = ..., version: _Optional[str] = ..., dimensions: _Optional[int] = ...) -> None: ...

class EmbedTextsRequest(_message.Message):
    __slots__ = ("texts",)
    TEXTS_FIELD_NUMBER: _ClassVar[int]
    texts: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, texts: _Optional[_Iterable[str]] = ...) -> None: ...

class EmbedTextsResponse(_message.Message):
    __slots__ = ("vectors", "model")
    VECTORS_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    vectors: _containers.RepeatedCompositeFieldContainer[Vector]
    model: ModelInfo
    def __init__(self, vectors: _Optional[_Iterable[_Union[Vector, _Mapping]]] = ..., model: _Optional[_Union[ModelInfo, _Mapping]] = ...) -> None: ...

class EmbedImagesRequest(_message.Message):
    __slots__ = ("images",)
    IMAGES_FIELD_NUMBER: _ClassVar[int]
    images: _containers.RepeatedCompositeFieldContainer[_image_pb2.ImageSource]
    def __init__(self, images: _Optional[_Iterable[_Union[_image_pb2.ImageSource, _Mapping]]] = ...) -> None: ...

class EmbedImagesResponse(_message.Message):
    __slots__ = ("vectors", "model")
    VECTORS_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    vectors: _containers.RepeatedCompositeFieldContainer[Vector]
    model: ModelInfo
    def __init__(self, vectors: _Optional[_Iterable[_Union[Vector, _Mapping]]] = ..., model: _Optional[_Union[ModelInfo, _Mapping]] = ...) -> None: ...

class EmbedClipTextsRequest(_message.Message):
    __slots__ = ("texts",)
    TEXTS_FIELD_NUMBER: _ClassVar[int]
    texts: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, texts: _Optional[_Iterable[str]] = ...) -> None: ...

class EmbedClipTextsResponse(_message.Message):
    __slots__ = ("vectors", "model")
    VECTORS_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    vectors: _containers.RepeatedCompositeFieldContainer[Vector]
    model: ModelInfo
    def __init__(self, vectors: _Optional[_Iterable[_Union[Vector, _Mapping]]] = ..., model: _Optional[_Union[ModelInfo, _Mapping]] = ...) -> None: ...

class InfoRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class InfoResponse(_message.Message):
    __slots__ = ("text", "clip_image", "clip_text")
    TEXT_FIELD_NUMBER: _ClassVar[int]
    CLIP_IMAGE_FIELD_NUMBER: _ClassVar[int]
    CLIP_TEXT_FIELD_NUMBER: _ClassVar[int]
    text: ModelInfo
    clip_image: ModelInfo
    clip_text: ModelInfo
    def __init__(self, text: _Optional[_Union[ModelInfo, _Mapping]] = ..., clip_image: _Optional[_Union[ModelInfo, _Mapping]] = ..., clip_text: _Optional[_Union[ModelInfo, _Mapping]] = ...) -> None: ...
