"""Optional scene boundary metadata; PySceneDetect is open source.

pip install 'scenedetect[opencv]'
"""
import argparse
import json
from pathlib import Path
from scenedetect import detect, ContentDetector

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--video', required=True)
    parser.add_argument('--out', required=True)
    a=parser.parse_args()
    shots=detect(a.video,ContentDetector(threshold=28.0),show_progress=False)
    result={'cuts':[round(start.get_seconds(),3) for start,end in shots[1:]][:80],
            'method':'PySceneDetect content threshold',
            'warning':'Scene cuts are visual changes, not guaranteed camera/speaker changes.'}
    Path(a.out).write_text(json.dumps(result),encoding='utf-8')
if __name__=='__main__':main()
