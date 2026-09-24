function _t(key, opts) {
 return typeof window.t === 'function' ? window.t(key, opts) : key;
}
let skillsList = [];
let currentEditingSkillName = null;
let skillModalAddMode = true;
let skillActivePath = 'SKILL.md';
let skillFileDirty = false;
let skillPackageFiles = [];
let skillModalControlsWired = false;
let isSavingSkill = false; // 
let skillsSearchKeyword = '';
let skillsSearchTimeout = null; // 
let skillsAutoRefreshTimer = null;
let isAutoRefreshingSkills = false;
const SKILLS_AUTO_REFRESH_INTERVAL_MS = 5000;
let skillsPagination = {
 currentPage: 1,
 pageSize: 20, // 20(,localStorage)
 total: 0
};
let skillsStats = {
 total: 0,
 totalCalls: 0,
 totalSuccess: 0,
 totalFailed: 0,
 skillsDir: '',
 stats: []
};

function isSkillsManagementPageActive() {
 const page = document.getElementById('page-skills-management');
 return !!(page && page.classList.contains('active'));
}

function shouldSkipSkillsAutoRefresh() {
 if (isSavingSkill || currentEditingSkillName) {
 return true;
 }

 const modal = document.getElementById('skill-modal');
 if (modal && modal.style.display === 'flex') {
 return true;
 }

 const searchInput = document.getElementById('skills-search');
 if (skillsSearchKeyword || (searchInput && searchInput.value.trim())) {
 return true;
 }

 return false;
}

