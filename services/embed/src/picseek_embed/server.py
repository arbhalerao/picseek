import argparse
import logging

import grpc
from picseek.embed.v1 import embed_pb2, embed_pb2_grpc
from picseek_runtime import Lazy, configure_logging, default_model_dir, serve

from picseek_embed import images
from picseek_embed.models import CLIP_IMAGE, CLIP_TEXT, TEXT, ImageEncoder, ModelSpec, TextEncoder

SERVICE_NAME = "picseek.embed.v1.EmbedService"
MAX_BATCH = 256

log = logging.getLogger("picseek_embed")


class EmbedServicer(embed_pb2_grpc.EmbedServiceServicer):
    def __init__(
        self, text: Lazy[TextEncoder], clip_image: Lazy[ImageEncoder], clip_text: Lazy[TextEncoder]
    ) -> None:
        self._text = text
        self._clip_image = clip_image
        self._clip_text = clip_text

    def EmbedTexts(self, request: embed_pb2.EmbedTextsRequest, context: grpc.ServicerContext):
        check_batch(len(request.texts), context)
        with self._text.use() as encoder:
            vectors = encoder.encode(list(request.texts))
        return embed_pb2.EmbedTextsResponse(vectors=to_proto(vectors), model=model_info(TEXT))

    def EmbedClipTexts(
        self, request: embed_pb2.EmbedClipTextsRequest, context: grpc.ServicerContext
    ):
        check_batch(len(request.texts), context)
        with self._clip_text.use() as encoder:
            vectors = encoder.encode(list(request.texts))
        return embed_pb2.EmbedClipTextsResponse(
            vectors=to_proto(vectors), model=model_info(CLIP_TEXT)
        )

    def EmbedImages(self, request: embed_pb2.EmbedImagesRequest, context: grpc.ServicerContext):
        check_batch(len(request.images), context)
        loaded, slots = [], []
        for i, source in enumerate(request.images):
            kind = source.WhichOneof("source")
            try:
                loaded.append(
                    images.load(
                        path=source.path if kind == "path" else None,
                        data=source.data if kind == "data" else None,
                    )
                )
                slots.append(i)
            except (FileNotFoundError, images.InvalidImageError) as e:
                log.warning("skipping image %d: %s", i, e)

        result = [embed_pb2.Vector() for _ in request.images]
        if loaded:
            with self._clip_image.use() as encoder:
                vectors = encoder.encode(loaded)
            for slot, vector in zip(slots, vectors, strict=True):
                result[slot] = embed_pb2.Vector(values=vector.tolist())
        return embed_pb2.EmbedImagesResponse(vectors=result, model=model_info(CLIP_IMAGE))

    def Info(self, request: embed_pb2.InfoRequest, context: grpc.ServicerContext):
        return embed_pb2.InfoResponse(
            text=model_info(TEXT),
            clip_image=model_info(CLIP_IMAGE),
            clip_text=model_info(CLIP_TEXT),
        )


def check_batch(size: int, context: grpc.ServicerContext) -> None:
    if size == 0:
        context.abort(grpc.StatusCode.INVALID_ARGUMENT, "empty batch")
    if size > MAX_BATCH:
        context.abort(grpc.StatusCode.INVALID_ARGUMENT, f"batch of {size} exceeds {MAX_BATCH}")


def to_proto(vectors) -> list[embed_pb2.Vector]:
    return [embed_pb2.Vector(values=v.tolist()) for v in vectors]


def model_info(spec: ModelSpec) -> embed_pb2.ModelInfo:
    return embed_pb2.ModelInfo(name=spec.name, version=spec.version, dimensions=spec.dimensions)


def main() -> None:
    parser = argparse.ArgumentParser(prog="picseek-embed")
    parser.add_argument("--socket", required=True, help="unix socket path to listen on")
    parser.add_argument("--threads", type=int, default=2)
    parser.add_argument("--model-dir", default=str(default_model_dir()))
    parser.add_argument("--idle-unload", type=float, default=300, help="seconds, 0 disables")
    parser.add_argument("--log-level", default="INFO")
    args = parser.parse_args()
    configure_logging(args.log_level)

    text = Lazy(
        "text-encoder", lambda: TextEncoder(TEXT, args.model_dir, args.threads), args.idle_unload
    )
    clip_image = Lazy(
        "clip-image",
        lambda: ImageEncoder(CLIP_IMAGE, args.model_dir, args.threads),
        args.idle_unload,
    )
    clip_text = Lazy(
        "clip-text", lambda: TextEncoder(CLIP_TEXT, args.model_dir, args.threads), args.idle_unload
    )

    def shutdown() -> None:
        for model in (text, clip_image, clip_text):
            model.close()

    serve(
        args.socket,
        SERVICE_NAME,
        lambda server: embed_pb2_grpc.add_EmbedServiceServicer_to_server(
            EmbedServicer(text, clip_image, clip_text), server
        ),
        on_shutdown=shutdown,
    )


if __name__ == "__main__":
    main()
