"""Template overlays for when an agent misses its deadline or fails.

Templates are built only from the moment (names, kind, and the metrics the
rules engine computed) and are marked `fallback: true` with producer
"template". They still cite the moment's evidence, so no evidence, no overlay
holds for them too (ADR 0008).
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Any

from touchline_agents.config import layout_for
from touchline_agents.contracts import Channel, Metric, Moment, Overlay
from touchline_agents.narrator import overlay_id

TEMPLATE_LOCALES = ("en", "pcm")
MAX_HEADLINE = 60
MAX_BODY = 160
MAX_METRICS = 4


@dataclass(frozen=True, slots=True)
class MetricFormat:
    label: str
    unit: str | None
    spec: str


METRICS: dict[str, MetricFormat] = {
    "pressure_index": MetricFormat("Pressure index", "per 10 opponent passes", ".1f"),
    "pressure_index_before": MetricFormat("Five minutes earlier", "per 10 opponent passes", ".1f"),
    "momentum": MetricFormat("Momentum", None, "+.2f"),
    "momentum_before": MetricFormat("Five minutes earlier", None, "+.2f"),
    "xg": MetricFormat("xG", None, ".2f"),
    "shot_speed_kmh": MetricFormat("Shot speed", "km/h", ".1f"),
    "pass_difficulty": MetricFormat("Pass difficulty", "out of 10", ".1f"),
    "pass_distance_m": MetricFormat("Pass distance", "m", ".1f"),
    "ball_speed_kmh": MetricFormat("Ball speed", "km/h", ".1f"),
    "chaos_per_min": MetricFormat("Possession changes", "per minute", ".1f"),
    "rhythm_per_min": MetricFormat("Events", "per minute", ".1f"),
    "tackles_won": MetricFormat("Tackles won", None, ".0f"),
    "passes_completed": MetricFormat("Passes completed", None, ".0f"),
}

HEADLINES: dict[str, dict[str, str]] = {
    "en": {
        "goal": "GOAL! {team} score",
        "card": "{card_title} card for {player}",
        "big_chance": "Big chance for {team}",
        "key_pass": "Brilliant pass from {player}",
        "momentum_swing": "{team} take control",
        "press_switch": "{team} step up the press",
        "chaos_spell": "End-to-end spell",
        "player_milestone": "Milestone for {player}",
    },
    "pcm": {
        "goal": "GOAL! {team} don score",
        "card": "{player} don collect {card} card",
        "big_chance": "{team} for don score dis one",
        "key_pass": "{player} pass am well well",
        "momentum_swing": "{team} don take control",
        "press_switch": "{team} dey press high now",
        "chaos_spell": "Di match don scatter",
        "player_milestone": "{player} dey do im work",
    },
}

PCM_BODIES: dict[str, str] = {
    "goal": (
        "{player} don put ball for net. "
        "Score now: {home_short} {home_goals}-{away_goals} {away_short}."
    ),
    "card": "Referee don show {player} {card} card.",
    "big_chance": "{player} get big chance (xG {xg}), but e no enter.",
    "key_pass": "{player} play pass wey hard well well: {pass_difficulty} out of 10.",
    "momentum_swing": "{team} dey run di game now. Momentum don reach {momentum}.",
    "press_switch": "{team} no dey give space again. Pressure index don reach {pressure_index}.",
    "chaos_spell": (
        "Possession dey change hand {chaos_per_min} times every minute. Nobody dey control."
    ),
    "milestone_passes": "{player} don complete {passes_completed} passes for dis match.",
    "milestone_tackles": "{player} don win {tackles_won} tackles for dis match.",
    "milestone_shot": "{player} first shot on target, but goalkeeper save am.",
}
PCM_GENERIC = "Na important moment for {team}."


class _Names:
    """Team and player names from get_match, with IDs as the fallback."""

    def __init__(self, match: Mapping[str, Any]) -> None:
        self.teams: dict[str, Mapping[str, Any]] = {}
        self.players: dict[str, str] = {}
        for side in ("home", "away"):
            team = match.get(side) or {}
            if team.get("id"):
                self.teams[str(team["id"])] = team
            for player in team.get("players") or []:
                self.players[str(player["id"])] = str(player["name"])
        self.home = match.get("home") or {}
        self.away = match.get("away") or {}

    def team(self, team_id: str | None) -> str:
        if not team_id:
            return ""
        return str(self.teams.get(team_id, {}).get("name", team_id))

    def player(self, player_id: str | None) -> str:
        if not player_id:
            return ""
        return self.players.get(player_id, player_id)


def _team_value(moment: Moment, names: _Names, key: str, value: float) -> float:
    """Momentum is stored home-relative (+1 home on top, -1 away on top). Show it
    from the moment's team's side, so "Rovers take control" reads +0.53, not -0.53.
    Only the sign changes; the number is still the tool's number."""
    away_id = names.away.get("id")
    if key.startswith("momentum") and away_id and moment.team_id == away_id:
        return -value
    return value


