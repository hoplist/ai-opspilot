import contextvars
import logging
import uuid

from fastapi import FastAPI, Request
from opentelemetry.trace import get_current_span


request_id_var = contextvars.ContextVar("request_id", default="")
event_id_var = contextvars.ContextVar("event_id", default="")
service_var = contextvars.ContextVar("service", default="devops-server")
biz_line_var = contextvars.ContextVar("biz_line", default="observability")


class RequestContextFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        span = get_current_span()
        span_context = span.get_span_context()

        record.request_id = request_id_var.get()
        record.event_id = event_id_var.get()
        record.service = service_var.get()
        record.biz_line = biz_line_var.get()
        record.trace_id = format(span_context.trace_id, "032x") if span_context.trace_id else ""
        record.span_id = format(span_context.span_id, "016x") if span_context.span_id else ""
        return True


def install_logging():
    handler = logging.StreamHandler()
    handler.addFilter(RequestContextFilter())
    handler.setFormatter(
        logging.Formatter(
            '{"service":"%(service)s","biz_line":"%(biz_line)s","request_id":"%(request_id)s","event_id":"%(event_id)s","trace_id":"%(trace_id)s","span_id":"%(span_id)s","level":"%(levelname)s","message":"%(message)s"}'
        )
    )

    logger = logging.getLogger()
    logger.handlers = [handler]
    logger.setLevel(logging.INFO)


app = FastAPI()
install_logging()


@app.middleware("http")
async def request_context_middleware(request: Request, call_next):
    request_id = request.headers.get("X-Request-Id") or uuid.uuid4().hex
    event_id = request.headers.get("X-Event-Id", "")

    request_id_var.set(request_id)
    event_id_var.set(event_id)
    service_var.set("devops-server")
    biz_line_var.set("observability")

    response = await call_next(request)
    response.headers["X-Request-Id"] = request_id
    if event_id:
        response.headers["X-Event-Id"] = event_id
    return response


@app.get("/health")
async def health():
    logging.getLogger(__name__).info("health_check_ok")
    return {"status": "ok"}


# Usage:
# 1. Keep Filebeat as the file collector if the app writes to file.
# 2. Or keep stdout logging and let Fluent Bit collect it in Kubernetes.
# 3. Add this middleware once, then all logs can share:
#    service / biz_line / request_id / trace_id / span_id / event_id
