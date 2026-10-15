# Fixtures

Recorded data for seed 7 with `scenarios/late-press.json` (Ironbridge Rovers win 5-2). The
simulator is deterministic, so moment and event IDs are stable.

| File | What it holds | Used by |
| --- | --- | --- |
| `late_press_model.json` | Model replies per agent and moment, replayed in order by `FakeModel` | tests, `make agents` |
| `late_press_tools.json` | Every MCP tool result the orchestrator needs for the whole match, replayed by `FakeTools` | unit tests |

## Model replies

Hand-written until Foundry is wired, in exactly the shape `agents/instructions/*.md` asks for,
using only numbers from the recorded tool results. They stage the behaviours the demo shows:

- `m-0001-mo-001` (Okoro's goal, 11:18): surfaced, written in English and Pidgin, approved.
- `m-0001-mo-018` (Ironbridge's press, 63:38): the analyst draft plants a wrong pressure index
  (7.4; the tools say 6.0). The Verifier rejects it in code and the rewrite fixes it.
- `m-0001-mo-017` (Hale's goal, 56:48): the Narrator's reply takes 10 s, past its deadline, so a
  template is published.
- `m-0001-mo-002`, `m-0001-mo-004`: skipped by the Moment Detector.

Moments with no recorded reply fail at the Moment Detector, so goals, cards, big chances, press
switches and momentum swings get templates and the rest are skipped.

An entry is `{"reply": {...}}`, optionally with `"delay_s"`, or `{"error": "..."}`. Record real
replies with `--record-model`:

```bash
.venv/bin/python -m touchline_agents --replay --model foundry --record-model fixtures/foundry.json
```

## Tool results

Regenerate after any change to the stats engine, the moment rules or the simulator; the
integration test fails until you do:

```bash
.venv/bin/python -m touchline_agents --replay --quiet --record-tools fixtures/late_press_tools.json --out /dev/null
```
