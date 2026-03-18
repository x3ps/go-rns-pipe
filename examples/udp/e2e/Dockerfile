FROM docker.io/library/python:3.12-slim
ENV PYTHONUNBUFFERED=1
WORKDIR /tests
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY conftest.py pytest.ini ./
COPY e2e/ e2e/
CMD ["python", "-m", "pytest", "-m", "e2e", "-vv", "--tb=short"]
