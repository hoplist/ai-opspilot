FROM python:3.12-slim

WORKDIR /opt/rca

ENV PYTHONUNBUFFERED=1
ENV RCA_SERVICE=backend

COPY auto_inspection ./auto_inspection
COPY dashboard ./dashboard
COPY dashboard-live ./dashboard-live
COPY dashboard-alerts ./dashboard-alerts
COPY dashboard-rca ./dashboard-rca
COPY docs ./docs
COPY runbooks ./runbooks
COPY backend_server.py ./backend_server.py
COPY auto_inspection_mcp.py ./auto_inspection_mcp.py
COPY bootstrap_dashboards.py ./bootstrap_dashboards.py
COPY bootstrap_opensearch.py ./bootstrap_opensearch.py
COPY pipeline.py ./pipeline.py
COPY weekly_inspection.py ./weekly_inspection.py
COPY config.example.json ./config.example.json
COPY docker-entrypoint.sh ./docker-entrypoint.sh

RUN python -m pip install --no-cache-dir requests minio pymysql \
    && chmod +x /opt/rca/docker-entrypoint.sh

EXPOSE 18080 18081

ENTRYPOINT ["/opt/rca/docker-entrypoint.sh"]
