"""Align actual speech to words using WhisperX. Requires Python + whisperx."""
import argparse,json,re
from pathlib import Path
from difflib import SequenceMatcher

def words(s):
    return re.sub(r"[^\w\s]"," ",s.lower()).split()

def main():
    p=argparse.ArgumentParser()
    for key in ("audio","reference","output"):p.add_argument("--"+key,required=True)
    p.add_argument("--language",default="en")
    p.add_argument("--model",default="small")
    a=p.parse_args()
    import torch,whisperx
    device="cuda" if torch.cuda.is_available() else "cpu"
    audio=whisperx.load_audio(a.audio)
    model=whisperx.load_model(a.model,device=device,compute_type="float16" if device=="cuda" else "int8",language=a.language)
    result=model.transcribe(audio,batch_size=8)
    align,metadata=whisperx.load_align_model(language_code=a.language,device=device)
    result=whisperx.align(result["segments"],align,metadata,audio,device,return_char_alignments=False)
    output=[]
    for word in result.get("word_segments",[]):
        if "start" not in word or "end" not in word:continue
        if float(word["end"])<=float(word["start"]):continue
        output.append(dict(start=float(word["start"]),end=float(word["end"]),word=word["word"]))
    original=Path(a.reference).read_text(encoding="utf8")
    agreement=SequenceMatcher(None,words(original),words(" ".join(w["word"] for w in output))).ratio()
    msg="Check names, jargon and caption placement before publishing."
    if agreement<.75:msg="Transcript and recognized speech differ substantially; manual review required."
    Path(a.output).write_text(json.dumps(dict(words=output,referenceAgreement=agreement,warning=msg,method="WhisperX speech alignment")))

if __name__=="__main__":main()
