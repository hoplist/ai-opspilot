# OpsPilot ELK

Elastic Stack deployment for the test environment:

- Elasticsearch 9.3.1
- Logstash 9.3.1
- Kibana 9.3.1

Logstash tails Kubernetes container logs and keeps only:

- `opspilot`
- `ai-dev`

Endpoints:

- Elasticsearch: `http://192.168.48.200:32090`
- Kibana: `http://192.168.48.200:32056`

Deploy:

```bash
kubectl apply -k deploy/opspilot/elk
```
