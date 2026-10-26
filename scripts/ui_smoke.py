#!/usr/bin/env python3
"""End-to-end smoke test of the studio UI with Playwright.

Starts the built binary, drives the real browser through import → find clips →
edit → settings → render → queue, and fails on any console error.

Usage:
    go build -o /tmp/studio ./cmd/studio
    python -m pip install playwright && python -m playwright install chromium
    python scripts/ui_smoke.py /tmp/studio [--keep]

Environment:
    STUDIO_PORT          port to bind (default 8765)
    REMOTION_BROWSER     Chrome binary for Remotion (optional; otherwise the
                         FFmpeg fallback renders the clip)
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path

from playwright.sync_api import sync_playwright

PORT = int(os.environ.get("STUDIO_PORT", "8765"))
BASE = f"http://127.0.0.1:{PORT}"


def make_fixture(folder: Path) -> tuple[Path, Path]:
    video = folder / "episode.mp4"
    srt = folder / "episode.srt"
    subprocess.run(
        ["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=60", "-f", "lavfi", "-i", "sine=frequency=400:duration=60", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", str(video)],
        check=True,
    )
    lines = []
    for i in range(55):
        lines.append(f"{i + 1}\n00:00:{i:02d},000 --> 00:00:{i + 1:02d},000\nThe biggest SEO mistake creators make is ignoring search intent and topical depth. Here's why it matters.\n")
    srt.write_text("\n".join(lines), encoding="utf-8")
    return video, srt


def main() -> int:
    if len(sys.argv) < 2:
        print(__doc__)
        return 2
    binary = sys.argv[1]
    repo = Path(__file__).resolve().parent.parent
    work = Path(tempfile.mkdtemp(prefix="studio-smoke-"))
    video, srt = make_fixture(work)
    env = dict(os.environ)
    if os.environ.get("REMOTION_BROWSER"):
        env["PODCAST_STUDIO_BROWSER"] = os.environ["REMOTION_BROWSER"]
    server = subprocess.Popen([binary, "-no-browser", "-no-token", "-port", str(PORT), "-state-dir", str(work / "state"), "-remotion", str(repo / "remotion")], env=env, stdout=open(work / "studio.out", "w"), stderr=subprocess.STDOUT)
    errors: list[str] = []
    try:
        for _ in range(50):
            try:
                import urllib.request

                urllib.request.urlopen(f"{BASE}/api/status", timeout=1).read()
                break
            except Exception:
                time.sleep(0.2)
        with sync_playwright() as p:
            browser = p.chromium.launch()
            page = browser.new_page(viewport={"width": 1440, "height": 900}, bypass_csp=True)
            page.on("console", lambda m: errors.append(f"console.{m.type}: {m.text}") if m.type == "error" else None)
            page.on("pageerror", lambda e: errors.append(f"pageerror: {e}"))

            page.goto(f"{BASE}/#/import")
            page.wait_for_function("document.querySelector('#speechProvider').options.length > 0")
            page.click("#importBtn")
            page.wait_for_function("document.querySelector('#videoPathErr').textContent.length > 0")
            page.fill("#videoPath", str(video))
            page.fill("#transcriptPath", str(srt))
            page.click("#importBtn")
            page.wait_for_selector("#page-clips.visible", timeout=60000)
            page.wait_for_function("document.querySelector('#projectName').textContent.includes('cues')")

            page.fill("#clipCount", "3")
            page.click("#analyzeBtn")
            page.wait_for_selector(".clip", timeout=30000)
            page.wait_for_selector("#editorForm:not(.hide)")
            page.fill("#editTitle", "A better title")
            page.wait_for_function("document.querySelector('#dirtyFlag').classList.contains('hide')", timeout=10000)
            status = json.loads(page.evaluate("fetch('/api/status').then(r => r.text())"))
            assert status["project"]["clips"][0]["title"] == "A better title", status["project"]["clips"][0]["title"]

            page.focus("#clipList")
            page.keyboard.press("ArrowDown")
            page.wait_for_function("document.querySelector('.clip[aria-selected=\"true\"] .idx').textContent === '02'")

            page.click("button[data-route=settings]")
            page.select_option("#captionFont", "Arial")
            page.wait_for_function("fetch('/api/status').then(r => r.json()).then(s => s.project.config.captionFont === 'Arial')", timeout=10000)

            page.click("button[data-route=clips]")
            page.click("#renderBtn")
            page.wait_for_selector("#page-queue.visible")
            page.wait_for_function("[...document.querySelectorAll('.job')].some(j => /render|clip/i.test(j.textContent) && /complete|failed/.test(j.querySelector('.tag.complete, .tag.failed')?.textContent || ''))", timeout=240000)
            jobs = json.loads(page.evaluate("fetch('/api/jobs').then(r => r.text())"))
            render = next(j for j in jobs if j["kind"] == "render")
            assert render["status"] == "complete", render
            assert Path(render["output"]).exists(), render["output"]
            print("render ok:", render["renderer"], render["output"])
            browser.close()
    finally:
        server.terminate()
        try:
            server.wait(timeout=10)
        except subprocess.TimeoutExpired:
            server.kill()
        if "--keep" not in sys.argv:
            import shutil

            shutil.rmtree(work, ignore_errors=True)
    if errors:
        print("Console errors:\n" + "\n".join(errors))
        return 1
    print("UI smoke OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
