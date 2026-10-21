package studio

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "mime/multipart"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "sort"
 "strconv"
 "strings"
 "time"
)

// CloudSpeech uses a dedicated hosted speech API, not ChatGPT Plus API credits.
// No model files or local transcription engines are installed.
type CloudSpeech struct {
 Provider string
 APIKey string
 Language string
}
const speechChunkSeconds = 470.0

func transcribePart(ctx context.Context, audioPath string, cfg CloudSpeech) ([]Word,error) {
 if strings.TrimSpace(cfg.APIKey)=="" {return nil,errors.New("Enter a cloud transcription API key (Groq or OpenAI); a ChatGPT Plus login is not a transcription API key")}
 endpoint,model:="",""
 switch cfg.Provider {
 case "groq": endpoint="https://api.groq.com/openai/v1/audio/transcriptions";model="whisper-large-v3-turbo"
 case "openai": endpoint="https://api.openai.com/v1/audio/transcriptions";model="whisper-1"
 default: return nil, errors.New("Choose Groq or OpenAI for cloud transcription")
 }
 f,err:=os.Open(audioPath);if err!=nil{return nil,err};defer f.Close()
 if st,err:=f.Stat();err!=nil{return nil,err}else if st.Size()>24*1024*1024{return nil,errors.New("audio chunk exceeds the cloud API's 25 MB limit")}
 body:=&bytes.Buffer{}
 mw:=multipart.NewWriter(body)
 _=mw.WriteField("model",model)
 _=mw.WriteField("response_format","verbose_json")
 _=mw.WriteField("timestamp_granularities[]","word")
 _=mw.WriteField("timestamp_granularities[]","segment")
 if cfg.Language!=""&&cfg.Language!="auto"{_=mw.WriteField("language",cfg.Language)}
 part,err:=mw.CreateFormFile("file","speech.mp3");if err!=nil{return nil,err}
 if _,err=io.Copy(part,f);err!=nil{return nil,err}
 if err=mw.Close();err!=nil{return nil,err}
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,endpoint,body);if err!=nil{return nil,err}
 req.Header.Set("Content-Type",mw.FormDataContentType())
 req.Header.Set("Authorization","Bearer "+cfg.APIKey)
 res,err:=(&http.Client{Timeout:4*time.Minute}).Do(req);if err!=nil{return nil,err};defer res.Body.Close()
 raw,err:=io.ReadAll(io.LimitReader(res.Body,8*1024*1024));if err!=nil{return nil,err}
 if res.StatusCode>=300{return nil,fmt.Errorf("%s transcription failed (HTTP %d): %s",cfg.Provider,res.StatusCode,truncate(string(raw),280))}
 var result struct{
  Words []Word `json:"words"`
  Text string `json:"text"`
 }
 if err=json.Unmarshal(raw,&result);err!=nil{return nil,fmt.Errorf("cloud transcription response: %w",err)}
 if len(result.Words)==0{return nil,fmt.Errorf("%s did not return word timestamps; check API tier and model support",cfg.Provider)}
 return result.Words,nil
}

// TranscribeVideoAudio extracts small MP3 chunks and asks the selected hosted
// Whisper API for measured word start/end times. It never guesses word times.
func TranscribeVideoAudio(ctx context.Context,video string,duration float64,cfg CloudSpeech)([]Word,error){
 if duration<=0{return nil,errors.New("video duration is missing")}
 if _,err:=exec.LookPath("ffmpeg");err!=nil{return nil,errors.New("FFmpeg is required to extract podcast audio")}
 tmp,err:=os.MkdirTemp("","podcast-cloud-speech-");if err!=nil{return nil,err}
 defer os.RemoveAll(tmp)
 all:=make([]Word,0)
 for base:=0.0;base<duration;base+=speechChunkSeconds{
  start:=base-1.0;if start<0{start=0}
  end:=base+speechChunkSeconds+1.0;if end>duration{end=duration}
  audio:=filepath.Join(tmp,"part.mp3")
  args:=[]string{"-hide_banner","-nostdin","-loglevel","error","-y","-ss",strconv.FormatFloat(start,'f',3,64),"-i",video,"-t",strconv.FormatFloat(end-start,'f',3,64),"-map","0:a:0","-vn","-ac","1","-ar","16000","-c:a","libmp3lame","-b:a","48k",audio}
  cmd:=exec.CommandContext(ctx,"ffmpeg",args...)
  if out,e:=cmd.CombinedOutput();e!=nil{return nil,fmt.Errorf("could not extract audio near %.0fs: %w: %s",base,e,truncate(string(out),200))}
  part,e:=transcribePart(ctx,audio,cfg)
  if e!=nil{return nil,fmt.Errorf("cloud transcription at %.0f–%.0f seconds: %w",base,end,e)}
  for _,w:=range part{
   w.Start+=start;w.End+=start
   middle:=(w.Start+w.End)/2
   if middle<base-0.01||middle>=base+speechChunkSeconds||w.Start<0||w.End<=w.Start||w.Start>duration+1{continue}
   w.Word=strings.TrimSpace(w.Word)
   if w.Word!=""{all=append(all,w)}
  }
  _=os.Remove(audio)
 }
 sort.SliceStable(all,func(i,j int)bool{return all[i].Start<all[j].Start})
 if len(all)==0{return nil,errors.New("speech service found no words in the video")}
 return all,nil
}
