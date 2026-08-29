# ---- build ----
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/wa-gateway ./cmd/wa-gateway

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/wa-gateway /wa-gateway
EXPOSE 3000
USER nonroot:nonroot
ENTRYPOINT ["/wa-gateway"]
