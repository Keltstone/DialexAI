'use strict';
// Every value from the API goes through textContent; nothing is ever assigned to innerHTML.
const $ = (id) => document.getElementById(id);
const SEV = ['info', 'low', 'medium', 'high', 'critical'];
let me = null, projects = [], project = '';

function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
    else if (v !== false && v != null) el.setAttribute(k, v);
  }
  for (const k of kids.flat()) if (k != null) el.append(k.nodeType ? k : String(k));
  return el;
}
const day = (t) => (t ? new Date(t * 1000).toISOString().slice(0, 10) : '');
const sev = (n) => h('span', { class: 'sev ' + SEV[n] }, SEV[n]);

function toast(msg) {
  const t = h('div', {}, msg);
  $('toast').append(t);
  setTimeout(() => t.remove(), 5000);
}

async function api(path, opts = {}) {
  const init = { credentials: 'same-origin', ...opts, headers: { 'X-Requested-With': 'sentinix' } };
  if (opts.body !== undefined) { init.body = JSON.stringify(opts.body); init.headers['Content-Type'] = 'application/json'; }
  const r = await fetch('/api' + path, init);
  if (r.status === 401) { location.href = '/auth/login'; throw new Error('sign in required'); }
  if (!r.ok) throw new Error((await r.text()).trim() || r.statusText);
  const text = await r.text();
  return text ? JSON.parse(text) : null;
}
const act = (fn) => async (e) => { try { await fn(e); route(); } catch (err) { toast(err.message); } };
const q = (o) => new URLSearchParams(Object.entries(o).filter(([, v]) => v)).toString();

function table(head, rows) {
  return h('div', { class: 'scroll' }, h('table', {},
    h('tr', {}, head.map((c) => h('th', {}, c))),
    rows.length ? rows : h('tr', {}, h('td', { colspan: head.length, class: 'mute' }, 'Nothing here.'))));
}

