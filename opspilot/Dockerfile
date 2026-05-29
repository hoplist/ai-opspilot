FROM m.daocloud.io/docker.io/library/alpine:3.20

RUN apk add --no-cache ca-certificates git openssh-client

COPY build/linux-amd64/opspilot-core /usr/local/bin/opspilot-core
COPY build/linux-amd64/opspilot /usr/local/bin/opspilot

ENV OPSPILOT_HOST=0.0.0.0
ENV OPSPILOT_PORT=18080

EXPOSE 18080

ENTRYPOINT ["/usr/local/bin/opspilot-core"]