def _fields(moment: Moment, names: _Names) -> dict[str, str]:
    card = "red" if moment.summary.lower().startswith("red") else "yellow"
    values = {
        "team": names.team(moment.team_id) or "Both teams",
        "player": names.player(moment.player_id),
        "card": card,
        "card_title": card.capitalize(),
        "home_short": str(names.home.get("short_name", "HOME")),
        "away_short": str(names.away.get("short_name", "AWAY")),
    }
    for key, raw in moment.metrics.items():
        value = _team_value(moment, names, key, raw)
        fmt = METRICS.get(key)
        values[key] = format(value, fmt.spec) if fmt else f"{value:g}"
    return values


def _pcm_body(moment: Moment, values: Mapping[str, str]) -> str:
    key = moment.kind
    if moment.kind == "player_milestone":
        if "passes_completed" in moment.metrics:
            key = "milestone_passes"
        elif "tackles_won" in moment.metrics:
            key = "milestone_tackles"
        else:
            key = "milestone_shot"
    try:
        return PCM_BODIES[key].format_map(values)
    except KeyError:
        return PCM_GENERIC.format_map(values)


def _clip(text: str, limit: int) -> str:
    if len(text) <= limit:
        return text
    cut = text[: limit - 1].rsplit(" ", 1)[0]
    return cut + "…"


def _metrics(moment: Moment, names: _Names) -> list[Metric]:
    out: list[Metric] = []
    for key, raw in moment.metrics.items():
        fmt = METRICS.get(key)
        if fmt is None:
            continue
        value = _team_value(moment, names, key, raw)
        metric: Metric = {"label": fmt.label, "value": format(value, fmt.spec)}
        if fmt.unit:
            metric["unit"] = fmt.unit
        out.append(metric)
    return out[:MAX_METRICS]


def fallback_overlays(
    moment: Moment,
    channels: Sequence[Channel],
    match: Mapping[str, Any],
    *,
    priority: int | None = None,
) -> list[Overlay]:
    """One template overlay per channel. Locales without templates get English."""
    names = _Names(match)
    values = _fields(moment, names)
    layout = layout_for(moment.kind)
    metrics = _metrics(moment, names)
    out: list[Overlay] = []
    seen: set[Channel] = set()
    for channel in channels:
        locale = channel.locale if channel.locale in TEMPLATE_LOCALES else "en"
        target = Channel(channel.audience, locale)
        if target in seen:
            continue
        seen.add(target)
        headline_template = HEADLINES[locale].get(moment.kind, "{team}")
        body = moment.summary if locale == "en" else _pcm_body(moment, values)
        overlay: Overlay = {
            "id": overlay_id(moment, target, fallback=True),
            "match_id": moment.match_id,
            "period": moment.period,
            "show_at_ms": moment.clock_ms,
            "ttl_ms": layout.ttl_ms,
            "priority": priority or layout.priority,
            "slot": layout.slot,
            "audience": target.audience,
            "locale": target.locale,
            "headline": _clip(headline_template.format_map(values), MAX_HEADLINE),
            "body": _clip(body, MAX_BODY),
            "evidence_event_ids": list(moment.event_ids),
            "producer": "template",
            "fallback": True,
        }
        if moment.team_id:
            overlay["team_id"] = moment.team_id
        if moment.player_id:
            overlay["player_id"] = moment.player_id
        if metrics:
            overlay["metrics"] = metrics
        out.append(overlay)
    return out
