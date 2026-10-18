FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
COPY gen/go gen/go
RUN CGO_ENABLED=1 go build -tags sqlite_fts5 -trimpath -ldflags="-s -w" -o /out/ ./cmd/... \
	&& mkdir -p /out/data /out/run/picseek

FROM gcr.io/distroless/base-debian12:nonroot
COPY --from=build /out/seekd /out/picseek /usr/local/bin/
COPY --from=build --chown=nonroot:nonroot /out/data /data
COPY --from=build --chown=nonroot:nonroot /out/run/picseek /run/picseek
COPY docker/config.toml /etc/picseek/config.toml
ENV XDG_CONFIG_HOME=/etc XDG_DATA_HOME=/data XDG_RUNTIME_DIR=/run
ENTRYPOINT ["seekd"]