const views = {
  async posture() {
    const p = await api('/posture?' + q({ project }));
    const tile = (i) => h('div', { class: 'card' }, h('div', { class: 'num ' + SEV[i] }, p.open[i]), SEV[i]);
    return h('div', {}, h('h1', {}, 'Posture'),
      h('div', { class: 'grid' }, [4, 3, 2, 1, 0].map(tile)),
      h('p', { class: 'mute' }, p.avg_fix_days ? `Average time to fix: ${p.avg_fix_days.toFixed(1)} days` : 'No fixes recorded yet.'),
      h('h2', {}, 'New vs fixed per week'),
      table(['Week', 'New', 'Fixed'], (p.weekly || []).map((w) => h('tr', {}, h('td', {}, day(w.Week || w.week)), h('td', {}, w.New), h('td', {}, w.Fixed)))),
      h('h2', {}, 'Oldest open criticals'),
      table(['Rule', 'Where', 'Since'], (p.oldest_criticals || []).map((f) => h('tr', {}, h('td', {}, h('a', { href: '#/finding/' + f.ID }, f.rule)), h('td', {}, f.file), h('td', {}, day(f.FirstSeen))))));
  },
  async findings() {
    const status = new URLSearchParams(location.hash.split('?')[1] || '').get('status') || 'open';
    const fs = await api('/findings?' + q({ project, status }));
    const sel = h('select', { 'aria-label': 'Status', onchange: (e) => { location.hash = '#/findings?status=' + e.target.value; } },
      ['open', 'fixed', 'accepted', 'false_positive'].map((s) => h('option', { value: s, selected: s === status }, s)));
    return h('div', {}, h('h1', {}, 'Findings'), h('div', { class: 'row' }, sel,
      h('a', { href: `/api/projects/${project}/report/html`, target: '_blank', rel: 'noopener' }, project ? 'Report' : '')),
      table(['Severity', 'Rule', 'Where', 'Owner', 'Ticket'], fs.map((f) => h('tr', {},
        h('td', {}, sev(f.severity)), h('td', {}, h('a', { href: '#/finding/' + f.ID }, f.rule)),
        h('td', {}, `${f.file}:${f.line}`), h('td', {}, f.Owner), h('td', {}, f.Ticket)))));
  },
  async finding() {
    const d = await api("/findings/" + location.hash.split("/")[2]); const f = d.finding, vd = d.verdict;
    const owner = h('input', { placeholder: 'owner', value: f.Owner, 'aria-label': 'Owner' });
    const reason = h('input', { placeholder: 'accept reason', 'aria-label': 'Reason' });
    const until = h('input', { type: 'date', 'aria-label': 'Accept until' });
    const triage = (status, extra = {}) => act(async () => {
      const r = await api(`/findings/${f.ID}/triage`, { method: 'POST', body: { status, owner: owner.value, ...extra } });
      if (r && r.approval) toast('Sent for approval by a second reviewer.');
    });
    return h('div', {}, h('h1', {}, f.rule), h('p', {}, sev(f.severity), ' ', f.message),
      h('div', { class: 'card' }, h('div', {}, `${f.file}:${f.line}`), h('pre', {}, f.evidence || ''),
        f.cwe ? h('div', { class: 'mute' }, f.cwe) : null, f.fix ? h('p', {}, 'Fix: ', f.fix) : null),
      vd ? h('div', { class: 'card' }, h('strong', {}, 'AI verdict (advisory): '), (vd.real ? 'real' : 'likely false positive') + `, ${vd.severity}, confidence ${Number(vd.confidence).toFixed(2)}. `, vd.reasoning,
        h('div', { class: 'mute' }, 'Models: ' + (vd.models || []).join(', ') + ((vd.dissent || []).length ? '. Disagreed: ' + vd.dissent.join(', ') : ''))) : null,
      h('p', { class: 'mute' }, `Status: ${f.Status}. First seen ${day(f.FirstSeen)}, last seen ${day(f.LastSeen)}. ${f.Ticket ? 'Ticket: ' + f.Ticket : ''}`),
      h('div', { class: 'row' }, owner, h('button', { class: 'plain', onclick: triage(f.Status) }, 'Assign'),
        h('button', { class: 'plain', onclick: triage('false_positive') }, 'False positive'),
        h('button', { class: 'plain', onclick: triage('open') }, 'Reopen')),
      h('div', { class: 'row' }, reason, until, h('button', { onclick: act(async () => {
        const r = await api(`/findings/${f.ID}/triage`, { method: 'POST', body: { status: 'accepted', owner: owner.value, reason: reason.value, until: Math.floor(new Date(until.value).getTime() / 1000) } });
        if (r) toast('Sent for approval by a second reviewer.');
      }) }, 'Accept risk')),
      h('div', { class: 'row' }, h('button', { class: 'plain', onclick: act(async () => { await api(`/findings/${f.ID}/ai/triage`, { method: 'POST' }); toast('AI triage done.'); }) }, 'AI triage'),
        h('button', { class: 'plain', onclick: act(async () => { const r = await api(`/findings/${f.ID}/ai/patch`, { method: 'POST' }); $('main').append(h('pre', {}, r.patch)); }) }, 'Draft patch')),
      h('div', { class: 'row' }, ['jira', 'linear'].map((t) => h('button', { class: 'plain', onclick: act(async () => {
        const r = await api(`/findings/${f.ID}/ticket`, { method: 'POST', body: { tracker: t, due: '' } });
        toast('Ticket ' + r.ticket);
      }) }, 'Create ' + t + ' ticket'))));
  },
  async surface() {
    if (!project) return h('p', {}, 'Pick a project.');
    const rows = await api(`/projects/${project}/surface`);
    return h('div', {}, h('h1', {}, 'Attack surface'), table(['When', 'Type', 'Change', 'Where', 'Detail'],
      rows.map((r) => h('tr', {}, h('td', {}, day(r.TS)), h('td', {}, r.type), h('td', {}, r.change), h('td', {}, `${r.file} (PR ${r.PR})`), h('td', {}, r.detail)))));
  },
  async scans() {
    if (!project) return h('p', {}, 'Pick a project.');
    const ss = await api(`/projects/${project}/scans`);
    return h('div', {}, h('h1', {}, 'Scans'),
      h('div', { class: 'row' }, h('button', { onclick: act(async () => { await api(`/projects/${project}/scan`, { method: 'POST' }); toast('Scan queued.'); }) }, 'Scan now')),
      table(['Started', 'Mode', 'Commit', 'New', 'Fixed', 'Unchanged', 'Engines', 'Packs'], ss.map((s) => h('tr', {},
        h('td', {}, day(s.Started)), h('td', {}, s.Mode), h('td', {}, (s.Commit || '').slice(0, 8)), h('td', {}, s.New), h('td', {}, s.Fixed),
        h('td', {}, s.Unchanged), h('td', {}, s.Engines), h('td', {}, s.Packs)))));
  },
  async packs() {
    const d = await api('/packs');
    return h('div', {}, h('h1', {}, 'Rule packs'),
      table(['Pack', 'Version', 'Stacks', 'Rules'], (d.packs || []).map((p) => h('tr', {}, h('td', {}, p.Name), h('td', {}, p.Version), h('td', {}, (p.Stacks || []).join(', ')), h('td', {}, p.Rules)))),
      h('h2', {}, 'Rule precision (90 days)'),
      table(['Rule', 'Findings', 'Dismissed', 'Precision'], (d.rule_stats || []).map((r) => h('tr', {}, h('td', {}, r.Rule), h('td', {}, r.Total), h('td', {}, r.Dismissed), h('td', {}, (r.Precision * 100).toFixed(0) + '%')))));
  },
  async approvals() {
    const as = await api('/approvals');
    const decide = (id, approve) => act(() => api(`/approvals/${id}/decide`, { method: 'POST', body: { approve } }));
    return h('div', {}, h('h1', {}, 'Approvals'), table(['Kind', 'Proposed by', 'Change', ''], as.map((a) => h('tr', {},
      h('td', {}, a.Kind), h('td', {}, a.Proposer), h('td', {}, h('pre', {}, JSON.stringify(a.Payload, null, 1))),
      h('td', {}, h('button', { onclick: decide(a.ID, true) }, 'Approve'), ' ', h('button', { class: 'plain', onclick: decide(a.ID, false) }, 'Reject'))))));
  },
  async audit() {
    const log = await api('/audit');
    return h('div', {}, h('h1', {}, 'Audit log'), table(['When', 'Actor', 'Action', 'Target', 'Commit'],
      log.map((a) => h('tr', {}, h('td', {}, new Date(a.TS * 1000).toISOString()), h('td', {}, a.Actor), h('td', {}, a.Action), h('td', {}, a.Target), h('td', {}, (a.Commit || '').slice(0, 8))))));
  },
  async settings() {
    const name = h('input', { placeholder: 'name', 'aria-label': 'Name' }), host = h('input', { placeholder: 'host (config name)', 'aria-label': 'Host' }), repo = h('input', { placeholder: 'owner/repo', 'aria-label': 'Repository' });
    const out = [h('h1', {}, 'Settings'),
      h('h2', {}, 'Projects'), table(['Name', 'Host', 'Repo', 'Schedule', 'AI', 'Model policy'], projects.map((p) => h('tr', {},
        h('td', {}, p.Name), h('td', {}, p.Host), h('td', {}, p.Repo), h('td', {}, p.Schedule || '-'), h('td', {}, p.AI ? 'on' : 'off'), h('td', {}, p.ModelPolicy)))),
      h('div', { class: 'row' }, name, host, repo, h('button', { onclick: act(() => api('/projects', { method: 'POST', body: { Name: name.value, Host: host.value, Repo: repo.value } })) }, 'Add project'))];
    if (project) {
      const p = projects.find((x) => x.ID === project) || {};
      const sched = h('input', { placeholder: 'nightly | weekly | cron', value: p.Schedule || '', 'aria-label': 'Schedule' });
      const policy = h('select', { 'aria-label': 'Model policy' }, ['local-only', 'cloud-redacted', 'cloud'].map((m) => h('option', { value: m, selected: m === p.ModelPolicy }, m)));
      const ai = h('input', { type: 'checkbox', checked: p.AI ? 'checked' : false, 'aria-label': 'AI add-on' });
      out.push(h('h2', {}, 'This project'), h('div', { class: 'row' }, sched, policy, h('label', {}, ai, ' AI add-on'),
        h('button', { onclick: act(async () => { await api('/projects/' + project, { method: 'PATCH', body: { Schedule: sched.value, ModelPolicy: policy.value, AI: ai.checked } }); await loadProjects(); }) }, 'Save')),
        h('div', { class: 'row' }, h('button', { class: 'plain', onclick: act(async () => { const t = await api(`/projects/${project}/tokens`, { method: 'POST', body: { Name: 'ci' } }); prompt('CI token (shown once)', t.token); }) }, 'Create CI token')));
    }
    try {
      const us = await api('/users');
      out.push(h('h2', {}, 'Users'), table(['Email', 'Role'], us.map((u) => h('tr', {}, h('td', {}, u.Email), h('td', {},
        h('select', { 'aria-label': 'Role for ' + u.Email, onchange: act((e) => api('/users/role', { method: 'PUT', body: { Email: u.Email, Role: e.target.value } })) },
          ['owner', 'admin', 'reviewer', 'developer', 'auditor'].map((r) => h('option', { value: r, selected: r === u.Role }, r))))))));
    } catch { /* not allowed to manage users */ }
    return h('div', {}, out);
  },
};

