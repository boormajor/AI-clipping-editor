"""Scene-cut metadata used by the AI framing review (optional).

Lists timestamps where the picture changes sharply so the editorial model can
avoid placing a shot change across a camera cut. Cuts are visual changes, not
guaranteed speaker changes.

Install (optional):
    python -m pip install -r scripts/requirements.txt

Exit codes: 0 success, 2 missing optional dependency, 1 other failure. On any
failure a JSON object with an "error" key is written to --out.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


def write(out: str, payload: dict) -> None:
    Path(out).write_text(json.dumps(payload), encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--video", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--threshold", type=float, default=28.0, help="ContentDetector threshold; lower finds more cuts")
    parser.add_argument("--max-cuts", type=int, default=80)
    args = parser.parse_args()

    try:
        from scenedetect import ContentDetector, detect  # type: ignore
    except ImportError as exc:
        write(args.out, {"error": f"optional dependency missing: {exc.name}", "install": "python -m pip install -r scripts/requirements.txt"})
        return 2

    try:
        shots = detect(args.video, ContentDetector(threshold=args.threshold), show_progress=False)
    except Exception as exc:  # noqa: BLE001 - report instead of traceback
        write(args.out, {"error": f"scene detection failed: {exc}"})
        return 1

    cuts = [round(start.get_seconds(), 3) for start, _end in shots[1:]]
    write(
        args.out,
        {
            "cuts": cuts[: args.max_cuts],
            "truncated": len(cuts) > args.max_cuts,
            "threshold": args.threshold,
            "method": "PySceneDetect content threshold",
            "warning": "Scene cuts are visual changes, not guaranteed camera/speaker changes.",
        },
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
