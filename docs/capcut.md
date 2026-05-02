# CapCut / Jianying / OpenCut Support

CapCut, Jianying, and OpenCut are optional. pookiepaws keeps FFmpeg as the core render path so ads can be created locally without relying on a proprietary draft format or a full editor app stack.

## Current Investigation

Open-source CapCut/Jianying automation projects exist, including:

- [capcut-mate](https://github.com/Hommy-master/capcut-mate): FastAPI toolkit for Jianying/CapCut-style draft generation.
- [cut_cli](https://cutcli.com/): CLI/SDK that generates CapCut/Jianying draft folders and exposes caption, asset, audio, effect, and keyframe commands.
- [pyJianYingDraft](https://github.com/GuanYixuan/pyJianYingDraft): Python draft-generation library for Jianying with related CapCut work.
- [pyCapCut](https://github.com/GuanYixuan/pyCapCut): Python CapCut draft-generation project.
- [CapCutAPI](https://github.com/renqingfei/CapCutAPI): Python-oriented draft file management and editing automation.
- [OpenCut](https://github.com/OpenCut-app/OpenCut): MIT-licensed open-source CapCut alternative. It is a full editor stack, so pookiepaws treats it as a future bridge target rather than the MVP render backend.

These are promising, but CapCut/Jianying draft schemas are version-sensitive and can break when desktop apps update.

## MVP Decision

- Core renderer: `scripts/media/render.py` with FFmpeg.
- Optional adapter: `internal/capcut` currently returns an unsupported result with a clear message.
- Optional bridge: `internal/opencut` currently returns an unsupported result with a clear message.
- Future integration should be added only after testing against current CapCut desktop versions and documenting supported platforms.

## Safety

CapCut automation must only operate on local draft files or user-approved workflows. It must not attempt to bypass app security, captchas, account restrictions, or platform rules.
