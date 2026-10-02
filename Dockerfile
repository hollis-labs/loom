FROM golang:1.26.6-bookworm AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/loom ./cmd/loom

FROM debian:bookworm-slim
COPY --from=go-build /out/loom /usr/local/bin/loom
EXPOSE 8080
ENTRYPOINT ["loom"]
# Supply LOOM_API_TOKEN at runtime; serve refuses this bind without it.
CMD ["serve", "-project=false", "-addr=:8080"]
