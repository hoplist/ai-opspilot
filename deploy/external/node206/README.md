# node206 External Observability

This directory mirrors the files deployed to node206.

Remote layout:

- `/opt/prometheus`
  - `docker-compose.yml`
  - `prometheus.yml`
  - `data/`
- `/opt/node-exporter`
  - `docker-compose.yml`
- `/opt/cadvisor`
  - `docker-compose.yml`
- `/opt/opspilot-agent`
  - `docker-compose.yml`

Ports:

- Prometheus: `9090`
- node-exporter: `9100`
- cAdvisor: `8080`
- OpsPilot Agent: `19080`

Start order:

```bash
cd /opt/node-exporter && docker compose up -d
cd /opt/cadvisor && docker compose up -d
cd /opt/prometheus && docker compose up -d
cd /opt/opspilot-agent && docker compose up -d
```

Prometheus datasource registered in OpsPilot:

- `node206-host=http://192.168.48.206:9090`

OpsPilot node agent registered in OpsPilot Core:

- `node206=http://192.168.48.206:19080`
