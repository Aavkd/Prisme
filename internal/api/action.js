'use strict';
const id = location.hash.slice(1);
history.replaceState(null, '', '/action');
const status = document.getElementById('status');
const button = document.getElementById('verify');
const decode = s => Uint8Array.from(atob(s.replace(/-/g, '+').replace(/_/g, '/')), c => c.charCodeAt(0));
const encode = b => btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
button.onclick = async () => {
 button.disabled = true;
 try {
  if (!/^[a-f0-9]{64}$/.test(id)) throw Error();
  const response = await fetch(`/v1/action-challenges/${id}/options`);
  if (!response.ok) throw Error();
  const {options} = await response.json();
  const pk = options.publicKey;
  pk.challenge = decode(pk.challenge);
  for (const c of pk.allowCredentials || []) c.id = decode(c.id);
  const c = await navigator.credentials.get({publicKey: pk});
  const credential = {id:c.id, rawId:encode(c.rawId),type:c.type,response:{clientDataJSON:encode(c.response.clientDataJSON),authenticatorData:encode(c.response.authenticatorData),signature:encode(c.response.signature),userHandle:c.response.userHandle?encode(c.response.userHandle):null},clientExtensionResults:c.getClientExtensionResults()};
  const result = await fetch(`/v1/action-challenges/${id}/passkey`, {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({credential})});
  if (!result.ok) throw Error();
  status.textContent = 'Vérification réussie. Revenez dans Prisme. / Verified. Return to Prisme.';
 } catch (_) { status.textContent = 'Annulé ou expiré. Recommencez dans Prisme. / Cancelled or expired. Start again in Prisme.'; }
};