function startSkillsAutoRefresh() {
 if (skillsAutoRefreshTimer) return;

 skillsAutoRefreshTimer = setInterval(async () => {
 if (!isSkillsManagementPageActive() || shouldSkipSkillsAutoRefresh()) {
 return;
 }
 if (isAutoRefreshingSkills) {
 return;
 }

 isAutoRefreshingSkills = true;
 try {
 await loadSkills(skillsPagination.currentPage, skillsPagination.pageSize);
 } finally {
 isAutoRefreshingSkills = false;
 }
 }, SKILLS_AUTO_REFRESH_INTERVAL_MS);
}
function getSkillsPageSize() {
 try {
 const saved = localStorage.getItem('skillsPageSize');
 if (saved) {
 const size = parseInt(saved);
 if ([10, 20, 50, 100].includes(size)) {
 return size;
 }
 }
 } catch (e) {
 console.warn('localStorage:', e);
 }
 return 20; // 20
}
function initSkillsPagination() {
 const savedPageSize = getSkillsPageSize();
 skillsPagination.pageSize = savedPageSize;
}
async function loadSkills(page = 1, pageSize = null) {
 try {
 if (pageSize === null) {
 pageSize = getSkillsPageSize();
 }
 skillsPagination.currentPage = page;
 skillsPagination.pageSize = pageSize;
 skillsSearchKeyword = '';
 const searchInput = document.getElementById('skills-search');
 if (searchInput) {
 searchInput.value = '';
 }
 const offset = (page - 1) * pageSize;
 const url = `/api/skills?limit=${pageSize}&offset=${offset}`;
 
 const response = await apiFetch(url);
 if (!response.ok) {
 throw new Error(_t('skills.loadListFailed'));
 }
 const data = await response.json();
 skillsList = data.skills || [];
 skillsPagination.total = data.total || 0;
 
 renderSkillsList();
 renderSkillsPagination();
 updateSkillsManagementStats();
 } catch (error) {
 console.error('Failed to load skills list:', error);
 showNotification(_t('skills.loadListFailed') + ': ' + error.message, 'error');
 const skillsListEl = document.getElementById('skills-list');
 if (skillsListEl) {
 skillsListEl.innerHTML = '<div class="empty-state">' + _t('skills.loadFailedShort') + ': ' + escapeHtml(error.message) + '</div>';
 }
 }
}
function renderSkillsList() {
 const skillsListEl = document.getElementById('skills-list');
 if (!skillsListEl) return;
 const filteredSkills = skillsList;

 if (filteredSkills.length === 0) {
 skillsListEl.innerHTML = '<div class="empty-state">' + 
 (skillsSearchKeyword ? _t('skills.noMatch') : _t('skills.noSkills')) + 
 '</div>';
 const paginationContainer = document.getElementById('skills-pagination');
 if (paginationContainer) {
 paginationContainer.innerHTML = '';
 }
 return;
 }

 skillsListEl.innerHTML = filteredSkills.map(skill => {
 const sid = skill.id || skill.name || '';
 const ver = skill.version ? _t('skills.cardVersion', { version: skill.version }) : '';
 const sc = typeof skill.script_count === 'number' && skill.script_count > 0
 ? _t('skills.cardScripts', { count: skill.script_count })
 : '';
 const fc = typeof skill.file_count === 'number' && skill.file_count > 0
 ? _t('skills.cardFiles', { count: skill.file_count })
 : '';
 const meta = [ver, fc, sc].filter(Boolean).join(' · ');
 return `
 <div class="skill-card">
 <div class="skill-card-header">
 <h3 class="skill-card-title">${escapeHtml(skill.name || sid)}</h3>
 ${meta ? `<div class="skill-card-meta">${escapeHtml(meta)}</div>` : ''}
 <div class="skill-card-description">${escapeHtml(skill.description || _t('skills.noDescription'))}</div>
 </div>
 <div class="skill-card-actions">
 <button type="button" class="btn-secondary btn-small" data-skill-view="${escapeHtml(sid)}">${_t('common.view')}</button>
 <button type="button" class="btn-secondary btn-small" data-skill-edit="${escapeHtml(sid)}">${_t('common.edit')}</button>
 <button type="button" class="btn-secondary btn-small btn-danger" data-skill-delete="${escapeHtml(sid)}">${_t('common.delete')}</button>
 </div>
 </div>
 `;
 }).join('');

 skillsListEl.querySelectorAll('[data-skill-view]').forEach(btn => {
 btn.addEventListener('click', () => viewSkill(btn.getAttribute('data-skill-view')));
 });
 skillsListEl.querySelectorAll('[data-skill-edit]').forEach(btn => {
 btn.addEventListener('click', () => editSkill(btn.getAttribute('data-skill-edit')));
 });
 skillsListEl.querySelectorAll('[data-skill-delete]').forEach(btn => {
 btn.addEventListener('click', () => deleteSkill(btn.getAttribute('data-skill-delete')));
 });
 setTimeout(() => {
 const paginationContainer = document.getElementById('skills-pagination');
 if (paginationContainer && !skillsSearchKeyword) {
 paginationContainer.style.display = 'block';
 paginationContainer.style.visibility = 'visible';
 }
 }, 0);
}
function renderSkillsPagination() {
 const paginationContainer = document.getElementById('skills-pagination');
 if (!paginationContainer) return;
 
 const total = skillsPagination.total;
 const pageSize = skillsPagination.pageSize;
 const currentPage = skillsPagination.currentPage;
 const totalPages = Math.ceil(total / pageSize);
 if (total === 0) {
 paginationContainer.innerHTML = '';
 return;
 }
 const start = total === 0 ? 0 : (currentPage - 1) * pageSize + 1;
 const end = total === 0 ? 0 : Math.min(currentPage * pageSize, total);
 
 let paginationHTML = '<div class="pagination">';
 
 const paginationShowText = _t('skillsPage.paginationShow', { start, end, total });
 const perPageLabelText = _t('skillsPage.perPageLabel');
 const firstPageText = _t('skillsPage.firstPage');
 const prevPageText = _t('skillsPage.prevPage');
 const pageOfText = _t('skillsPage.pageOf', { current: currentPage, total: totalPages || 1 });
 const nextPageText = _t('skillsPage.nextPage');
 const lastPageText = _t('skillsPage.lastPage');
 paginationHTML += `
 <div class="pagination-info">
 <span>${escapeHtml(paginationShowText)}</span>
 <label class="pagination-page-size">
 ${escapeHtml(perPageLabelText)}
 <select id="skills-page-size-pagination" onchange="changeSkillsPageSize()">
 <option value="10" ${pageSize === 10 ? 'selected' : ''}>10</option>
 <option value="20" ${pageSize === 20 ? 'selected' : ''}>20</option>
 <option value="50" ${pageSize === 50 ? 'selected' : ''}>50</option>
 <option value="100" ${pageSize === 100 ? 'selected' : ''}>100</option>
 </select>
 </label>
 </div>
 `;
 paginationHTML += `
 <div class="pagination-controls">
 <button class="btn-secondary" onclick="loadSkills(1, ${pageSize})" ${currentPage === 1 || total === 0 ? 'disabled' : ''}>${escapeHtml(firstPageText)}</button>
 <button class="btn-secondary" onclick="loadSkills(${currentPage - 1}, ${pageSize})" ${currentPage === 1 || total === 0 ? 'disabled' : ''}>${escapeHtml(prevPageText)}</button>
 <span class="pagination-page">${escapeHtml(pageOfText)}</span>
 <button class="btn-secondary" onclick="loadSkills(${currentPage + 1}, ${pageSize})" ${currentPage >= totalPages || total === 0 ? 'disabled' : ''}>${escapeHtml(nextPageText)}</button>
 <button class="btn-secondary" onclick="loadSkills(${totalPages || 1}, ${pageSize})" ${currentPage >= totalPages || total === 0 ? 'disabled' : ''}>${escapeHtml(lastPageText)}</button>
 </div>
 `;
 
 paginationHTML += '</div>';
 
 paginationContainer.innerHTML = paginationHTML;
 function alignPaginationWidth() {
 const skillsList = document.getElementById('skills-list');
 if (skillsList && paginationContainer) {
 paginationContainer.style.display = '';
 paginationContainer.style.visibility = 'visible';
 paginationContainer.style.opacity = '1';
 const listClientWidth = skillsList.clientWidth; // ()
 const listScrollHeight = skillsList.scrollHeight; // 
 const listClientHeight = skillsList.clientHeight; // 
 const hasScrollbar = listScrollHeight > listClientHeight;
 if (hasScrollbar && listClientWidth > 0) {
 paginationContainer.style.width = `${listClientWidth}px`;
 } else {
 paginationContainer.style.width = '100%';
 }
 }
 }
 alignPaginationWidth();
 const resizeObserver = new ResizeObserver(() => {
 alignPaginationWidth();
 });
 
 const skillsList = document.getElementById('skills-list');
 if (skillsList) {
 resizeObserver.observe(skillsList);
 }
 paginationContainer.style.display = 'block';
 paginationContainer.style.visibility = 'visible';
}
async function changeSkillsPageSize() {
 const pageSizeSelect = document.getElementById('skills-page-size-pagination');
 if (!pageSizeSelect) return;
 
 const newPageSize = parseInt(pageSizeSelect.value);
 if (isNaN(newPageSize) || newPageSize <= 0) return;
 try {
 localStorage.setItem('skillsPageSize', newPageSize.toString());
 } catch (e) {
 console.warn('savelocalStorage:', e);
 }
 skillsPagination.pageSize = newPageSize;
 const totalPages = Math.ceil(skillsPagination.total / newPageSize);
 const currentPage = Math.min(skillsPagination.currentPage, totalPages || 1);
 skillsPagination.currentPage = currentPage;
 await loadSkills(currentPage, newPageSize);
}
function updateSkillsManagementStats() {
 const statsEl = document.getElementById('skills-management-stats');
 if (!statsEl) return;

 const totalEl = statsEl.querySelector('.skill-stat-value');
 if (totalEl) {
 totalEl.textContent = skillsPagination.total;
 }
}
function handleSkillsSearchInput() {
 clearTimeout(skillsSearchTimeout);
 skillsSearchTimeout = setTimeout(() => {
 searchSkills();
 }, 300);
}

