FROM python:3.12-slim
RUN pip install --no-cache-dir rns==1.1.4
RUN mkdir -p /root/.reticulum
COPY config/config /root/.reticulum/config
COPY reflector.py /bench/reflector.py
CMD ["sh", "-c", "rnsd & sleep 3 && exec python3 /bench/reflector.py"]
