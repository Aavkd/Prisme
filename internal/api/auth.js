'use strict';
const flow = location.hash.slice(1);
history.replaceState(null, '', '/auth');
const status = document.querySelector('#status');
const decode = v => Uint8Array.from(atob(v.replace(/-/g,'+').replace(/_/g,'/')), c => c.charCodeAt(0));
const encode = v => btoa(String.fromCharCode(...new Uint8Array(v))).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');
async function request(path, body) {
 const response = await fetch(path, {method:'POST',credentials:'omit',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
 if (!response.ok) throw new Error('Request rejected');
 return response.json();
}
async function authenticate(kind) {
 const buttons = document.querySelectorAll('button'); buttons.forEach(b=>b.disabled=true);
 try {
  if (!/^[a-f0-9]{64}$/.test(flow)) throw new Error('Invalid flow');
  const authorization = document.querySelector('#authorization').value;
  document.querySelector('#authorization').value='';
  const begin = await request('/v1/auth/begin',{flow_id:flow,kind,authorization});
  document.querySelector('#device').textContent=begin.device_name;
  const pk = begin.options.publicKey; pk.challenge=decode(pk.challenge);
  if (pk.user) pk.user.id=decode(pk.user.id);
  for (const field of ['allowCredentials','excludeCredentials']) for (const item of pk[field]||[]) item.id=decode(item.id);
  status.textContent='Confirmez sur votre appareil / Confirm on your device';
  const credential=kind==='login'?await navigator.credentials.get({publicKey:pk}):await navigator.credentials.create({publicKey:pk});
  const result={id:credential.id,rawId:encode(credential.rawId),type:credential.type,response:{},clientExtensionResults:credential.getClientExtensionResults()};
  for(const field of ['clientDataJSON','attestationObject','authenticatorData','signature','userHandle']) if(credential.response[field])result.response[field]=encode(credential.response[field]);
  if(credential.response.getTransports)result.response.transports=credential.response.getTransports();
  await request('/v1/auth/finish',{ceremony_id:begin.ceremony_id,credential:result});
  status.textContent='Connexion autorisée. Revenez dans Prisme. / Sign-in approved. Return to Prisme.';
 } catch (_) { status.textContent='Connexion annulée, expirée ou refusée. Réessayez depuis Prisme. / Sign-in cancelled, expired or rejected. Retry from Prisme.'; buttons.forEach(b=>b.disabled=false); }
}
document.querySelector('#login').addEventListener('click',()=>authenticate('login'));
document.querySelector('#register').addEventListener('click',()=>authenticate('register'));
document.addEventListener('visibilitychange',()=>{document.querySelector('#authorization').value='';});
