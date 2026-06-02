"""ClawMatrix tools for Google ADK example agents."""

from __future__ import annotations

import json
import os
import subprocess
from typing import Any


def list_clawmatrix_agents() -> dict[str, Any]:
    """List ClawMatrix agents this agent is authorized to contact through clutch."""

    source = os.environ.get("AGENT_GROUP", "agent")
    try:
        peers = _fetch_connections(source)
    except Exception as exc:
        return {"ok": False, "source": source, "agents": [], "error": str(exc)}

    agents = []
    for peer in peers:
        name = str(peer.get("name", "")).strip()
        if not name:
            continue
        agents.append(
            {
                "name": name,
                "runner": _peer_runner(peer),
            }
        )
    return {"ok": True, "source": source, "agents": agents}


def delegate_to_clawmatrix_agent(target: str, message: str) -> dict[str, Any]:
    """Send a task to an authorized ClawMatrix peer through clutch A2A delegation."""

    source = os.environ.get("AGENT_GROUP", "agent")
    target = target.strip()
    message = message.strip()
    if not target or not message:
        return {"ok": False, "source": source, "target": target, "error": "target and message are required"}

    try:
        peers = _fetch_connections(source)
    except Exception as exc:
        return {"ok": False, "source": source, "target": target, "error": str(exc)}

    allowed = {str(peer.get("name", "")).strip() for peer in peers}
    resolved_target = _resolve_agent_name(target, allowed)
    if not resolved_target:
        return {
            "ok": False,
            "source": source,
            "target": target,
            "allowed_agents": sorted(a for a in allowed if a),
            "error": "target is not an authorized ClawMatrix connection for this agent",
        }

    try:
        out = _run_clutch_json(
            source,
            "delegate",
            resolved_target,
            message,
            f"adk-{source}-delegate",
        )
    except Exception as exc:
        return {"ok": False, "source": source, "target": resolved_target, "error": str(exc)}

    if "error" in out:
        return {"ok": False, "source": source, "target": resolved_target, "error": out["error"]}
    text = _extract_task_text(out.get("result"))
    return {"ok": True, "source": source, "target": resolved_target, "response": text}


def _fetch_connections(source: str) -> list[dict[str, Any]]:
    out = _run_clutch_json(source, "connections")
    if isinstance(out, list):
        return [entry for entry in out if isinstance(entry, dict)]
    if isinstance(out, dict):
        entries = out.get(source)
        if isinstance(entries, list):
            return [entry for entry in entries if isinstance(entry, dict)]
    return []


def _peer_runner(peer: dict[str, Any]) -> str:
    agents = peer.get("agents")
    if not isinstance(agents, list):
        return ""
    for agent in agents:
        if isinstance(agent, dict) and agent.get("status") == "healthy":
            return str(agent.get("runner", "")).strip()
    return ""


def _resolve_agent_name(target: str, allowed: set[str]) -> str:
    normalized = _normalize_name(target)
    for name in allowed:
        if _normalize_name(name) == normalized:
            return name

    matches = [name for name in allowed if normalized and normalized in _normalize_name(name)]
    if len(matches) == 1:
        return matches[0]
    return ""


def _normalize_name(name: str) -> str:
    return "".join(ch for ch in name.lower() if ch.isalnum())


def _run_clutch_json(source: str, *args: str) -> Any:
    cmd = ["clutch", "--name", source, *args]
    env = os.environ.copy()
    env.setdefault("CLUTCH_URL", "http://127.0.0.1:8080")
    proc = subprocess.run(
        cmd,
        env=env,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=180,
    )
    if proc.returncode != 0:
        detail = (proc.stderr or proc.stdout).strip()
        raise RuntimeError(detail or f"clutch exited with status {proc.returncode}")
    try:
        return json.loads(proc.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"clutch returned non-JSON output: {proc.stdout.strip()}") from exc


def _extract_task_text(task: Any) -> str:
    if not isinstance(task, dict):
        return ""
    artifacts = task.get("artifacts") or []
    for artifact in artifacts:
        if artifact.get("name") != "final-response":
            continue
        text = _parts_text(artifact.get("parts") or [])
        if text:
            return text
    history = task.get("history") or []
    for message in reversed(history):
        if message.get("role") == "agent":
            text = _parts_text(message.get("parts") or [])
            if text:
                return text
    status = task.get("status") or {}
    return _parts_text(((status.get("message") or {}).get("parts")) or [])


def _parts_text(parts: list[dict[str, Any]]) -> str:
    return "\n".join(
        str(part.get("text", "")).strip()
        for part in parts
        if part.get("kind") == "text" and str(part.get("text", "")).strip()
    ).strip()

