FROM m.daocloud.io/docker.io/library/alpine:3.20

RUN addgroup -S opspilot && adduser -S -G opspilot -u 65532 opspilot

COPY build/linux-amd64/opspilot-core /usr/local/bin/opspilot-core
COPY build/linux-amd64/opspilot /usr/local/bin/opspilot

ENV OPSPILOT_HOST=0.0.0.0
ENV OPSPILOT_PORT=18080

EXPOSE 18080

USER 65532:65532

ENTRYPOINT ["/usr/local/bin/opspilot-core"]
