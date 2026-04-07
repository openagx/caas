FROM python:3.12-slim

WORKDIR /app

# Install system deps for confluent-kafka (librdkafka)
RUN apt-get update && apt-get install -y --no-install-recommends gcc librdkafka-dev && \
    rm -rf /var/lib/apt/lists/*

COPY services/fraud-pipeline/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY services/fraud-pipeline/ .

EXPOSE 50054

CMD ["python", "main.py"]
