import React from 'react';
import {AbsoluteFill, Audio, Composition, Easing, interpolate, OffthreadVideo, Sequence, spring, staticFile, useCurrentFrame, useVideoConfig} from 'remotion';

type Word = {start:number; end:number; word:string};
type Cue = {start:number; end:number; text:string};
type Shot = {start:number; end:number; mode:'both'|'left'|'right'; pan?:number};
type Asset = {media:string; start:number; duration:number};

// Caption presets. Word-level animations (reveal/pop/wipe/box) only take effect
// when real WhisperX word timings are present; without them the whole SRT cue is
// shown with the group entrance animation, never fabricated per-word timing.
export type CaptionPreset =
  'kinetic'|'editorial'|'bold'|'minimal'|'karaoke'|'boxed'|'podcast'|'yellow'|'highlight'|
  'popline'|'wordbox'|'wipe'|'slide'|'bounce'|'typewriter';
type Entrance = 'fade'|'pop'|'slide'|'bounce';
type WordFx = 'none'|'color'|'punch'|'wipe'|'box';
type CaptionSpec = {entrance:Entrance; word:WordFx; reveal:boolean; box:boolean; accentAll:boolean; stroke:boolean};
const CAPTION_SPECS:Record<CaptionPreset,CaptionSpec>={
  kinetic:   {entrance:'pop',    word:'punch', reveal:false, box:false, accentAll:false, stroke:true},
  editorial: {entrance:'fade',   word:'none',  reveal:false, box:false, accentAll:false, stroke:false},
  bold:      {entrance:'fade',   word:'none',  reveal:false, box:false, accentAll:false, stroke:true},
  minimal:   {entrance:'fade',   word:'none',  reveal:false, box:false, accentAll:false, stroke:false},
  karaoke:   {entrance:'fade',   word:'color', reveal:false, box:false, accentAll:false, stroke:true},
  boxed:     {entrance:'pop',    word:'none',  reveal:false, box:true,  accentAll:false, stroke:false},
  podcast:   {entrance:'fade',   word:'none',  reveal:false, box:true,  accentAll:false, stroke:false},
  yellow:    {entrance:'fade',   word:'none',  reveal:false, box:false, accentAll:true,  stroke:true},
  highlight: {entrance:'fade',   word:'none',  reveal:false, box:true,  accentAll:true,  stroke:false},
  popline:   {entrance:'fade',   word:'punch', reveal:true,  box:false, accentAll:false, stroke:true},
  wordbox:   {entrance:'fade',   word:'box',   reveal:false, box:false, accentAll:false, stroke:true},
  wipe:      {entrance:'fade',   word:'wipe',  reveal:false, box:false, accentAll:false, stroke:true},
  slide:     {entrance:'slide',  word:'color', reveal:false, box:false, accentAll:false, stroke:true},
  bounce:    {entrance:'bounce', word:'punch', reveal:true,  box:false, accentAll:false, stroke:true},
  typewriter:{entrance:'fade',   word:'none',  reveal:true,  box:false, accentAll:false, stroke:true},
};
export type EditProps = {
  media:string; duration:number; width:number; height:number; fps:number;
  layout:'focus'|'two-up'|'dynamic'; pan:number; shots:Shot[];
  cues:Cue[]; words:Word[]; assets:Asset[];
  captions:boolean; captionPreset:CaptionPreset;
  captionFont:string; captionSize:number; captionColor:string; accentColor:string;
  captionOffset:number; title:string; callout:string; graphics:boolean; zoom:boolean; emphasis?:boolean;
};
// Power words / numbers get accent emphasis when the "emphasis" option is on.
const POWER=new Set(['now','new','free','stop','never','always','secret','how','why','best','worst','biggest','mistake','proven','instantly','fast','easy','money','growth','win','warning','huge','important','truth','results','million','billion','everyone','nobody','everything','nothing']);
function isEmph(raw:string):boolean{
 const w=raw.toLowerCase().replace(/[^a-z0-9]/g,'');
 if(!w)return false;
 if(/[0-9]/.test(w))return true;
 const letters=raw.replace(/[^A-Za-z]/g,'');
 if(letters.length>=2 && raw===raw.toUpperCase() && /[A-Z]/.test(raw))return true;
 return POWER.has(w);
}
const defaults:EditProps={media:'example.mp4',duration:5,width:1080,height:1920,fps:30,layout:'two-up',pan:.5,shots:[],cues:[],words:[],assets:[],captions:true,captionPreset:'kinetic',captionFont:'Arial',captionSize:76,captionColor:'#FFFFFF',accentColor:'#FACC15',captionOffset:0,title:'',callout:'',graphics:false,zoom:false,emphasis:false};
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
// One measured word, animated per the active caption spec. Nothing here invents
// timing: reveal/punch/wipe are all driven by the word's own measured start/end.
function WordSpan({w,spec,t,accent,base,emph}:{w:Word;spec:CaptionSpec;t:number;accent:string;base:string;emph:boolean}){
 const active=t>=w.start&&t<w.end;
 const revealed=!spec.reveal||t>=w.start-.001;
 const popIn=spec.reveal&&(spec.word==='punch')?interpolate(t-w.start,[0,.08,.2],[.6,1.12,1],{extrapolateLeft:'clamp',extrapolateRight:'clamp',easing:Easing.out(Easing.cubic)}):1;
 const activePunch=active&&spec.word==='punch'?1.08:1;
 const style:React.CSSProperties={display:'inline-block',transform:`scale(${popIn*activePunch})`,transformOrigin:'center bottom',opacity:revealed?1:0,transition:'none'};
 if(spec.word==='box'){
  if(active){style.background=accent;style.color='#0b1020';style.borderRadius='.14em';style.padding='0 .16em';(style as any).boxDecorationBreak='clone';(style as any).WebkitBoxDecorationBreak='clone';}
  else{style.color=base;}
 }else if(spec.word==='wipe'){
  if(active){
   const prog=clamp((t-w.start)/Math.max(.08,w.end-w.start),0,1)*100;
   style.backgroundImage=`linear-gradient(90deg, ${accent} ${prog}%, ${base} ${prog}%)`;
   (style as any).WebkitBackgroundClip='text';style.backgroundClip='text';style.color='transparent';(style as any).WebkitTextFillColor='transparent';
  }else{
   style.color=t>=w.end?accent:base; // spoken words stay accent, unspoken stay base
  }
 }else if(spec.word==='color'||spec.word==='punch'){
  style.color=active?accent:base;
 }else{
  style.color=base;
 }
 // Keyword emphasis: lift non-active power words/numbers to the accent color.
 if(emph&&!active&&spec.word!=='box'&&spec.word!=='wipe'&&style.color===base&&isEmph(w.word))style.color=accent;
 return <><span style={style}>{w.word}</span>{' '}</>;
}
function CaptionLayer({p}:{p:EditProps}){
 const frame=useCurrentFrame();const {fps,height}=useVideoConfig();const t=frame/fps-(p.captionOffset||0);
 if(!p.captions)return null;
 const aligned=(p.words||[]).length>0;
 const group=aligned?captionGroups(p.words).find(g=>t>=g[0].start&&t<g[g.length-1].end):undefined;
 const cue=aligned?undefined:(p.cues||[]).find(c=>t>=c.start&&t<c.end);
 if(!group&&!cue)return null;
 const spec=CAPTION_SPECS[p.captionPreset]||CAPTION_SPECS.kinetic;
 const accent=p.accentColor||'#FACC15';
 const baseColor=spec.accentAll?accent:(p.captionColor||'#fff');
 const cueStart=group?group[0].start:cue!.start;
 const since=Math.max(0,t-cueStart);
 const fade=interpolate(since,[0,.1,.23],[0,.93,1],{extrapolateRight:'clamp'});
 // Group entrance animation.
 let tx=0,scale=1;
 if(spec.entrance==='pop')scale=interpolate(since,[0,.09,.22],[.84,1.06,1],{extrapolateRight:'clamp',easing:Easing.out(Easing.cubic)});
 else if(spec.entrance==='slide')tx=interpolate(since,[0,.26],[54,0],{extrapolateRight:'clamp',easing:Easing.out(Easing.cubic)});
 else if(spec.entrance==='bounce')scale=spring({fps,frame:Math.max(0,Math.round(since*fps)),config:{damping:11,mass:.7,stiffness:150}});
 const box=spec.box;
 const fontSize=clamp(p.captionSize||76,30,150);
 const text=cue?.text||group?.map(w=>w.word).join(' ')||'';
 if(!text.trim())return null;
 return <AbsoluteFill style={{justifyContent:'flex-end',alignItems:'center',pointerEvents:'none',paddingBottom:Math.max(110,Math.round(height*.16))}}>
  <div style={{maxWidth:'91%',padding:box?'16px 24px':'10px 8px',borderRadius:box?22:8,background:box?'rgba(8,13,28,.88)':'transparent',transform:`translateY(${tx}px) scale(${scale})`,opacity:fade,textAlign:'center',fontFamily:p.captionFont||'Arial',fontWeight:850,fontSize,lineHeight:1.14,letterSpacing:-1.1,color:baseColor,textShadow:box?'0 2px 8px rgba(0,0,0,.5)':'0 4px 4px rgba(0,0,0,.95),0 0 12px rgba(0,0,0,.85)',WebkitTextStroke:spec.stroke?'2px rgba(0,0,0,.82)':'0',overflowWrap:'break-word'}}>
 {group ? group.map((w,i)=><WordSpan key={i} w={w} spec={spec} t={t} accent={accent} base={baseColor} emph={!!p.emphasis}/>)
   : (p.emphasis ? text.split(/\s+/).map((tok,i)=><React.Fragment key={i}><span style={{color:isEmph(tok)?accent:baseColor}}>{tok}</span>{' '}</React.Fragment>) : text)}
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