const tabs = [['posture', 'Posture'], ['findings', 'Findings'], ['surface', 'Attack surface'], ['scans', 'Scans'], ['packs', 'Rule packs'], ['approvals', 'Approvals'], ['audit', 'Audit'], ['settings', 'Settings']];

async function route() {
  const name = (location.hash.slice(2).split(/[/?]/)[0]) || 'posture';
  const view = views[name] || views.posture;
  $('nav').replaceChildren(...tabs.map(([k, label]) => h('a', { href: '#/' + k, 'aria-current': k === name ? 'page' : false }, label)));
  try { $('main').replaceChildren(await view()); } catch (e) { $('main').replaceChildren(h('p', { class: 'critical' }, e.message)); }
}

async function loadProjects() {
  projects = await api('/projects');
  const sel = $('project');
  sel.replaceChildren(h('option', { value: '' }, 'All projects'), ...projects.map((p) => h('option', { value: p.ID, selected: p.ID === project }, p.Name)));
}

(async () => {
  try {
    me = await api('/me');
    $('who').textContent = `${me.email} (${me.role})`;
    await loadProjects();
    $('project').addEventListener('change', (e) => { project = e.target.value; route(); });
    addEventListener('hashchange', route);
    route();
  } catch (e) { $('main').textContent = e.message; }
})();
