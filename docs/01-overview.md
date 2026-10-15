# 01 Overview

## The problem

A score alert says what happened, not why. Fans get "GOAL 1–0". Broadcasters have one or two graphics operators who must spot a statistic, build a graphic and get it on air within seconds, so they cannot produce explained graphics for every market, language and level of football knowledge. Most of the story behind a match — the press that forced the mistake, the spell of pressure before the goal, the player quietly running the midfield — never reaches the people watching.

## Who it is for

| User | What they want | What Touchline gives them |
| --- | --- | --- |
| Casual fan | To understand why something happened, in their own language | Short, plain-language explanations on screen, in English, Nigerian Pidgin or Yoruba, with an "explain simply" style |
| Data-hungry analyst | The numbers behind the moment | Full metrics: pass difficulty, xG, pressure index, momentum, with the events that support them |
| Fan of one club or player | The match told from their side | Club focus and a player-focus mode that follows one player throughout |
| Broadcaster or streamer | Explained graphics without more operators | A timed, machine-readable overlay feed that a graphics engine can render |

## What Touchline does

Touchline is a live match producer made of AI agents. It reads a stream of synthetic match events, computes statistics with deterministic code, has agents decide which moments matter and explain why, verifies every claim against the events that back it, and pushes timed overlays to a pitch view. Two viewers watching the same match see two genuinely different experiences.

## How it meets the brief

The official rules ask for "one pipeline with five stages". Touchline implements each one.

| Rule stage | Touchline |
| --- | --- |
| (a) Ingest synthetic match events as they happen | Steerable Go simulator streaming to Azure Event Hubs |
| (b) Interpret them into statistics, patterns and context | Deterministic stats engine exposed to agents through an MCP server |
| (c) Explain why a moment matters | Moment Detector and Narrator agents, checked by a Verifier |
| (d) Render insight on screen beside the match | Timed overlay JSON over SignalR to a pitch view; a format a rendering partner could consume |
| (e) Personalize for analyst and casual fan | Viewer profiles: role, club, player focus, language, favourite metric |

It also covers the optional feature areas the rules list:

| Rules feature | Touchline |
| --- | --- |
| Player identification with speed and distance thresholds | Player name tags on events; ball and shot speed on every pass and shot; threshold-triggered overlays (planned) |
| Pass quality: distance, accuracy, difficulty | Built: pass distance, team and player accuracy, a 0–10 difficulty rating |
| Speed metrics: ball speed, shot speed | Built: `ball_speed_kmh` on passes and shots, max shot speed per team |
| Narrative generation and match recaps | Narrator agent and full-time recap (planned) |
| Explainability: control vs chaos, pressure changes, rhythm | Built: momentum, pressure index, chaos and rhythm over a rolling window |
| Data innovation: create or extend synthetic datasets | Built: our own simulator with replayable scenarios |
| Multi-language storytelling | Narrator writes per locale (planned) |
| Auto-eventing from video | Out of scope; see Non-goals |

## Judging criteria and how we score

| Criterion (20% each) | Our answer |
| --- | --- |
| Technological implementation | Clean Go and Python code, tests and CI from day one, JSON Schemas with drift tests, Foundry, Agent Framework, MCP, Event Hubs, Container Apps |
| Agentic design and innovation | Four agents with distinct roles, shared state, explicit handoffs and visible failure recovery; agents call deterministic tools over MCP instead of inventing numbers |
| Real-world impact | Broadcasters can serve every market without more operators; fans get the "why" in their language |
| User experience and presentation | One match shown two ways side by side; an agent trace that shows the system thinking and correcting itself |
| Adherence to category | Targeting **Best Multi-Agent System**: distinct roles, shared state, handoffs, recovery from failure, and an end-to-end result one agent could not produce |

## Target category

**Best Multi-Agent System.** Its description reads like a checklist, and Touchline's design answers each point:

- *Specialized agents with distinct roles*: Moment Detector, Narrator, Verifier, Director.
- *Coordinate through shared state and effective handoffs*: match state and the moment queue live in Redis; each handoff is a typed message.
- *Recover from failures*: a rejected claim is rewritten; a timed-out agent is replaced by a template overlay; nothing wrong reaches the screen.
- *An end-to-end outcome a single agent could not achieve as effectively*: one agent writing and checking its own claims is weaker than a separate verifier with tool access.

## Non-goals

- Auto-eventing from video. The rules list it as a feature to consider; it needs footage we do not have rights to, and it would take the whole build window.
- Real match data, real clubs or real players. The rules require synthetic data.
- Betting, odds or predictions.
- Mobile apps. The viewer is a web page that works on a phone.
