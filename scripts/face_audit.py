"""Face occupancy estimates for AI framing reports (not speaker identification).

Install: python -m pip install mediapipe==0.10.21 opencv-python-headless
Reads a temporary MP4 clip. Does not upload images or identify people.
"""
import argparse
import json
from pathlib import Path


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--video', required=True)
    ap.add_argument('--out', required=True)
    ap.add_argument('--sample-seconds', type=float, default=2.0)
    args=ap.parse_args()
    import cv2
    import mediapipe as mp
    cap=cv2.VideoCapture(args.video)
    fps=cap.get(cv2.CAP_PROP_FPS) or 25
    total=cap.get(cv2.CAP_PROP_FRAME_COUNT)/fps
    if total<=0:
        raise RuntimeError('Video is unreadable')
    samples=[]
    detector=mp.solutions.face_detection.FaceDetection(model_selection=1,min_detection_confidence=0.48)
    step=max(1.0,args.sample_seconds)
    t=0.
    while t<total and len(samples)<40:
        cap.set(cv2.CAP_PROP_POS_MSEC,t*1000)
        ok,frame=cap.read()
        if not ok:break
        width=frame.shape[1]
        if width>720:
            frame=cv2.resize(frame,(720,round(frame.shape[0]*720/width)))
        res=detector.process(cv2.cvtColor(frame,cv2.COLOR_BGR2RGB))
        boxes=[]
        for face in res.detections or []:
            box=face.location_data.relative_bounding_box
            x=max(0.,float(box.xmin));y=max(0.,float(box.ymin))
            w=min(1-x,float(box.width));h=min(1-y,float(box.height))
            if w<=0 or h<=0:continue
            boxes.append({'x':round(x,3),'y':round(y,3),'w':round(w,3),'h':round(h,3),
                          'edgeRisk':x<.055 or x+w>.945 or y<.03 or y+h>.97})
        samples.append({'at':round(t,2),'faces':boxes})
        t+=step
    detector.close();cap.release()
    both=sum(1 for x in samples if len(x['faces'])>=2)
    risk=sum(1 for x in samples for f in x['faces'] if f['edgeRisk'])
    result={'samples':samples,'samplesWithTwoFaces':both,'edgeRiskCount':risk,
            'twoPersonLikely':both>=max(2,len(samples)//2),
            'limitations':'Bounding boxes alone cannot determine who is speaking or detect semantic mistakes.'}
    Path(args.out).write_text(json.dumps(result),encoding='utf-8')

if __name__=='__main__':main()
