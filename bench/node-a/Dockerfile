FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY . .
RUN cd bench/pipe-bridge && go build -o /pipe-bridge .
RUN cd examples/tcp && go build -o /rns-tcp-iface .

FROM python:3.12-slim
RUN pip install --no-cache-dir rns==1.1.4
RUN apt-get update && apt-get install -y --no-install-recommends procps \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /pipe-bridge /bench/pipe-bridge
COPY --from=builder /rns-tcp-iface /bench/rns-tcp-iface
RUN mkdir -p /root/.reticulum
COPY bench/node-a/config/config /root/.reticulum/config
CMD ["rnsd"]
