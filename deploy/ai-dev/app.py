import logging
import os
import re
import time
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI, HTTPException
from kubernetes import client, config
from kubernetes.client.rest import ApiException
from pydantic import BaseModel, Field

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("deer-flow-provisioner")

K8S_NAMESPACE = os.getenv("K8S_NAMESPACE", "ai-dev")
SANDBOX_IMAGE = os.getenv(
    "SANDBOX_IMAGE",
    "docker-hub.tpo.xzoa.com/xagent/all-in-one-sandbox:latest",
)
SKILLS_HOST_PATH = os.getenv("SKILLS_HOST_PATH", "/data/ai-dev/DeerFlow/skills")
THREADS_HOST_PATH = os.getenv("THREADS_HOST_PATH", "/data/ai-dev/DeerFlow/threads")
NODE_HOST = os.getenv("NODE_HOST", "192.168.48.201")
SANDBOX_NODE_NAME = os.getenv("SANDBOX_NODE_NAME", "k8s-worker-1")
SANDBOX_READY_TIMEOUT_SECONDS = int(os.getenv("SANDBOX_READY_TIMEOUT_SECONDS", "600"))
SAFE_ID = r"^[A-Za-z0-9_\-]+$"

core_v1 = None


class CreateSandboxRequest(BaseModel):
    sandbox_id: str = Field(pattern=SAFE_ID)
    thread_id: str = Field(pattern=SAFE_ID)


class SandboxResponse(BaseModel):
    sandbox_id: str
    sandbox_url: str
    status: str


def pod_name(sandbox_id):
    return f"sandbox-{sandbox_id}"


def svc_name(sandbox_id):
    return f"sandbox-{sandbox_id}-svc"


def sandbox_url(node_port):
    return f"http://{NODE_HOST}:{node_port}"


def validate_id(value):
    if not re.match(SAFE_ID, value):
        raise ValueError("Only letters, numbers, hyphens, and underscores are allowed.")
    return value


def userdata_path(thread_id):
    return str(Path(THREADS_HOST_PATH) / validate_id(thread_id) / "user-data")


def build_pod(sandbox_id, thread_id):
    sandbox_id = validate_id(sandbox_id)
    thread_id = validate_id(thread_id)
    return client.V1Pod(
        metadata=client.V1ObjectMeta(
            name=pod_name(sandbox_id),
            namespace=K8S_NAMESPACE,
            labels={
                "app": "deer-flow-sandbox",
                "sandbox-id": sandbox_id,
                "app.kubernetes.io/name": "deer-flow",
                "app.kubernetes.io/component": "sandbox",
            },
        ),
        spec=client.V1PodSpec(
            node_selector={"kubernetes.io/hostname": SANDBOX_NODE_NAME},
            restart_policy="Always",
            init_containers=[
                client.V1Container(
                    name="fix-user-data-permissions",
                    image=SANDBOX_IMAGE,
                    image_pull_policy="IfNotPresent",
                    command=[
                        "/bin/sh",
                        "-ec",
                        (
                            "mkdir -p /mnt/user-data/workspace /mnt/user-data/uploads /mnt/user-data/outputs "
                            "&& chown -R 1000:1000 /mnt/user-data "
                            "&& chmod -R u+rwX,g+rwX,o-rwx /mnt/user-data "
                            "&& chmod 2775 /mnt/user-data /mnt/user-data/workspace /mnt/user-data/uploads /mnt/user-data/outputs"
                        ),
                    ],
                    volume_mounts=[
                        client.V1VolumeMount(name="user-data", mount_path="/mnt/user-data", read_only=False),
                    ],
                    security_context=client.V1SecurityContext(
                        run_as_user=0,
                        run_as_group=0,
                        privileged=False,
                        allow_privilege_escalation=False,
                    ),
                )
            ],
            containers=[
                client.V1Container(
                    name="sandbox",
                    image=SANDBOX_IMAGE,
                    image_pull_policy="IfNotPresent",
                    ports=[client.V1ContainerPort(name="http", container_port=8080)],
                    readiness_probe=client.V1Probe(
                        http_get=client.V1HTTPGetAction(path="/v1/sandbox", port=8080),
                        initial_delay_seconds=5,
                        period_seconds=5,
                        timeout_seconds=3,
                        failure_threshold=3,
                    ),
                    liveness_probe=client.V1Probe(
                        http_get=client.V1HTTPGetAction(path="/v1/sandbox", port=8080),
                        initial_delay_seconds=10,
                        period_seconds=10,
                        timeout_seconds=3,
                        failure_threshold=3,
                    ),
                    resources=client.V1ResourceRequirements(
                        requests={"cpu": "100m", "memory": "256Mi", "ephemeral-storage": "500Mi"},
                        limits={"cpu": "1000m", "memory": "1Gi", "ephemeral-storage": "500Mi"},
                    ),
                    volume_mounts=[
                        client.V1VolumeMount(name="skills", mount_path="/mnt/skills", read_only=True),
                        client.V1VolumeMount(name="user-data", mount_path="/mnt/user-data", read_only=False),
                    ],
                    security_context=client.V1SecurityContext(
                        privileged=False,
                        allow_privilege_escalation=True,
                    ),
                )
            ],
            volumes=[
                client.V1Volume(
                    name="skills",
                    host_path=client.V1HostPathVolumeSource(path=SKILLS_HOST_PATH, type="Directory"),
                ),
                client.V1Volume(
                    name="user-data",
                    host_path=client.V1HostPathVolumeSource(path=userdata_path(thread_id), type="DirectoryOrCreate"),
                ),
            ],
        ),
    )