async function searchSkills() {
 const searchInput = document.getElementById('skills-search');
 if (!searchInput) return;
 
 skillsSearchKeyword = searchInput.value.trim();
 const clearBtn = document.getElementById('skills-search-clear');
 if (clearBtn) {
 clearBtn.style.display = skillsSearchKeyword ? 'block' : 'none';
 }
 
 if (skillsSearchKeyword) {
 try {
 const response = await apiFetch(`/api/skills?search=${encodeURIComponent(skillsSearchKeyword)}&limit=10000&offset=0`);
 if (!response.ok) {
 throw new Error(_t('skills.loadListFailed'));
 }
 const data = await response.json();
 skillsList = data.skills || [];
 skillsPagination.total = data.total || 0;
 renderSkillsList();
 const paginationContainer = document.getElementById('skills-pagination');
 if (paginationContainer) {
 paginationContainer.innerHTML = '';
 }
 updateSkillsManagementStats();
 } catch (error) {
 console.error('skillsfailed:', error);
 showNotification(_t('skills.searchFailed') + ': ' + error.message, 'error');
 }
 } else {
 await loadSkills(1, skillsPagination.pageSize);
 }
}
function clearSkillsSearch() {
 const searchInput = document.getElementById('skills-search');
 if (searchInput) {
 searchInput.value = '';
 }
 skillsSearchKeyword = '';
 const clearBtn = document.getElementById('skills-search-clear');
 if (clearBtn) {
 clearBtn.style.display = 'none';
 }
 loadSkills(1, skillsPagination.pageSize);
}
async function refreshSkills() {
 await loadSkills(skillsPagination.currentPage, skillsPagination.pageSize);
 showNotification(_t('skills.refreshed'), 'success');
}
function wireSkillModalOnce() {
 if (skillModalControlsWired) return;
 skillModalControlsWired = true;
 const addTa = document.getElementById('skill-content-add');
 const edTa = document.getElementById('skill-content');
 if (addTa) addTa.addEventListener('input', () => { if (skillModalAddMode) skillFileDirty = true; });
 if (edTa) edTa.addEventListener('input', () => { if (!skillModalAddMode) skillFileDirty = true; });
 const nb = document.getElementById('skill-new-file-btn');
 if (nb) {
 nb.addEventListener('click', () => {
 if (!currentEditingSkillName) return;
 const inp = document.getElementById('skill-new-file-path');
 const p = (inp && inp.value || '').trim();
 if (!p) {
 showNotification(_t('skillModal.newFilePathRequired'), 'error');
 return;
 }
 if (p.includes('..') || p.startsWith('/')) {
 showNotification(_t('skillModal.newFilePathInvalid'), 'error');
 return;
 }
 selectSkillPackageFile(currentEditingSkillName, p, { force: true, freshContent: '' });
 if (inp) inp.value = '';
 });
 }
}

