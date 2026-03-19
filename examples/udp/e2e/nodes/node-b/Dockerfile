FROM docker.io/library/golang:1.26-alpine AS builder
WORKDIR /src
COPY . .
RUN cd examples/udp && go build -o /rns-udp-iface .

FROM docker.io/library/python:3.12-slim
RUN pip install --no-cache-dir rns==1.1.4
RUN apt-get update && apt-get install -y --no-install-recommends procps \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /rns-udp-iface /app/rns-udp-iface
RUN mkdir -p /root/.reticulum
COPY examples/udp/e2e/nodes/node-b/config/config /root/.reticulum/config
COPY examples/udp/e2e/nodes/node-b/reflector.py /app/reflector.py
CMD ["python3", "/app/reflector.py"]
