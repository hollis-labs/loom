FROM golang:1.26.10-bookworm AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/loom ./cmd/loom

FROM debian:bookworm-slim
COPY --from=go-build /out/loom /usr/local/bin/loom
EXPOSE 8080
ENTRYPOINT ["loom"]
# The data directory is /data (--no-project: XDG, via XDG_DATA_HOME). The listen address is
# httphost.addr, set in the file named by SUBSTRATE_CONFIG; the image's default is loopback,
# which a container port mapping cannot reach, so mount a config that sets "0.0.0.0:8080".
# auth.mode is enforce: nothing is served without a signed-in owner or a key.
ENV XDG_DATA_HOME=/data
VOLUME /data
CMD ["serve", "--no-project"]
