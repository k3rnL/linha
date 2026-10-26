#!/usr/bin/env python3
"""Start/stop a localhost application-storage fixture using owned platform services.

Requires bin/linha-server, boto3 and scripts/platform-fixtures.py start. The output
is a test-only connection file consumable by external SDK/worker smoke tests.
"""
import argparse
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request

import boto3

ROOT = Path(__file__).resolve().parents[1]
STATE = Path(tempfile.gettempdir()) / "linha-workload-runtime.json"
PLATFORM = Path(tempfile.gettempdir()) / "linha-release-fixtures.json"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["start", "clean"])
    args = parser.parse_args()
    if args.action == "clean":
        if STATE.exists():
            state = json.loads(STATE.read_text())
            binary = str(ROOT / "bin/linha-server")
            process = Path(f"/proc/{state['pid']}/cmdline")
            if process.exists():
                assert (
                    process.read_bytes().split(b"\0")[0].decode() == binary
                ), "refusing to stop an unowned process"
                os.kill(state["pid"], signal.SIGTERM)
            STATE.unlink()
        return
    assert not STATE.exists(), "fixture state already exists; clean it first"
    services = json.loads(PLATFORM.read_text())
    assert [s["name"] for s in services] == ["linha-release-pg", "linha-release-s3"]
    endpoint = "http://127.0.0.1:" + services[1]["port"]
    bucket = "linha-application-tests"
    client = boto3.client(
        "s3",
        endpoint_url=endpoint,
        region_name="us-east-1",
        aws_access_key_id="linha-test",
        aws_secret_access_key="linha-test-only-secret",
    )
    client.create_bucket(Bucket=bucket)
    root = Path(tempfile.mkdtemp(prefix="linha-application-fixture-"))
    (root / "results").mkdir()
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    env = dict(
        os.environ,
        LINHA_DATABASE_URL=f"postgres://postgres:linha-release-only@127.0.0.1:{services[0]['port']}/linha?sslmode=disable",
        LINHA_SECURITY_ENABLED="false",
        LINHA_ENABLE_FAKE_ENGINE="true",
        LINHA_LOCAL_ROOT=str(root / "results"),
        LINHA_LISTEN=f"127.0.0.1:{port}",
        AWS_ACCESS_KEY_ID="linha-test",
        AWS_SECRET_ACCESS_KEY="linha-test-only-secret",
        LINHA_S3_DESTINATIONS=json.dumps(
            {
                "data": {
                    "bucket": bucket,
                    "prefix": "business",
                    "endpoint": endpoint,
                    "region": "us-east-1",
                    "pathStyle": True,
                    "allowWorkloadAccess": True,
                }
            }
        ),
    )
    with (root / "server.log").open("w") as log:
        process = subprocess.Popen(
            [str(ROOT / "bin/linha-server")],
            env=env,
            stdout=log,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
    url = f"http://127.0.0.1:{port}"
    try:
        for _ in range(100):
            if process.poll() is not None:
                raise RuntimeError("fixture server exited: " + str(root / "server.log"))
            try:
                urllib.request.urlopen(url + "/readyz", timeout=1).close()
                break
            except OSError:
                time.sleep(0.1)
        else:
            raise TimeoutError("fixture server did not become ready")
        STATE.write_text(
            json.dumps(
                {
                    "pid": process.pid,
                    "url": url,
                    "bucket": bucket,
                    "endpoint": endpoint,
                    "prefix": "business",
                    "root": str(root),
                }
            )
        )
        print(STATE)
    except BaseException:
        process.terminate()
        process.wait(timeout=10)
        raise


if __name__ == "__main__":
    main()
