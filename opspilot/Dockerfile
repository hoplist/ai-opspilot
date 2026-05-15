FROM golang:1.23.2-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY opspilot ./opspilot

RUN go build -trimpath -ldflags="-s -w" -o /out/opspilot-core ./opspilot/core
RUN go build -trimpath -ldflags="-s -w" -o /out/opspilot ./opspilot/cli

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /out/opspilot-core /usr/local/bin/opspilot-core
COPY --from=builder /out/opspilot /usr/local/bin/opspilot

ENV OPSPILOT_HOST=0.0.0.0
ENV OPSPILOT_PORT=18080

EXPOSE 18080

ENTRYPOINT ["/usr/local/bin/opspilot-core"]
