FROM docker.io/library/golang:1.24-alpine AS builder
WORKDIR /src
COPY . .
RUN cd examples/tcp && go build -o /rns-tcp-iface .

FROM docker.io/library/python:3.12-slim
RUN pip install --no-cache-dir rns==1.1.4
COPY --from=builder /rns-tcp-iface /app/rns-tcp-iface
RUN mkdir -p /root/.reticulum
COPY examples/tcp/e2e/nodes/node-b/config/config /root/.reticulum/config
COPY examples/tcp/e2e/nodes/node-b/reflector.py /app/reflector.py
CMD ["python3", "/app/reflector.py"]
