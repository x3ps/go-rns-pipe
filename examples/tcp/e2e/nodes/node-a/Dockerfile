FROM docker.io/library/golang:1.26-alpine AS builder
WORKDIR /src
COPY . .
RUN cd examples/tcp && go build -o /rns-tcp-iface .

FROM docker.io/library/python:3.12-slim
RUN pip install --no-cache-dir rns==1.1.4
RUN apt-get update && apt-get install -y --no-install-recommends procps \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /rns-tcp-iface /app/rns-tcp-iface
COPY examples/tcp/e2e/nodes/node-a/control_server.py /app/control_server.py
COPY examples/tcp/e2e/nodes/node-a/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
RUN mkdir -p /root/.reticulum
COPY examples/tcp/e2e/nodes/node-a/config/config /root/.reticulum/config
CMD ["/app/entrypoint.sh"]
