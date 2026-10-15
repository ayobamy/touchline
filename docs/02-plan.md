# 02 Plan

Touchline is built by one person in the 13 days from **Thursday 15 October** to **Tuesday 27 October 2026**. The submission deadline is 11:59 PM Pacific Time on 27 October, which is about 8:00 AM on 28 October in Lagos. Treat the evening of 27 October in Lagos as the real deadline.

## Scope for a solo build

The full design has six agents. For one builder it is cut to four, with the cut roles folded in so nothing the judges look for is lost:

| Full design | Solo build |
| --- | --- |
| Moment Detector | Moment Detector |
| Tactical Analyst | Folded into the Narrator's instructions |
| Narrator | Narrator |
| Localizer | Folded into the Narrator: it writes per locale directly |
| Verifier | Verifier |
| Director | Director |

Other cuts:

- **Languages:** English and Nigerian Pidgin (`pcm`). Add Yoruba (`yo`) only if time allows.
- **State:** Azure Managed Redis only. No Cosmos DB.
- **Frontend:** one page with a canvas pitch and an overlay layer. No component framework unless it saves time.
- **Out:** SMS and voice channels, the Producer Copilot approval desk, auto-eventing from video.

## Schedule

| Dates | Milestone | Done when |
| --- | --- | --- |
| Thu 15 – Sat 17 Oct | **M1 Data and stats** | Simulator, scenarios, stats engine and contracts are built and tested; candidate-moment rules flag about 45 moments a match; the MCP server exposes nine read-only tools over HTTP and stdio *(all done 15 Oct)*. |
| Sun 18 – Wed 21 Oct | **M2 Agents** | The four agents run on Foundry against a replayed match; every overlay that comes out passes `overlay.Validate`; a deliberate bad claim is rejected and rewritten; a forced timeout produces a fallback overlay. |
| Thu 22 – Sat 24 Oct | **M3 Viewer** | The pitch view replays a match with live overlays; analyst and casual-fan views run side by side; a full-time recap is generated per profile. |
| Sun 25 – Mon 26 Oct | **M4 Ship** | Deployed on Azure Container Apps with a public URL and a "replay match" button; README, architecture diagram and judge testing instructions are final. |
| Tue 27 Oct | **M5 Submit** | Video recorded and on YouTube, project page complete, repository public, submitted before the Lagos evening. |

Each milestone ends with a commit on `main` that passes CI.

## Daily rhythm

- Start each day by replaying `scenarios/late-press.json` end to end. If the replay breaks, fix that first.
- End each day by updating the status table in [docs/README.md](README.md).
- Record a 10-second screen clip whenever something visual works. These clips become the demo video's raw material.

## Risks

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| Agent latency makes overlays arrive late | High | High | Replay at a fixed speed in the demo; the Director drops anything older than its time-to-live; keep prompts short; use a small, fast model for the Moment Detector |
| Foundry or Agent Framework setup takes longer than expected | Medium | High | Get one agent answering one tool call on day 4 before building the rest; keep the orchestrator thin so a fallback to direct model calls is a small change |
| Pidgin output reads badly | Medium | Medium | Review a sample set yourself; keep a short style guide and examples in the Narrator's instructions |
| Azure costs or quota limits | Low | Medium | Scale Container Apps to zero outside demos; one small Redis tier; set a budget alert |
| Running out of time for the UI | Medium | High | Start the viewer on day 8 even if agents are imperfect; a polished small UI beats an unfinished large one |
| Video does not land in two minutes | Medium | High | Script it on day 11 (see [08](08-demo-and-submission.md)); record in short takes |

## Stretch goals, in order

1. Yoruba as a third language.
2. Threshold-triggered player tags (sprint speed or distance over a threshold).
3. Producer Copilot: a human approval queue in front of the overlay feed.
4. Low-data channel: the same explanations as SMS or WhatsApp voice notes.
