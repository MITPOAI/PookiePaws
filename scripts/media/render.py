#!/usr/bin/env python3
"""Render a pookiepaws edit_plan.json with FFmpeg."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--plan", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()

    ffmpeg = os.environ.get("FFMPEG") or shutil.which("ffmpeg")
    if not ffmpeg:
        print("ffmpeg was not found on PATH. Install FFmpeg to render videos.", file=sys.stderr)
        return 127

    plan_path = Path(args.plan)
    out_path = Path(args.out)
    plan = json.loads(plan_path.read_text(encoding="utf-8"))
    width = int(plan.get("width") or 1080)
    height = int(plan.get("height") or 1920)
    fps = int(plan.get("fps") or 30)
    scenes = plan.get("scenes") or []
    if not scenes:
        print("edit plan has no scenes", file=sys.stderr)
        return 2

    out_path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="pookiepaws-render-") as tmp:
        tmpdir = Path(tmp)
        segments = []
        for idx, scene in enumerate(scenes):
            segment = tmpdir / f"segment-{idx:03d}.mp4"
            render_scene(ffmpeg, scene, segment, width, height, fps)
            segments.append(segment)

        list_file = tmpdir / "segments.txt"
        list_file.write_text(
            "".join(f"file '{seg.as_posix()}'\n" for seg in segments),
            encoding="utf-8",
        )
        cmd = [
            ffmpeg,
            "-y",
            "-safe",
            "0",
            "-f",
            "concat",
            "-i",
            str(list_file),
            "-c",
            "copy",
            str(out_path),
        ]
        run(cmd)
    return 0


def render_scene(ffmpeg: str, scene: dict, out_path: Path, width: int, height: int, fps: int) -> None:
    start = float(scene.get("start") or 0)
    end = float(scene.get("end") or 0)
    duration = max(0.1, end - start)
    background = scene.get("background") or ""
    background_color = normalize_color(scene.get("background_color") or "#202124")

    if background and Path(background).exists():
        cmd = [
            ffmpeg,
            "-y",
            "-loop",
            "1",
            "-t",
            f"{duration:.3f}",
            "-i",
            background,
            "-vf",
            scene_filter(scene, duration, width, height, image_input=True),
            "-r",
            str(fps),
            "-an",
            "-c:v",
            "libx264",
            "-pix_fmt",
            "yuv420p",
            str(out_path),
        ]
    else:
        cmd = [
            ffmpeg,
            "-y",
            "-f",
            "lavfi",
            "-i",
            f"color=c={background_color}:s={width}x{height}:d={duration:.3f}:r={fps}",
            "-vf",
            scene_filter(scene, duration, width, height, image_input=False),
            "-an",
            "-c:v",
            "libx264",
            "-pix_fmt",
            "yuv420p",
            str(out_path),
        ]
    run(cmd)


def scene_filter(scene: dict, duration: float, width: int, height: int, image_input: bool) -> str:
    filters = []
    if image_input:
        filters.append(
            f"scale={width}:{height}:force_original_aspect_ratio=increase,crop={width}:{height}"
        )
    filters.extend(
        [
            "format=rgba",
            f"fade=t=in:st=0:d={min(0.25, duration / 4):.3f}:alpha=1",
            f"fade=t=out:st={max(0, duration - 0.25):.3f}:d={min(0.25, duration / 4):.3f}:alpha=1",
            f"drawbox=x=0:y=0:w=iw:h=ih:color=black@0.18:t=fill",
        ]
    )

    text = str(scene.get("text") or "").strip()
    subtext = str(scene.get("subtext") or "").strip()
    animation = str(scene.get("animation") or "fade").lower()
    text_y = {
        "slide": "(h-text_h)/2-90",
        "bounce": "(h-text_h)/2-140",
        "pop": "(h-text_h)/2-120",
    }.get(animation, "(h-text_h)/2-120")

    if text:
        filters.append(
            "drawtext="
            f"{font_option()}"
            f"text='{escape_drawtext(text)}':"
            "fontcolor=white:"
            "borderw=5:"
            "bordercolor=black@0.75:"
            "fontsize=86:"
            "x=(w-text_w)/2:"
            f"y={text_y}:"
            f"alpha='{alpha_expr(duration)}'"
        )
    if subtext:
        filters.append(
            "drawtext="
            f"{font_option()}"
            f"text='{escape_drawtext(subtext)}':"
            "fontcolor=white:"
            "borderw=3:"
            "bordercolor=black@0.65:"
            "fontsize=46:"
            "x=(w-text_w)/2:"
            "y=(h-text_h)/2+20:"
            f"alpha='{alpha_expr(duration)}'"
        )
    if scene.get("cta"):
        cta_x = int(width * 0.18)
        cta_y = int(height * 0.74)
        cta_w = int(width * 0.64)
        cta_h = int(height * 0.08)
        filters.append(
            f"drawbox=x={cta_x}:y={cta_y}:w={cta_w}:h={cta_h}:color=white@0.90:t=fill"
        )
        filters.append(
            "drawtext="
            f"{font_option()}"
            f"text='{escape_drawtext(str(scene.get('cta')))}':"
            "fontcolor=black:"
            "fontsize=42:"
            "x=(w-text_w)/2:"
            f"y={cta_y}+({cta_h}-text_h)/2"
        )
    filters.append("format=yuv420p")
    return ",".join(filters)


def alpha_expr(duration: float) -> str:
    if duration <= 0.5:
        return "1"
    return f"if(lt(t,0.25),t/0.25,if(gt(t,{duration - 0.25:.3f}),max(0,({duration:.3f}-t)/0.25),1))"


def escape_drawtext(value: str) -> str:
    return (
        value.replace("\\", "\\\\")
        .replace("'", "\\'")
        .replace(":", "\\:")
        .replace("%", "\\%")
        .replace(",", "\\,")
        .replace("\n", " ")
    )


def font_option() -> str:
    font = os.environ.get("FONTFILE") or default_fontfile()
    if not font:
        return ""
    return f"fontfile='{escape_filter_path(font)}':"


def default_fontfile() -> str:
    candidates = [
        "C:/Windows/Fonts/arial.ttf",
        "C:/Windows/Fonts/segoeui.ttf",
        "/System/Library/Fonts/Supplemental/Arial.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
    ]
    for candidate in candidates:
        if Path(candidate).exists():
            return candidate
    return ""


def escape_filter_path(value: str) -> str:
    return value.replace("\\", "/").replace(":", "\\:").replace("'", "\\'")


def normalize_color(value: str) -> str:
    value = value.strip()
    if value.startswith("#") and len(value) == 7:
        return "0x" + value[1:]
    return value or "0x202124"


def run(cmd: list[str]) -> None:
    proc = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    if proc.returncode != 0:
        print(proc.stdout, file=sys.stderr)
        raise SystemExit(proc.returncode)


if __name__ == "__main__":
    raise SystemExit(main())