def build_service(sandbox_id):
    sandbox_id = validate_id(sandbox_id)
    return client.V1Service(
        metadata=client.V1ObjectMeta(
            name=svc_name(sandbox_id),
            namespace=K8S_NAMESPACE,
            labels={
                "app": "deer-flow-sandbox",
                "sandbox-id": sandbox_id,
                "app.kubernetes.io/name": "deer-flow",
                "app.kubernetes.io/component": "sandbox",
            },
        ),
        spec=client.V1ServiceSpec(
            type="NodePort",
            selector={"sandbox-id": sandbox_id},
            ports=[client.V1ServicePort(name="http", port=8080, target_port=8080)],
        ),
    )


def node_port(sandbox_id):
    try:
        svc = core_v1.read_namespaced_service(svc_name(sandbox_id), K8S_NAMESPACE)
        for port in svc.spec.ports or []:
            if port.name == "http":
                return port.node_port
    except ApiException:
        return None
    return None


def pod_phase(sandbox_id):
    try:
        pod = core_v1.read_namespaced_pod(pod_name(sandbox_id), K8S_NAMESPACE)
        return pod.status.phase or "Unknown"
    except ApiException:
        return "NotFound"


def pod_ready(sandbox_id):
    try:
        pod = core_v1.read_namespaced_pod(pod_name(sandbox_id), K8S_NAMESPACE)
    except ApiException:
        return False
    for condition in pod.status.conditions or []:
        if condition.type == "Ready" and condition.status == "True":
            return True
    return False


def pod_waiting_reason(sandbox_id):
    try:
        pod = core_v1.read_namespaced_pod(pod_name(sandbox_id), K8S_NAMESPACE)
    except ApiException as exc:
        return f"pod read failed: {exc.reason}"
    reasons = []
    for container_status in pod.status.container_statuses or []:
        waiting = container_status.state.waiting if container_status.state else None
        if waiting:
            reasons.append(f"{container_status.name}: {waiting.reason} {waiting.message or ''}".strip())
    if reasons:
        return "; ".join(reasons)
    return pod.status.phase or "Unknown"


def wait_for_pod_ready(sandbox_id):
    deadline = time.time() + SANDBOX_READY_TIMEOUT_SECONDS
    while time.time() < deadline:
        if pod_ready(sandbox_id):
            return
        phase = pod_phase(sandbox_id)
        if phase in {"Failed", "Succeeded"}:
            raise HTTPException(
                status_code=500,
                detail=f"Sandbox pod ended before ready: phase={phase}, reason={pod_waiting_reason(sandbox_id)}",
            )
        time.sleep(2)
    raise HTTPException(
        status_code=504,
        detail=(
            f"Sandbox pod was not Ready within {SANDBOX_READY_TIMEOUT_SECONDS}s: "
            f"phase={pod_phase(sandbox_id)}, reason={pod_waiting_reason(sandbox_id)}"
        ),
    )


