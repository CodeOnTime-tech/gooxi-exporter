FROM golang:1.23 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /gooxi-exporter .

FROM alpine:3
COPY --from=builder /gooxi-exporter /gooxi-exporter
EXPOSE 9108
USER nobody
ENTRYPOINT ["/gooxi-exporter"]
