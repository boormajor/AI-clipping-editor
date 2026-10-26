"""Face occupancy estimates used by the AI framing review (optional).

Reports approximate face bounding boxes sampled across a temporary clip so the
editorial model can pick a safe layout. It does not identify people and never
uploads frames.

Install (optional):
    python -m pip install -r scripts/requirements.txt

Exit codes: 0 success, 2 missing optional dependency, 1 other failure. On any
failure a JSON object with an "error" key is written to --out so the caller can
show a useful message instead of a traceback.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path


def write(out: str, payload: dict) -> None:
    Path(out).write_text(json.dumps(payload), encoding="utf-8")


def probe_duration(video: str) -> float:
    """Fallback duration via ffprobe for containers that report no frame count."""
    try:
        res = subprocess.run(
            ["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", video],
            capture_output=True,
            text=True,
            check=True,
            timeout=30,
        )
        return float(res.stdout.strip() or 0)
    except Exception:  # noqa: BLE001 - best effort only
        return 0.0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--video", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--samples", type=int, default=40, help="number of evenly spaced frames to inspect")
    ap.add_argument("--min-confidence", type=float, default=0.48)
    args = ap.parse_args()

    try:
        import cv2  # type: ignore
        import mediapipe as mp  # type: ignore
    except ImportError as exc:
        write(args.out, {"error": f"optional dependency missing: {exc.name}", "install": "python -m pip install -r scripts/requirements.txt"})
        return 2

    cap = cv2.VideoCapture(args.video)
    if not cap.isOpened():
        write(args.out, {"error": "video could not be opened"})
        return 1
    fps = cap.get(cv2.CAP_PROP_FPS) or 25.0
    total = cap.get(cv2.CAP_PROP_FRAME_COUNT) / fps if fps else 0.0
    if total <= 0:
        total = probe_duration(args.video)
    if total <= 0:
        cap.release()
        write(args.out, {"error": "video duration is unknown"})
        return 1

    samples = []
    count = max(4, min(args.samples, 120))
    step = total / count
    detector = mp.solutions.face_detection.FaceDetection(model_selection=1, min_detection_confidence=args.min_confidence)
    try:
        for i in range(count):
            t = i * step
            cap.set(cv2.CAP_PROP_POS_MSEC, t * 1000)
            ok, frame = cap.read()
            if not ok:
                break
            width = frame.shape[1]
            if width > 720:
                frame = cv2.resize(frame, (720, round(frame.shape[0] * 720 / width)))
            res = detector.process(cv2.cvtColor(frame, cv2.COLOR_BGR2RGB))
            boxes = []
            for face in res.detections or []:
                box = face.location_data.relative_bounding_box
                x = max(0.0, float(box.xmin))
                y = max(0.0, float(box.ymin))
                w = min(1 - x, float(box.width))
                h = min(1 - y, float(box.height))
                if w <= 0 or h <= 0:
                    continue
                boxes.append(
                    {
                        "x": round(x, 3),
                        "y": round(y, 3),
                        "w": round(w, 3),
                        "h": round(h, 3),
                        "edgeRisk": x < 0.055 or x + w > 0.945 or y < 0.03 or y + h > 0.97,
                    }
                )
            samples.append({"at": round(t, 2), "faces": boxes})
    finally:
        detector.close()
        cap.release()

    both = sum(1 for s in samples if len(s["faces"]) >= 2)
    risk = sum(1 for s in samples for f in s["faces"] if f["edgeRisk"])
    write(
        args.out,
        {
            "samples": samples,
            "sampleStepSeconds": round(step, 2),
            "samplesWithTwoFaces": both,
            "edgeRiskCount": risk,
            "twoPersonLikely": both >= max(2, len(samples) // 2),
            "limitations": "Bounding boxes alone cannot determine who is speaking or detect semantic mistakes.",
        },
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
