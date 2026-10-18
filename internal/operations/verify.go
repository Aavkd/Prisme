package operations

import "net/http"

func verifyPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="fr"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Prisme · Vérification</title><h1>Prisme · Vérification / Verification</h1><p id="target"></p><p>Vérifiez la cible et l'action dans l'application. / Check the target and action in the app.</p><button id="verify">Vérifier avec une passkey / Verify with passkey</button><p id="status" role="status"></p><script src="/v1/operations/verify.js" defer></script></html>`))
}
func verifyScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = w.Write([]byte(verifyJS))
}

const verifyJS = `'use strict';
const id=location.hash.slice(1);history.replaceState(null,'','/v1/operations/verify');
const status=document.getElementById('status'),button=document.getElementById('verify');
const decode=s=>Uint8Array.from(atob(s.replace(/-/g,'+').replace(/_/g,'/')),c=>c.charCodeAt(0));
const encode=b=>btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');
let options;
(async()=>{try{if(!/^[a-f0-9]{64}$/.test(id))throw Error();const r=await fetch('/v1/operations/challenges/'+id+'/options');if(!r.ok)throw Error();const v=await r.json();options=v.options;document.getElementById('target').textContent=v.target+' · '+v.action;}catch(_){button.disabled=true;status.textContent='Expiré / Expired';}})();
button.onclick=async()=>{button.disabled=true;try{
const pk=options.publicKey;pk.challenge=decode(pk.challenge);for(const c of pk.allowCredentials||[])c.id=decode(c.id);
const c=await navigator.credentials.get({publicKey:pk});const credential={id:c.id,rawId:encode(c.rawId),type:c.type,response:{clientDataJSON:encode(c.response.clientDataJSON),authenticatorData:encode(c.response.authenticatorData),signature:encode(c.response.signature),userHandle:c.response.userHandle?encode(c.response.userHandle):null},clientExtensionResults:c.getClientExtensionResults()};
const r=await fetch('/v1/operations/challenges/'+id+'/passkey',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({credential})});if(!r.ok)throw Error();status.textContent='Vérifié. Revenez dans Prisme. / Verified. Return to Prisme.';
}catch(_){status.textContent='Annulé ou expiré. / Cancelled or expired.';}};`
