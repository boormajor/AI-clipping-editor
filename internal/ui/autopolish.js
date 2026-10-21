'use strict';
// Post-render critic/revision loop, using the AI provider selected in app.js.
$('autoPolish').onclick=()=>busy($('autoPolish'),async()=>{
  const c=chosen();if(!c)throw Error('Select a clip first');
  if(['heuristic','local','ollama'].includes(provider.provider))throw Error('Choose ChatGPT or Claude in AI Connections first.');
  const copy={...c,title:$('editTitle').value,start:Number($('editStart').value),end:Number($('editEnd').value),pan:Number($('editPan').value)/100,layout:$('editLayout').value,captions:$('editCaptions').checked,captionStyle:$('editCaptionStyle').value,callout:$('editCallout').value};
  await api('clip',copy);
  await saveConfig();
  const result=await api('auto-polish',{id:copy.id,passes:Number($('autoPasses').value),provider});
  toast('Auto polish started — quality checks and revisions will run in the queue.');
  setPage('exports');await refreshJobs();
});
