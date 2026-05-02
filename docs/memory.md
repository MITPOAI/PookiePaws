# Memory System

pookiepaws stores reusable local knowledge in SQLite. The default path is:

```text
~/.pookiepaws/memory/pookiepaws.db
```

Set `POOKIEPAWS_HOME` or pass `--home` to use another runtime directory.

## Stored Data

Brand profile:

- brand name, niche, colors, fonts, tone, target audience
- preferred video style and CTA style
- banned words and styles
- platform preferences for TikTok, Instagram, YouTube Shorts, and Facebook
- successful and failed past prompts

Project history:

- user request, generated brief, prompts, provider/model, edit plan, output path, review report
- feedback score, corrections, and lessons learned

## Commands

```powershell
pookiepaws memory show
pookiepaws memory update --brand-name "PookiePaws" --colors "#ff4fa3,#202124,#ffffff"
pookiepaws memory search "large captions"
pookiepaws memory export --out memory-export.json
pookiepaws memory reset --yes
```

## Feedback Learning

Feedback is explicit. A score of `4` or `5` copies the project prompts into `successful_past_prompts`; lower scores copy them into `failed_past_prompts`. Lessons are appended to the platform preference used by that project.

No background model training occurs.