@asynccontextmanager
async def lifespan(_app):
    global core_v1
    config.load_incluster_config()
    core_v1 = client.CoreV1Api()
    logger.info(
        "Provisioner ready: namespace=%s node=%s skills=%s threads=%s",
        K8S_NAMESPACE,
        SANDBOX_NODE_NAME,
        SKILLS_HOST_PATH,
        THREADS_HOST_PATH,
    )
    yield


app = FastAPI(title="DeerFlow Sandbox Provisioner", lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.post("/api/sandboxes", response_model=SandboxResponse)
async def create_sandbox(req: CreateSandboxRequest):
    existing = node_port(req.sandbox_id)
    if existing:
        return SandboxResponse(sandbox_id=req.sandbox_id, sandbox_url=sandbox_url(existing), status=pod_phase(req.sandbox_id))
    try:
        core_v1.create_namespaced_pod(K8S_NAMESPACE, build_pod(req.sandbox_id, req.thread_id))
    except ApiException as exc:
        if exc.status != 409:
            raise HTTPException(status_code=500, detail=f"Pod creation failed: {exc.reason}")
    try:
        core_v1.create_namespaced_service(K8S_NAMESPACE, build_service(req.sandbox_id))
    except ApiException as exc:
        if exc.status != 409:
            try:
                core_v1.delete_namespaced_pod(pod_name(req.sandbox_id), K8S_NAMESPACE)
            except ApiException:
                pass
            raise HTTPException(status_code=500, detail=f"Service creation failed: {exc.reason}")
    for _ in range(20):
        allocated = node_port(req.sandbox_id)
        if allocated:
            wait_for_pod_ready(req.sandbox_id)
            return SandboxResponse(sandbox_id=req.sandbox_id, sandbox_url=sandbox_url(allocated), status=pod_phase(req.sandbox_id))
        time.sleep(0.5)
    raise HTTPException(status_code=500, detail="NodePort was not allocated in time")


@app.get("/api/sandboxes/{sandbox_id}", response_model=SandboxResponse)
async def get_sandbox(sandbox_id: str):
    allocated = node_port(validate_id(sandbox_id))
    if not allocated:
        raise HTTPException(status_code=404, detail=f"Sandbox '{sandbox_id}' not found")
    return SandboxResponse(sandbox_id=sandbox_id, sandbox_url=sandbox_url(allocated), status=pod_phase(sandbox_id))


@app.get("/api/sandboxes")
async def list_sandboxes():
    services = core_v1.list_namespaced_service(K8S_NAMESPACE, label_selector="app=deer-flow-sandbox")
    sandboxes = []
    for svc in services.items:
        sid = (svc.metadata.labels or {}).get("sandbox-id")
        if sid:
            allocated = node_port(sid)
            if allocated:
                sandboxes.append(SandboxResponse(sandbox_id=sid, sandbox_url=sandbox_url(allocated), status=pod_phase(sid)))
    return {"sandboxes": sandboxes, "count": len(sandboxes)}


@app.delete("/api/sandboxes/{sandbox_id}")
async def delete_sandbox(sandbox_id: str):
    sandbox_id = validate_id(sandbox_id)
    errors = []
    for kind, delete_func, name in [
        ("service", core_v1.delete_namespaced_service, svc_name(sandbox_id)),
        ("pod", core_v1.delete_namespaced_pod, pod_name(sandbox_id)),
    ]:
        try:
            delete_func(name, K8S_NAMESPACE)
        except ApiException as exc:
            if exc.status != 404:
                errors.append(f"{kind}: {exc.reason}")
    if errors:
        raise HTTPException(status_code=500, detail=", ".join(errors))
    return {"ok": True, "sandbox_id": sandbox_id}
