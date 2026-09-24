function _t(key, opts) {
 return typeof window.t === 'function' ? window.t(key, opts) : key;
}
let currentRole = localStorage.getItem('currentRole') || '';
let roles = [];
let rolesSearchKeyword = ''; // 
let rolesSearchTimeout = null; // 
let allRoleTools = []; // tool(tool)
let roleToolsPagination = {
 page: 1,
 pageSize: 20,
 total: 0,
 totalPages: 1
};
let roleToolsSearchKeyword = ''; // tool
let roleToolStateMap = new Map(); // toolstatus:toolKey -> { enabled: boolean, ... }
let roleUsesAllTools = false; // tool(noconfigtools)
let totalEnabledToolsInMCP = 0; // enabledtool(MCP Managementfetch,APIfetch)
let roleConfiguredTools = new Set(); // configtool(tool)
let allRoleSkills = []; // skills
let roleSkillsSearchKeyword = ''; // Skills
let roleSelectedSkills = new Set(); // skills
function sortRoles(rolesArray) {
 const sortedRoles = [...rolesArray];
 const defaultRole = sortedRoles.find(r => r.name === '');
 const otherRoles = sortedRoles.filter(r => r.name !== '');
 otherRoles.sort((a, b) => {
 const nameA = a.name || '';
 const nameB = b.name || '';
 return nameA.localeCompare(nameB, 'zh-CN');
 });
 const result = defaultRole ? [defaultRole, ...otherRoles] : otherRoles;
 return result;
}
async function loadRoles() {
 try {
 const response = await apiFetch('/api/roles');
 if (!response.ok) {
 throw new Error('loadfailed');
 }
 const data = await response.json();
 roles = data.roles || [];
 updateRoleSelectorDisplay();
 renderRoleSelectionSidebar(); // 
 return roles;
 } catch (error) {
 console.error('loadfailed:', error);
 var loadFailedLabel = (typeof window !== 'undefined' && typeof window.t === 'function')
 ? window.t('roles.loadFailed')
 : 'Failed to load roles';
 showNotification(loadFailedLabel + ': ' + error.message, 'error');
 return [];
 }
}
function handleRoleChange(roleName) {
 const oldRole = currentRole;
 currentRole = roleName || '';
 localStorage.setItem('currentRole', currentRole);
 updateRoleSelectorDisplay();
 renderRoleSelectionSidebar(); // updatestatus
 if (oldRole !== currentRole && typeof window !== 'undefined') {
 window._mentionToolsRoleChanged = true;
 }
}
function updateRoleSelectorDisplay() {
 const roleSelectorBtn = document.getElementById('role-selector-btn');
 const roleSelectorIcon = document.getElementById('role-selector-icon');
 const roleSelectorText = document.getElementById('role-selector-text');
 
 if (!roleSelectorBtn || !roleSelectorIcon || !roleSelectorText) return;

 let selectedRole;
 if (currentRole && currentRole !== '') {
 selectedRole = roles.find(r => r.name === currentRole);
 } else {
 selectedRole = roles.find(r => r.name === '');
 }

 if (selectedRole) {
 roleSelectorIcon.textContent = roleMonogram(selectedRole.name);
 const isDefaultRole = selectedRole.name === '' || !selectedRole.name;
 const displayName = isDefaultRole && typeof window.t === 'function'
 ? window.t('chat.defaultRole') : (selectedRole.name || (typeof window.t === 'function' ? window.t('chat.defaultRole') : 'Default'));
 roleSelectorText.setAttribute('data-i18n-skip-text', isDefaultRole ? 'false' : 'true');
 roleSelectorText.textContent = displayName;
 } else {
 roleSelectorText.setAttribute('data-i18n-skip-text', 'false');
 roleSelectorIcon.textContent = roleMonogram('');
 roleSelectorText.textContent = typeof window.t === 'function' ? window.t('chat.defaultRole') : 'Default';
 }
}
// Professional, emoji-free role glyph: a 1–2 letter monogram from the role name.
function roleMonogram(name) {
 var n = (name || '').trim();
 if (!n) return 'DF';
 var parts = n.split(/[\s\-_]+/).filter(Boolean);
 var s = parts.length >= 2 ? (parts[0][0] + parts[1][0]) : n.slice(0, 2);
 return s.toUpperCase();
}
function renderRoleSelectionSidebar() {
 const roleList = document.getElementById('role-selection-list');
 if (!roleList) return;
 roleList.innerHTML = '';
 function getRoleIcon(role) {
 if (role.icon) {
 let icon = role.icon;
 const unicodeMatch = icon.match(/^"?\\U([0-9A-F]{8})"?$/i);
 if (unicodeMatch) {
 try {
 const codePoint = parseInt(unicodeMatch[1], 16);
 icon = String.fromCodePoint(codePoint);
 } catch (e) {
 console.warn(' icon Unicode failed:', icon, e);
 }
 }
 return icon;
 }
 return '';
 }
 const sortedRoles = sortRoles(roles);
 const enabledSortedRoles = sortedRoles.filter(r => r.enabled !== false);
 
 enabledSortedRoles.forEach(role => {
 const isDefaultRole = role.name === '';
 const isSelected = isDefaultRole ? (currentRole === '' || currentRole === '') : (currentRole === role.name);
 const roleItem = document.createElement('div');
 roleItem.className = 'role-selection-item-main' + (isSelected ? ' selected' : '');
 roleItem.onclick = () => {
 selectRole(role.name);
 closeRoleSelectionPanel(); // 
 };
 const icon = roleMonogram(role.name);
 let description = role.description || _t('roles.noDescription');
 if (isDefaultRole && !role.description) {
 description = _t('roles.defaultRoleDescription');
 }
 
 roleItem.innerHTML = `
 <div class="role-selection-item-icon-main">${icon}</div>
 <div class="role-selection-item-content-main">
 <div class="role-selection-item-name-main">${escapeHtml(role.name)}</div>
 <div class="role-selection-item-description-main">${escapeHtml(description)}</div>
 </div>
 ${isSelected ? '<div class="role-selection-checkmark-main">' + (typeof window.pyIcon === 'function' ? window.pyIcon('check', { size: 14 }) : '') + '</div>' : ''}
 `;
 roleList.appendChild(roleItem);
 });
}
function selectRole(roleName) {
 if (roleName === '') {
 roleName = '';
 }
 handleRoleChange(roleName);
 renderRoleSelectionSidebar(); // updatestatus
}
function toggleRoleSelectionPanel() {
 const panel = document.getElementById('role-selection-panel');
 const roleSelectorBtn = document.getElementById('role-selector-btn');
 if (!panel) return;
 
 const isHidden = panel.style.display === 'none' || !panel.style.display;
 
 if (isHidden) {
 if (typeof closeAgentModePanel === 'function') {
 closeAgentModePanel();
 }
 panel.style.display = 'flex'; // flex
 if (roleSelectorBtn) {
 roleSelectorBtn.classList.add('active');
 }
 setTimeout(() => {
 const wrapper = document.querySelector('.role-selector-wrapper');
 if (wrapper) {
 const rect = wrapper.getBoundingClientRect();
 const panelHeight = panel.offsetHeight || 400;
 const viewportHeight = window.innerHeight;
 if (rect.top - panelHeight < 0) {
 const scrollY = window.scrollY + rect.top - panelHeight - 20;
 window.scrollTo({ top: Math.max(0, scrollY), behavior: 'smooth' });
 }
 }
 }, 10);
 } else {
 panel.style.display = 'none';
 if (roleSelectorBtn) {
 roleSelectorBtn.classList.remove('active');
 }
 }
}
function closeRoleSelectionPanel() {
 const panel = document.getElementById('role-selection-panel');
 const roleSelectorBtn = document.getElementById('role-selector-btn');
 if (panel) {
 panel.style.display = 'none';
 }
 if (roleSelectorBtn) {
 roleSelectorBtn.classList.remove('active');
 }
}
function escapeHtml(text) {
 const div = document.createElement('div');
 div.textContent = text;
 return div.innerHTML;
}
async function refreshRoles() {
 await loadRoles();
 const currentPage = typeof window.currentPage === 'function' ? window.currentPage() : (window.currentPage || 'chat');
 if (currentPage === 'roles-management') {
 renderRolesList();
 }
 renderRoleSelectionSidebar();
 showNotification('refresh', 'success');
}
function renderRolesList() {
 const rolesList = document.getElementById('roles-list');
 if (!rolesList) return;
 let filteredRoles = roles;
 if (rolesSearchKeyword) {
 const keyword = rolesSearchKeyword.toLowerCase();
 filteredRoles = roles.filter(role => 
 role.name.toLowerCase().includes(keyword) ||
 (role.description && role.description.toLowerCase().includes(keyword))
 );
 }

 if (filteredRoles.length === 0) {
 rolesList.innerHTML = '<div class="empty-state">' + 
 (rolesSearchKeyword ? _t('roles.noMatchingRoles') : _t('roles.noRoles')) + 
 '</div>';
 return;
 }
 const sortedRoles = sortRoles(filteredRoles);
 
 rolesList.innerHTML = sortedRoles.map(role => {
 let roleIcon = roleMonogram(role.name);
 let toolsDisplay = '';
 let toolsCount = 0;
 if (role.name === '') {
 toolsDisplay = _t('roleModal.usingAllTools');
 } else if (role.tools && role.tools.length > 0) {
 toolsCount = role.tools.length;
 const toolNames = role.tools.slice(0, 5).map(tool => {
 const toolName = tool.includes('::') ? tool.split('::')[1] : tool;
 return escapeHtml(toolName);
 });
 if (toolsCount <= 5) {
 toolsDisplay = toolNames.join(', ');
 } else {
 toolsDisplay = toolNames.join(', ') + _t('roleModal.andNMore', { count: toolsCount });
 }
 } else if (role.mcps && role.mcps.length > 0) {
 toolsCount = role.mcps.length;
 toolsDisplay = _t('roleModal.andNMore', { count: toolsCount });
 } else {
 toolsDisplay = _t('roleModal.usingAllTools');
 }

 return `
 <div class="role-card">
 <div class="role-card-header">
 <h3 class="role-card-title">
 <span class="role-card-icon">${roleIcon}</span>
 ${escapeHtml(role.name)}
 </h3>
 <span class="role-card-badge ${role.enabled !== false ? 'enabled' : 'disabled'}">
 ${role.enabled !== false ? _t('roles.enabled') : _t('roles.disabled')}
 </span>
 </div>
 <div class="role-card-description">${escapeHtml(role.description || _t('roles.noDescriptionShort'))}</div>
 <div class="role-card-tools">
 <span class="role-card-tools-label">${_t('roleModal.toolsLabel')}</span>
 <span class="role-card-tools-value">${toolsDisplay}</span>
 </div>
 <div class="role-card-actions">
 <button class="btn-secondary btn-small" onclick="editRole('${escapeHtml(role.name)}')">${_t('common.edit')}</button>
 ${role.name !== '' ? `<button class="btn-secondary btn-small btn-danger" onclick="deleteRole('${escapeHtml(role.name)}')">${_t('common.delete')}</button>` : ''}
 </div>
 </div>
 `;
 }).join('');
}
function handleRolesSearchInput() {
 clearTimeout(rolesSearchTimeout);
 rolesSearchTimeout = setTimeout(() => {
 searchRoles();
 }, 300);
}
function searchRoles() {
 const searchInput = document.getElementById('roles-search');
 if (!searchInput) return;
 
 rolesSearchKeyword = searchInput.value.trim();
 const clearBtn = document.getElementById('roles-search-clear');
 if (clearBtn) {
 clearBtn.style.display = rolesSearchKeyword ? 'block' : 'none';
 }
 
 renderRolesList();
}
function clearRolesSearch() {
 const searchInput = document.getElementById('roles-search');
 if (searchInput) {
 searchInput.value = '';
 }
 rolesSearchKeyword = '';
 const clearBtn = document.getElementById('roles-search-clear');
 if (clearBtn) {
 clearBtn.style.display = 'none';
 }
 renderRolesList();
}
function getToolKey(tool) {
 if (tool.is_external && tool.external_mcp) {
 return `${tool.external_mcp}::${tool.name}`;
 }
 return tool.name;
}
function saveCurrentRolePageToolStates() {
 document.querySelectorAll('#role-tools-list .role-tool-item').forEach(item => {
 const toolKey = item.dataset.toolKey;
 const checkbox = item.querySelector('input[type="checkbox"]');
 if (toolKey && checkbox) {
 const toolName = item.dataset.toolName;
 const isExternal = item.dataset.isExternal === 'true';
 const externalMcp = item.dataset.externalMcp || '';
 const existingState = roleToolStateMap.get(toolKey);
 roleToolStateMap.set(toolKey, {
 enabled: checkbox.checked,
 is_external: isExternal,
 external_mcp: externalMcp,
 name: toolName,
 mcpEnabled: existingState ? existingState.mcpEnabled : true // MCPenabledstatus
 });
 }
 });
}
async function loadRoleTools(page = 1, searchKeyword = '') {
 try {
 saveCurrentRolePageToolStates();
 
 const pageSize = roleToolsPagination.pageSize;
 let url = `/api/config/tools?page=${page}&page_size=${pageSize}`;
 if (searchKeyword) {
 url += `&search=${encodeURIComponent(searchKeyword)}`;
 }
 
 const response = await apiFetch(url);
 if (!response.ok) {
 throw new Error('Failed to fetch tool list');
 }
 
 const result = await response.json();
 allRoleTools = result.tools || [];
 roleToolsPagination = {
 page: result.page || page,
 pageSize: result.page_size || pageSize,
 total: result.total || 0,
 totalPages: result.total_pages || 1
 };
 if (result.total_enabled !== undefined) {
 totalEnabledToolsInMCP = result.total_enabled;
 }
 allRoleTools.forEach(tool => {
 const toolKey = getToolKey(tool);
 if (!roleToolStateMap.has(toolKey)) {
 let enabled = false;
 if (roleUsesAllTools) {
 enabled = tool.enabled ? true : false;
 } else {
 enabled = roleConfiguredTools.has(toolKey);
 }
 roleToolStateMap.set(toolKey, {
 enabled: enabled,
 is_external: tool.is_external || false,
 external_mcp: tool.external_mcp || '',
 name: tool.name,
 mcpEnabled: tool.enabled // saveMCP Managementenabledstatus
 });
 } else {
 const state = roleToolStateMap.get(toolKey);
 if (roleUsesAllTools && tool.enabled) {
 state.enabled = true;
 }
 state.is_external = tool.is_external || false;
 state.external_mcp = tool.external_mcp || '';
 state.mcpEnabled = tool.enabled; // updateMCP Managementenabledstatus
 if (!state.name || state.name === toolKey.split('::').pop()) {
 state.name = tool.name; // updatetoolname
 }
 }
 });
 
 renderRoleToolsList();
 renderRoleToolsPagination();
 updateRoleToolsStats();
 } catch (error) {
 console.error('Failed to load tool list:', error);
 const toolsList = document.getElementById('role-tools-list');
 if (toolsList) {
 toolsList.innerHTML = `<div class="tools-error">${_t('roleModal.loadToolsFailed')}: ${escapeHtml(error.message)}</div>`;
 }
 }
}
function renderRoleToolsList() {
 const toolsList = document.getElementById('role-tools-list');
 if (!toolsList) return;
 toolsList.innerHTML = '';
 
 const listContainer = document.createElement('div');
 listContainer.className = 'role-tools-list-items';
 listContainer.innerHTML = '';
 
 if (allRoleTools.length === 0) {
 listContainer.innerHTML = '<div class="tools-empty">' + _t('roleModal.noTools') + '</div>';
 toolsList.appendChild(listContainer);
 return;
 }
 
 allRoleTools.forEach(tool => {
 const toolKey = getToolKey(tool);
 const toolItem = document.createElement('div');
 toolItem.className = 'role-tool-item';
 toolItem.dataset.toolKey = toolKey;
 toolItem.dataset.toolName = tool.name;
 toolItem.dataset.isExternal = tool.is_external ? 'true' : 'false';
 toolItem.dataset.externalMcp = tool.external_mcp || '';
 const toolState = roleToolStateMap.get(toolKey) || {
 enabled: tool.enabled,
 is_external: tool.is_external || false,
 external_mcp: tool.external_mcp || ''
 };
 let externalBadge = '';
 if (toolState.is_external || tool.is_external) {
 const externalMcpName = toolState.external_mcp || tool.external_mcp || '';
 const badgeText = externalMcpName ? ` (${escapeHtml(externalMcpName)})` : '';
 const badgeTitle = externalMcpName ? `MCPtool - :${escapeHtml(externalMcpName)}` : 'MCPtool';
 externalBadge = `<span class="external-tool-badge" title="${badgeTitle}">${badgeText}</span>`;
 }
 const checkboxId = `role-tool-${escapeHtml(toolKey).replace(/::/g, '--')}`;
 
 toolItem.innerHTML = `
 <input type="checkbox" id="${checkboxId}" ${toolState.enabled ? 'checked' : ''} 
 onchange="handleRoleToolCheckboxChange('${escapeHtml(toolKey)}', this.checked)" />
 <div class="role-tool-item-info">
 <div class="role-tool-item-name">
 ${escapeHtml(tool.name)}
 ${externalBadge}
 </div>
 <div class="role-tool-item-desc">${escapeHtml(tool.description || '')}</div>
 </div>
 `;
 listContainer.appendChild(toolItem);
 });
 
 toolsList.appendChild(listContainer);
}
function renderRoleToolsPagination() {
 const toolsList = document.getElementById('role-tools-list');
 if (!toolsList) return;
 const oldPagination = toolsList.querySelector('.role-tools-pagination');
 if (oldPagination) {
 oldPagination.remove();
 }
 if (roleToolsPagination.totalPages <= 1) {
 return;
 }
 
 const pagination = document.createElement('div');
 pagination.className = 'role-tools-pagination';
 
 const { page, totalPages, total } = roleToolsPagination;
 const startItem = (page - 1) * roleToolsPagination.pageSize + 1;
 const endItem = Math.min(page * roleToolsPagination.pageSize, total);
 
 const paginationShowText = _t('roleModal.paginationShow', { start: startItem, end: endItem, total: total }) +
 (roleToolsSearchKeyword ? _t('roleModal.paginationSearch', { keyword: roleToolsSearchKeyword }) : '');
 pagination.innerHTML = `
 <div class="pagination-info">${paginationShowText}</div>
 <div class="pagination-controls">
 <button class="btn-secondary" onclick="loadRoleTools(1, '${escapeHtml(roleToolsSearchKeyword)}')" ${page === 1 ? 'disabled' : ''}>${_t('roleModal.firstPage')}</button>
 <button class="btn-secondary" onclick="loadRoleTools(${page - 1}, '${escapeHtml(roleToolsSearchKeyword)}')" ${page === 1 ? 'disabled' : ''}>${_t('roleModal.prevPage')}</button>
 <span class="pagination-page">${_t('roleModal.pageOf', { page: page, total: totalPages })}</span>
 <button class="btn-secondary" onclick="loadRoleTools(${page + 1}, '${escapeHtml(roleToolsSearchKeyword)}')" ${page === totalPages ? 'disabled' : ''}>${_t('roleModal.nextPage')}</button>
 <button class="btn-secondary" onclick="loadRoleTools(${totalPages}, '${escapeHtml(roleToolsSearchKeyword)}')" ${page === totalPages ? 'disabled' : ''}>${_t('roleModal.lastPage')}</button>
 </div>
 `;
 
 toolsList.appendChild(pagination);
}
function handleRoleToolCheckboxChange(toolKey, enabled) {
 const toolItem = document.querySelector(`.role-tool-item[data-tool-key="${toolKey}"]`);
 if (toolItem) {
 const toolName = toolItem.dataset.toolName;
 const isExternal = toolItem.dataset.isExternal === 'true';
 const externalMcp = toolItem.dataset.externalMcp || '';
 const existingState = roleToolStateMap.get(toolKey);
 roleToolStateMap.set(toolKey, {
 enabled: enabled,
 is_external: isExternal,
 external_mcp: externalMcp,
 name: toolName,
 mcpEnabled: existingState ? existingState.mcpEnabled : true // MCPenabledstatus
 });
 }
 updateRoleToolsStats();
}
function selectAllRoleTools() {
 document.querySelectorAll('#role-tools-list input[type="checkbox"]').forEach(checkbox => {
 const toolItem = checkbox.closest('.role-tool-item');
 if (toolItem) {
 const toolKey = toolItem.dataset.toolKey;
 const toolName = toolItem.dataset.toolName;
 const isExternal = toolItem.dataset.isExternal === 'true';
 const externalMcp = toolItem.dataset.externalMcp || '';
 if (toolKey) {
 const existingState = roleToolStateMap.get(toolKey);
 const shouldEnable = existingState && existingState.mcpEnabled !== false;
 checkbox.checked = shouldEnable;
 roleToolStateMap.set(toolKey, {
 enabled: shouldEnable,
 is_external: isExternal,
 external_mcp: externalMcp,
 name: toolName,
 mcpEnabled: existingState ? existingState.mcpEnabled : true
 });
 }
 }
 });
 updateRoleToolsStats();
}
function deselectAllRoleTools() {
 document.querySelectorAll('#role-tools-list input[type="checkbox"]').forEach(checkbox => {
 checkbox.checked = false;
 const toolItem = checkbox.closest('.role-tool-item');
 if (toolItem) {
 const toolKey = toolItem.dataset.toolKey;
 const toolName = toolItem.dataset.toolName;
 const isExternal = toolItem.dataset.isExternal === 'true';
 const externalMcp = toolItem.dataset.externalMcp || '';
 if (toolKey) {
 const existingState = roleToolStateMap.get(toolKey);
 roleToolStateMap.set(toolKey, {
 enabled: false,
 is_external: isExternal,
 external_mcp: externalMcp,
 name: toolName,
 mcpEnabled: existingState ? existingState.mcpEnabled : true // MCPenabledstatus
 });
 }
 }
 });
 updateRoleToolsStats();
}
function searchRoleTools(keyword) {
 roleToolsSearchKeyword = keyword;
 const clearBtn = document.getElementById('role-tools-search-clear');
 if (clearBtn) {
 clearBtn.style.display = keyword ? 'block' : 'none';
 }
 loadRoleTools(1, keyword);
}
function clearRoleToolsSearch() {
 document.getElementById('role-tools-search').value = '';
 searchRoleTools('');
}
function updateRoleToolsStats() {
 const statsEl = document.getElementById('role-tools-stats');
 if (!statsEl) return;
 const currentPageEnabled = Array.from(document.querySelectorAll('#role-tools-list input[type="checkbox"]:checked')).length;
 let currentPageEnabledInMCP = 0;
 allRoleTools.forEach(tool => {
 const toolKey = getToolKey(tool);
 const state = roleToolStateMap.get(toolKey);
 const mcpEnabled = state ? (state.mcpEnabled !== false) : (tool.enabled !== false);
 if (mcpEnabled) {
 currentPageEnabledInMCP++;
 }
 });
 if (roleUsesAllTools) {
 const totalEnabled = totalEnabledToolsInMCP || 0;
 const currentPageTotal = document.querySelectorAll('#role-tools-list input[type="checkbox"]').length;
 const totalTools = roleToolsPagination.total || 0;
 statsEl.innerHTML = `
 <span title="${_t('roleModal.currentPageSelectedTitle')}">${_t('roleModal.currentPageSelected', { current: currentPageEnabled, total: currentPageTotal })}</span>
 <span title="${_t('roleModal.totalSelectedTitle')}">${_t('roleModal.totalSelected', { current: totalEnabled, total: totalTools })} <em>${_t('roleModal.usingAllEnabledTools')}</em></span>
 `;
 return;
 }
 let totalSelected = 0;
 roleToolStateMap.forEach(state => {
 if (state.enabled && state.mcpEnabled !== false) {
 totalSelected++;
 }
 });
 document.querySelectorAll('#role-tools-list input[type="checkbox"]').forEach(checkbox => {
 const toolItem = checkbox.closest('.role-tool-item');
 if (toolItem) {
 const toolKey = toolItem.dataset.toolKey;
 const savedState = roleToolStateMap.get(toolKey);
 if (savedState && savedState.enabled !== checkbox.checked && savedState.mcpEnabled !== false) {
 if (checkbox.checked && !savedState.enabled) {
 totalSelected++;
 } else if (!checkbox.checked && savedState.enabled) {
 totalSelected--;
 }
 }
 }
 });
 let totalEnabledForRole = totalEnabledToolsInMCP || 0;
 if (totalEnabledForRole === 0) {
 roleToolStateMap.forEach(state => {
 if (state.mcpEnabled !== false) { // mcpEnabled true undefined(enabled)
 totalEnabledForRole++;
 }
 });
 }
 const currentPageTotal = document.querySelectorAll('#role-tools-list input[type="checkbox"]').length;
 const totalTools = roleToolsPagination.total || 0;
 
 statsEl.innerHTML = `
 <span title="${_t('roleModal.currentPageSelectedTitle')}">${_t('roleModal.currentPageSelected', { current: currentPageEnabled, total: currentPageTotal })}</span>
 <span title="${_t('roleModal.totalSelectedTitle')}">${_t('roleModal.totalSelected', { current: totalSelected, total: totalTools })}</span>
 `;
}
async function getSelectedRoleTools() {
 saveCurrentRolePageToolStates();
 const selectedTools = [];
 roleToolStateMap.forEach((state, toolKey) => {
 if (state.enabled && state.mcpEnabled !== false) {
 selectedTools.push(toolKey);
 }
 });
 
 return selectedTools;
}
function setSelectedRoleTools(selectedToolKeys) {
 const selectedSet = new Set(selectedToolKeys || []);
 roleToolStateMap.forEach((state, toolKey) => {
 state.enabled = selectedSet.has(toolKey);
 });
 document.querySelectorAll('#role-tools-list .role-tool-item').forEach(item => {
 const toolKey = item.dataset.toolKey;
 const checkbox = item.querySelector('input[type="checkbox"]');
 if (toolKey && checkbox) {
 checkbox.checked = selectedSet.has(toolKey);
 }
 });
 
 updateRoleToolsStats();
}
async function showAddRoleModal() {
 const modal = document.getElementById('role-modal');
 if (!modal) return;

 document.getElementById('role-modal-title').textContent = _t('roleModal.addRole');
 document.getElementById('role-name').value = '';
 document.getElementById('role-name').disabled = false;
 document.getElementById('role-description').value = '';
 document.getElementById('role-icon').value = '';
 document.getElementById('role-user-prompt').value = '';
 document.getElementById('role-enabled').checked = true;
 const toolsSection = document.getElementById('role-tools-section');
 const defaultHint = document.getElementById('role-tools-default-hint');
 const toolsControls = document.querySelector('.role-tools-controls');
 const toolsList = document.getElementById('role-tools-list');
 const formHint = toolsSection ? toolsSection.querySelector('.form-hint') : null;
 
 if (defaultHint) {
 defaultHint.style.display = 'none';
 }
 if (toolsControls) {
 toolsControls.style.display = 'block';
 }
 if (toolsList) {
 toolsList.style.display = 'block';
 }
 if (formHint) {
 formHint.style.display = 'block';
 }
 roleToolStateMap.clear();
 roleConfiguredTools.clear(); // configtool
 roleUsesAllTools = false; // tool
 roleToolsSearchKeyword = '';
 const searchInput = document.getElementById('role-tools-search');
 if (searchInput) {
 searchInput.value = '';
 }
 const clearBtn = document.getElementById('role-tools-search-clear');
 if (clearBtn) {
 clearBtn.style.display = 'none';
 }
 if (toolsList) {
 toolsList.innerHTML = '';
 }
 roleSelectedSkills.clear();
 roleSkillsSearchKeyword = '';
 const skillsSearchInput = document.getElementById('role-skills-search');
 if (skillsSearchInput) {
 skillsSearchInput.value = '';
 }
 const skillsClearBtn = document.getElementById('role-skills-search-clear');
 if (skillsClearBtn) {
 skillsClearBtn.style.display = 'none';
 }
 await loadRoleTools(1, '');
 if (toolsList) {
 toolsList.style.display = 'block';
 }
 updateRoleToolsStats();
 await loadRoleSkills();

 modal.style.display = 'flex';
}
async function editRole(roleName) {
 const role = roles.find(r => r.name === roleName);
 if (!role) {
 showNotification(_t('roleModal.roleNotFound'), 'error');
 return;
 }

 const modal = document.getElementById('role-modal');
 if (!modal) return;

 document.getElementById('role-modal-title').textContent = _t('roleModal.editRole');
 document.getElementById('role-name').value = role.name;
 document.getElementById('role-name').disabled = true; // name
 document.getElementById('role-description').value = role.description || '';
 let iconValue = role.icon || '';
 if (iconValue && iconValue.startsWith('\\U')) {
 try {
 const codePoint = parseInt(iconValue.substring(2), 16);
 iconValue = String.fromCodePoint(codePoint);
 } catch (e) {
 }
 }
 document.getElementById('role-icon').value = iconValue;
 document.getElementById('role-user-prompt').value = role.user_prompt || '';
 document.getElementById('role-enabled').checked = role.enabled !== false;
 const isDefaultRole = roleName === '';
 const toolsSection = document.getElementById('role-tools-section');
 const defaultHint = document.getElementById('role-tools-default-hint');
 const toolsControls = document.querySelector('.role-tools-controls');
 const toolsList = document.getElementById('role-tools-list');
 const formHint = toolsSection ? toolsSection.querySelector('.form-hint') : null;
 
 if (isDefaultRole) {
 if (defaultHint) {
 defaultHint.style.display = 'block';
 }
 if (toolsControls) {
 toolsControls.style.display = 'none';
 }
 if (toolsList) {
 toolsList.style.display = 'none';
 }
 if (formHint) {
 formHint.style.display = 'none';
 }
 } else {
 if (defaultHint) {
 defaultHint.style.display = 'none';
 }
 if (toolsControls) {
 toolsControls.style.display = 'block';
 }
 if (toolsList) {
 toolsList.style.display = 'block';
 }
 if (formHint) {
 formHint.style.display = 'block';
 }
 roleToolStateMap.clear();
 roleConfiguredTools.clear(); // configtool
 roleToolsSearchKeyword = '';
 const searchInput = document.getElementById('role-tools-search');
 if (searchInput) {
 searchInput.value = '';
 }
 const clearBtn = document.getElementById('role-tools-search-clear');
 if (clearBtn) {
 clearBtn.style.display = 'none';
 }
 const selectedTools = role.tools || (role.mcps && role.mcps.length > 0 ? role.mcps : []);
 roleUsesAllTools = !role.tools || role.tools.length === 0;
 if (selectedTools.length > 0) {
 selectedTools.forEach(toolKey => {
 roleConfiguredTools.add(toolKey);
 });
 }
 if (selectedTools.length > 0) {
 roleUsesAllTools = false; // configtool,tool
 selectedTools.forEach(toolKey => {
 if (!roleToolStateMap.has(toolKey)) {
 roleToolStateMap.set(toolKey, {
 enabled: true,
 is_external: false,
 external_mcp: '',
 name: toolKey.split('::').pop() || toolKey // toolKeytoolname
 });
 } else {
 const state = roleToolStateMap.get(toolKey);
 state.enabled = true;
 }
 });
 }
 await loadRoleTools(1, '');
 if (roleUsesAllTools) {
 document.querySelectorAll('#role-tools-list input[type="checkbox"]').forEach(checkbox => {
 const toolItem = checkbox.closest('.role-tool-item');
 if (toolItem) {
 const toolKey = toolItem.dataset.toolKey;
 const toolName = toolItem.dataset.toolName;
 const isExternal = toolItem.dataset.isExternal === 'true';
 const externalMcp = toolItem.dataset.externalMcp || '';
 if (toolKey) {
 const state = roleToolStateMap.get(toolKey);
 const shouldEnable = state ? (state.mcpEnabled !== false) : true;
 checkbox.checked = shouldEnable;
 if (state) {
 state.enabled = shouldEnable;
 } else {
 roleToolStateMap.set(toolKey, {
 enabled: shouldEnable,
 is_external: isExternal,
 external_mcp: externalMcp,
 name: toolName,
 mcpEnabled: true // enabled,loadRoleToolsupdate
 });
 }
 }
 }
 });
 updateRoleToolsStats();
 } else if (selectedTools.length > 0) {
 setSelectedRoleTools(selectedTools);
 }
 }
 await loadRoleSkills();
 const selectedSkills = role.skills || [];
 roleSelectedSkills.clear();
 selectedSkills.forEach(skill => {
 roleSelectedSkills.add(skill);
 });
 renderRoleSkills();

 modal.style.display = 'flex';
}
function closeRoleModal() {
 const modal = document.getElementById('role-modal');
 if (modal) {
 modal.style.display = 'none';
 }
}
function getAllSelectedRoleTools() {
 saveCurrentRolePageToolStates();
 const selectedTools = [];
 roleToolStateMap.forEach((state, toolKey) => {
 if (state.enabled) {
 selectedTools.push({
 key: toolKey,
 name: state.name || toolKey.split('::').pop() || toolKey,
 mcpEnabled: state.mcpEnabled !== false // mcpEnabled false enabled,enabled
 });
 }
 });
 
 return selectedTools;
}
function getDisabledTools(selectedTools) {
 return selectedTools.filter(tool => {
 const state = roleToolStateMap.get(tool.key);
 return state && state.mcpEnabled === false;
 });
}
async function loadAllToolsToStateMap() {
 try {
 const pageSize = 100; // page
 let page = 1;
 let hasMore = true;
 while (hasMore) {
 const url = `/api/config/tools?page=${page}&page_size=${pageSize}`;
 const response = await apiFetch(url);
 if (!response.ok) {
 throw new Error('Failed to fetch tool list');
 }
 
 const result = await response.json();
 result.tools.forEach(tool => {
 const toolKey = getToolKey(tool);
 if (!roleToolStateMap.has(toolKey)) {
 let enabled = false;
 if (roleUsesAllTools) {
 enabled = tool.enabled ? true : false;
 } else {
 enabled = roleConfiguredTools.has(toolKey);
 }
 roleToolStateMap.set(toolKey, {
 enabled: enabled,
 is_external: tool.is_external || false,
 external_mcp: tool.external_mcp || '',
 name: tool.name,
 mcpEnabled: tool.enabled // saveMCP Managementenabledstatus
 });
 } else {
 const state = roleToolStateMap.get(toolKey);
 state.is_external = tool.is_external || false;
 state.external_mcp = tool.external_mcp || '';
 state.mcpEnabled = tool.enabled; // updateMCP Managementenabledstatus
 if (!state.name || state.name === toolKey.split('::').pop()) {
 state.name = tool.name; // updatetoolname
 }
 }
 });
 if (page >= result.total_pages) {
 hasMore = false;
 } else {
 page++;
 }
 }
 } catch (error) {
 console.error('loadtoolstatusfailed:', error);
 throw error;
 }
}
async function saveRole() {
 const name = document.getElementById('role-name').value.trim();
 if (!name) {
 showNotification(_t('roleModal.roleNameRequired'), 'error');
 return;
 }

 const description = document.getElementById('role-description').value.trim();
 let icon = document.getElementById('role-icon').value.trim();
 if (icon) {
 const codePoint = icon.codePointAt(0);
 if (codePoint && codePoint > 0x7F) {
 icon = '\\U' + codePoint.toString(16).toUpperCase().padStart(8, '0');
 }
 }
 const userPrompt = document.getElementById('role-user-prompt').value.trim();
 const enabled = document.getElementById('role-enabled').checked;

 const isEdit = document.getElementById('role-name').disabled;
 const isDefaultRole = name === '';
 const isFirstUserRole = !isEdit && !isDefaultRole && roles.filter(r => r.name !== '').length === 0;
 let tools = [];
 let disabledTools = []; // MCP Managementenabledtool
 
 if (!isDefaultRole) {
 saveCurrentRolePageToolStates();
 let allSelectedTools = getAllSelectedRoleTools();
 if (isFirstUserRole && allSelectedTools.length === 0) {
 roleUsesAllTools = true;
 showNotification(_t('roleModal.firstRoleNoToolsHint'), 'info');
 } else if (roleUsesAllTools) {
 let hasUnselectedTools = false;
 roleToolStateMap.forEach((state) => {
 if (state.mcpEnabled !== false && !state.enabled) {
 hasUnselectedTools = true;
 }
 });
 if (hasUnselectedTools) {
 await loadAllToolsToStateMap();
 roleToolStateMap.forEach((state, toolKey) => {
 if (state.mcpEnabled !== false && state.enabled !== false) {
 state.enabled = true;
 }
 });
 
 roleUsesAllTools = false;
 } else {
 await loadAllToolsToStateMap();
 let hasDisabledToolsSelected = false;
 roleToolStateMap.forEach((state) => {
 if (state.enabled && state.mcpEnabled === false) {
 hasDisabledToolsSelected = true;
 }
 });
 if (!hasDisabledToolsSelected) {
 roleToolStateMap.forEach((state) => {
 if (state.mcpEnabled !== false) {
 state.enabled = true;
 }
 });
 }
 allSelectedTools = getAllSelectedRoleTools();
 }
 }
 disabledTools = getDisabledTools(allSelectedTools);
 if (disabledTools.length > 0) {
 const toolNames = disabledTools.map(t => t.name).join('/');
 const message = ` ${disabledTools.length} toolMCP Managementenabled,config:\n\n${toolNames}\n\n"MCP Management"enabledtool,config.\n\nsave？(saveenabledtool)`;
 
 if (!confirm(message)) {
 return; // cancelsave
 }
 }
 if (!roleUsesAllTools) {
 tools = await getSelectedRoleTools();
 }
 }
 const skills = Array.from(roleSelectedSkills);

 const roleData = {
 name: name,
 description: description,
 icon: icon || undefined, // is empty,
 user_prompt: userPrompt,
 tools: tools, // is empty,tool
 skills: skills, // Skills
 enabled: enabled
 };
 const url = isEdit ? `/api/roles/${encodeURIComponent(name)}` : '/api/roles';
 const method = isEdit ? 'PUT' : 'POST';

 try {
 const response = await apiFetch(url, {
 method: method,
 headers: {
 'Content-Type': 'application/json'
 },
 body: JSON.stringify(roleData)
 });

 if (!response.ok) {
 const error = await response.json();
 throw new Error(error.error || 'savefailed');
 }
 if (disabledTools.length > 0) {
 let toolNames = disabledTools.map(t => t.name).join('/');
 if (toolNames.length > 100) {
 toolNames = toolNames.substring(0, 100) + '...';
 }
 showNotification(
 `${isEdit ? 'update' : 'create'}, ${disabledTools.length} MCP Managementenabledtool:${toolNames}."MCP Management"enabledtool,config.`,
 'warning'
 );
 } else {
 showNotification(isEdit ? 'update' : 'create', 'success');
 }
 
 closeRoleModal();
 await refreshRoles();
 } catch (error) {
 console.error('savefailed:', error);
 showNotification('savefailed: ' + error.message, 'error');
 }
}
async function deleteRole(roleName) {
 if (roleName === '') {
 showNotification(_t('roleModal.cannotDeleteDefaultRole'), 'error');
 return;
 }

 if (!confirm(`delete"${roleName}"？.`)) {
 return;
 }

 try {
 const response = await apiFetch(`/api/roles/${encodeURIComponent(roleName)}`, {
 method: 'DELETE'
 });

 if (!response.ok) {
 const error = await response.json();
 throw new Error(error.error || 'deletefailed');
 }

 showNotification('delete', 'success');
 if (currentRole === roleName) {
 handleRoleChange('');
 }

 await refreshRoles();
 } catch (error) {
 console.error('deletefailed:', error);
 showNotification('deletefailed: ' + error.message, 'error');
 }
}
if (typeof switchPage === 'function') {
 const originalSwitchPage = switchPage;
 switchPage = function(page) {
 originalSwitchPage(page);
 if (page === 'roles-management') {
 loadRoles().then(() => renderRolesList());
 }
 };
}
document.addEventListener('click', (e) => {
 const roleSelectModal = document.getElementById('role-select-modal');
 if (roleSelectModal && e.target === roleSelectModal) {
 closeRoleSelectModal();
 }

 const roleModal = document.getElementById('role-modal');
 if (roleModal && e.target === roleModal) {
 closeRoleModal();
 }
 const roleSelectionPanel = document.getElementById('role-selection-panel');
 const roleSelectorWrapper = document.querySelector('.role-selector-wrapper');
 if (roleSelectionPanel && roleSelectionPanel.style.display !== 'none' && roleSelectionPanel.style.display) {
 if (!roleSelectorWrapper?.contains(e.target)) {
 closeRoleSelectionPanel();
 }
 }
});
document.addEventListener('DOMContentLoaded', () => {
 loadRoles();
 updateRoleSelectorDisplay();
});
document.addEventListener('languagechange', () => {
 updateRoleSelectorDisplay();
});
function getCurrentRole() {
 return currentRole || '';
}
if (typeof window !== 'undefined') {
 window.getCurrentRole = getCurrentRole;
 window.toggleRoleSelectionPanel = toggleRoleSelectionPanel;
 window.closeRoleSelectionPanel = closeRoleSelectionPanel;
 window.currentSelectedRole = getCurrentRole();
 const originalHandleRoleChange = handleRoleChange;
 handleRoleChange = function(roleName) {
 originalHandleRoleChange(roleName);
 if (typeof window !== 'undefined') {
 window.currentSelectedRole = getCurrentRole();
 }
 };
}
async function loadRoleSkills() {
 try {
 const response = await apiFetch('/api/roles/skills/list');
 if (!response.ok) {
 throw new Error('loadskillsfailed');
 }
 const data = await response.json();
 allRoleSkills = data.skills || [];
 renderRoleSkills();
 } catch (error) {
 console.error('Failed to load skills list:', error);
 allRoleSkills = [];
 const skillsList = document.getElementById('role-skills-list');
 if (skillsList) {
 skillsList.innerHTML = '<div class="skills-error">' + _t('roleModal.loadSkillsFailed') + ': ' + error.message + '</div>';
 }
 }
}
function renderRoleSkills() {
 const skillsList = document.getElementById('role-skills-list');
 if (!skillsList) return;
 let filteredSkills = allRoleSkills;
 if (roleSkillsSearchKeyword) {
 const keyword = roleSkillsSearchKeyword.toLowerCase();
 filteredSkills = allRoleSkills.filter(skill => 
 skill.toLowerCase().includes(keyword)
 );
 }

 if (filteredSkills.length === 0) {
 skillsList.innerHTML = '<div class="skills-empty">' + 
 (roleSkillsSearchKeyword ? _t('roleModal.noMatchingSkills') : _t('roleModal.noSkillsAvailable')) + 
 '</div>';
 updateRoleSkillsStats();
 return;
 }
 skillsList.innerHTML = filteredSkills.map(skill => {
 const isSelected = roleSelectedSkills.has(skill);
 return `
 <div class="role-skill-item" data-skill="${skill}">
 <label class="checkbox-label">
 <input type="checkbox" class="modern-checkbox" 
 ${isSelected ? 'checked' : ''} 
 onchange="toggleRoleSkill('${skill}', this.checked)" />
 <span class="checkbox-custom"></span>
 <span class="checkbox-text">${escapeHtml(skill)}</span>
 </label>
 </div>
 `;
 }).join('');

 updateRoleSkillsStats();
}
function toggleRoleSkill(skill, checked) {
 if (checked) {
 roleSelectedSkills.add(skill);
 } else {
 roleSelectedSkills.delete(skill);
 }
 updateRoleSkillsStats();
}
function selectAllRoleSkills() {
 let filteredSkills = allRoleSkills;
 if (roleSkillsSearchKeyword) {
 const keyword = roleSkillsSearchKeyword.toLowerCase();
 filteredSkills = allRoleSkills.filter(skill => 
 skill.toLowerCase().includes(keyword)
 );
 }
 filteredSkills.forEach(skill => {
 roleSelectedSkills.add(skill);
 });
 renderRoleSkills();
}
function deselectAllRoleSkills() {
 let filteredSkills = allRoleSkills;
 if (roleSkillsSearchKeyword) {
 const keyword = roleSkillsSearchKeyword.toLowerCase();
 filteredSkills = allRoleSkills.filter(skill => 
 skill.toLowerCase().includes(keyword)
 );
 }
 filteredSkills.forEach(skill => {
 roleSelectedSkills.delete(skill);
 });
 renderRoleSkills();
}
function searchRoleSkills(keyword) {
 roleSkillsSearchKeyword = keyword;
 const clearBtn = document.getElementById('role-skills-search-clear');
 if (clearBtn) {
 clearBtn.style.display = keyword ? 'block' : 'none';
 }
 renderRoleSkills();
}
function clearRoleSkillsSearch() {
 const searchInput = document.getElementById('role-skills-search');
 if (searchInput) {
 searchInput.value = '';
 }
 roleSkillsSearchKeyword = '';
 const clearBtn = document.getElementById('role-skills-search-clear');
 if (clearBtn) {
 clearBtn.style.display = 'none';
 }
 renderRoleSkills();
}
function updateRoleSkillsStats() {
 const statsEl = document.getElementById('role-skills-stats');
 if (!statsEl) return;

 let filteredSkills = allRoleSkills;
 if (roleSkillsSearchKeyword) {
 const keyword = roleSkillsSearchKeyword.toLowerCase();
 filteredSkills = allRoleSkills.filter(skill => 
 skill.toLowerCase().includes(keyword)
 );
 }

 const selectedCount = Array.from(roleSelectedSkills).filter(skill => 
 filteredSkills.includes(skill)
 ).length;

 statsEl.textContent = _t('roleModal.skillsSelectedCount', { count: selectedCount, total: filteredSkills.length });
}
function escapeHtml(text) {
 const div = document.createElement('div');
 div.textContent = text;
 return div.innerHTML;
}
