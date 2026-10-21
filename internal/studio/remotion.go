package studio

import (
 "context"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strconv"
 "strings"
 "time"
)

type remotionAsset struct {Media string `json:"media"`;Start float64 `json:"start"`;Duration float64 `json:"duration"`}
type remotionProps struct {
 Media string `json:"media"`;Duration float64 `json:"duration"`;Width int `json:"width"`;Height int `json:"height"`;FPS int `json:"fps"`
 Layout string `json:"layout"`;Pan float64 `json:"pan"`;Shots []Shot `json:"shots"`;Cues []Cue `json:"cues"`;Words []Word `json:"words"`
 Assets []remotionAsset `json:"assets"`;Captions bool `json:"captions"`;CaptionPreset string `json:"captionPreset"`
 CaptionFont string `json:"captionFont"`;CaptionSize int `json:"captionSize"`;CaptionColor string `json:"captionColor"`
 AccentColor string `json:"accentColor"`;CaptionOffset float64 `json:"captionOffset"`
 Title string `json:"title"`;Callout string `json:"callout"`;Graphics bool `json:"graphics"`;Zoom bool `json:"zoom"`
}
func FindRemotion()(string,error) {
 roots:=[]string{}
 if env:=os.Getenv("PODCAST_STUDIO_REMOTION");env!=""{roots=append(roots,env)}
 if cwd,e:=os.Getwd();e==nil{roots=append(roots,filepath.Join(cwd,"remotion"))}
 if exe,e:=os.Executable();e==nil{roots=append(roots,filepath.Join(filepath.Dir(exe),"remotion"))}
 for _,root:=range roots{if _,e:=os.Stat(filepath.Join(root,"package.json"));e==nil{return root,nil}}
 return "",fmt.Errorf("Remotion folder is missing. Extract the entire Windows bundle")
}
func RemotionInstalled()bool {
 root,e:=FindRemotion();if e!=nil{return false}
 _,e=os.Stat(filepath.Join(root,"node_modules","@remotion","cli"));return e==nil
}
func makeRemotionProps(p Project,c Clip)(remotionProps,error){
 cfg:=p.Config
 if c.End<=c.Start||c.Start<0||c.End>p.Duration+.5{return remotionProps{},fmt.Errorf("invalid clip bounds")}
 if cfg.Width<240||cfg.Height<240||cfg.Width>3840||cfg.Height>3840{return remotionProps{},fmt.Errorf("invalid dimensions")}
 if cfg.FPS!=24&&cfg.FPS!=25&&cfg.FPS!=30&&cfg.FPS!=60{return remotionProps{},fmt.Errorf("invalid frame rate")}
 dur:=c.End-c.Start
 color:=func(s,otherwise string)string{
  if len(s)!=7||s[0]!='#'{return otherwise}
  for _,r:=range s[1:]{if !strings.ContainsRune("0123456789abcdefABCDEF",r){return otherwise}}
  return s
 }
 x:=remotionProps{Duration:dur,Width:cfg.Width,Height:cfg.Height,FPS:cfg.FPS,Layout:c.Layout,Pan:c.Pan,Shots:c.Shots,Cues:[]Cue{},Words:c.Words,
 Assets:[]remotionAsset{},Captions:cfg.Captions&&c.Captions,CaptionPreset:c.CaptionStyle,CaptionFont:validCaptionFont(cfg.CaptionFont),
 CaptionSize:cfg.CaptionSize,CaptionColor:color(cfg.CaptionColor,"#FFFFFF"),AccentColor:color(cfg.AccentColor,"#FACC15"),
 CaptionOffset:cfg.CaptionOffset,Title:c.Title,Callout:c.Callout,Graphics:cfg.Graphics,Zoom:cfg.Zoom}
 if x.CaptionPreset==""{x.CaptionPreset=cfg.CaptionStyle};if x.CaptionPreset==""{x.CaptionPreset="kinetic"}
 if x.CaptionSize<24||x.CaptionSize>150{x.CaptionSize=76}
 if x.Layout!="focus"&&x.Layout!="two-up"&&x.Layout!="dynamic"{x.Layout="two-up"}
 for _,cue:=range p.Cues{
  start,end:=cue.Start-c.Start,cue.End-c.Start
  if end<=0||start>=dur{continue}
  if start<0{start=0};if end>dur{end=dur}
  if end>start{x.Cues=append(x.Cues,Cue{Start:start,End:end,Text:cue.Text})}
 }
 for _,shot:=range x.Shots{if shot.Start<0||shot.End>dur+.1||shot.End<=shot.Start{return remotionProps{},fmt.Errorf("invalid shot range")}}
 return x,nil
}
func RenderRemotion(ctx context.Context,p Project,c Clip)(string,string,error){
 root,e:=FindRemotion();if e!=nil{return "","",e}
 if !RemotionInstalled(){return "","",fmt.Errorf("Remotion is required. Run npm install inside the remotion folder")}
 props,e:=makeRemotionProps(p,c);if e!=nil{return "","",e}
 folder:=p.OutputDir;if folder==""{folder=filepath.Join(filepath.Dir(p.VideoPath),"PodcastAI-Exports")}
 if e=os.MkdirAll(folder,0755);e!=nil{return "","",e}
 jobs:=filepath.Join(root,"public","jobs");if e=os.MkdirAll(jobs,0755);e!=nil{return "","",e}
 job,e:=os.MkdirTemp(jobs,"clip-");if e!=nil{return "","",e};defer os.RemoveAll(job)
 id:=filepath.Base(job);src:=filepath.Join(job,"source.mp4")
 cmd:=exec.CommandContext(ctx,"ffmpeg","-y","-ss",strconv.FormatFloat(c.Start,'f',3,64),"-i",p.VideoPath,"-t",strconv.FormatFloat(c.End-c.Start,'f',3,64),"-c:v","libx264","-preset","veryfast","-crf","18","-c:a","aac",src)
 if b,err:=cmd.CombinedOutput();err!=nil{return "",string(b),fmt.Errorf("preparing source clip failed: %w",err)}
 props.Media="jobs/"+id+"/source.mp4"
 if p.Config.Broll {
  for i,asset:=range c.Broll{
   if asset.Path==""||asset.Start<0||asset.Duration<=0{continue}
   a:=filepath.Join(job,fmt.Sprintf("asset-%d.mp4",i))
   convert:=exec.CommandContext(ctx,"ffmpeg","-y","-i",asset.Path,"-t",strconv.FormatFloat(asset.Duration,'f',3,64),"-an",a)
   if b,err:=convert.CombinedOutput();err!=nil{return "",string(b),fmt.Errorf("asset preparation failed: %w",err)}
   props.Assets=append(props.Assets,remotionAsset{Media:"jobs/"+id+"/"+filepath.Base(a),Start:asset.Start,Duration:asset.Duration})
  }
 }
 b,e:=json.Marshal(props);if e!=nil{return "","",e}
 propFile:=filepath.Join(job,"props.json");if e=os.WriteFile(propFile,b,0600);e!=nil{return "","",e}
 out:=filepath.Join(folder,c.ID+".mp4")
 npx:="npx";if runtime.GOOS=="windows"{npx="npx.cmd"}
 render:=exec.CommandContext(ctx,npx,"--no-install","remotion","render","src/index.ts","PodcastClip",out,"--props",propFile,"--codec","h264")
 render.Dir=root
 logs,err:=render.CombinedOutput()
 if err!=nil{return "",string(logs),fmt.Errorf("Remotion render failed: %w",err)}
 return out,string(logs),nil
}
func RenderTimeout()time.Duration{return 60*time.Minute}
