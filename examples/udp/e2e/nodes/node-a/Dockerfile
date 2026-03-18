FROM docker.io/library/golang:1.24-alpine AS builder
WORKDIR /src
COPY . .
RUN cd examples/udp && go build -o /rns-udp-iface .

FROM docker.io/library/python:3.12-slim
RUN pip install --no-cache-dir rns==1.1.4
RUN apt-get update && apt-get install -y --no-install-recommends procps \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /rns-udp-iface /app/rns-udp-iface
COPY examples/udp/e2e/nodes/node-a/control_server.py /app/control_server.py
COPY examples/udp/e2e/nodes/node-a/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
RUN mkdir -p /root/.reticulum
COPY examples/udp/e2e/nodes/node-a/config/config /root/.reticulum/config
CMD ["/app/entrypoint.sh"]
