#!/usr/bin/env python3
"""Create/remove only labelled disposable platform fixtures; never targets production."""
import argparse
import json
import pathlib
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("action", choices=["start", "clean"])
args = parser.parse_args()
state = pathlib.Path("/tmp/linha-release-fixtures.json")


def run(*a, **kw):
    return subprocess.run(a, check=True, **kw)


if args.action == "start":
    specs = [
        (
            "linha-release-pg",
            "postgres:17",
            5432,
            {"POSTGRES_PASSWORD": "linha-release-only", "POSTGRES_DB": "linha"},
            [],
        ),
        (
            "linha-release-s3",
            "linha/minio:validation-glibc",
            9000,
            {
                "MINIO_ROOT_USER": "linha-test",
                "MINIO_ROOT_PASSWORD": "linha-test-only-secret",
            },
            ["server", "/data"],
        ),
    ]
    # Fail before changing anything if a name is already in use.
    for name, _, _, _, _ in specs:
        if (
            subprocess.run(["docker", "inspect", name], capture_output=True).returncode
            == 0
        ):
            raise SystemExit(
                name + " already exists; inspect it before recreating fixtures"
            )
    created = []
    try:
        for name, image, port, env, command in specs:
            cmd = [
                "docker",
                "run",
                "-d",
                "--name",
                name,
                "--label",
                "linha.validation=release",
                "-p",
                f"127.0.0.1::{port}",
            ]
            for k, v in env.items():
                cmd += ["-e", k + "=" + v]
            run(*cmd, image, *command, stdout=subprocess.DEVNULL)
            created.append(name)
        data = [
            {
                "name": name,
                "port": subprocess.check_output(
                    ["docker", "port", name, str(port) + "/tcp"], text=True
                )
                .strip()
                .rsplit(":", 1)[1],
            }
            for name, _, port, _, _ in specs
        ]
        state.write_text(json.dumps(data))
        print(state)
    except BaseException:
        for name in created:
            subprocess.run(["docker", "rm", "-fv", name], capture_output=True)
        raise
else:
    for name in ["linha-release-pg", "linha-release-s3"]:
        p = subprocess.run(["docker", "inspect", name], capture_output=True, text=True)
        if p.returncode:
            continue
        c = json.loads(p.stdout)[0]
        assert c["Config"]["Labels"].get("linha.validation") == "release", (
            "refusing unowned container " + name
        )
        run("docker", "rm", "-fv", name)
    state.unlink(missing_ok=True)
    clusterState = pathlib.Path("/tmp/linha-platform-cluster-state.json")
    if clusterState.exists():
        s = json.loads(clusterState.read_text())
        assert (
            s["cluster"].startswith("linha-platform-")
            and s["context"] == "kind-" + s["cluster"]
        )
        for name in [s["postgres"], s["s3"]]:
            assert name.startswith(s["cluster"] + "-")
            p = subprocess.run(
                ["docker", "inspect", name], capture_output=True, text=True
            )
            if p.returncode:
                continue
            assert (
                json.loads(p.stdout)[0]["Config"]["Labels"].get("linha.validation")
                == "platform-cluster"
            )
            run("docker", "rm", "-fv", name)
        run(
            "kind",
            "delete",
            "cluster",
            "--name",
            s["cluster"],
            "--kubeconfig",
            s["kubeconfig"],
        )
        clusterState.unlink()
