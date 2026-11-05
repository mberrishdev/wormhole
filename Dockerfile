FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /wormhole-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /wormhole-server /wormhole-server
ENTRYPOINT ["/wormhole-server"]
CMD ["serve", "--config", "/etc/wormhole/container.json"]