function showAddSkillModal() {
 wireSkillModalOnce();
 const modal = document.getElementById('skill-modal');
 if (!modal) return;

 skillModalAddMode = true;
 skillFileDirty = false;
 skillActivePath = 'SKILL.md';
 skillPackageFiles = [];
 const pkg = document.getElementById('skill-package-editor');
 const addEd = document.getElementById('skill-add-editor');
 if (pkg) pkg.style.display = 'none';
 if (addEd) addEd.style.display = 'block';

 document.getElementById('skill-modal-title').textContent = _t('skills.addSkill');
 document.getElementById('skill-name').value = '';
 document.getElementById('skill-name').disabled = false;
 document.getElementById('skill-description').value = '';
 const addTa = document.getElementById('skill-content-add');
 if (addTa) addTa.value = '';

 modal.style.display = 'flex';
}

function renderSkillPackageTree() {
 const el = document.getElementById('skill-package-tree');
 if (!el) return;
 const rows = (skillPackageFiles || []).filter(f => f.path && f.path !== '.').sort((a, b) =>
 String(a.path).localeCompare(String(b.path)));
 if (rows.length === 0) {
 el.innerHTML = '<div class="empty-state" style="padding:8px;">' + escapeHtml(_t('skillModal.noPackageFiles')) + '</div>';
 return;
 }
 el.innerHTML = rows.map(f => {
 const path = f.path || '';
 if (f.is_dir) {
 return `<div style="padding:4px 6px;opacity:0.85;font-weight:600;">${escapeHtml(path)}/</div>`;
 }
 const sel = path === skillActivePath
 ? 'font-weight:600;background:rgba(99,102,241,0.12);'
 : '';
 return `<div style="padding:4px 6px;cursor:pointer;border-radius:4px;margin-bottom:2px;${sel}" data-skill-tree-path="${escapeHtml(path)}" class="skill-tree-item">${escapeHtml(path)}</div>`;
 }).join('');
 el.querySelectorAll('[data-skill-tree-path]').forEach(node => {
 node.addEventListener('click', () => {
 const p = node.getAttribute('data-skill-tree-path');
 if (p) selectSkillPackageFile(currentEditingSkillName, p, {});
 });
 });
}

