'use strict';
const $=id=>document.getElementById(id);
const P={simple:{captions:true,reframe:true,zoom:false,broll:false,stock:false,graphics:false,transitions:false,sfx:false},advanced:{captions:true,reframe:true,zoom:true,broll:true,stock:true,graphics:true,transitions:true,sfx:true}};
const features=[['Essential editing',[['captions','Generate subtitles'],['reframe','Vertical reframing with adjustable focus'],['zoom','Dynamic zooms and punch-ins']]],['Additional visuals',[['broll','B-roll video layers'],['stock','Search free stock videos (planned)'],['graphics','Motion graphic text callouts'],['transitions','Animated transitions (planned)']]],['Audio editing',[['sfx','Sound effects (planned)']]]];
let project={clips:[],cues:[],config:{}}, selectedID='', jobs=[], availableTools={}, currentPage='studio';
let provider={provider:'heuristic',model:'',apiKey:''};
async function api(path,body){const settings={method:body===undefined?'GET':'POST',headers:{'Accept':'application/json'}};if(body!==undefined){settings.headers['Content-Type']='application/json';settings.body=JSON.stringify(body)}const res=await fetch('/api/'+path,settings);let v;try{v=await res.json()}catch{throw Error('Unexpected server response ('+res.status+')')}if(!res.ok)throw Error(v.error||'Server error '+res.status);return v}
function toast(s,err=false){const el=$('toast');el.textContent=s;el.classList.toggle('error',err);el.classList.remove('hide');clearTimeout(el._h);el._h=setTimeout(()=>el.classList.add('hide'),5100)}
async function busy(btn,fn){if(btn.disabled)return;btn.disabled=true;let old=btn.textContent;btn.textContent='Working…';try{await fn()}catch(e){toast(e.message,true)}finally{btn.disabled=false;btn.textContent=old}}
function esc(s){return String(s??'').replace(/[&<>"']/g,x=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[x]))}
function secs(s){let m=Math.floor(s/60),n=Math.floor(s%60);return String(m).padStart(2,'0')+':'+String(n).padStart(2,'0')}
function setPage(s){currentPage=s;document.querySelectorAll('.page').forEach(x=>x.classList.toggle('visible',x.id==='page-'+s));document.querySelectorAll('[data-page]').forEach(x=>x.classList.toggle('active',x.dataset.page===s));$('pageTitle').textContent=({studio:'Studio',settings:'Editing settings',providers:'AI connections',exports:'Render queue'})[s]||s;if(s==='exports')refreshJobs()}
document.querySelectorAll('[data-page]').forEach(x=>x.onclick=()=>setPage(x.dataset.page));
function drawOptions(){const target=$('switches');target.innerHTML='';features.forEach(([group,items])=>{const wrap=document.createElement('div');wrap.className='switch-group';wrap.innerHTML='<h3>'+esc(group)+'</h3>';items.forEach(([key,label])=>{const el=document.createElement('label');el.className='switchline';el.innerHTML='<span>'+esc(label)+'</span><input type="checkbox" id="feature-'+key+'">';wrap.appendChild(el);el.querySelector('input').onchange=()=>{$('preset').value='custom'}});target.appendChild(wrap)})}
drawOptions();
$('preset').onchange=()=>{const preset=$('preset').value;if(P[preset]){for(const [key,on] of Object.entries(P[preset]))$('feature-'+key).checked=on}};
function syncConfig(cfg){$('preset').value=cfg.preset||'simple';Object.keys(P.advanced).forEach(k=>$('feature-'+k).checked=!!cfg[k]);$('captionStyle').value=cfg.captionStyle||'kinetic';$('captionEmphasis').checked=cfg.captionEmphasis===true;$('captionFont').value=cfg.captionFont||'Arial';$('captionSize').value=cfg.captionSize||76;$('captionWords').value=cfg.captionWords||3;$('captionOffset').value=cfg.captionOffset||0;$('captionColor').value=cfg.captionColor||'#ffffff';$('accentColor').value=cfg.accentColor||'#facc15';$('instructions').value=cfg.instructions||'';$('renderWidth').value=cfg.width||1080;$('renderHeight').value=cfg.height||1920;$('renderFPS').value=cfg.fps||30;$('gpu').checked=cfg.gpu!==false}
function formConfig(){let cfg={...project.config,preset:$('preset').value,captionStyle:$('captionStyle').value,captionEmphasis:$('captionEmphasis').checked,captionFont:$('captionFont').value,captionSize:Number($('captionSize').value),captionWords:Number($('captionWords').value),captionOffset:Number($('captionOffset').value),captionColor:$('captionColor').value,accentColor:$('accentColor').value,instructions:$('instructions').value,width:Number($('renderWidth').value),height:Number($('renderHeight').value),fps:Number($('renderFPS').value),gpu:$('gpu').checked};Object.keys(P.advanced).forEach(k=>cfg[k]=$('feature-'+k).checked);return cfg}
async function saveConfig(){const r=await api('config',formConfig());assign(r);toast('Editing preferences saved for this project')}
$('saveConfig').onclick=()=>busy($('saveConfig'),saveConfig);
document.querySelectorAll('.aspect').forEach(b=>b.onclick=()=>{$('renderWidth').value=b.dataset.w;$('renderHeight').value=b.dataset.h;document.querySelectorAll('.aspect').forEach(x=>x.classList.toggle('active',x===b))});
$('loadProject').onclick=()=>busy($('loadProject'),async()=>{const r=await api('load',{videoPath:$('videoPath').value.trim(),transcriptPath:$('transcriptPath').value.trim(),outputDir:$('outputDir').value.trim(),name:'',speechProvider:$('speechProvider').value,speechKey:$('speechKey').value.trim(),speechLanguage:$('speechLanguage').value});selectedID='';assign(r);toast('Podcast imported! Ready to discover highlights.')});
document.querySelectorAll('.browse').forEach(btn=>btn.onclick=()=>busy(btn,async()=>{let r=await api('pick',{kind:btn.dataset.kind});if(r.path)$(btn.dataset.target).value=r.path}));
$('analyze').onclick=()=>busy($('analyze'),async()=>{if(!project.videoPath)throw Error('Import a podcast video first');const res=await api('analyze',{count:Number($('clipCount').value),min:Number($('clipMin').value),max:Number($('clipMax').value),provider});selectedID='';assign(res);toast('Found '+project.clips.length+' candidate clips. Running AI editorial checks…');if(provider.provider!=='heuristic'&&provider.provider!=='local'&&provider.provider!==''){await reviewBatch()}});
// Review every suggested clip explicitly. A review is NOT an audio or pixel-level QA pass.
const reviewedClips=new Map();
async function reviewBatch(){
 const clips=[...(project.clips||[])];
 if(!clips.length)throw Error('Find clips first');
 if(['heuristic','local',''].includes(provider.provider))throw Error('Connect ChatGPT or Claude in AI Connections to review clips');
 const label=$('reviewBatchStatus');
 reviewedClips.clear();
 for(let i=0;i<clips.length;i++){
  label.textContent='AI editorial review '+(i+1)+' / '+clips.length+' · '+clips[i].title;
  try{
   const result=await api('review',{id:clips[i].id,provider});
   reviewedClips.set(clips[i].id,{ok:!!result.contextOK,issues:result.issues||[],summary:result.summary||''});
  }catch(e){reviewedClips.set(clips[i].id,{ok:false,issues:[e.message],summary:'AI review failed'})}
  renderClips();
 }
 const ok=[...reviewedClips.values()].filter(x=>x.ok).length;
 label.textContent='Editorial review complete: '+ok+'/'+clips.length+' context checks passed. This does not verify captions or rendered framing.';
 const state=await api('status');project=state.project;renderClips();renderEditor();
}
$('reviewAll').onclick=()=>busy($('reviewAll'),reviewBatch);
$('saveProject').onclick=$('saveTop').onclick=$('saveClips').onclick=()=>busy($('saveTop'),async()=>{await saveConfig();const r=await api('save',{});toast('Project saved: '+r.path)});
$('openProject').onclick=()=>busy($('openProject'),async()=>{let chosen=await api('pick',{kind:'project'});if(!chosen.path)return;let res=await api('open',{path:chosen.path});selectedID='';assign(res);toast('Project opened')});
$('provider').onchange=()=>{const p=$('provider').value;
 $('apiKeyGroup').classList.toggle('hide',p!=='openai'&&p!=='anthropic');
 $('chatgptConnectBox').classList.toggle('hide',p!=='chatgpt');
 $('claudeConnectBox').classList.toggle('hide',p!=='claude-code');
 const defaults={heuristic:'',chatgpt:'Select an available model below', 'claude-code':'Uses your Claude Code default',openai:'gpt-4.1-mini',anthropic:'claude-sonnet-4-5'};
 $('model').placeholder=defaults[p]||'';
 if(p==='chatgpt')refreshChatGPT();if(p==='claude-code')refreshClaude();
};$('provider').onchange();
async function refreshChatGPT(){
 try{const s=await api('auth/status');$('chatgptStatus').textContent=s.pending?'Authorization in progress…':s.connected?'Connected as '+(s.email||'ChatGPT user')+' · plan usage enabled':(s.error||'Not connected. Sign in and approve ChatGPT plan usage.');
 $('chatgptDisconnect').disabled=!s.connected;
 if(s.connected){const r=await api('auth/chatgpt/models');let list=r.models||[];$('availableModels').replaceChildren(...list.map(m=>{let el=document.createElement('option');el.value=m.slug;el.label=m.name||m.slug;return el}));if(list.length&&!$('model').value)$('model').value=list[0].slug;}
 }catch(e){$('chatgptStatus').textContent=e.message;}
}
$('chatgptConnect').onclick=()=>busy($('chatgptConnect'),async()=>{await api('auth/chatgpt/start',{});toast('Complete sign-in in the browser window opened by Podcast AI Studio.');let start=Date.now();let polling=setInterval(async()=>{try{const s=await api('auth/status');if(s.connected||s.error||Date.now()-start>300000){clearInterval(polling);refreshChatGPT();if(s.connected)toast('ChatGPT plan connected successfully');if(s.error)toast(s.error,true)}}catch{clearInterval(polling)}},2200)});
$('chatgptDisconnect').onclick=()=>busy($('chatgptDisconnect'),async()=>{await api('auth/chatgpt/disconnect',{});$('model').value='';$('availableModels').replaceChildren();await refreshChatGPT();toast('Local ChatGPT connection cleared. Remote app access can be revoked in ChatGPT settings.')});
$('chatgptModels').onclick=()=>busy($('chatgptModels'),refreshChatGPT);
async function refreshClaude(){try{const s=await api('auth/claude/status');$('claudeStatus').textContent=s.connected?'Connected: '+s.message:s.message}catch(e){$('claudeStatus').textContent=e.message}}
$('claudeLogin').onclick=()=>busy($('claudeLogin'),async()=>{await api('auth/claude/login',{});toast('Follow the login steps in the newly opened terminal, then click Recheck sign-in.')});
$('claudeCheck').onclick=()=>busy($('claudeCheck'),refreshClaude);

$('applyAI').onclick=()=>{provider={provider:$('provider').value,model:$('model').value.trim(),apiKey:$('apiKey').value.trim()};const labels={heuristic:'Free local heuristic',chatgpt:'ChatGPT plan', 'claude-code':'Claude Pro · Official CLI',openai:'OpenAI API · separate billing',anthropic:'Claude API · separate billing'};$('aiSummary').textContent=labels[provider.provider];toast('AI provider set for this session');setPage('studio')};
function assign(r){project=r.project;availableTools=r.tools||availableTools;$('videoPath').value=project.videoPath||$('videoPath').value;$('transcriptPath').value=project.transcriptPath||$('transcriptPath').value;$('outputDir').value=project.outputDir||$('outputDir').value;$('projectInfo').textContent=project.videoPath?(project.name+' · '+secs(project.duration)+' · '+project.cues.length+' transcript cues'):'No podcast imported yet';syncConfig(project.config||{});const missing=[];if(!r.remotion && !availableTools.remotion)missing.push('Remotion (npm install)');if(!availableTools.ffmpeg)missing.push('ffmpeg');if(!availableTools.ffprobe)missing.push('ffprobe');$('toolStatus').textContent=missing.length?'Missing: '+missing.join(', '):'● FFmpeg + Remotion ready';$('toolStatus').style.color=missing.length?'#ffb0b0':'';renderClips();if(selectedID&&!project.clips.some(c=>c.id===selectedID))selectedID='';if(!selectedID&&project.clips.length)selectedID=project.clips[0].id;renderClips();renderEditor()}
function renderClips(){const target=$('clipList');$('clipCountBadge').textContent=project.clips.length;if(!project.clips.length){target.innerHTML='<div class="empty-state"><div class="empty-glyph">✺</div><h3>Your best moments will appear here</h3><p>Import a podcast with timed subtitles, then click Find best moments.</p></div>';return}target.innerHTML=project.clips.map((c,i)=>'<div class="clip-card '+(c.id===selectedID?'selected':'')+'" data-id="'+esc(c.id)+'"><div class="clip-index">'+String(i+1).padStart(2,'0')+'</div><div class="clip-details"><h3>'+esc(c.title)+'</h3><p><span class="clip-time">'+secs(c.start)+' – '+secs(c.end)+'</span> &nbsp;·&nbsp; '+Math.round(c.end-c.start)+' sec</p></div><div class="clip-score">'+Math.round(c.score)+' / 100</div><div class="clip-review">'+(reviewedClips.has(c.id)?(reviewedClips.get(c.id).ok?'AI context checked':'Needs review: '+esc((reviewedClips.get(c.id).issues||[]).join('; ').slice(0,160))):'Editorial review pending')+'</div></div>').join('');document.querySelectorAll('.clip-card').forEach(x=>x.onclick=()=>{selectedID=x.dataset.id;renderClips();renderEditor()})}
function chosen(){return project.clips.find(c=>c.id===selectedID)}
function renderEditor(){const c=chosen();$('editorEmpty').classList.toggle('hide',!!c);$('editorForm').classList.toggle('hide',!c);$('renderSelected').disabled=!c;$('previewPlaceholder').classList.toggle('hide',!!project.videoPath);if(!c){$('previewTitle').textContent='Choose a clip to begin';return}
 $('editTitle').value=c.title;$('editLayout').value=c.layout||'focus';$('visionStatus').textContent='Optional local check.';$('syncStatus').textContent=c.words?.length?(c.words.length+' audio-aligned words'):'';$('aiReviewSummary').textContent='';$('editStart').value=c.start;$('editEnd').value=c.end;$('editPan').value=Math.round((c.pan??.5)*100);$('editCaptions').checked=!!c.captions;$('editCaptionStyle').value=c.captionStyle||'kinetic';$('editCallout').value=c.callout||'';$('brollPath').value='';$('panLabel').textContent=Math.round(c.pan*100)+'%';$('previewTitle').textContent=c.title;$('previewTime').textContent=secs(c.start)+' – '+secs(c.end);$('captionPreview').style.color=['yellow','highlight','karaoke'].includes(c.captionStyle)?'#ffee78':'#ffffff';$('captionPreview').style.display=project.config.captions&&c.captions?'block':'none';$('previewVideo').style.objectPosition=Math.round((c.pan??.5)*100)+'% center';renderBroll();drawTimeline();const player=$('previewVideo');if(player.dataset.path!==(project.videoPath||'')){player.src='/api/media';player.dataset.path=project.videoPath||''}try{player.currentTime=c.start}catch{}}
function renderBroll(){const c=chosen();if(!c)return;$('brollCount').textContent=(c.broll||[]).length;$('brollList').innerHTML=(c.broll||[]).map((b,i)=>'<div class="broll-entry"><span title="'+esc(b.path)+'">'+esc(b.path.split(/[\\/]/).pop())+' · '+b.start+'–'+(b.start+b.duration)+'s</span><button data-index="'+i+'" aria-label="Remove asset">✕</button></div>').join('');document.querySelectorAll('.broll-entry button').forEach(b=>b.onclick=()=>{c.broll.splice(Number(b.dataset.index),1);renderBroll();drawTimeline()})}
$('editPan').oninput=()=>{$('panLabel').textContent=$('editPan').value+'%';$('previewVideo').style.objectPosition=$('editPan').value+'% center'};
$('addBroll').onclick=()=>{const c=chosen();if(!c)return;const path=$('brollPath').value.trim();if(!path)return toast('Choose a local B-roll video file',true);if(!c.broll)c.broll=[];c.broll.push({path,start:Number($('brollStart').value),duration:Number($('brollDuration').value)});renderBroll();drawTimeline();toast('B-roll added to clip timeline. Click Apply to save it.')};
$('updateClip').onclick=()=>busy($('updateClip'),async()=>{const c=chosen();if(!c)return;const copy={...c,title:$('editTitle').value,start:Number($('editStart').value),end:Number($('editEnd').value),pan:Number($('editPan').value)/100,layout:$('editLayout').value,captions:$('editCaptions').checked,captionStyle:$('editCaptionStyle').value,callout:$('editCallout').value};await api('clip',copy);Object.assign(c,copy);renderClips();renderEditor();toast('Clip changes applied')});
$('renderSelected').onclick=()=>busy($('renderSelected'),async()=>{await saveConfig();let result=await api('render',{id:selectedID});toast('Render queued. Check the Render queue tab for progress.');setPage('exports');refreshJobs()});
const player=$('previewVideo');player.addEventListener('timeupdate',()=>{const c=chosen();if(!c)return;if(player.currentTime>c.end+.05&&player.currentTime>c.start){player.pause();player.currentTime=c.start};const cue=(project.cues||[]).find(x=>x.start+(project.config.captionOffset||0)<=player.currentTime&&x.end+(project.config.captionOffset||0)>=player.currentTime);$('captionPreview').textContent=cue?cue.text:' ';});
async function refreshJobs(){try{jobs=await api('jobs');const out=$('jobList');out.classList.toggle('empty-state',!jobs.length);out.innerHTML=jobs.length?jobs.map(j=>{
 const active=(j.status==='rendering'||j.status==='queued');
 const prog=active?'<div class="job-progress"><i style="width:'+(j.progress||0)+'%"></i></div>':'';
 const cancelBtn=active?'<button class="btn secondary small jobcancel" data-id="'+esc(j.id)+'">Cancel</button>':'';
 const view=j.status==='complete'?'<a class="btn secondary small" href="/api/media?id='+encodeURIComponent(j.id)+'" target="_blank" rel="noopener">View MP4 ↗</a>':'';
 const fb=j.renderer==='ffmpeg-fallback'?'<span class="jobstatus" title="Remotion was unavailable; exported with the FFmpeg fallback renderer">FFmpeg</span>':'';
 const label=active?('Rendering… '+(j.progress||0)+'%'):(j.output||j.error||'Done');
 return '<div class="job-row"><div style="flex:1;min-width:0"><b>'+esc(j.clipId)+'</b><br><small>'+esc(label)+'</small></div>'+prog+fb+'<span class="jobstatus '+esc(j.status)+'">'+esc(j.status)+'</span>'+cancelBtn+view+'</div>';
}).join(''):'No renders yet.';
out.querySelectorAll('.jobcancel').forEach(b=>b.onclick=()=>busy(b,async()=>{await api('render/cancel',{id:b.dataset.id});refreshJobs()}));
}catch(e){toast(e.message,true)}}
$('refreshJobs').onclick=refreshJobs;setInterval(()=>{if(currentPage==='exports')refreshJobs()},1600);
api('status').then(assign).catch(e=>toast(e.message,true));

let stockResults=[];
$('searchStock').onclick=()=>busy($('searchStock'),async()=>{if(!chosen())throw Error('Select a clip first');let r=await api('stock/search',{query:$('stockQuery').value.trim(),apiKey:$('pexelsKey').value.trim()});stockResults=r.videos||[];$('stockResults').innerHTML=stockResults.length?stockResults.map((v,i)=>'<button class="stock-item" data-idx="'+i+'" title="Download video from Pexels and add to this clip"><img src="'+esc(v.preview)+'" loading="lazy" alt="Stock preview"><span>'+esc(v.credit||'Pexels contributor')+'</span></button>').join(''):'<div class="stock-wait">No stock results. Try another search.</div>';document.querySelectorAll('.stock-item').forEach(b=>b.onclick=()=>busy(b,async()=>{let v=stockResults[Number(b.dataset.idx)];let result=await api('stock/download',{url:v.url,id:v.id});let c=chosen();if(!c)return;c.broll=c.broll||[];c.broll.push({path:result.path,start:Number($('brollStart').value),duration:Number($('brollDuration').value)});renderBroll();toast('Downloaded Pexels footage. Click Apply clip changes to keep it.') }))});

$('syncWords').onclick=()=>busy($('syncWords'),async()=>{
 const clip=chosen();if(!clip)throw Error('Choose a clip first');
 $('syncStatus').textContent='Analyzing actual audio with WhisperX. This can take several minutes…';
 const result=await api('align',{id:clip.id,model:'small',language:$('syncLang').value});
 const state=await api('status');assign(state);
 $('syncStatus').textContent=result.words+' recognized words · '+Math.round(result.agreement*100)+'% transcript agreement. '+result.warning;
 toast('Audio aligned. Review the recognition before exporting.');
});
$('aiReview').onclick=()=>busy($('aiReview'),async()=>{
 const clip=chosen();if(!clip)throw Error('Choose a clip first');
 $('aiReviewSummary').textContent='Online AI is reviewing context and composing a shot plan…';
 const result=await api('review',{id:clip.id,provider});
 const state=await api('status');assign(state);
 $('aiReviewSummary').textContent=(result.contextOK?'Context reviewed. ':'Missing context. ')+(result.issues||[]).join(' • ')+(result.faceData?' · Face metadata reviewed.':' · No measured face data available.');
 toast('AI updated the title and proposed framing. Review before rendering.');
});
// Batch: render every suggested clip
$('renderAll').onclick=()=>busy($('renderAll'),async()=>{
 const list=(project.clips||[]).filter(c=>c.selected!==false);
 if(!list.length)throw Error('No clips to render');
 await saveConfig();
 for(const c of list){try{await api('render',{id:c.id})}catch(e){toast('Render failed for '+c.id+': '+e.message,true)}}
 toast('Queued '+list.length+' renders. See the Render queue.');setPage('exports');refreshJobs();
});
// Clean up: filler-word & dead-air detection with optional edge trim
$('cleanClip').onclick=()=>busy($('cleanClip'),async()=>{
 const c=chosen();if(!c)throw Error('Choose a clip first');
 const r=await api('clean',{id:c.id});const rep=r.report||{};
 let msg=rep.fillers+' filler words · '+rep.silences+' long pauses';
 if(rep.deadAir>0.15){
  msg+=' · '+rep.deadAir.toFixed(1)+'s of dead air at the edges';
  $('cleanStatus').textContent=msg;
  if(confirm('Trim '+rep.deadAir.toFixed(1)+'s of leading/trailing dead air from this clip?')){
   $('editStart').value=Math.round(r.suggestedStart*100)/100;$('editEnd').value=Math.round(r.suggestedEnd*100)/100;drawTimeline();$('updateClip').click();
  }
 }else{
  $('cleanStatus').textContent=msg+' · edges already tight'+(rep.fillerWords&&rep.fillerWords.length?(' · fillers: '+rep.fillerWords.slice(0,8).join(', ')):'');
 }
});
// Transcript editor: fix recognized caption text for cues within this clip
$('editCaptionsBtn').onclick=()=>{
 const c=chosen();if(!c)return;const box=$('transcriptEditor');
 if(!box.classList.contains('hide')){box.classList.add('hide');box.innerHTML='';return}
 const within=(project.cues||[]).map((cue,idx)=>({cue,idx})).filter(x=>x.cue.end>c.start&&x.cue.start<c.end);
 if(!within.length){toast('No transcript cues overlap this clip',true);return}
 box.innerHTML=within.map(x=>'<div class="tcue"><span>'+secs(x.cue.start)+'</span><input data-idx="'+x.idx+'" value="'+esc(x.cue.text)+'"></div>').join('')+'<div class="te-actions"><button class="btn primary small" id="teSave">Save transcript</button><button class="btn secondary small" id="teCancel">Close</button></div>';
 box.classList.remove('hide');
 $('teCancel').onclick=()=>{box.classList.add('hide');box.innerHTML=''};
 $('teSave').onclick=()=>busy($('teSave'),async()=>{
  const cues=(project.cues||[]).map(cc=>({start:cc.start,end:cc.end,text:cc.text}));
  box.querySelectorAll('.tcue input').forEach(inp=>{const i=Number(inp.dataset.idx);if(cues[i])cues[i].text=inp.value});
  const state=await api('transcript',{cues});assign(state);
  box.classList.add('hide');box.innerHTML='';toast('Transcript updated. Re-render to see new captions.');
 });
};
// Generate honest YouTube/social metadata for the clip
$('genMeta').onclick=()=>busy($('genMeta'),async()=>{
 const c=chosen();if(!c)throw Error('Choose a clip first');
 const out=$('metaOutput');out.classList.remove('hide');out.innerHTML='<p class="muted smalltext">Writing titles &amp; tags…</p>';
 const r=await api('metadata',{id:c.id,platform:'youtube',provider});
 const block=(label,val)=>'<div class="meta-block"><h4>'+label+'</h4><div class="meta-val">'+esc(val)+'</div><button class="btn secondary small copy" data-copy="'+esc(val)+'">Copy</button></div>';
 const hashtags=(r.hashtags||[]).map(h=>'#'+h).join(' ');const tags=(r.tags||[]).join(', ');
 out.innerHTML=block('Title',r.title||'')+block('Description',r.description||'')+block('Hashtags',hashtags)+block('Tags',tags);
 out.querySelectorAll('.copy').forEach(b=>b.onclick=()=>{if(navigator.clipboard)navigator.clipboard.writeText(b.dataset.copy);toast('Copied')});
 toast('Metadata ready. Review before posting.');
});
function drawTimeline(){const c=chosen();if(!c)return;const start=Number($('editStart').value),end=Number($('editEnd').value),span=Math.max(1,end-start);const first=Math.max(0,Math.min(c.start,start)-10);const last=Math.min(project.duration,Math.max(c.end,end)+10);const total=Math.max(1,last-first);$('timeWindowStart').textContent=secs(first);$('timeWindowEnd').textContent=secs(last);const pct=n=>Math.max(0,Math.min(100,(n-first)/total*100));$('trimStart').style.left=pct(start)+'%';$('trimEnd').style.left=pct(end)+'%';$('trimSelection').style.left=pct(start)+'%';$('trimSelection').style.width=Math.max(0,pct(end)-pct(start))+'%';$('cueStripes').innerHTML=(project.cues||[]).filter(a=>a.end>=first&&a.start<=last).map(a=>'<div class="cue-stripe" title="'+esc(a.text)+'" style="left:'+pct(a.start)+'%;width:'+Math.max(.15,pct(a.end)-pct(a.start))+'%"></div>').join('');$('brollTrack').innerHTML=(c.broll||[]).map(v=>'<div class="broll-block" title="'+esc(v.path)+'" style="left:'+pct(start+v.start)+'%;width:'+Math.max(.2,pct(start+v.start+v.duration)-pct(start+v.start))+'%"></div>').join('');$('trimTrack').dataset.first=first;$('trimTrack').dataset.last=last;}
['editStart','editEnd'].forEach(id=>$(id).addEventListener('input',drawTimeline));
for(const [handle,key,other] of [['trimStart','editStart','editEnd'],['trimEnd','editEnd','editStart']]){const el=$(handle);el.addEventListener('pointerdown',e=>{e.preventDefault();el.setPointerCapture(e.pointerId)});el.addEventListener('pointermove',e=>{if(!el.hasPointerCapture(e.pointerId))return;const track=$('trimTrack'),rect=track.getBoundingClientRect(),first=Number(track.dataset.first),last=Number(track.dataset.last);let time=first+Math.max(0,Math.min(1,(e.clientX-rect.left)/rect.width))*(last-first);if(key==='editStart')time=Math.min(time,Number($(other).value)-.3);else time=Math.max(time,Number($(other).value)+.3);$(key).value=Math.round(time*100)/100;drawTimeline();const player=$('previewVideo');player.currentTime=time});el.addEventListener('pointerup',e=>{try{el.releasePointerCapture(e.pointerId)}catch{}})}
