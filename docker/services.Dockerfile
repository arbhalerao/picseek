FROM python:3.12-slim-bookworm AS build
COPY --from=ghcr.io/astral-sh/uv:0.12.23 /uv /usr/local/bin/uv
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy UV_PYTHON_DOWNLOADS=0
WORKDIR /src
COPY pyproject.toml uv.lock .python-version ./
COPY eval/pyproject.toml eval/pyproject.toml
COPY gen/python gen/python
COPY services services
RUN UV_PROJECT_ENVIRONMENT=/opt/ocr uv sync --frozen --no-dev --no-editable --package picseek-ocr \
	&& UV_PROJECT_ENVIRONMENT=/opt/embed uv sync --frozen --no-dev --no-editable --package picseek-embed

FROM python:3.12-slim-bookworm AS runtime
RUN useradd --uid 65532 --user-group --create-home --shell /usr/sbin/nologin picseek \
	&& mkdir -p /opt/picseek/models /run/picseek \
	&& chown picseek:picseek /opt/picseek/models /run/picseek
ENV PICSEEK_MODEL_DIR=/opt/picseek/models PYTHONUNBUFFERED=1

FROM runtime AS ocr
COPY --from=build /opt/ocr /opt/ocr
ENV PATH=/opt/ocr/bin:$PATH
USER picseek
RUN picseek-ocr --download-models --log-level WARNING
ENTRYPOINT ["picseek-ocr"]

FROM runtime AS embed
COPY --from=build /opt/embed /opt/embed
ENV PATH=/opt/embed/bin:$PATH
USER picseek
ENTRYPOINT ["picseek-embed"]