async function selectSkillPackageFile(skillId, path, opts) {
 const force = opts && opts.force;
 const freshContent = opts && Object.prototype.hasOwnProperty.call(opts, 'freshContent')
 ? opts.freshContent
 : null;
 if (!force && skillFileDirty) {
 if (!confirm(_t('skillModal.unsavedSwitch'))) {
 return;
 }
 }
 skillActivePath = path;
 const label = document.getElementById('skill-active-path');
 if (label) label.textContent = path;
 const hint = document.getElementById('skill-body-hint-edit');
 if (hint) hint.style.display = path === 'SKILL.md' ? 'block' : 'none';
 const ta = document.getElementById('skill-content');
 if (!ta) return;

 if (freshContent !== null) {
 ta.value = freshContent;
 skillFileDirty = true;
 renderSkillPackageTree();
 return;
 }

 try {
 if (path === 'SKILL.md') {
 const response = await apiFetch(`/api/skills/${encodeURIComponent(skillId)}?depth=full`);
 if (!response.ok) throw new Error(_t('skills.loadDetailFailed'));
 const data = await response.json();
 const skill = data.skill;
 ta.value = skill && skill.content != null ? skill.content : '';
 } else {
 const response = await apiFetch(`/api/skills/${encodeURIComponent(skillId)}/file?path=${encodeURIComponent(path)}`);
 if (!response.ok) throw new Error(_t('skills.loadDetailFailed'));
 const data = await response.json();
 ta.value = data.content != null ? data.content : '';
 }
 skillFileDirty = false;
 renderSkillPackageTree();
 } catch (e) {
 console.error(e);
 showNotification(_t('skills.loadDetailFailed') + ': ' + e.message, 'error');
 }
}
async function editSkill(skillId) {
 wireSkillModalOnce();
 try {
 const [detailRes, filesRes] = await Promise.all([
 apiFetch(`/api/skills/${encodeURIComponent(skillId)}?depth=full`),
 apiFetch(`/api/skills/${encodeURIComponent(skillId)}/files`)
 ]);
 if (!detailRes.ok) {
 throw new Error(_t('skills.loadDetailFailed'));
 }
 const data = await detailRes.json();
 const skill = data.skill;

 const modal = document.getElementById('skill-modal');
 if (!modal) return;

 skillModalAddMode = false;
 skillFileDirty = false;
 skillActivePath = 'SKILL.md';
 const pkg = document.getElementById('skill-package-editor');
 const addEd = document.getElementById('skill-add-editor');
 if (pkg) pkg.style.display = 'block';
 if (addEd) addEd.style.display = 'none';

 document.getElementById('skill-modal-title').textContent = _t('skills.editSkill');
 document.getElementById('skill-name').value = skill.id || skillId;
 document.getElementById('skill-name').disabled = true;
 document.getElementById('skill-description').value = skill.description || '';

 if (filesRes.ok) {
 const fd = await filesRes.json();
 skillPackageFiles = fd.files || [];
 } else {
 skillPackageFiles = [];
 }
 renderSkillPackageTree();

 const ta = document.getElementById('skill-content');
 if (ta) ta.value = skill.content || '';
 const hint = document.getElementById('skill-body-hint-edit');
 if (hint) hint.style.display = 'block';

 currentEditingSkillName = skillId;
 modal.style.display = 'flex';
 } catch (error) {
 console.error('loadskilldetailsfailed:', error);
 showNotification(_t('skills.loadDetailFailed') + ': ' + error.message, 'error');
 }
}
async function viewSkill(skillId) {
 try {
 const sumRes = await apiFetch(`/api/skills/${encodeURIComponent(skillId)}?depth=summary`);
 if (!sumRes.ok) {
 throw new Error(_t('skills.loadDetailFailed'));
 }
 const sumData = await sumRes.json();
 const sumSkill = sumData.skill;

 const modal = document.createElement('div');
 modal.className = 'modal';
 modal.id = 'skill-view-modal';
 const viewTitle = _t('skills.viewSkillTitle', { name: sumSkill.name || skillId });
 const descLabel = _t('skills.descriptionLabel');
 const pathLabel = _t('skills.pathLabel');
 const modTimeLabel = _t('skills.modTimeLabel');
 const contentLabel = _t('skills.contentLabel');
 const closeBtn = _t('common.close');
 const editBtn = _t('common.edit');
 const loadFullLabel = _t('skills.loadFullBody');
 const scriptsLabel = _t('skills.scriptsHeading');

 let scriptsBlock = '';
 if (Array.isArray(sumSkill.scripts) && sumSkill.scripts.length > 0) {
 const lines = sumSkill.scripts.map(s => {
 const rel = escapeHtml(s.rel_path || s.RelPath || '');
 const dn = escapeHtml(s.description || s.Description || '');
 return `<li><code>${rel}</code>${dn ? ' — ' + dn : ''}</li>`;
 }).join('');
 scriptsBlock = `<div style="margin-bottom: 16px;"><strong>${escapeHtml(scriptsLabel)}</strong><ul style="margin:8px 0 0 18px;">${lines}</ul></div>`;
 }

 modal.innerHTML = `
 <div class="modal-content" style="max-width: 900px; max-height: 90vh;">
 <div class="modal-header">
 <h2>${escapeHtml(viewTitle)}</h2>
 <span class="modal-close" data-skill-view-close>&times;</span>
 </div>
 <div class="modal-body" style="overflow-y: auto; max-height: calc(90vh - 120px);">
 ${sumSkill.version ? `<div style="margin-bottom: 8px;"><strong>${escapeHtml(_t('skills.versionLabel'))}</strong> ${escapeHtml(sumSkill.version)}</div>` : ''}
 ${sumSkill.description ? `<div style="margin-bottom: 16px;"><strong>${escapeHtml(descLabel)}</strong> ${escapeHtml(sumSkill.description)}</div>` : ''}
 ${scriptsBlock}
 <div style="margin-bottom: 8px;"><strong>${escapeHtml(pathLabel)}</strong> ${escapeHtml(sumSkill.path || '')}</div>
 <div style="margin-bottom: 16px;"><strong>${escapeHtml(modTimeLabel)}</strong> ${escapeHtml(sumSkill.mod_time || '')}</div>
 <div style="margin-bottom: 8px;"><strong>${escapeHtml(contentLabel)}</strong> <span style="opacity:0.8;font-size:12px;">${escapeHtml(_t('skills.summaryHint'))}</span></div>
 <pre id="skill-view-body" style="background: #f5f5f5; padding: 16px; border-radius: 4px; overflow-x: auto; white-space: pre-wrap; word-wrap: break-word;">${escapeHtml(sumSkill.content || '')}</pre>
 </div>
 <div class="modal-footer">
 <button type="button" class="btn-secondary" data-skill-load-full>${escapeHtml(loadFullLabel)}</button>
 <button type="button" class="btn-secondary" data-skill-view-close>${escapeHtml(closeBtn)}</button>
 <button type="button" class="btn-primary" data-skill-view-edit>${escapeHtml(editBtn)}</button>
 </div>
 </div>
 `;
 document.body.appendChild(modal);
 modal.style.display = 'flex';

 const close = () => closeSkillViewModal();
 modal.querySelectorAll('[data-skill-view-close]').forEach(el => el.addEventListener('click', close));
 modal.querySelector('[data-skill-view-edit]').addEventListener('click', () => {
 close();
 editSkill(skillId);
 });
 modal.querySelector('[data-skill-load-full]').addEventListener('click', async () => {
 const pre = modal.querySelector('#skill-view-body');
 const btn = modal.querySelector('[data-skill-load-full]');
 if (!pre || !btn) return;
 btn.disabled = true;
 try {
 const fullRes = await apiFetch(`/api/skills/${encodeURIComponent(skillId)}?depth=full`);
 if (!fullRes.ok) throw new Error(_t('skills.loadDetailFailed'));
 const fullData = await fullRes.json();
 pre.textContent = fullData.skill && fullData.skill.content != null ? fullData.skill.content : '';
 } catch (e) {
 showNotification(_t('skills.loadFullFailed') + ': ' + e.message, 'error');
 } finally {
 btn.disabled = false;
 }
 });
 } catch (error) {
 console.error('skillfailed:', error);
 showNotification(_t('skills.viewFailed') + ': ' + error.message, 'error');
 }
}
function closeSkillViewModal() {
 const modal = document.getElementById('skill-view-modal');
 if (modal) {
 modal.remove();
 }
}
function closeSkillModal() {
 const modal = document.getElementById('skill-modal');
 if (modal) {
 modal.style.display = 'none';
 currentEditingSkillName = null;
 skillModalAddMode = true;
 skillFileDirty = false;
 skillPackageFiles = [];
 skillActivePath = 'SKILL.md';
 }
}
async function saveSkill() {
 if (isSavingSkill) return;

 const name = document.getElementById('skill-name').value.trim();
 const description = document.getElementById('skill-description').value.trim();

 if (!name) {
 showNotification(_t('skills.nameRequired'), 'error');
 return;
 }

 if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(name)) {
 showNotification(_t('skills.nameInvalid'), 'error');
 return;
 }

 if (skillModalAddMode || !currentEditingSkillName) {
 if (!description) {
 showNotification(_t('skills.descriptionRequired'), 'error');
 return;
 }
 const content = (document.getElementById('skill-content-add') || {}).value;
 const body = (content || '').trim();
 if (!body) {
 showNotification(_t('skills.contentRequired'), 'error');
 return;
 }
 }

 isSavingSkill = true;
 const saveBtn = document.querySelector('#skill-modal .btn-primary');
 if (saveBtn) {
 saveBtn.disabled = true;
 saveBtn.textContent = _t('skills.saving');
 }

 try {
 if (skillModalAddMode || !currentEditingSkillName) {
 const content = (document.getElementById('skill-content-add') || {}).value;
 const body = (content || '').trim();
 const response = await apiFetch('/api/skills', {
 method: 'POST',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({ name, description, content: body })
 });
 if (!response.ok) {
 const error = await response.json();
 throw new Error(error.error || _t('skills.saveFailed'));
 }
 showNotification(_t('skills.createdSuccess'), 'success');
 closeSkillModal();
 await loadSkills(skillsPagination.currentPage, skillsPagination.pageSize);
 return;
 }

 const path = skillActivePath || 'SKILL.md';
 const ta = document.getElementById('skill-content');
 const raw = ta ? ta.value : '';
 if (path === 'SKILL.md') {
 if (!raw.trim()) {
 showNotification(_t('skills.contentRequired'), 'error');
 return;
 }
 const response = await apiFetch(`/api/skills/${encodeURIComponent(currentEditingSkillName)}`, {
 method: 'PUT',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({
 description: description,
 content: raw.trim()
 })
 });
 if (!response.ok) {
 const error = await response.json();
 throw new Error(error.error || _t('skills.saveFailed'));
 }
 } else {
 const response = await apiFetch(`/api/skills/${encodeURIComponent(currentEditingSkillName)}/file`, {
 method: 'PUT',
 headers: { 'Content-Type': 'application/json' },
 body: JSON.stringify({ path: path, content: raw })
 });
 if (!response.ok) {
 const error = await response.json();
 throw new Error(error.error || _t('skills.saveFailed'));
 }
 }

 skillFileDirty = false;
 showNotification(_t('skills.saveSuccess'), 'success');
 const filesRes = await apiFetch(`/api/skills/${encodeURIComponent(currentEditingSkillName)}/files`);
 if (filesRes.ok) {
 const fd = await filesRes.json();
 skillPackageFiles = fd.files || [];
 renderSkillPackageTree();
 }
 await loadSkills(skillsPagination.currentPage, skillsPagination.pageSize);
 } catch (error) {
 console.error('saveskillfailed:', error);
 showNotification(_t('skills.saveFailed') + ': ' + error.message, 'error');
 } finally {
 isSavingSkill = false;
 if (saveBtn) {
 saveBtn.disabled = false;
 saveBtn.textContent = _t('common.save');
 }
 }
}
async function deleteSkill(skillName) {
 let boundRoles = [];
 try {
 const checkResponse = await apiFetch(`/api/skills/${encodeURIComponent(skillName)}/bound-roles`);
 if (checkResponse.ok) {
 const checkData = await checkResponse.json();
 boundRoles = checkData.bound_roles || [];
 }
 } catch (error) {
 console.warn('skillfailed:', error);
 }
 let confirmMessage = _t('skills.deleteConfirm', { name: skillName });
 if (boundRoles.length > 0) {
 const rolesList = boundRoles.join('/');
 confirmMessage = _t('skills.deleteConfirmWithRoles', { name: skillName, count: boundRoles.length, roles: rolesList });
 }

 if (!confirm(confirmMessage)) {
 return;
 }

 try {
 const response = await apiFetch(`/api/skills/${encodeURIComponent(skillName)}`, {
 method: 'DELETE'
 });

 if (!response.ok) {
 const error = await response.json();
 throw new Error(error.error || _t('skills.deleteFailed'));
 }

 const data = await response.json();
 let successMessage = _t('skills.deleteSuccess');
 if (data.affected_roles && data.affected_roles.length > 0) {
 const rolesList = data.affected_roles.join('/');
 successMessage = _t('skills.deleteSuccessWithRoles', { count: data.affected_roles.length, roles: rolesList });
 }
 showNotification(successMessage, 'success');
 const currentPage = skillsPagination.currentPage;
 const totalAfterDelete = skillsPagination.total - 1;
 const totalPages = Math.ceil(totalAfterDelete / skillsPagination.pageSize);
 const pageToLoad = currentPage > totalPages && totalPages > 0 ? totalPages : currentPage;
 await loadSkills(pageToLoad, skillsPagination.pageSize);
 } catch (error) {
 console.error('deleteskillfailed:', error);
 showNotification(_t('skills.deleteFailed') + ': ' + error.message, 'error');
 }
}
async function loadSkillsMonitor() {
 try {
 const response = await apiFetch('/api/skills/stats');
 if (!response.ok) {
 throw new Error(_t('skills.loadStatsFailed'));
 }
 const data = await response.json();
 
 skillsStats = {
 total: data.total_skills || 0,
 totalCalls: data.total_calls || 0,
 totalSuccess: data.total_success || 0,
 totalFailed: data.total_failed || 0,
 skillsDir: data.skills_dir || '',
 stats: data.stats || []
 };

 renderSkillsMonitor();
 } catch (error) {
 console.error('loadskillsfailed:', error);
 showNotification(_t('skills.loadStatsFailed') + ': ' + error.message, 'error');
 const statsEl = document.getElementById('skills-stats');
 if (statsEl) {
 statsEl.innerHTML = '<div class="monitor-error">' + _t('skills.loadStatsErrorShort') + ': ' + escapeHtml(error.message) + '</div>';
 }
 const monitorListEl = document.getElementById('skills-monitor-list');
 if (monitorListEl) {
 monitorListEl.innerHTML = '<div class="monitor-error">' + _t('skills.loadCallStatsError') + ': ' + escapeHtml(error.message) + '</div>';
 }
 }
}
function renderSkillsMonitor() {
 const statsEl = document.getElementById('skills-stats');
 if (statsEl) {
 const successRate = skillsStats.totalCalls > 0 
 ? ((skillsStats.totalSuccess / skillsStats.totalCalls) * 100).toFixed(1) 
 : '0.0';
 
 statsEl.innerHTML = `
 <div class="monitor-stat-card">
 <div class="monitor-stat-label">${_t('skills.totalSkillsCount')}</div>
 <div class="monitor-stat-value">${skillsStats.total}</div>
 </div>
 <div class="monitor-stat-card">
 <div class="monitor-stat-label">${_t('skills.totalCallsCount')}</div>
 <div class="monitor-stat-value">${skillsStats.totalCalls}</div>
 </div>
 <div class="monitor-stat-card">
 <div class="monitor-stat-label">${_t('skills.successfulCalls')}</div>
 <div class="monitor-stat-value" style="color: #28a745;">${skillsStats.totalSuccess}</div>
 </div>
 <div class="monitor-stat-card">
 <div class="monitor-stat-label">${_t('skills.failedCalls')}</div>
 <div class="monitor-stat-value" style="color: #dc3545;">${skillsStats.totalFailed}</div>
 </div>
 <div class="monitor-stat-card">
 <div class="monitor-stat-label">${_t('skills.successRate')}</div>
 <div class="monitor-stat-value">${successRate}%</div>
 </div>
 `;
 }
 const monitorListEl = document.getElementById('skills-monitor-list');
 if (!monitorListEl) return;

 const stats = skillsStats.stats || [];
 if (stats.length === 0) {
 monitorListEl.innerHTML = '<div class="monitor-empty">' + _t('skills.noCallRecords') + '</div>';
 return;
 }
 const sortedStats = [...stats].sort((a, b) => {
 const callsA = b.total_calls || 0;
 const callsB = a.total_calls || 0;
 if (callsA !== callsB) {
 return callsA - callsB;
 }
 return (a.skill_name || '').localeCompare(b.skill_name || '');
 });

 monitorListEl.innerHTML = `
 <table class="monitor-table">
 <thead>
 <tr>
 <th style="text-align: left !important;">${_t('skills.skillName')}</th>
 <th style="text-align: center;">${_t('skills.totalCalls')}</th>
 <th style="text-align: center;">${_t('skills.success')}</th>
 <th style="text-align: center;">${_t('skills.failure')}</th>
 <th style="text-align: center;">${_t('skills.successRate')}</th>
 <th style="text-align: left;">${_t('skills.lastCallTime')}</th>
 </tr>
 </thead>
 <tbody>
 ${sortedStats.map(stat => {
 const totalCalls = stat.total_calls || 0;
 const successCalls = stat.success_calls || 0;
 const failedCalls = stat.failed_calls || 0;
 const successRate = totalCalls > 0 ? ((successCalls / totalCalls) * 100).toFixed(1) : '0.0';
 const lastCallTime = stat.last_call_time && stat.last_call_time !== '-' ? stat.last_call_time : '-';
 
 return `
 <tr>
 <td style="text-align: left !important;"><strong>${escapeHtml(stat.skill_name || '')}</strong></td>
 <td style="text-align: center;">${totalCalls}</td>
 <td style="text-align: center; color: #28a745; font-weight: 500;">${successCalls}</td>
 <td style="text-align: center; color: #dc3545; font-weight: 500;">${failedCalls}</td>
 <td style="text-align: center;">${successRate}%</td>
 <td style="color: var(--text-secondary);">${escapeHtml(lastCallTime)}</td>
 </tr>
 `;
 }).join('')}
 </tbody>
 </table>
 `;
}
async function refreshSkillsMonitor() {
 await loadSkillsMonitor();
 showNotification(_t('skills.refreshed'), 'success');
}
async function clearSkillsStats() {
 if (!confirm(_t('skills.clearStatsConfirm'))) {
 return;
 }

 try {
 const response = await apiFetch('/api/skills/stats', {
 method: 'DELETE'
 });

 if (!response.ok) {
 const error = await response.json();
 throw new Error(error.error || _t('skills.clearStatsFailed'));
 }

 showNotification(_t('skills.statsCleared'), 'success');
 await loadSkillsMonitor();
 } catch (error) {
 console.error('failed:', error);
 showNotification(_t('skills.clearStatsFailed') + ': ' + error.message, 'error');
 }
}
function escapeHtml(text) {
 if (!text) return '';
 const div = document.createElement('div');
 div.textContent = text;
 return div.innerHTML;
}
document.addEventListener('languagechange', function () {
 const page = document.getElementById('page-skills-management');
 if (page && page.classList.contains('active')) {
 renderSkillsList();
 if (!skillsSearchKeyword) {
 renderSkillsPagination();
 }
 }
 const pkg = document.getElementById('skill-package-editor');
 if (pkg && pkg.style.display !== 'none' && currentEditingSkillName) {
 renderSkillPackageTree();
 }
});

document.addEventListener('DOMContentLoaded', function () {
 startSkillsAutoRefresh();
});
