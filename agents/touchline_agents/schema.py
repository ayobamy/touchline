"""Validation against schema/overlay.schema.json, the gate before anything is published."""

from __future__ import annotations

import json
import os
from collections.abc import Mapping
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator

SCHEMA_ENV = "TOUCHLINE_OVERLAY_SCHEMA"


def overlay_schema_path() -> Path:
    """$TOUCHLINE_OVERLAY_SCHEMA, or schema/overlay.schema.json in the repository."""
    if override := os.environ.get(SCHEMA_ENV):
        return Path(override)
    for parent in Path(__file__).resolve().parents:
        candidate = parent / "schema" / "overlay.schema.json"
        if candidate.is_file():
            return candidate
    raise FileNotFoundError(f"schema/overlay.schema.json not found; set {SCHEMA_ENV}")


class OverlayValidator:
    """Checks overlays against the published JSON Schema."""

    def __init__(self, path: Path | None = None) -> None:
        self.path = path or overlay_schema_path()
        schema = json.loads(self.path.read_text(encoding="utf-8"))
        Draft202012Validator.check_schema(schema)
        self.schema: dict[str, Any] = schema
        self._validator = Draft202012Validator(schema)

    def errors(self, overlay: Mapping[str, Any]) -> list[str]:
        """Every way the overlay breaks the schema, as readable lines. Empty means valid."""
        found = []
        for err in self._validator.iter_errors(dict(overlay)):
            where = "/".join(str(p) for p in err.absolute_path) or "overlay"
            found.append(f"{where}: {err.message}")
        return sorted(found)

    def max_length(self, field: str) -> int:
        """A string field's maxLength from the schema, so prompts never drift from it."""
        value = self.schema["properties"][field]["maxLength"]
        return int(value)

    def max_items(self, field: str) -> int:
        value = self.schema["properties"][field]["maxItems"]
        return int(value)
