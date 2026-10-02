#!/usr/bin/env python3
"""Real Go/PostgreSQL/JVM smoke test, including process restart and raw/typed parity."""
import contextlib
import json
import os
import pathlib
import socket
import subprocess
import tempfile
import time
import urllib.request
import urllib.error
import uuid
import xml.etree.ElementTree as ET

ROOT = pathlib.Path(__file__).resolve().parents[1]
JAVA = str(pathlib.Path(os.environ.get("JAVA_HOME", "/usr")) / "bin/java")
VERSION = ET.parse(ROOT / "pom.xml").findtext(
    "{http://maven.apache.org/POM/4.0.0}version"
)
JAR = ROOT / f"examples/jvm/target/linha-examples_2.12-{VERSION}.jar"


def wait_for(check, processes=(), timeout=40):
    until = time.monotonic() + timeout
    while time.monotonic() < until:
        for p in processes:
            if p.poll() is not None:
                raise RuntimeError(f"process exited with {p.returncode}")
        try:
            value = check()
            if value:
                return value
        except (urllib.error.URLError, ConnectionError):
            pass
        time.sleep(0.1)
    raise TimeoutError("integration check did not complete")


def main():
    name = "linha-e2e-" + uuid.uuid4().hex[:12]
    processes = []
    with tempfile.TemporaryDirectory(prefix="linha-e2e-") as temporary:
        temp = pathlib.Path(temporary)
        logs = []

        def start(command, env):
            log = open(temp / f"process-{len(logs)}.log", "w+")
            logs.append(log)
            process = subprocess.Popen(
                command, cwd=ROOT, env=env, stdout=log, stderr=subprocess.STDOUT
            )
            processes.append(process)
            return process

        def stop(process):
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()

        try:
            subprocess.run(
                [
                    "docker",
                    "run",
                    "-d",
                    "--name",
                    name,
                    "-e",
                    "POSTGRES_USER=linha",
                    "-e",
                    "POSTGRES_PASSWORD=linha-test-only",
                    "-e",
                    "POSTGRES_DB=linha",
                    "-p",
                    "127.0.0.1::5432",
                    "postgres:17",
                ],
                check=True,
                capture_output=True,
            )
            port = (
                subprocess.check_output(["docker", "port", name, "5432/tcp"], text=True)
                .strip()
                .split(":")[-1]
            )
            with socket.socket() as sock:
                sock.bind(("127.0.0.1", 0))
                api_port = sock.getsockname()[1]
            url = f"http://127.0.0.1:{api_port}"
            root = temp / "results"
            root.mkdir()
            env = dict(
                os.environ,
                LINHA_DATABASE_HOST="127.0.0.1",
                LINHA_DATABASE_PORT=port,
                LINHA_DATABASE_NAME="linha",
                LINHA_DATABASE_USERNAME="linha",
                LINHA_DATABASE_PASSWORD="linha-test-only",
                LINHA_DATABASE_SSL_MODE="disable",
                LINHA_LOCAL_ROOT=str(root),
                LINHA_SECURITY_ENABLED="false",
                LINHA_ENABLE_FAKE_ENGINE="true",
                LINHA_LISTEN=f"127.0.0.1:{api_port}",
                LINHA_CLIENT_TOKEN="",
            )
            for key in ("LINHA_MODE", "LINHA_DATABASE_URL", "LINHA_NAMESPACE"):
                env.pop(key, None)
            wait_for(
                lambda: subprocess.run(
                    [
                        "docker",
                        "exec",
                        name,
                        "pg_isready",
                        "-h",
                        "127.0.0.1",
                        "-U",
                        "linha",
                    ],
                    capture_output=True,
                ).returncode
                == 0
            )
            lease = ""

            def call(path, body=None, method=None):
                req = urllib.request.Request(
                    url + path,
                    data=None if body is None else json.dumps(body).encode(),
                    method=method,
                    headers={
                        "Content-Type": "application/json",
                        "X-Linha-Client-Lease": lease,
                    },
                )
                with urllib.request.urlopen(req, timeout=10) as response:
                    data = response.read()
                    return (
                        json.loads(data)
                        if response.headers.get_content_type() == "application/json"
                        else data
                    )

            server = start([str(ROOT / "bin/linha-server")], env)
            wait_for(lambda: call("/readyz"), [server])
            accepted = json.loads(
                subprocess.check_output(
                    [
                        JAVA,
                        "-cp",
                        str(JAR),
                        "linha.example.ClientMain",
                        "submit",
                        url,
                        str(root),
                    ],
                    env=env,
                    text=True,
                    timeout=40,
                )
            )
            job, context = accepted["jobId"], accepted["contextId"]
            attachment = call(f"/v1/contexts/{context}/clients", {"clientId": name})
            lease = attachment["clientLease"]["id"]
            assert (
                call("/v1/jobs/" + job)["state"] == "QUEUED"
            ), "acceptance waited for a worker"
            duplicate = call(
                f"/v1/contexts/{context}/jobs",
                {
                    "handler": "example.sum",
                    "version": 1,
                    "payload": {"numbers": [1, 2, 3]},
                    "idempotencyKey": "scala-e2e",
                },
            )
            assert duplicate["id"] == job, "raw/typed idempotency mismatch"
            credentials = call(f"/v1/contexts/{context}/dev-workers", {})
            credential_file = temp / "worker.json"
            credential_file.write_text(json.dumps(credentials))
            credential_file.chmod(0o600)
            worker = start(
                [
                    JAVA,
                    "-cp",
                    str(JAR),
                    "linha.example.WorkerMain",
                    url,
                    str(credential_file),
                ],
                env,
            )
            wait_for(
                lambda: call("/v1/jobs/" + job)["state"] == "SUCCEEDED",
                [server, worker],
            )
            file_job = call(
                f"/v1/contexts/{context}/jobs",
                {
                    "handler": "example.summary",
                    "version": 1,
                    "payload": {"text": "durable file result"},
                    "idempotencyKey": "file-test",
                },
            )["id"]
            wait_for(
                lambda: call("/v1/jobs/" + file_job)["state"] == "SUCCEEDED",
                [server, worker],
            )
            metadata = call("/v1/jobs/" + file_job + "/result")
            assert (
                call(
                    f"/v1/jobs/{file_job}/files/{metadata['files'][0]['allocationId']}"
                )
                == b"durable file result"
            )
            raw_job = call(
                f"/v1/contexts/{context}/jobs",
                {
                    "handler": "example.raw-sum",
                    "version": 1,
                    "payload": {"numbers": [10, 20]},
                    "idempotencyKey": "raw-test",
                },
            )["id"]
            wait_for(
                lambda: call("/v1/jobs/" + raw_job)["state"] == "SUCCEEDED",
                [server, worker],
            )
            failed = call(
                f"/v1/contexts/{context}/jobs",
                {
                    "handler": "example.partial-failure",
                    "version": 1,
                    "payload": {},
                    "idempotencyKey": "partial-test",
                },
            )["id"]
            wait_for(
                lambda: call("/v1/jobs/" + failed)["state"] == "FAILED",
                [server, worker],
            )
            failed_state = call("/v1/jobs/" + failed)
            assert failed_state.get("result") is None
            diagnostic = failed_state["failure"]
            assert (
                diagnostic["exceptionType"] == "java.lang.IllegalStateException"
            ), diagnostic
            assert diagnostic["phase"] == "handler", diagnostic
            assert (
                "Caused by: java.io.IOException: example source read failed"
                in diagnostic["stackTrace"]
            )
            assert (
                "Suppressed: java.lang.IllegalArgumentException: example suppressed detail"
                in diagnostic["stackTrace"]
            )
            assert "linha.example.FailAfterWriting" in diagnostic["stackTrace"]
            assert not diagnostic.get("stackTraceTruncated", False)
            assert not list(
                root.rglob("partial.txt")
            ), "failed callback published partial output"
            stop(worker)
            stop(server)
            server = start([str(ROOT / "bin/linha-server")], env)
            wait_for(lambda: call("/readyz"), [server])
            result = json.loads(
                subprocess.check_output(
                    [
                        JAVA,
                        "-cp",
                        str(JAR),
                        "linha.example.ClientMain",
                        "result",
                        url,
                        job,
                    ],
                    env=env,
                    text=True,
                    timeout=40,
                )
            )
            assert result == {"value": 6}, result
            assert call("/v1/jobs/" + job)["state"] == "SUCCEEDED"
            assert call("/v1/jobs/" + failed)["failure"] == diagnostic
            attempts = call("/v1/jobs/" + failed + "/attempts")["items"]
            assert attempts[0]["failure"] == diagnostic
            print(
                "PASS: exception type, nested/suppressed stack trace and phase survive HTTP/worker/server restart in job and attempt history"
            )
            assert (
                call(
                    f"/v1/jobs/{file_job}/files/{metadata['files'][0]['allocationId']}"
                )
                == b"durable file result"
            )
            print(
                "PASS: typed and raw Scala jobs, immediate durable ID, file output, and result retrieval after client/server/worker restart"
            )
        except Exception:
            for log in logs:
                log.flush()
                log.seek(0)
                print(log.read()[-12000:])
            raise
        finally:
            for process in reversed(processes):
                stop(process)
            for log in logs:
                log.close()
            subprocess.run(["docker", "rm", "-f", "-v", name], capture_output=True)


if __name__ == "__main__":
    main()
