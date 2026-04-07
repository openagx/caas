FROM python:3.12-slim

WORKDIR /app

COPY ml/serving/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY ml/serving/main.py .

EXPOSE 8090

CMD ["python", "main.py"]
