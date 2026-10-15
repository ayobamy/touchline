# 08 Demo and submission

## The two-minute video

Judges are not required to watch past two minutes, and many will watch without sound. Aim for **1:50**, with captions throughout.

| Time | On screen | Voice-over or caption |
| --- | --- | --- |
| 0:00–0:12 | A phone notification: "GOAL 1–0" | "Fans get the score. They don't get the story." |
| 0:12–0:25 | The Touchline pitch view replaying a match | "Touchline is a team of AI agents that explains the game as it happens." |
| 0:25–1:05 | Split screen: analyst view in English, casual-fan view in Pidgin. Rovers' press builds; overlays explain the pressure index rising; then the goal | "Same match, two experiences. The numbers come from code; the agents explain why they matter." |
| 1:05–1:30 | The agent-trace panel: the Verifier rejects a wrong number and the Narrator fixes it; a forced timeout produces a fallback overlay | "Every claim is checked against the events. When an agent fails, the screen is never wrong or empty." |
| 1:30–1:45 | Architecture diagram | "Built on Microsoft Foundry, Agent Framework, MCP, Event Hubs and Container Apps." |
| 1:45–1:50 | Repository and live URL | "Try it yourself." |

Recording tips:

- Record each segment separately and cut them together.
- Use seed 7 with `scenarios/late-press.json` so the story is the same every take.
- Plant the Verifier rejection with a test flag rather than hoping it happens on camera.
- No league names, club crests, real players or copyrighted music. All clubs and players in Touchline are fictional.

## Submission checklist

From the official rules, section 4.4:

- [ ] Project is newly built during the hackathon period
- [ ] Project pitch: what it is, which Microsoft and Azure technologies it uses, what it does, what problem it solves
- [ ] Demo video under two minutes, showing the project working
- [ ] Video uploaded publicly to YouTube or Vimeo, and the URL added to the project page
- [ ] Video contains no third-party trademarks or copyrighted music
- [ ] GitHub repository is **public** and its URL is on the project page
- [ ] All materials in English (Pidgin overlays are subtitled or explained in English)
- [ ] Working project available to judges free of charge until judging ends: live URL plus testing instructions
- [ ] Submitted before 27 October 2026, 11:59 PM Pacific Time

## Testing instructions for judges

Put these in the README and on the project page:

1. Open the live URL.
2. Press **Replay match**. A seeded match starts at 30× speed.
3. Switch profiles with the selector, or press **Split screen** to compare analyst and casual-fan views.
4. Open **Agent trace** to see each moment's journey through the agents.

To run the data pipeline locally with only Go installed:

```bash
go run ./cmd/simulator -seed 7 -scenario scenarios/late-press.json -speed 60 | go run ./cmd/stats -every 5 -pretty
```

## Project page fields

| Field | Content |
| --- | --- |
| Title | Touchline: Live Match Intelligence for Every Fan |
| Tagline | AI agents that explain the game as it happens, in your language, for your club. |
| Challenge | Best Multi-Agent System, if offered as a choice |
| Media | A screenshot of the split-screen view; then the video |
| Repository | https://github.com/ayobamy/touchline |
