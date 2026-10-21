import React from 'react';
import {AbsoluteFill, Audio, Composition, Easing, interpolate, OffthreadVideo, Sequence, spring, staticFile, useCurrentFrame, useVideoConfig} from 'remotion';

type Word = {start:number; end:number; word:string};
type Cue = {start:number; end:number; text:string};
type Shot = {start:number; end:number; mode:'both'|'left'|'right'; pan?:number};
type Asset = {media:string; start:number; duration:number};
export type EditProps = {
  media:string; duration:number; width:number; height:number; fps:number;
  layout:'focus'|'two-up'|'dynamic'; pan:number; shots:Shot[];
  cues:Cue[]; words:Word[]; assets:Asset[];
  captions:boolean; captionPreset:'kinetic'|'editorial'|'bold'|'minimal'|'karaoke'|'boxed'|'podcast'|'yellow'|'highlight';
  captionFont:string; captionSize:number; captionColor:string; accentColor:string;
  captionOffset:number; title:string; callout:string; graphics:boolean; zoom:boolean;
};
const defaults:EditProps={media:'example.mp4',duration:5,width:1080,height:1920,fps:30,layout:'two-up',pan:.5,shots:[],cues:[],words:[],assets:[],captions:true,captionPreset:'kinetic',captionFont:'Arial',captionSize:76,captionColor:'#FFFFFF',accentColor:'#FACC15',captionOffset:0,title:'',callout:'',graphics:false,zoom:false};
const clamp=(n:number,min:number,max:number)=>Math.max(min,Math.min(max,n));
function shotAt(t:number,p:EditProps):Shot {
 const found=p.shots?.find(s=>t>=s.start&&t<s.end);
 if(found)return found;
 if(p.layout==='two-up')return {start:0,end:p.duration,mode:'both'};
 if(p.layout==='focus')return {start:0,end:p.duration,mode:(p.pan??.5)<.5?'left':'right'};
 // Conservative alternate framing: most of the time show BOTH rather than exclude a speaker.
 const index=Math.floor(t/8);
 return {start:index*8,end:index*8+8,mode:index%3===0?'left':index%3===1?'both':'right'};
}
function Panel({media,position,zoom}:{media:string;position:'left'|'right'|'full';zoom:boolean}){
 const frame=useCurrentFrame();const {fps}=useVideoConfig();
 const z=zoom?interpolate(Math.sin((frame/fps)*.55),[-1,1],[1,1.055]):1;
 const pos=position==='left'?'left center':position==='right'?'right center':'center';
 return <AbsoluteFill style={{overflow:'hidden',background:'#060912'}}><OffthreadVideo src={staticFile(media)} muted style={{height:'100%',width:position==='full'?'100%':'200%',objectFit:'cover',objectPosition:pos,position:'absolute',left:position==='right' ? '-100%' : 0,transform:`scale(${z})`,transformOrigin:position==='right'?'75% center':'25% center'}} /></AbsoluteFill>;
}
function VideoStage({p}:{p:EditProps}){
 const frame=useCurrentFrame();const {fps,width,height}=useVideoConfig();const t=frame/fps;
 const shot=shotAt(t,p),isBoth=shot.mode==='both';
 const border=4;const pad=32;
 return <AbsoluteFill style={{background:'#090A12'}}>
 {isBoth ? <>
   <div style={{position:'absolute',left:pad,top:72,width:width-pad*2,height:Math.floor(height*.425),borderRadius:26,overflow:'hidden',border:`${border}px solid #293448`}}><Panel media={p.media} position="left" zoom={false}/></div>
   <div style={{position:'absolute',left:pad,top:Math.floor(height*.5),width:width-pad*2,height:Math.floor(height*.425),borderRadius:26,overflow:'hidden',border:`${border}px solid #293448`}}><Panel media={p.media} position="right" zoom={false}/></div>
 </> : <Panel media={p.media} position={shot.mode==='both'?'full':shot.mode} zoom={p.zoom}/>}
 <div style={{position:'absolute',inset:0,background:'linear-gradient(0deg,rgba(0,0,0,.52) 0%,rgba(0,0,0,0) 42%)',pointerEvents:'none'}}/>
 </AbsoluteFill>;
}
// Caption timing comes from WhisperX word alignment when available; otherwise use
// the *whole SRT cue*, never fabricated evenly spaced word timings.
// Group *measured* words by pauses and short, phone-readable phrases.
// No timestamp interpolation: a word is highlighted only inside its measured range.
function captionGroups(words:Word[]):Word[][]{
 const groups:Word[][]=[];let current:Word[]=[];
 for(const word of words){
  if(current.length>0){const last=current[current.length-1];
   if(word.start-last.end>.32 || word.end-current[0].start>2.35 || current.length>=6){groups.push(current);current=[];}
  }
  current.push(word);
 }
 if(current.length)groups.push(current);
 return groups;
}
function CaptionLayer({p}:{p:EditProps}){
 const frame=useCurrentFrame();const {fps,height}=useVideoConfig();const t=frame/fps-(p.captionOffset||0);
 if(!p.captions)return null;
 const aligned=(p.words||[]).length>0;
 const group=aligned?captionGroups(p.words).find(g=>t>=g[0].start&&t<g[g.length-1].end):undefined;
 const cue=aligned?undefined:(p.cues||[]).find(c=>t>=c.start&&t<c.end);
 if(!group&&!cue)return null;
 const cueStart=group?group[0].start:cue!.start;
 const since=Math.max(0,t-cueStart);
 const fade=interpolate(since,[0,.1,.23],[0,.93,1],{extrapolateRight:'clamp'});
 const scale=p.captionPreset==='kinetic'?interpolate(since,[0,.09,.22],[.84,1.06,1],{extrapolateRight:'clamp',easing:Easing.out(Easing.cubic)}):1;
 const preset=p.captionPreset;
 const box=preset==='boxed'||preset==='podcast'||preset==='highlight';
 // 'yellow' and 'highlight' tint the whole caption with the accent color; other
 // presets use the chosen caption color, with the accent reserved for the active
 // word in word-aligned (karaoke/kinetic) renders.
 const accentText=preset==='yellow'||preset==='highlight';
 const baseColor=accentText?(p.accentColor||'#FACC15'):(p.captionColor||'#fff');
 const fontSize=clamp(p.captionSize||76,30,150);
 const text=cue?.text||group?.map(w=>w.word).join(' ')||'';
 if(!text.trim())return null;
 return <AbsoluteFill style={{justifyContent:'flex-end',alignItems:'center',pointerEvents:'none',paddingBottom:Math.max(110,Math.round(height*.16))}}>
  <div style={{maxWidth:'91%',padding:box?'16px 24px':'10px 8px',borderRadius:box?22:8,background:box?'rgba(8,13,28,.88)':'transparent',transform:`scale(${scale})`,opacity:fade,textAlign:'center',fontFamily:p.captionFont||'Arial',fontWeight:850,fontSize,lineHeight:1.11,letterSpacing:-1.1,color:baseColor,textShadow:box?'0 2px 8px rgba(0,0,0,.5)':'0 4px 4px rgba(0,0,0,.95),0 0 12px rgba(0,0,0,.85)',WebkitTextStroke:preset==='bold'||preset==='kinetic'?'2px rgba(0,0,0,.82)':'0',overflowWrap:'break-word'}}>
 {group ? group.map((w,i)=><React.Fragment key={i}><span style={{color:t>=w.start&&t<w.end && (preset==='karaoke'||preset==='kinetic')?p.accentColor:baseColor,display:'inline-block',transform:t>=w.start&&t<w.end && preset==='kinetic'?'scale(1.075)':'scale(1)'}}>{w.word}</span>{' '}</React.Fragment>) : text}
 </div>
 </AbsoluteFill>;
}
function GraphicsLayer({p}:{p:EditProps}){
 const frame=useCurrentFrame();const {fps}=useVideoConfig();
 if(!p.graphics || !p.callout.trim() || frame/fps>3.4)return null;
 const scale=spring({fps,frame,config:{damping:18,mass:.8,stiffness:130}});
 return <div style={{position:'absolute',top:90,left:60,right:60,transform:`translateY(${(1-scale)*-70}px)`,opacity:scale,background:'rgba(12,15,28,.92)',borderLeft:`8px solid ${p.accentColor}`,padding:'20px 24px',borderRadius:14,color:'white',fontFamily:p.captionFont,fontWeight:800,fontSize:48,textAlign:'center'}}>{p.callout}</div>;
}
export const PodcastClip:React.FC<EditProps>=(p)=>{
 const frame=useCurrentFrame();const {fps}=useVideoConfig();
 return <AbsoluteFill style={{overflow:'hidden',background:'#090A12'}}>
 <VideoStage p={p}/>
 {/* Dialogue audio is emitted only ONCE from the main OffthreadVideo, separately
     from the multiple muted visual panels. */}
 <Audio src={staticFile(p.media)}/>
 {p.assets.map((asset,i)=><Sequence key={i} from={Math.round(asset.start*fps)} durationInFrames={Math.max(1,Math.round(asset.duration*fps))}>
 <OffthreadVideo muted src={staticFile(asset.media)} style={{position:'absolute',width:'100%',height:'100%',objectFit:'cover'}}/>
 </Sequence>)}
 <GraphicsLayer p={p}/>
 <CaptionLayer p={p}/>
 </AbsoluteFill>;
};
export const PodcastRoot:React.FC=()=> <Composition id="PodcastClip" component={PodcastClip} defaultProps={defaults} durationInFrames={150} fps={30} width={1080} height={1920} calculateMetadata={({props})=>({durationInFrames:Math.max(1,Math.ceil(props.duration*props.fps)),fps:props.fps,width:props.width,height:props.height})}/>;
