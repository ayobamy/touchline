"""Touchline agents: turn candidate moments into explained, verified, timed overlays.

The orchestrator runs each moment through four steps - Moment Detector,
Narrator, Verifier, Director - with a deadline per step. Every number an agent
states comes from the Touchline MCP tools, and every overlay passes
schema/overlay.schema.json before it is published.
"""

__version__ = "0.1.0"
