FROM python:3.13-slim

WORKDIR /app

COPY opspilot /app/opspilot

ENV PYTHONUNBUFFERED=1
ENV OPSPILOT_HOST=0.0.0.0
ENV OPSPILOT_PORT=18080

EXPOSE 18080

CMD ["python", "-m", "opspilot.core"]
